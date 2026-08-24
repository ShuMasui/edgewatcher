//go:build integration

// These tests run against a real DynamoDB Local instance (no AWS account,
// no credentials). Start one with:
//
//	docker run -d --name ew-dynamodb-local -p 8000:8000 amazon/dynamodb-local
//
// then:
//
//	DYNAMO_ENDPOINT=http://localhost:8000 go test -race -tags=integration ./internal/store/...
//
// A fake cannot validate a ConditionExpression, a Query key condition, or a
// GSI's projection — this file exists to exercise the real engine on
// exactly those points.
package store

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
)

func putRawItem(t *testing.T, ctx context.Context, client *dynamodb.Client, table string, v any) {
	t.Helper()
	item, err := attributevalue.MarshalMap(v)
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	if _, err := client.PutItem(ctx, &dynamodb.PutItemInput{TableName: &table, Item: item}); err != nil {
		t.Fatalf("put item: %v", err)
	}
}

func TestGetDeviceForAuth(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d1"), SK: deviceKey("d1"),
		EntityType: "Device", DeviceID: "d1", OwnerID: "owner-1",
		Name: "Backyard", Status: "PAIRED", Interval: 5,
		SessionTokenHash: "hash123", SessionExpiresAt: 999999,
		GSI1PK: ownerGSI1PK("owner-1"), GSI1SK: deviceGSI1SK("d1"),
	}
	putRawItem(t, ctx, client, table, dev)

	got, err := s.GetDeviceForAuth(ctx, "d1")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if got.Status != "PAIRED" || got.SessionTokenHash != "hash123" || got.OwnerID != "owner-1" {
		t.Fatalf("unexpected device: %+v", got)
	}

	// Not found is a distinct, mapped case.
	_, err = s.GetDeviceForAuth(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing device")
	}
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeDeviceNotFound {
		t.Fatalf("expected CodeDeviceNotFound, got %s", apiErr.Code)
	}
}

func TestListOwnerDevices_MixesDevicesAndPairingSessions(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	owner := "owner-42"

	dev := Device{
		PK: deviceKey("d1"), SK: deviceKey("d1"),
		EntityType: "Device", DeviceID: "d1", OwnerID: owner,
		Name: "Front yard", Status: "PENDING", Interval: 10,
		GSI1PK: ownerGSI1PK(owner), GSI1SK: deviceGSI1SK("d1"),
	}
	putRawItem(t, ctx, client, table, dev)

	session := PairingSession{
		PK: deviceKey("d1"), SK: pairingSK("CODE123"),
		EntityType: "PairingSession", DeviceID: "d1", OwnerID: owner,
		PairingCode: "CODE123", Status: "PENDING", ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		GSI1PK: ownerGSI1PK(owner), GSI1SK: pairingGSI1SK("d1"),
		GSI2PK: pairingGSI2Key("CODE123"), GSI2SK: pairingGSI2Key("CODE123"),
	}
	putRawItem(t, ctx, client, table, session)

	// A device belonging to a different owner must not leak in.
	otherDev := Device{
		PK: deviceKey("d2"), SK: deviceKey("d2"),
		EntityType: "Device", DeviceID: "d2", OwnerID: "someone-else",
		Name: "Not mine", Status: "PAIRED", Interval: 5,
		GSI1PK: ownerGSI1PK("someone-else"), GSI1SK: deviceGSI1SK("d2"),
	}
	putRawItem(t, ctx, client, table, otherDev)

	rows, err := s.ListOwnerDevices(ctx, owner)
	if err != nil {
		t.Fatalf("ListOwnerDevices: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
	}

	var sawDevice, sawPairing bool
	for _, r := range rows {
		if r.OwnerID() != owner {
			t.Fatalf("OwnerID() = %q, want %q", r.OwnerID(), owner)
		}
		switch {
		case r.IsDevice():
			sawDevice = true
			if r.DeviceID != "d1" || r.Name != "Front yard" {
				t.Fatalf("unexpected device row: %+v", r)
			}
		case r.IsPairingSession():
			sawPairing = true
			if r.PairingCode != "CODE123" {
				t.Fatalf("unexpected pairing row: %+v", r)
			}
		default:
			t.Fatalf("row is neither device nor pairing session: %+v", r)
		}
	}
	if !sawDevice || !sawPairing {
		t.Fatalf("expected both a device row and a pairing row, got sawDevice=%v sawPairing=%v", sawDevice, sawPairing)
	}

	// An owner with nothing gets an empty slice, not an error.
	empty, err := s.ListOwnerDevices(ctx, "nobody")
	if err != nil {
		t.Fatalf("ListOwnerDevices(nobody): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no rows for unknown owner, got %d", len(empty))
	}
}

// TestListOwnerDevices_ExcludesArchivedDevice pins the structural basis for
// "ARCHIVED devices don't count toward the device limit"
// (docs/engineering/dynamodb.md §4, docs/03-web.md §1.9, and Task 11's 429):
// archiving a device swaps its GSI1PK from "OWNER#<ownerId>" to the literal
// partition "ARCHIVED", so it simply isn't a member of the OWNER#<ownerId>
// partition ListOwnerDevices queries — no FilterExpression involved, and
// none should ever be added here.
func TestListOwnerDevices_ExcludesArchivedDevice(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	owner := "owner-archived-check"

	live := Device{
		PK: deviceKey("d-live"), SK: deviceKey("d-live"),
		EntityType: "Device", DeviceID: "d-live", OwnerID: owner,
		Name: "Still here", Status: "PAIRED", Interval: 5,
		GSI1PK: ownerGSI1PK(owner), GSI1SK: deviceGSI1SK("d-live"),
	}
	putRawItem(t, ctx, client, table, live)

	// Mimics Task 7's ArchiveDevice: GSI1PK swapped to the "ARCHIVED"
	// partition, PK/SK and ownership otherwise unchanged.
	archived := Device{
		PK: deviceKey("d-archived"), SK: deviceKey("d-archived"),
		EntityType: "Device", DeviceID: "d-archived", OwnerID: owner,
		Name: "Deleted", Status: "ARCHIVED", Interval: 5,
		ArchivedAt: time.Now().Format(time.RFC3339),
		GSI1PK:     "ARCHIVED", GSI1SK: deviceGSI1SK("d-archived"),
	}
	putRawItem(t, ctx, client, table, archived)

	rows, err := s.ListOwnerDevices(ctx, owner)
	if err != nil {
		t.Fatalf("ListOwnerDevices: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row (the live device), got %d: %+v", len(rows), rows)
	}
	if rows[0].DeviceID != "d-live" {
		t.Fatalf("expected the live device, got %+v — an ARCHIVED device leaked into the owner's list", rows[0])
	}
}

// TestGSI1ProjectionExcludesOwnerID is the drift-detection test the task
// brief requires: if someone widens the Terraform GSI1 projection to
// include ownerId, this test must fail loudly rather than production
// silently starting to trust an ownerId read off a projected row (which
// would previously have been impossible: OwnerListRow has no OwnerID
// field, only a method deriving it from GSI1PK).
func TestGSI1ProjectionExcludesOwnerID(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)

	owner := "owner-projection-check"
	dev := Device{
		PK: deviceKey("d9"), SK: deviceKey("d9"),
		EntityType: "Device", DeviceID: "d9", OwnerID: owner,
		Name: "n", Status: "PAIRED", Interval: 5,
		GSI1PK: ownerGSI1PK(owner), GSI1SK: deviceGSI1SK("d9"),
	}
	putRawItem(t, ctx, client, table, dev)

	out, err := client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &table,
		IndexName:              strPtr("GSI1"),
		KeyConditionExpression: strPtr("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": stringAV(ownerGSI1PK(owner)),
		},
	})
	if err != nil {
		t.Fatalf("query GSI1: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.Items))
	}
	if _, present := out.Items[0]["ownerId"]; present {
		t.Fatal("GSI1 row unexpectedly carries ownerId — the projection has drifted from docs/engineering/dynamodb.md §4")
	}
	// Sanity: the attribute we do expect on this row is still there, so a
	// failure above is a real drift and not a broken test fixture.
	if _, present := out.Items[0]["deviceId"]; !present {
		t.Fatal("GSI1 row missing deviceId, projection or fixture is broken")
	}
}

func TestQueryObservationsByDay_ExcludesAdjacentDays(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	deviceID := "d-hist"
	day := "2026-06-15"

	mustPutObservation := func(label string, at time.Time) {
		id, err := ids.NewULID(at)
		if err != nil {
			t.Fatalf("NewULID: %v", err)
		}
		obs := Observation{
			PK: deviceKey(deviceID), SK: observationSK(id),
			EntityType: "Observation", ObservationID: id, DeviceID: deviceID,
			CapturedAt: at.Format(time.RFC3339), ImageKey: "img/" + label, ThumbnailKey: "thumb/" + label,
			ExpiresAt: at.Add(24 * time.Hour).Unix(),
		}
		putRawItem(t, ctx, client, table, obs)
	}

	// In-range: start of day, midday, end of day.
	mustPutObservation("day-start", time.Date(2026, 6, 15, 0, 0, 0, 0, ids.JST))
	mustPutObservation("day-mid", time.Date(2026, 6, 15, 12, 30, 0, 0, ids.JST))
	mustPutObservation("day-end", time.Date(2026, 6, 15, 23, 59, 59, 999_000_000, ids.JST))

	// Adjacent-day boundary items that must be excluded.
	mustPutObservation("prev-day-end", time.Date(2026, 6, 14, 23, 59, 59, 999_000_000, ids.JST))
	mustPutObservation("next-day-start", time.Date(2026, 6, 16, 0, 0, 0, 0, ids.JST))

	obs, err := s.QueryObservationsByDay(ctx, deviceID, day)
	if err != nil {
		t.Fatalf("QueryObservationsByDay: %v", err)
	}
	if len(obs) != 3 {
		labels := make([]string, len(obs))
		for i, o := range obs {
			labels[i] = o.ImageKey
		}
		t.Fatalf("expected exactly 3 in-range observations, got %d: %v", len(obs), labels)
	}

	// Descending order: day-end, day-mid, day-start.
	wantOrder := []string{"img/day-end", "img/day-mid", "img/day-start"}
	for i, want := range wantOrder {
		if obs[i].ImageKey != want {
			t.Fatalf("obs[%d].ImageKey = %q, want %q (full order: %v)", i, obs[i].ImageKey, want, obs)
		}
	}
}

func TestGetObservation(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	deviceID := "d-obs"
	at := time.Date(2026, 6, 15, 9, 0, 0, 0, ids.JST)
	id, err := ids.NewULID(at)
	if err != nil {
		t.Fatalf("NewULID: %v", err)
	}
	lat, lng := 35.6, 139.7
	obs := Observation{
		PK: deviceKey(deviceID), SK: observationSK(id),
		EntityType: "Observation", ObservationID: id, DeviceID: deviceID,
		CapturedAt: at.Format(time.RFC3339), ImageKey: "img/one", ThumbnailKey: "thumb/one",
		Lat: &lat, Lng: &lng, ExpiresAt: at.Add(24 * time.Hour).Unix(),
	}
	putRawItem(t, ctx, client, table, obs)

	got, err := s.GetObservation(ctx, deviceID, id)
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}
	if got.ImageKey != "img/one" || got.Lat == nil || *got.Lat != 35.6 {
		t.Fatalf("unexpected observation: %+v", got)
	}

	_, err = s.GetObservation(ctx, deviceID, "01ARZ3NDEKTSV4RRFFQ69G5FAV")
	if err == nil {
		t.Fatal("expected error for missing observation")
	}
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeObservationNotFound {
		t.Fatalf("expected CodeObservationNotFound, got %s", apiErr.Code)
	}
}

func TestFindPairingByCode(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	code := "PAIRCODE1"
	session := PairingSession{
		PK: deviceKey("d-pair"), SK: pairingSK(code),
		EntityType: "PairingSession", DeviceID: "d-pair", OwnerID: "owner-7",
		PairingCode: code, Status: "PENDING", ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		GSI1PK: ownerGSI1PK("owner-7"), GSI1SK: pairingGSI1SK("d-pair"),
		GSI2PK: pairingGSI2Key(code), GSI2SK: pairingGSI2Key(code),
	}
	putRawItem(t, ctx, client, table, session)

	got, err := s.FindPairingByCode(ctx, code)
	if err != nil {
		t.Fatalf("FindPairingByCode: %v", err)
	}
	if got.DeviceID != "d-pair" || got.Status != "PENDING" {
		t.Fatalf("unexpected session: %+v", got)
	}

	// A GSI2 miss is PAIRING_NOT_FOUND, distinct from expired/consumed
	// (which are decided later, in code, from the returned item — not by
	// this lookup).
	_, err = s.FindPairingByCode(ctx, "NEVER-ISSUED")
	if err == nil {
		t.Fatal("expected error for unknown pairing code")
	}
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodePairingNotFound {
		t.Fatalf("expected CodePairingNotFound, got %s", apiErr.Code)
	}
}

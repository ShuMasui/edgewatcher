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

// TestCreateDeviceWithPairing_WritesBothItemsAtomically pins expression A:
// one TransactWriteItems Put-ing a Device (PENDING) and its first
// PairingSession together, both readable afterward.
func TestCreateDeviceWithPairing_WritesBothItemsAtomically(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	dev, session, err := s.CreateDeviceWithPairing(ctx, CreateDeviceWithPairingInput{
		OwnerID: "owner-1", DeviceID: "d-new", Name: "Backyard", Interval: 5,
		PairingCode: "NEWCODE1", Now: now,
	})
	if err != nil {
		t.Fatalf("CreateDeviceWithPairing: %v", err)
	}
	if dev.Status != DeviceStatusPending {
		t.Fatalf("expected device status PENDING, got %s", dev.Status)
	}
	if session.Status != PairingStatusPending {
		t.Fatalf("expected session status PENDING, got %s", session.Status)
	}

	gotDev, err := s.GetDeviceForAuth(ctx, "d-new")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if gotDev.Status != DeviceStatusPending || gotDev.OwnerID != "owner-1" {
		t.Fatalf("unexpected device row: %+v", gotDev)
	}

	gotSession, err := s.FindPairingByCode(ctx, "NEWCODE1")
	if err != nil {
		t.Fatalf("FindPairingByCode: %v", err)
	}
	if gotSession.DeviceID != "d-new" || gotSession.Status != PairingStatusPending {
		t.Fatalf("unexpected pairing session row: %+v", gotSession)
	}

	// A deviceId collision must fail the whole transaction, leaving neither
	// item double-written or corrupted.
	_, _, err = s.CreateDeviceWithPairing(ctx, CreateDeviceWithPairingInput{
		OwnerID: "owner-2", DeviceID: "d-new", Name: "Collide", Interval: 10,
		PairingCode: "OTHERCODE", Now: now,
	})
	if err == nil {
		t.Fatal("expected a deviceId collision to fail the transaction")
	}
}

// consumePairingFixture creates a PENDING Device + PairingSession pair via
// CreateDeviceWithPairing, so ConsumePairing tests start from the exact
// state that transaction produces rather than a hand-built fixture.
func consumePairingFixture(t *testing.T, ctx context.Context, s *Store, deviceID, pairingCode string, now time.Time) {
	t.Helper()
	_, _, err := s.CreateDeviceWithPairing(ctx, CreateDeviceWithPairingInput{
		OwnerID: "owner-consume", DeviceID: deviceID, Name: "n", Interval: 5,
		PairingCode: pairingCode, Now: now,
	})
	if err != nil {
		t.Fatalf("fixture CreateDeviceWithPairing: %v", err)
	}
}

// TestConsumePairing_Success pins expression B's happy path: PairingSession
// flips to CONSUMED and drops out of GSI2 (unresolvable via
// FindPairingByCode), Device flips to PAIRED with the hash and deviceInfo
// set.
func TestConsumePairing_Success(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	consumePairingFixture(t, ctx, s, "d-ok", "OKCODE", now)

	err := s.ConsumePairing(ctx, ConsumePairingInput{
		DeviceID: "d-ok", PairingCode: "OKCODE", DeviceSecretHash: "secret-hash",
		DeviceInfo: DeviceInfo{Model: "Pixel 8", OSVersion: "14", AppVersion: "1.2.0"},
		Now:        now.Add(1 * time.Second),
	})
	if err != nil {
		t.Fatalf("ConsumePairing: %v", err)
	}

	dev, err := s.GetDeviceForAuth(ctx, "d-ok")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if dev.Status != DeviceStatusPaired || dev.DeviceSecretHash != "secret-hash" {
		t.Fatalf("unexpected device after consume: %+v", dev)
	}
	if dev.DeviceInfo == nil || dev.DeviceInfo.Model != "Pixel 8" {
		t.Fatalf("expected deviceInfo to be set, got %+v", dev.DeviceInfo)
	}

	// GSI2 must STILL resolve this code. The tidy-up instinct is to drop
	// the index entry on consumption, but then a replayed code and a code
	// GSI2 has not yet propagated both surface as PAIRING_NOT_FOUND — and
	// 404 is the one answer the device retries (docs/04-native.md §1.4),
	// so a device whose pairing succeeded but whose response was lost
	// would re-scan and end up reporting 無効な QR for a pairing that
	// worked. docs/engineering/dynamodb.md §4 requires the distinction:
	// only a request that reached the base table and failed its condition
	// counts as an invalid QR. Reuse is blocked by the status condition
	// (asserted below), not by the index.
	replayed, err := s.FindPairingByCode(ctx, "OKCODE")
	if err != nil {
		t.Fatalf("GSI2 must still resolve a consumed code so a replay can be classified: %v", err)
	}
	if replayed.Status != PairingStatusConsumed {
		t.Fatalf("resolved session status = %s, want CONSUMED", replayed.Status)
	}

	// And a replayed consume is refused with the code the device can act
	// on, rather than the retryable 404.
	replayErr := s.ConsumePairing(ctx, ConsumePairingInput{
		DeviceID: "d-ok", PairingCode: "OKCODE", DeviceSecretHash: "second-hash",
		Now: now.Add(2 * time.Second),
	})
	if apiErr := apierr.AsError(replayErr); apiErr.Code != apierr.CodePairingCodeConsumed {
		t.Fatalf("replayed consume gave %v, want CodePairingCodeConsumed", replayErr)
	}
	// The first pairing's credential must survive the refused replay.
	dev, err = s.GetDeviceForAuth(ctx, "d-ok")
	if err != nil {
		t.Fatalf("GetDeviceForAuth after replay: %v", err)
	}
	if dev.DeviceSecretHash != "secret-hash" {
		t.Fatalf("deviceSecretHash = %q, want the original — a refused replay must not overwrite it", dev.DeviceSecretHash)
	}

	// The base-table row itself is still there, just CONSUMED.
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &table,
		Key:       map[string]types.AttributeValue{"PK": stringAV(deviceKey("d-ok")), "SK": stringAV(pairingSK("OKCODE"))},
	})
	if err != nil {
		t.Fatalf("GetItem pairing session: %v", err)
	}
	var session PairingSession
	if err := attributevalue.UnmarshalMap(out.Item, &session); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if session.Status != PairingStatusConsumed {
		t.Fatalf("expected CONSUMED, got %s", session.Status)
	}
	if _, present := out.Item["GSI2PK"]; !present {
		t.Fatal("GSI2PK must remain on the consumed PairingSession row — see the FindPairingByCode assertion above")
	}
}

// TestConsumePairing_Expired_LeavesBothItemsUntouched pins the "distinguish
// expired from consumed, and leave both items untouched" requirement.
func TestConsumePairing_Expired_LeavesBothItemsUntouched(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	issuedAt := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	consumePairingFixture(t, ctx, s, "d-exp", "EXPCODE", issuedAt)

	// Attempt consumption well past the 5-minute TTL.
	tooLate := issuedAt.Add(10 * time.Minute)
	err := s.ConsumePairing(ctx, ConsumePairingInput{
		DeviceID: "d-exp", PairingCode: "EXPCODE", DeviceSecretHash: "secret-hash",
		DeviceInfo: DeviceInfo{Model: "m"}, Now: tooLate,
	})
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodePairingCodeExpired {
		t.Fatalf("expected CodePairingCodeExpired, got %v", err)
	}

	// Neither item was touched: PairingSession still PENDING and still
	// resolvable through GSI2; Device still PENDING with no credentials.
	session, err := s.FindPairingByCode(ctx, "EXPCODE")
	if err != nil {
		t.Fatalf("FindPairingByCode after failed (expired) consume: %v", err)
	}
	if session.Status != PairingStatusPending {
		t.Fatalf("expired-but-rejected consume must not touch PairingSession, got status %s", session.Status)
	}

	dev, err := s.GetDeviceForAuth(ctx, "d-exp")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if dev.Status != DeviceStatusPending || dev.DeviceSecretHash != "" {
		t.Fatalf("expired-but-rejected consume must not touch Device, got %+v", dev)
	}
}

// TestConsumePairing_AlreadyConsumed_DistinctFromExpired pins the other
// half of the expired-vs-consumed distinction.
func TestConsumePairing_AlreadyConsumed_DistinctFromExpired(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	consumePairingFixture(t, ctx, s, "d-replay", "REPLAYCODE", now)

	firstAttempt := now.Add(1 * time.Second)
	if err := s.ConsumePairing(ctx, ConsumePairingInput{
		DeviceID: "d-replay", PairingCode: "REPLAYCODE", DeviceSecretHash: "hash-1",
		DeviceInfo: DeviceInfo{Model: "m"}, Now: firstAttempt,
	}); err != nil {
		t.Fatalf("first ConsumePairing: %v", err)
	}

	// Replay with the same code, still within its TTL: must be classified
	// as CONSUMED, not EXPIRED, even though the deadline hasn't passed.
	secondAttempt := firstAttempt.Add(1 * time.Second)
	err := s.ConsumePairing(ctx, ConsumePairingInput{
		DeviceID: "d-replay", PairingCode: "REPLAYCODE", DeviceSecretHash: "hash-2",
		DeviceInfo: DeviceInfo{Model: "m"}, Now: secondAttempt,
	})
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodePairingCodeConsumed {
		t.Fatalf("expected CodePairingCodeConsumed for a replayed-but-not-yet-expired code, got %v", err)
	}

	// The device must still carry the first attempt's hash, not the
	// replay's.
	dev, err := s.GetDeviceForAuth(ctx, "d-replay")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if dev.DeviceSecretHash != "hash-1" {
		t.Fatalf("replay must not overwrite the device's credentials, got hash %q", dev.DeviceSecretHash)
	}
}

// TestConsumePairing_ArchivedDevice_RejectsRepairing is the addition beyond
// docs/06-auth.md: a device that has been deleted (ARCHIVED) must not be
// re-pairable even if the attacker still holds a live, unexpired QR.
func TestConsumePairing_ArchivedDevice_RejectsRepairing(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	consumePairingFixture(t, ctx, s, "d-arch", "ARCHCODE", now)

	if _, err := s.ArchiveDevice(ctx, "d-arch", now.Add(1*time.Second)); err != nil {
		t.Fatalf("ArchiveDevice: %v", err)
	}

	err := s.ConsumePairing(ctx, ConsumePairingInput{
		DeviceID: "d-arch", PairingCode: "ARCHCODE", DeviceSecretHash: "hash",
		DeviceInfo: DeviceInfo{Model: "m"}, Now: now.Add(2 * time.Second),
	})
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeDeviceNotFound {
		t.Fatalf("expected CodeDeviceNotFound for re-pairing an ARCHIVED device, got %v", err)
	}
}

// TestTouchDeviceLatest_ConditionalOnCapturedAt pins expression C and its
// fallback: an older capturedAt is rejected (latest* unchanged) but
// lastReceivedAt still advances, and interval comes back via ALL_NEW on
// both paths.
func TestTouchDeviceLatest_ConditionalOnCapturedAt(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-touch"), SK: deviceKey("d-touch"), EntityType: "Device",
		DeviceID: "d-touch", OwnerID: "owner-touch", Name: "n", Status: DeviceStatusPaired, Interval: 15,
		GSI1PK: ownerGSI1PK("owner-touch"), GSI1SK: deviceGSI1SK("d-touch"),
	}
	putRawItem(t, ctx, client, table, dev)

	newest := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	got, err := s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "d-touch", ReceivedAt: newest, LatestCapturedAt: newest, LatestThumbnailKey: "thumb/newest",
	})
	if err != nil {
		t.Fatalf("TouchDeviceLatest (newest): %v", err)
	}
	if got.LatestThumbnailKey != "thumb/newest" || got.Interval != 15 {
		t.Fatalf("unexpected device after first touch: %+v", got)
	}

	// An older, backfilled photo arrives next: latest* must not rewind,
	// but lastReceivedAt must still advance, and interval must still come
	// back.
	older := newest.Add(-1 * time.Hour)
	receivedAt := newest.Add(1 * time.Minute)
	got, err = s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "d-touch", ReceivedAt: receivedAt, LatestCapturedAt: older, LatestThumbnailKey: "thumb/older",
	})
	if err != nil {
		t.Fatalf("TouchDeviceLatest (older, fallback path): %v", err)
	}
	if got.LatestThumbnailKey != "thumb/newest" {
		t.Fatalf("latestThumbnailKey must not rewind to an older photo, got %q", got.LatestThumbnailKey)
	}
	if got.LatestCapturedAt != newest.Format(time.RFC3339) {
		t.Fatalf("latestCapturedAt must not rewind, got %q", got.LatestCapturedAt)
	}
	if got.LastReceivedAt != receivedAt.Format(time.RFC3339) {
		t.Fatalf("lastReceivedAt must advance unconditionally even on the fallback path, got %q", got.LastReceivedAt)
	}
	if got.Interval != 15 {
		t.Fatalf("fallback path must still return interval via ALL_NEW, got %d", got.Interval)
	}

	// A genuinely newer photo after the backfill must still win.
	newerStill := newest.Add(1 * time.Hour)
	got, err = s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "d-touch", ReceivedAt: newerStill, LatestCapturedAt: newerStill, LatestThumbnailKey: "thumb/newer-still",
	})
	if err != nil {
		t.Fatalf("TouchDeviceLatest (newer still): %v", err)
	}
	if got.LatestThumbnailKey != "thumb/newer-still" {
		t.Fatalf("a genuinely newer photo must update latestThumbnailKey, got %q", got.LatestThumbnailKey)
	}
}

// TestTouchDeviceLatest_NonexistentDevice pins the mandatory
// attribute_exists(PK) guard on both the primary and fallback paths:
// UpdateItem is an upsert, and device-api has no read permission to check
// existence first, so a nonexistent device must fail rather than being
// silently created as an attribute fragment.
func TestTouchDeviceLatest_NonexistentDevice(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	_, err := s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "ghost", ReceivedAt: now, LatestCapturedAt: now, LatestThumbnailKey: "thumb/1",
	})
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeDeviceNotFound {
		t.Fatalf("expected CodeDeviceNotFound, got %v", err)
	}

	// Must not have resurrected a fragment.
	out, err := client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &table,
		Key:       map[string]types.AttributeValue{"PK": stringAV(deviceKey("ghost")), "SK": stringAV(deviceKey("ghost"))},
	})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if out.Item != nil {
		t.Fatalf("TouchDeviceLatest against a nonexistent device must not create one, got %+v", out.Item)
	}
}

// TestPutObservation_ReplayIsNoOp pins expression D end to end: a retried
// Put with the same observationId is a no-op returning success, with
// exactly one row present.
func TestPutObservation_ReplayIsNoOp(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	at := time.Date(2026, 6, 15, 9, 0, 0, 0, ids.JST)
	obsID, err := ids.NewULID(at)
	if err != nil {
		t.Fatalf("NewULID: %v", err)
	}
	obs := Observation{
		PK: deviceKey("d-obs2"), SK: observationSK(obsID),
		EntityType: "Observation", ObservationID: obsID, DeviceID: "d-obs2",
		CapturedAt: at.Format(time.RFC3339), ImageKey: "img/1", ThumbnailKey: "thumb/1",
		ExpiresAt: at.Add(24 * time.Hour).Unix(),
	}
	if err := s.PutObservation(ctx, obs); err != nil {
		t.Fatalf("first PutObservation: %v", err)
	}

	// Retry with the exact same observationId (device resends after a lost
	// response) — must return nil, not an error.
	replay := obs
	replay.ImageKey = "img/should-not-overwrite"
	if err := s.PutObservation(ctx, replay); err != nil {
		t.Fatalf("replayed PutObservation must return nil (success), got %v", err)
	}

	out, err := s.QueryObservationsByDay(ctx, "d-obs2", "2026-06-15")
	if err != nil {
		t.Fatalf("QueryObservationsByDay: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly 1 row after a replay, got %d: %+v", len(out), out)
	}
	if out[0].ImageKey != "img/1" {
		t.Fatalf("replay must not overwrite the original row, got imageKey %q", out[0].ImageKey)
	}
}

// TestArchiveDevice_RemovesFromOwnerList exercises ArchiveDevice through
// the Store method (rather than a hand-built fixture) and confirms it has
// the same structural effect TestListOwnerDevices_ExcludesArchivedDevice
// pins directly.
func TestArchiveDevice_RemovesFromOwnerList(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	owner := "owner-archive-method"
	dev := Device{
		PK: deviceKey("d-to-archive"), SK: deviceKey("d-to-archive"), EntityType: "Device",
		DeviceID: "d-to-archive", OwnerID: owner, Name: "n", Status: DeviceStatusPaired, Interval: 5,
		DeviceSecretHash: "secret", SessionTokenHash: "session",
		GSI1PK: ownerGSI1PK(owner), GSI1SK: deviceGSI1SK("d-to-archive"),
	}
	putRawItem(t, ctx, client, table, dev)

	rows, err := s.ListOwnerDevices(ctx, owner)
	if err != nil || len(rows) != 1 {
		t.Fatalf("sanity check before archive failed: rows=%+v err=%v", rows, err)
	}

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	archived, err := s.ArchiveDevice(ctx, "d-to-archive", now)
	if err != nil {
		t.Fatalf("ArchiveDevice: %v", err)
	}
	if archived.Status != DeviceStatusArchived || archived.GSI1PK != "ARCHIVED" {
		t.Fatalf("unexpected archived device: %+v", archived)
	}
	if archived.DeviceSecretHash != "" || archived.SessionTokenHash != "" {
		t.Fatalf("ArchiveDevice must also invalidate credentials, got %+v", archived)
	}

	rows, err = s.ListOwnerDevices(ctx, owner)
	if err != nil {
		t.Fatalf("ListOwnerDevices after archive: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected the archived device to vanish from the owner's list, got %+v", rows)
	}

	// Archiving a device that doesn't exist must fail rather than create one.
	if _, err := s.ArchiveDevice(ctx, "never-existed", now); err == nil {
		t.Fatal("expected an error archiving a nonexistent device")
	}
}

// TestDisconnectDevice_RemovesBothHashes pins the requirement that
// disconnecting removes BOTH deviceSecretHash and sessionTokenHash — only
// removing the session token would let the device silently re-obtain a new
// one via its still-valid deviceSecret.
func TestDisconnectDevice_RemovesBothHashes_Integration(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-disc"), SK: deviceKey("d-disc"), EntityType: "Device",
		DeviceID: "d-disc", OwnerID: "owner-disc", Name: "n", Status: DeviceStatusPaired, Interval: 5,
		DeviceSecretHash: "device-secret-hash", SessionTokenHash: "session-token-hash", SessionExpiresAt: 999999,
		GSI1PK: ownerGSI1PK("owner-disc"), GSI1SK: deviceGSI1SK("d-disc"),
	}
	putRawItem(t, ctx, client, table, dev)

	got, err := s.DisconnectDevice(ctx, "d-disc")
	if err != nil {
		t.Fatalf("DisconnectDevice: %v", err)
	}
	if got.Status != DeviceStatusDisconnected {
		t.Fatalf("expected status DISCONNECTED, got %s", got.Status)
	}
	if got.DeviceSecretHash != "" {
		t.Fatalf("DisconnectDevice must remove deviceSecretHash, got %q", got.DeviceSecretHash)
	}
	if got.SessionTokenHash != "" {
		t.Fatalf("DisconnectDevice must remove sessionTokenHash, got %q", got.SessionTokenHash)
	}

	// Disconnecting an ARCHIVED device must fail.
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := s.ArchiveDevice(ctx, "d-disc", now); err != nil {
		t.Fatalf("ArchiveDevice: %v", err)
	}
	if _, err := s.DisconnectDevice(ctx, "d-disc"); err == nil {
		t.Fatal("expected DisconnectDevice on an ARCHIVED device to fail")
	}
}

// TestUpdateDeviceProfile_RenamesAndChangesInterval pins the reserved-word
// escaping (#name, #interval, #status) this method's expression needs.
func TestUpdateDeviceProfile_RenamesAndChangesInterval(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-profile"), SK: deviceKey("d-profile"), EntityType: "Device",
		DeviceID: "d-profile", OwnerID: "owner-profile", Name: "Old Name", Status: DeviceStatusPaired, Interval: 5,
		GSI1PK: ownerGSI1PK("owner-profile"), GSI1SK: deviceGSI1SK("d-profile"),
	}
	putRawItem(t, ctx, client, table, dev)

	newName := "New Name"
	newInterval := 15
	got, err := s.UpdateDeviceProfile(ctx, UpdateDeviceProfileInput{
		DeviceID: "d-profile", Name: &newName, Interval: &newInterval,
	})
	if err != nil {
		t.Fatalf("UpdateDeviceProfile: %v", err)
	}
	if got.Name != "New Name" || got.Interval != 15 {
		t.Fatalf("unexpected device after update: %+v", got)
	}
}

// TestUpdateDeviceProfile_RejectsArchivedDevice pins I2: editing an
// ARCHIVED (deleted) device must fail rather than silently succeed.
func TestUpdateDeviceProfile_RejectsArchivedDevice(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-profile-arch"), SK: deviceKey("d-profile-arch"), EntityType: "Device",
		DeviceID: "d-profile-arch", OwnerID: "owner-profile", Name: "Old Name", Status: DeviceStatusPaired, Interval: 5,
		GSI1PK: ownerGSI1PK("owner-profile"), GSI1SK: deviceGSI1SK("d-profile-arch"),
	}
	putRawItem(t, ctx, client, table, dev)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := s.ArchiveDevice(ctx, "d-profile-arch", now); err != nil {
		t.Fatalf("ArchiveDevice: %v", err)
	}

	newName := "Should Not Apply"
	_, err := s.UpdateDeviceProfile(ctx, UpdateDeviceProfileInput{
		DeviceID: "d-profile-arch", Name: &newName,
	})
	if err == nil {
		t.Fatal("expected UpdateDeviceProfile on an ARCHIVED device to fail")
	}
}

// TestRotateSessionToken_ReplacesRatherThanAdds pins "session per device,
// rotated on every exchange" (docs/06-auth.md §3).
func TestRotateSessionToken_ReplacesRatherThanAdds(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-rot"), SK: deviceKey("d-rot"), EntityType: "Device",
		DeviceID: "d-rot", OwnerID: "owner-rot", Name: "n", Status: DeviceStatusPaired, Interval: 5,
		SessionTokenHash: "old-token-hash", SessionExpiresAt: 1000,
		GSI1PK: ownerGSI1PK("owner-rot"), GSI1SK: deviceGSI1SK("d-rot"),
	}
	putRawItem(t, ctx, client, table, dev)

	got, err := s.RotateSessionToken(ctx, "d-rot", "new-token-hash", 2000)
	if err != nil {
		t.Fatalf("RotateSessionToken: %v", err)
	}
	if got.SessionTokenHash != "new-token-hash" || got.SessionExpiresAt != 2000 {
		t.Fatalf("unexpected device after rotate: %+v", got)
	}

	// A disconnected device must not be handed a session.
	if _, err := s.DisconnectDevice(ctx, "d-rot"); err != nil {
		t.Fatalf("DisconnectDevice: %v", err)
	}
	if _, err := s.RotateSessionToken(ctx, "d-rot", "another-hash", 3000); err == nil {
		t.Fatal("expected RotateSessionToken on a DISCONNECTED device to fail")
	}
}

// TestCreatePairingSession_ReissuesWithoutRecreatingDevice pins
// docs/03-web.md §1.8.2: re-issuing a QR writes only a new PairingSession,
// leaving the Device row (and deviceId) untouched.
func TestCreatePairingSession_ReissuesWithoutRecreatingDevice(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	consumePairingFixture(t, ctx, s, "d-reissue", "FIRSTCODE", now)

	session, err := s.CreatePairingSession(ctx, CreatePairingSessionInput{
		DeviceID: "d-reissue", OwnerID: "owner-consume", PairingCode: "SECONDCODE", Now: now.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}
	if session.DeviceID != "d-reissue" {
		t.Fatalf("unexpected session: %+v", session)
	}

	got, err := s.FindPairingByCode(ctx, "SECONDCODE")
	if err != nil {
		t.Fatalf("FindPairingByCode: %v", err)
	}
	if got.DeviceID != "d-reissue" {
		t.Fatalf("unexpected: %+v", got)
	}

	dev, err := s.GetDeviceForAuth(ctx, "d-reissue")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if dev.Status != DeviceStatusPending {
		t.Fatalf("re-issuing a QR must not touch the Device row, got %+v", dev)
	}
}

// Note (R11): CreatePairingSession deliberately has no Device-row guard —
// see its doc comment in pairing.go for why a TransactWriteItems
// ConditionCheck was tried and reverted (dynamodb:ConditionCheckItem is
// not granted to web-api). Re-issuing against an archived or nonexistent
// deviceId is a pointless write (ConsumePairing's own status <> ARCHIVED
// condition is the real, load-bearing gate and holds regardless), not an
// exploitable one, so there is no test here asserting rejection — there
// is nothing in this method that rejects it.

// TestTouchDeviceLatest_NormalizesTimestampsAcrossOffsets pins I1: a
// latestCapturedAt written in one UTC offset must still be correctly
// superseded by a genuinely later timestamp written in a different offset.
// Before formatISO normalized every write to UTC, a JST ("+09:00") write
// followed by a genuinely later UTC ("Z") write would lose the
// lexicographic compare purely because of the differing offset — freezing
// the dashboard's thumbnail while lastReceivedAt kept advancing.
func TestTouchDeviceLatest_NormalizesTimestampsAcrossOffsets(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-offset"), SK: deviceKey("d-offset"), EntityType: "Device",
		DeviceID: "d-offset", OwnerID: "owner-offset", Name: "n", Status: DeviceStatusPaired, Interval: 5,
		GSI1PK: ownerGSI1PK("owner-offset"), GSI1SK: deviceGSI1SK("d-offset"),
	}
	putRawItem(t, ctx, client, table, dev)

	// First upload: captured at 2026-06-15T09:00:00+09:00, i.e. 00:00:00Z.
	firstCaptured := time.Date(2026, 6, 15, 9, 0, 0, 0, ids.JST)
	got, err := s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "d-offset", ReceivedAt: firstCaptured, LatestCapturedAt: firstCaptured, LatestThumbnailKey: "thumb/jst",
	})
	if err != nil {
		t.Fatalf("TouchDeviceLatest (JST): %v", err)
	}
	if got.LatestThumbnailKey != "thumb/jst" {
		t.Fatalf("unexpected device after first touch: %+v", got)
	}

	// Second upload, one hour later in wall-clock time, expressed in UTC:
	// 2026-06-15T01:00:00Z. Lexicographically "...T01..." < "...T09...",
	// so without UTC normalization this would be wrongly rejected as
	// "older".
	secondCaptured := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)
	if !secondCaptured.After(firstCaptured) {
		t.Fatalf("test fixture is wrong: secondCaptured must be after firstCaptured, got %v vs %v", secondCaptured, firstCaptured)
	}
	got, err = s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "d-offset", ReceivedAt: secondCaptured, LatestCapturedAt: secondCaptured, LatestThumbnailKey: "thumb/utc-later",
	})
	if err != nil {
		t.Fatalf("TouchDeviceLatest (UTC, genuinely later): %v", err)
	}
	if got.LatestThumbnailKey != "thumb/utc-later" {
		t.Fatalf("a genuinely later capturedAt in a different UTC offset must still advance latest*, got %q", got.LatestThumbnailKey)
	}
	if got.LatestCapturedAt != secondCaptured.UTC().Format(time.RFC3339) {
		t.Fatalf("latestCapturedAt must be stored normalized to UTC, got %q", got.LatestCapturedAt)
	}
}

// TestTouchDeviceLatest_RejectsArchivedDevice pins Minor 7: an upload
// reaching TouchDeviceLatest for an ARCHIVED device must fail on both the
// primary and fallback paths — defense-in-depth alongside the authorizer,
// which is otherwise the only thing stopping this write.
func TestTouchDeviceLatest_RejectsArchivedDevice(t *testing.T) {
	ctx := context.Background()
	client := newIntegrationClient(t)
	table := createTestTable(t, client)
	s := New(client, table)

	dev := Device{
		PK: deviceKey("d-touch-arch"), SK: deviceKey("d-touch-arch"), EntityType: "Device",
		DeviceID: "d-touch-arch", OwnerID: "owner-touch-arch", Name: "n", Status: DeviceStatusPaired, Interval: 5,
		GSI1PK: ownerGSI1PK("owner-touch-arch"), GSI1SK: deviceGSI1SK("d-touch-arch"),
	}
	putRawItem(t, ctx, client, table, dev)

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := s.ArchiveDevice(ctx, "d-touch-arch", now); err != nil {
		t.Fatalf("ArchiveDevice: %v", err)
	}

	_, err := s.TouchDeviceLatest(ctx, TouchDeviceLatestInput{
		DeviceID: "d-touch-arch", ReceivedAt: now.Add(1 * time.Minute), LatestCapturedAt: now.Add(1 * time.Minute), LatestThumbnailKey: "thumb/should-not-write",
	})
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeDeviceNotFound {
		t.Fatalf("expected CodeDeviceNotFound writing to an ARCHIVED device, got %v", err)
	}

	// Neither path must have written anything: latestThumbnailKey stays
	// unset and lastReceivedAt must not have advanced either.
	got, err := s.GetDeviceForAuth(ctx, "d-touch-arch")
	if err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}
	if got.LatestThumbnailKey != "" || got.LastReceivedAt != "" {
		t.Fatalf("TouchDeviceLatest must not write anything to an ARCHIVED device, got %+v", got)
	}
}

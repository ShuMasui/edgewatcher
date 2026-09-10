package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/ShuMasui/edgewatcher/backend/internal/images"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

const fixedNow = "2026-09-08T02:00:00Z"

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad fixture timestamp %q: %v", s, err)
	}
	return ts
}

// fakeSigner returns a URL derived from the key so a test can tell which
// key was signed, and records every key it was asked for.
type fakeSigner struct {
	keys []string
	err  error
}

func (f *fakeSigner) SignGetObject(_ context.Context, key string, now time.Time) (images.SignedURL, error) {
	f.keys = append(f.keys, key)
	if f.err != nil {
		return images.SignedURL{}, f.err
	}
	return images.SignedURL{URL: "https://signed.example/" + key, ExpiresAt: now.Add(900 * time.Second).Unix()}, nil
}

func newMapper(f *fakeSigner) *Mapper { return NewMapper(f) }

// --- golden JSON: the wire contract ------------------------------------
//
// web/src/types/domain.ts is the binding contract (the plan's spec
// authority), and it is TypeScript — the compiler on the other side cannot
// see these structs. A renamed or dropped json tag is invisible to every
// Go test except one that looks at the bytes, so these tests look at the
// bytes.

// TestDeviceJSON_FieldNames pins Device's serialized shape against
// domain.ts's Device interface.
func TestDeviceJSON_FieldNames(t *testing.T) {
	m := newMapper(&fakeSigner{})
	dev := store.Device{
		DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関", Status: store.DeviceStatusPaired,
		Interval: 5, LastReceivedAt: "2026-09-08T01:55:00Z",
		LatestThumbnailKey: "observations/dev-1/2026-09-08/obs-1_thumb.jpg",
		LatestCapturedAt:   "2026-09-08T01:54:00Z",
		CreatedAt:          "2026-09-01T00:00:00Z",
	}

	got, err := m.Device(context.Background(), dev, nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("Device: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{
		"deviceId", "ownerId", "name", "status", "interval",
		"lastReceivedAt", "latestThumbnailUrl", "latestCapturedAt",
		"createdAt", "activePairingSession",
	}
	assertExactKeys(t, decoded, want)

	if decoded["latestThumbnailUrl"] != "https://signed.example/observations/dev-1/2026-09-08/obs-1_thumb.jpg" {
		t.Errorf("latestThumbnailUrl = %v, want the signed URL for latestThumbnailKey", decoded["latestThumbnailUrl"])
	}
}

// TestDeviceJSON_NeverLeaksCredentials is the reason this mapper exists at
// all rather than store.Device being serialized directly. store.Device
// carries deviceSecretHash and sessionTokenHash; a hash is not a secret, but
// nothing about the web contract needs them and shipping them hands an
// attacker the exact target value to compare a guess against.
func TestDeviceJSON_NeverLeaksCredentials(t *testing.T) {
	m := newMapper(&fakeSigner{})
	dev := store.Device{
		DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関", Status: store.DeviceStatusPaired,
		Interval:         5,
		DeviceSecretHash: "deadbeef", SessionTokenHash: "cafebabe", SessionExpiresAt: 99,
	}
	got, err := m.Device(context.Background(), dev, nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("Device: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"deadbeef", "cafebabe", "sessionTokenHash", "deviceSecretHash", "sessionExpiresAt"} {
		if bytesContain(raw, forbidden) {
			t.Errorf("serialized Device contains %q: %s", forbidden, raw)
		}
	}
}

// TestDeviceJSON_OmitsAbsentOptionals pins that a freshly created PENDING
// device — no uploads yet — omits the optional fields rather than sending
// empty strings. domain.ts marks them `?`, and the web treats "" as a
// present-but-empty value in some places (a thumbnail URL of "" would render
// a broken image).
func TestDeviceJSON_OmitsAbsentOptionals(t *testing.T) {
	m := newMapper(&fakeSigner{})
	dev := store.Device{
		DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関",
		Status: store.DeviceStatusPending, Interval: 5,
		CreatedAt: "2026-09-08T01:00:00Z",
	}
	got, err := m.Device(context.Background(), dev, nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("Device: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertExactKeys(t, decoded, []string{
		"deviceId", "ownerId", "name", "status", "interval", "createdAt", "activePairingSession",
	})
}

// TestDeviceJSON_ActivePairingSessionIsExplicitNull is the one optional
// field that must NOT be omitted. docs/03-web.md drives the dashboard's
// "ペアリング待ち" row off this field, and `undefined` and `null` are
// distinguishable in TypeScript: omitting it would leave a stale value in
// place on a client that merges responses, where an explicit null clears it.
func TestDeviceJSON_ActivePairingSessionIsExplicitNull(t *testing.T) {
	m := newMapper(&fakeSigner{})
	dev := store.Device{DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関", Status: store.DeviceStatusPaired, Interval: 5}

	got, err := m.Device(context.Background(), dev, nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("Device: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesContain(raw, `"activePairingSession":null`) {
		t.Errorf("want an explicit null activePairingSession, got: %s", raw)
	}
}

// TestPairingSessionJSON_FieldNames pins domain.ts's PairingSession.
func TestPairingSessionJSON_FieldNames(t *testing.T) {
	sess := store.PairingSession{
		PairingCode: "ABC123", ExpiresAt: 1788000000,
		Status: store.PairingStatusPending, CreatedAt: "2026-09-08T01:59:00Z",
	}
	raw, err := json.Marshal(PairingSessionDTO(sess))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertExactKeys(t, decoded, []string{"pairingCode", "expiresAt", "status", "createdAt"})
}

// TestObservationJSON_FieldNames pins domain.ts's Observation, and in
// particular that a list entry carries thumbnailUrl but NOT imageUrl.
// domain.ts marks imageUrl "when fetched on-demand": the full image is
// issued only by GET /observations/{id}/image (docs/05-backend.md §1.5), and
// signing every full image for a day's worth of observations would hand the
// browser a page of credentials it will not use.
func TestObservationJSON_FieldNames(t *testing.T) {
	m := newMapper(&fakeSigner{})
	lat, lng := 35.68, 139.76
	obs := []store.Observation{{
		ObservationID: "obs-1", DeviceID: "dev-1", CapturedAt: "2026-09-08T01:54:00Z",
		ImageKey:     "observations/dev-1/2026-09-08/obs-1.jpg",
		ThumbnailKey: "observations/dev-1/2026-09-08/obs-1_thumb.jpg",
		Lat:          &lat, Lng: &lng, ExpiresAt: 1788086400,
	}}

	got, err := m.Observations(context.Background(), obs, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("Observations: %v", err)
	}
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertExactKeys(t, decoded, []string{
		"observationId", "deviceId", "capturedAt", "thumbnailUrl", "lat", "lng", "expiresAt",
	})
	if decoded["thumbnailUrl"] != "https://signed.example/observations/dev-1/2026-09-08/obs-1_thumb.jpg" {
		t.Errorf("thumbnailUrl = %v, want the signed thumbnail key", decoded["thumbnailUrl"])
	}
}

// TestObservations_SignsOnlyThumbnails is the machine-checkable half of the
// test above: not merely that imageUrl is absent from the JSON, but that no
// full-size key was signed at all.
func TestObservations_SignsOnlyThumbnails(t *testing.T) {
	f := &fakeSigner{}
	m := newMapper(f)
	obs := []store.Observation{
		{ObservationID: "obs-1", DeviceID: "dev-1", ImageKey: "full/1.jpg", ThumbnailKey: "thumb/1.jpg"},
		{ObservationID: "obs-2", DeviceID: "dev-1", ImageKey: "full/2.jpg", ThumbnailKey: "thumb/2.jpg"},
	}
	if _, err := m.Observations(context.Background(), obs, mustParse(t, fixedNow)); err != nil {
		t.Fatalf("Observations: %v", err)
	}
	want := []string{"thumb/1.jpg", "thumb/2.jpg"}
	if len(f.keys) != len(want) {
		t.Fatalf("signed %v, want exactly %v", f.keys, want)
	}
	for i := range want {
		if f.keys[i] != want[i] {
			t.Errorf("signed key %d = %q, want %q", i, f.keys[i], want[i])
		}
	}
}

// TestObservations_EmptyIsEmptyArrayNotNull: a day with no photos must
// serialize as [] so the web's `.map` does not have to guard for null.
func TestObservations_EmptyIsEmptyArrayNotNull(t *testing.T) {
	m := newMapper(&fakeSigner{})
	got, err := m.Observations(context.Background(), nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("Observations: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("empty observations marshalled to %s, want []", raw)
	}
}

// --- GSI1 rows ----------------------------------------------------------

// TestDeviceFromRow_RecoversOwnerID guards the one field a GSI1 row cannot
// carry: docs/engineering/dynamodb.md §4's INCLUDE projection omits ownerId,
// so it has to come back out of GSI1PK. A mapper that read a (nonexistent)
// ownerId attribute would produce "" and ship a Device the web cannot match
// to its own user.
func TestDeviceFromRow_RecoversOwnerID(t *testing.T) {
	m := newMapper(&fakeSigner{})
	row := store.OwnerListRow{
		GSI1PK: "OWNER#owner-1", GSI1SK: "DEVICE#dev-1",
		DeviceID: "dev-1", Name: "玄関", Status: store.DeviceStatusPaired, Interval: 5,
	}
	got, err := m.DeviceFromRow(context.Background(), row, nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("DeviceFromRow: %v", err)
	}
	if got.OwnerID != "owner-1" {
		t.Errorf("OwnerID = %q, want %q", got.OwnerID, "owner-1")
	}
}

// TestDeviceFromRow_OmitsThumbnailWhenNoKey: a device that has never
// uploaded has no latestThumbnailKey, and asking the signer for "" would
// produce a working URL to a nonexistent object — a broken image in the
// dashboard instead of the "まだ画像がありません" placeholder.
func TestDeviceFromRow_OmitsThumbnailWhenNoKey(t *testing.T) {
	f := &fakeSigner{}
	m := newMapper(f)
	row := store.OwnerListRow{
		GSI1PK: "OWNER#owner-1", GSI1SK: "DEVICE#dev-1",
		DeviceID: "dev-1", Name: "玄関", Status: store.DeviceStatusPending, Interval: 5,
	}
	got, err := m.DeviceFromRow(context.Background(), row, nil, mustParse(t, fixedNow))
	if err != nil {
		t.Fatalf("DeviceFromRow: %v", err)
	}
	if got.LatestThumbnailURL != "" {
		t.Errorf("LatestThumbnailURL = %q, want empty", got.LatestThumbnailURL)
	}
	if len(f.keys) != 0 {
		t.Errorf("signer was called with %v, want no call at all", f.keys)
	}
}

// TestDevice_SignerFailurePropagates: a signing failure must not be
// swallowed into a Device with a blank thumbnail. Silently degrading here
// would present "this device has never uploaded" for a device that has.
func TestDevice_SignerFailurePropagates(t *testing.T) {
	wantErr := errors.New("presign exploded")
	m := newMapper(&fakeSigner{err: wantErr})
	dev := store.Device{
		DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired,
		LatestThumbnailKey: "thumb/1.jpg",
	}
	if _, err := m.Device(context.Background(), dev, nil, mustParse(t, fixedNow)); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, wantErr)
	}
}

// --- active pairing session policy --------------------------------------

// TestActivePairingSession pins the join rule docs/05-backend.md §1.2 and
// docs/06-auth.md §2 imply: the dashboard shows a QR only while it can still
// be scanned. A consumed code is useless, and an expired one is refused by
// ConsumePairing's condition — surfacing either would show the owner a QR
// that silently does nothing.
func TestActivePairingSession(t *testing.T) {
	now := mustParse(t, fixedNow)
	cases := []struct {
		name    string
		session *store.PairingSession
		wantNil bool
	}{
		{"no session at all", nil, true},
		{"pending and unexpired", &store.PairingSession{Status: store.PairingStatusPending, ExpiresAt: now.Add(time.Minute).Unix()}, false},
		{"consumed", &store.PairingSession{Status: store.PairingStatusConsumed, ExpiresAt: now.Add(time.Minute).Unix()}, true},
		{"expired", &store.PairingSession{Status: store.PairingStatusPending, ExpiresAt: now.Add(-time.Second).Unix()}, true},
		// The boundary matches ConsumePairing's condition, expiresAt > now:
		// at exactly expiresAt the code is already refused server-side, so
		// showing it would be a QR that cannot be redeemed.
		{"exactly at expiry", &store.PairingSession{Status: store.PairingStatusPending, ExpiresAt: now.Unix()}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ActivePairingSession(tc.session, now)
			if tc.wantNil && got != nil {
				t.Fatalf("got %+v, want nil", got)
			}
			if !tc.wantNil && got == nil {
				t.Fatal("got nil, want a session")
			}
		})
	}
}

// --- app config ---------------------------------------------------------

// TestAppConfigJSON_FieldNames pins domain.ts's AppConfig.
func TestAppConfigJSON_FieldNames(t *testing.T) {
	raw, err := json.Marshal(AppConfig{RetentionDays: 1, DeviceLimit: 10, IntervalOptions: IntervalOptions})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertExactKeys(t, decoded, []string{"retentionDays", "deviceLimit", "intervalOptions"})
}

// TestIntervalOptions matches docs/03-web.md's 5/10/15 minute choices, which
// PATCH /devices/{id} validates against.
func TestIntervalOptions(t *testing.T) {
	if got, want := IntervalOptions, []int{5, 10, 15}; len(got) != len(want) {
		t.Fatalf("IntervalOptions = %v, want %v", got, want)
	}
	for i := range IntervalOptions {
		if IntervalOptions[i] != []int{5, 10, 15}[i] {
			t.Fatalf("IntervalOptions = %v, want [5 10 15]", IntervalOptions)
		}
	}
}

// TestDomainTypesStillDeclareTheseFields reads web/src/types/domain.ts and
// asserts every json tag this package emits is named there.
//
// The golden tests above pin Go against a list typed by hand in Go, which
// drifts the moment someone edits the TypeScript. This one reads the actual
// contract file, so renaming a field in domain.ts fails the Go build's
// tests rather than surfacing as a runtime `undefined` in the browser.
func TestDomainTypesStillDeclareTheseFields(t *testing.T) {
	src, err := os.ReadFile("../../../web/src/types/domain.ts")
	if err != nil {
		t.Skipf("domain.ts not readable from here (%v); the golden tests above still apply", err)
	}
	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(\w+)\??:`).FindAllStringSubmatch(string(src), -1) {
		declared[m[1]] = true
	}
	emitted := []string{
		"deviceId", "ownerId", "name", "status", "interval", "lastReceivedAt",
		"latestThumbnailUrl", "latestCapturedAt", "createdAt", "archivedAt",
		"activePairingSession", "pairingCode", "expiresAt", "consumedAt",
		"observationId", "capturedAt", "thumbnailUrl", "imageUrl", "lat", "lng",
		"retentionDays", "deviceLimit", "intervalOptions",
	}
	for _, field := range emitted {
		if !declared[field] {
			t.Errorf("this package emits %q but web/src/types/domain.ts does not declare it", field)
		}
	}
}

// --- helpers ------------------------------------------------------------

func assertExactKeys(t *testing.T, got map[string]any, want []string) {
	t.Helper()
	gotKeys := make([]string, 0, len(got))
	for k := range got {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)

	if len(gotKeys) != len(sorted) {
		t.Fatalf("keys = %v, want %v", gotKeys, sorted)
	}
	for i := range sorted {
		if gotKeys[i] != sorted[i] {
			t.Fatalf("keys = %v, want %v", gotKeys, sorted)
		}
	}
}

func bytesContain(haystack []byte, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(string(haystack), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

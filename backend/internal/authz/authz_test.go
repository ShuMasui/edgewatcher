package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
	"github.com/ShuMasui/edgewatcher/backend/internal/tokens"
)

// fakeStore is an in-memory DeviceGetter stub keyed by deviceId, so tests
// never touch a real AWS account or DynamoDB (per the task's hard
// requirement). A deviceId mapped to a nil *store.Device with ok=true
// (see nilDevices) exercises the "found, but nil" shape M5 guards against.
type fakeStore struct {
	devices    map[string]*store.Device
	nilDevices map[string]bool
}

func (f *fakeStore) GetDeviceForAuth(_ context.Context, deviceID string) (*store.Device, error) {
	if f.nilDevices[deviceID] {
		return nil, nil
	}
	dev, ok := f.devices[deviceID]
	if !ok {
		return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
	}
	return dev, nil
}

const fixedNowUnix = 1_700_000_000 // arbitrary fixed instant

func fixedClock() clock.Clock {
	return clock.Fixed{At: time.Unix(fixedNowUnix, 0)}
}

func pairedDevice(deviceID, ownerID, token string, expiresAt int64) *store.Device {
	return &store.Device{
		DeviceID:         deviceID,
		OwnerID:          ownerID,
		Status:           store.DeviceStatusPaired,
		SessionTokenHash: tokens.HashSessionToken(token),
		SessionExpiresAt: expiresAt,
	}
}

func newAuthorizer(devices map[string]*store.Device, logger *slog.Logger) *Authorizer {
	return New(&fakeStore{devices: devices}, fixedClock(), logger)
}

func requestWithAuth(header string) events.APIGatewayV2CustomAuthorizerV2Request {
	req := events.APIGatewayV2CustomAuthorizerV2Request{}
	if header != "" {
		req.Headers = map[string]string{"authorization": header}
	}
	return req
}

// TestAuthorize_ValidToken uses the bare token — the documented wire
// format (docs/06-auth.md §3, docs/05-backend.md §1.1: "Authorization:
// <sessionToken>", no scheme) — as the primary, canonical shape.
func TestAuthorize_ValidToken(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	resp, err := a.Authorize(context.Background(), requestWithAuth(token))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if !resp.IsAuthorized {
		t.Fatal("expected IsAuthorized true for a valid bare token")
	}
	if resp.Context["deviceId"] != "dev-1" {
		t.Errorf("context deviceId = %v, want dev-1", resp.Context["deviceId"])
	}
	if resp.Context["ownerId"] != "owner-1" {
		t.Errorf("context ownerId = %v, want owner-1", resp.Context["ownerId"])
	}
}

// TestAuthorize_BearerPrefixTolerated pins the other accepted shape: an
// optional "Bearer " prefix must not change the outcome. Ruling R12:
// tolerated, not required — the bare token above is the documented,
// primary form.
func TestAuthorize_BearerPrefixTolerated(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if !resp.IsAuthorized {
		t.Fatal("expected IsAuthorized true when the token carries an optional 'Bearer ' prefix")
	}
	if resp.Context["deviceId"] != "dev-1" {
		t.Errorf("context deviceId = %v, want dev-1", resp.Context["deviceId"])
	}
}

func TestAuthorize_MissingHeader(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	req := events.APIGatewayV2CustomAuthorizerV2Request{} // Headers is nil
	resp, err := a.Authorize(context.Background(), req)
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false when the header map is nil")
	}
}

func TestAuthorize_UppercaseHeaderIsIgnored(t *testing.T) {
	// Payload format 2.0 always lowercases header keys; a real request
	// never has "Authorization". This guards against the header lookup
	// silently changing back to the capitalized key.
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	req := events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"Authorization": token},
	}
	resp, err := a.Authorize(context.Background(), req)
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false: only the lowercase 'authorization' key must be read")
	}
}

func TestAuthorize_MalformedHeader(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	cases := []string{
		"",               // empty header value
		"Bearer ",        // empty token after stripping the prefix
		"notoken",        // no "<deviceId>." separator at all
		".novalue",       // empty deviceId before the separator
		"Bearer notoken", // still no separator once the prefix is stripped
	}
	for _, header := range cases {
		t.Run(header, func(t *testing.T) {
			resp, err := a.Authorize(context.Background(), requestWithAuth(header))
			if err != nil {
				t.Fatalf("Authorize returned error: %v", err)
			}
			if resp.IsAuthorized {
				t.Fatalf("expected IsAuthorized false for malformed header %q", header)
			}
		})
	}
}

func TestAuthorize_UnknownDevice(t *testing.T) {
	a := newAuthorizer(map[string]*store.Device{}, nil)

	resp, err := a.Authorize(context.Background(), requestWithAuth("unknown-device.random"))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false for an unknown device")
	}
}

// TestAuthorize_NilDeviceFromStore pins M5: DeviceGetter is a public
// interface, and a (nil, nil) return — "found, but nil" — is a shape
// store.GetDeviceForAuth never produces today but that the interface
// itself does not forbid. Without an explicit guard this would panic on
// dev.Status, turning a deny into a 500 rather than a denial.
func TestAuthorize_NilDeviceFromStore(t *testing.T) {
	a := &Authorizer{
		store:  &fakeStore{devices: map[string]*store.Device{}, nilDevices: map[string]bool{"dev-1": true}},
		clock:  fixedClock(),
		logger: nil,
	}

	resp, err := a.Authorize(context.Background(), requestWithAuth("dev-1.somerandomvalue"))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false when the store returns a nil device with no error")
	}
}

func TestAuthorize_StatusNotPaired(t *testing.T) {
	token := "dev-1.somerandomvalue"

	statuses := []string{
		store.DeviceStatusPending,
		store.DeviceStatusDisconnected,
		store.DeviceStatusArchived,
	}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
			dev.Status = status
			a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

			resp, err := a.Authorize(context.Background(), requestWithAuth(token))
			if err != nil {
				t.Fatalf("Authorize returned error: %v", err)
			}
			if resp.IsAuthorized {
				t.Fatalf("expected IsAuthorized false for status %q", status)
			}
		})
	}
}

func TestAuthorize_HashMismatch(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	// A different token with the same deviceId prefix: GetItem succeeds,
	// but the hash must not match.
	resp, err := a.Authorize(context.Background(), requestWithAuth("dev-1.wrongvalue"))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false for a hash mismatch")
	}
}

// TestAuthorize_DisconnectedDeviceEmptyHash pins M4: a DISCONNECTED/
// ARCHIVED-shaped device with no session at all (empty hash, zero
// expiry) must deny. Status alone already denies such a device today,
// but this goes through the full Authorize path with a PAIRED status and
// an empty hash so the empty-hash guard in validHash is the thing
// actually pinned, independent of check ordering.
func TestAuthorize_DisconnectedDeviceEmptyHash(t *testing.T) {
	dev := &store.Device{
		DeviceID:         "dev-1",
		OwnerID:          "owner-1",
		Status:           store.DeviceStatusPaired,
		SessionTokenHash: "",
		SessionExpiresAt: 0,
	}
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	// Even an empty-string "token" must not validate against an empty
	// stored hash.
	resp, err := a.Authorize(context.Background(), requestWithAuth("dev-1."))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false when SessionTokenHash is empty")
	}
}

func TestAuthorize_ExpiredSessionBoundary(t *testing.T) {
	token := "dev-1.somerandomvalue"

	t.Run("exactly now is expired", func(t *testing.T) {
		dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix) // == now
		a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

		resp, err := a.Authorize(context.Background(), requestWithAuth(token))
		if err != nil {
			t.Fatalf("Authorize returned error: %v", err)
		}
		if resp.IsAuthorized {
			t.Fatal("expected IsAuthorized false when sessionExpiresAt == now")
		}
	})

	t.Run("one second past is expired", func(t *testing.T) {
		dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix-1)
		a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

		resp, err := a.Authorize(context.Background(), requestWithAuth(token))
		if err != nil {
			t.Fatalf("Authorize returned error: %v", err)
		}
		if resp.IsAuthorized {
			t.Fatal("expected IsAuthorized false when sessionExpiresAt < now")
		}
	})

	t.Run("one second future is valid", func(t *testing.T) {
		dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+1)
		a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

		resp, err := a.Authorize(context.Background(), requestWithAuth(token))
		if err != nil {
			t.Fatalf("Authorize returned error: %v", err)
		}
		if !resp.IsAuthorized {
			t.Fatal("expected IsAuthorized true when sessionExpiresAt is one second after now")
		}
	})
}

// TestValidHash_MatchesOwnHash checks validHash's pass/fail behavior. It
// does NOT, and cannot, distinguish subtle.ConstantTimeCompare from a
// naive bytes.Equal/== — both produce identical true/false results for
// every input this package ever compares (fixed-width SHA-256 hex
// digests on both sides), so no functional test can catch a regression
// from one to the other. That property rests on the crypto/subtle import
// surviving code review, not on anything asserted here.
func TestValidHash_MatchesOwnHash(t *testing.T) {
	if !validHash("token-1", tokens.HashSessionToken("token-1")) {
		t.Fatal("validHash must accept a token against its own hash")
	}
	if validHash("token-1", tokens.HashSessionToken("token-2")) {
		t.Fatal("validHash must reject a token against a different token's hash")
	}
}

// TestAuthorize_NoSecretInLogs asserts the secret token value never
// appears in emitted log bytes, across both the success and failure
// paths — reusing the pattern internal/logging established (assert the
// secret's value is absent from the logged output).
func TestAuthorize_NoSecretInLogs(t *testing.T) {
	token := "dev-1.top-secret-random-value"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, logger)

	// Success path.
	if _, err := a.Authorize(context.Background(), requestWithAuth(token)); err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	// Failure path (hash mismatch) with a related but distinct token.
	if _, err := a.Authorize(context.Background(), requestWithAuth("dev-1.another-secret-value")); err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}

	logged := buf.String()
	if bytes.Contains(buf.Bytes(), []byte(token)) {
		t.Fatalf("log output leaked the session token: %s", logged)
	}
	if bytes.Contains(buf.Bytes(), []byte("another-secret-value")) {
		t.Fatalf("log output leaked the rejected token: %s", logged)
	}
	if bytes.Contains(buf.Bytes(), []byte(dev.SessionTokenHash)) {
		t.Fatalf("log output leaked the session token hash: %s", logged)
	}

	// Sanity: something was actually logged, so the assertions above are
	// not vacuously true.
	lines := 0
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for {
		var v map[string]any
		if err := dec.Decode(&v); err != nil {
			break
		}
		lines++
	}
	if lines < 2 {
		t.Fatalf("expected at least 2 log lines (success + failure), got %d: %s", lines, logged)
	}
}

func TestExtractToken(t *testing.T) {
	cases := []struct {
		name       string
		headers    map[string]string
		wantOK     bool
		wantToken  string
		wantDevice string
	}{
		{"nil headers", nil, false, "", ""},
		{"no authorization key", map[string]string{"content-type": "application/json"}, false, "", ""},
		{"bare token is the documented, primary format", map[string]string{"authorization": "dev-1.abc"}, true, "dev-1.abc", "dev-1"},
		{"optional Bearer prefix tolerated", map[string]string{"authorization": "Bearer dev-1.abc"}, true, "dev-1.abc", "dev-1"},
		{"empty token", map[string]string{"authorization": "Bearer "}, false, "", ""},
		{"no dot separator", map[string]string{"authorization": "abcdef"}, false, "", ""},
		{"empty device id", map[string]string{"authorization": ".abcdef"}, false, "", ""},
		{"capitalized key rejected", map[string]string{"Authorization": "dev-1.abcdef"}, false, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, deviceID, ok := extractToken(tc.headers)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok {
				if token != tc.wantToken {
					t.Errorf("token = %q, want %q", token, tc.wantToken)
				}
				if deviceID != tc.wantDevice {
					t.Errorf("deviceID = %q, want %q", deviceID, tc.wantDevice)
				}
			}
		})
	}
}

// TestAuthorize_WrongAuthScheme pins the one behaviour the Bearer-tolerance
// change (review round 1, C1) moved without breaking: a scheme other than
// the documented bare token or the tolerated "Bearer " prefix must still
// deny. The denial no longer happens in extractToken — "Basic dev-1.abc"
// parses cleanly into deviceId "Basic dev-1" — it happens one layer down
// at the device lookup, which finds nothing under that garbage id. Nothing
// asserted that after the inversion, so a future widening of the prefix
// tolerance (say, a case-insensitive or scheme-agnostic strip) could start
// authorizing these without failing a test.
//
// The store here holds a genuinely valid dev-1 session, so a pass would
// mean the wrong-scheme header successfully reached a real device — not
// merely that some unrelated lookup missed.
func TestAuthorize_WrongAuthScheme(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	cases := map[string]string{
		"Basic scheme":          "Basic " + token,
		"lowercase bearer":      "bearer " + token,
		"doubled Bearer prefix": "Bearer Bearer " + token,
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := a.Authorize(context.Background(), requestWithAuth(header))
			if err != nil {
				t.Fatalf("Authorize returned error: %v", err)
			}
			if resp.IsAuthorized {
				t.Fatalf("expected IsAuthorized false for header %q", header)
			}
		})
	}
}

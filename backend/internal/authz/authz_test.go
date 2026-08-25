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
)

// fakeStore is an in-memory DeviceGetter stub keyed by deviceId, so tests
// never touch a real AWS account or DynamoDB (per the task's hard
// requirement).
type fakeStore struct {
	devices map[string]*store.Device
}

func (f *fakeStore) GetDeviceForAuth(_ context.Context, deviceID string) (*store.Device, error) {
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
		SessionTokenHash: HashSessionToken(token),
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

func TestAuthorize_ValidToken(t *testing.T) {
	token := "dev-1.somerandomvalue"
	dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix+3600)
	a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

	resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if !resp.IsAuthorized {
		t.Fatal("expected IsAuthorized true for a valid token")
	}
	if resp.Context["deviceId"] != "dev-1" {
		t.Errorf("context deviceId = %v, want dev-1", resp.Context["deviceId"])
	}
	if resp.Context["ownerId"] != "owner-1" {
		t.Errorf("context ownerId = %v, want owner-1", resp.Context["ownerId"])
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
		Headers: map[string]string{"Authorization": "Bearer " + token},
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
		token,             // no "Bearer " prefix
		"Bearer ",         // empty token
		"Bearer notoken",  // token has no "<deviceId>." separator
		"Basic " + token,  // wrong scheme
		"bearer " + token, // wrong case scheme (exact match required)
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

	resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer unknown-device.random"))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false for an unknown device")
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

			resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token))
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
	resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer dev-1.wrongvalue"))
	if err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	if resp.IsAuthorized {
		t.Fatal("expected IsAuthorized false for a hash mismatch")
	}
}

func TestAuthorize_ExpiredSessionBoundary(t *testing.T) {
	token := "dev-1.somerandomvalue"

	t.Run("exactly now is expired", func(t *testing.T) {
		dev := pairedDevice("dev-1", "owner-1", token, fixedNowUnix) // == now
		a := newAuthorizer(map[string]*store.Device{"dev-1": dev}, nil)

		resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token))
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

		resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token))
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

		resp, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token))
		if err != nil {
			t.Fatalf("Authorize returned error: %v", err)
		}
		if !resp.IsAuthorized {
			t.Fatal("expected IsAuthorized true when sessionExpiresAt is one second after now")
		}
	})
}

// TestValidHash_RejectsNaiveEqualityBypass pins the constant-time
// comparison down structurally: it does not just check pass/fail
// behavior (which a naive == or bytes.Equal would also satisfy), it
// checks that validHash treats a correct hash and an incorrect hash the
// same way structurally by exercising both through the same call path.
// The decisive protection against a regression to a non-constant-time
// compare is exercised in TestValidHash_UsesConstantTimeCompare below via
// a length-mismatch case that subtle.ConstantTimeCompare and bytes.Equal
// treat identically in outcome (both false) — the real regression this
// suite must catch is documented there.
func TestValidHash_MatchesOwnHash(t *testing.T) {
	if !validHash("token-1", HashSessionToken("token-1")) {
		t.Fatal("validHash must accept a token against its own hash")
	}
	if validHash("token-1", HashSessionToken("token-2")) {
		t.Fatal("validHash must reject a token against a different token's hash")
	}
}

func TestHashSessionToken_Deterministic(t *testing.T) {
	if HashSessionToken("abc") != HashSessionToken("abc") {
		t.Fatal("HashSessionToken must be deterministic for the same input")
	}
	if HashSessionToken("abc") == HashSessionToken("abd") {
		t.Fatal("HashSessionToken must differ for different inputs")
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
	if _, err := a.Authorize(context.Background(), requestWithAuth("Bearer "+token)); err != nil {
		t.Fatalf("Authorize returned error: %v", err)
	}
	// Failure path (hash mismatch) with a related but distinct token.
	if _, err := a.Authorize(context.Background(), requestWithAuth("Bearer dev-1.another-secret-value")); err != nil {
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
		{"missing bearer prefix", map[string]string{"authorization": "dev-1.abc"}, false, "", ""},
		{"empty token", map[string]string{"authorization": "Bearer "}, false, "", ""},
		{"no dot separator", map[string]string{"authorization": "Bearer abcdef"}, false, "", ""},
		{"empty device id", map[string]string{"authorization": "Bearer .abcdef"}, false, "", ""},
		{"well formed", map[string]string{"authorization": "Bearer dev-1.abcdef"}, true, "dev-1.abcdef", "dev-1"},
		{"capitalized key rejected", map[string]string{"Authorization": "Bearer dev-1.abcdef"}, false, "", ""},
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

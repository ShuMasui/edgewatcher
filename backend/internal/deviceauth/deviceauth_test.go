package deviceauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/authz"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
	"github.com/ShuMasui/edgewatcher/backend/internal/tokens"
)

var fixedNow = time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC)

// fakeStore implements both this package's Store and authz.DeviceGetter, so
// one fixture can carry a device through issuance and then through
// authorization in the round-trip test.
type fakeStore struct {
	devices  map[string]*store.Device
	sessions map[string]*store.PairingSession // keyed by pairing code

	findErr    error
	consumeErr error
	rotateErr  error

	consumed []store.ConsumePairingInput
	rotated  []rotateCall
}

type rotateCall struct {
	deviceID  string
	hash      string
	expiresAt int64
}

func (f *fakeStore) FindPairingByCode(_ context.Context, code string) (*store.PairingSession, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	s, ok := f.sessions[code]
	if !ok {
		return nil, apierr.New(apierr.CodePairingNotFound, "pairing code not found")
	}
	return s, nil
}

func (f *fakeStore) ConsumePairing(_ context.Context, in store.ConsumePairingInput) error {
	f.consumed = append(f.consumed, in)
	if f.consumeErr != nil {
		return f.consumeErr
	}
	if dev, ok := f.devices[in.DeviceID]; ok {
		dev.Status = store.DeviceStatusPaired
		dev.DeviceSecretHash = in.DeviceSecretHash
		dev.DeviceInfo = &in.DeviceInfo
	}
	return nil
}

func (f *fakeStore) GetDeviceForAuth(_ context.Context, deviceID string) (*store.Device, error) {
	dev, ok := f.devices[deviceID]
	if !ok {
		return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
	}
	return dev, nil
}

func (f *fakeStore) RotateSessionToken(_ context.Context, deviceID, hash string, expiresAt int64) (*store.Device, error) {
	f.rotated = append(f.rotated, rotateCall{deviceID, hash, expiresAt})
	if f.rotateErr != nil {
		return nil, f.rotateErr
	}
	dev, ok := f.devices[deviceID]
	if !ok {
		return nil, apierr.New(apierr.CodeUnauthorized, "device not eligible for a session")
	}
	dev.SessionTokenHash = hash
	dev.SessionExpiresAt = expiresAt
	return dev, nil
}

func newHandler(f *fakeStore, logger *slog.Logger) *Handler {
	return New(f, clock.Fixed{At: fixedNow}, logger)
}

func jsonRequest(body string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{Body: body}
}

func assertCode(t *testing.T, err error, want apierr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with code %s, got nil", want)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *apierr.Error", err)
	}
	if apiErr.Code != want {
		t.Fatalf("code = %s, want %s", apiErr.Code, want)
	}
}

func decodeBody[T any](t *testing.T, resp httpx.Response) T {
	t.Helper()
	raw, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal response body: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response body: %v", err)
	}
	return out
}

func pendingFixture() *fakeStore {
	return &fakeStore{
		devices: map[string]*store.Device{
			"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関", Status: store.DeviceStatusPending, Interval: 5},
		},
		sessions: map[string]*store.PairingSession{
			"CODE123": {DeviceID: "dev-1", OwnerID: "owner-1", PairingCode: "CODE123",
				Status: store.PairingStatusPending, ExpiresAt: fixedNow.Add(5 * time.Minute).Unix()},
		},
	}
}

// --- POST /device/pair ---------------------------------------------------

// TestPair_Success walks the happy path of docs/06-auth.md §2 and pins the
// one property the whole credential model rests on: the response carries a
// deviceSecret, and what reaches the store is only its hash.
func TestPair_Success(t *testing.T) {
	f := pendingFixture()
	h := newHandler(f, nil)

	resp, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"CODE123","deviceInfo":{"model":"Pixel 8","osVersion":"14","appVersion":"1.0.0"}}`))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	body := decodeBody[PairResponse](t, resp)

	if body.DeviceID != "dev-1" {
		t.Errorf("deviceId = %q, want %q", body.DeviceID, "dev-1")
	}
	if body.DeviceSecret == "" {
		t.Fatal("deviceSecret is empty")
	}
	if len(f.consumed) != 1 {
		t.Fatalf("ConsumePairing called %d times, want 1", len(f.consumed))
	}
	got := f.consumed[0]
	if got.DeviceSecretHash == body.DeviceSecret {
		t.Fatal("the raw deviceSecret was stored — only its hash may be persisted")
	}
	if want := tokens.HashDeviceSecret(body.DeviceSecret); got.DeviceSecretHash != want {
		t.Errorf("stored hash = %q, want tokens.HashDeviceSecret(secret) = %q", got.DeviceSecretHash, want)
	}
	if got.PairingCode != "CODE123" || got.DeviceID != "dev-1" {
		t.Errorf("ConsumePairing input = %+v, want the resolved deviceId and the submitted code", got)
	}
	if !got.Now.Equal(fixedNow) {
		t.Errorf("ConsumePairing Now = %v, want the handler's clock %v", got.Now, fixedNow)
	}
}

// TestPair_StoresDeviceInfo: docs/06-auth.md §2 has the pairing transaction
// set deviceInfo alongside the credential, and it is the only point in the
// system where the device describes itself. Dropping it would leave the
// Web's device detail permanently blank with no error anywhere.
func TestPair_StoresDeviceInfo(t *testing.T) {
	f := pendingFixture()
	h := newHandler(f, nil)

	if _, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"CODE123","deviceInfo":{"model":"Pixel 8","osVersion":"14","appVersion":"1.0.0"}}`)); err != nil {
		t.Fatalf("Pair: %v", err)
	}
	got := f.consumed[0].DeviceInfo
	if got.Model != "Pixel 8" || got.OSVersion != "14" || got.AppVersion != "1.0.0" {
		t.Errorf("DeviceInfo = %+v, want the submitted values", got)
	}
}

// TestPair_UnknownCode is 404, and the distinction from the two 409s below
// is load-bearing rather than cosmetic: docs/06-auth.md §2 has GSI2 as
// eventually consistent, so a device that scans a just-issued QR can see a
// spurious miss and retries up to three times (docs/04-native.md §1.4).
// 404 is the ONLY case it retries. Returning 409 here would abandon a
// pairing that would have succeeded a second later; returning 404 for a
// consumed code would make a device retry a QR that can never work.
func TestPair_UnknownCode(t *testing.T) {
	h := newHandler(pendingFixture(), nil)
	_, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"NOPE","deviceInfo":{}}`))
	assertCode(t, err, apierr.CodePairingNotFound)
}

// TestPair_ConsumedCode: 409 PAIRING_CODE_CONSUMED, surfaced from the
// store's transaction classification rather than from a read-then-write
// check here — the single-use guarantee is the ConditionExpression's, and
// re-deriving it in the handler would open the double-scan race the
// condition exists to close (docs/06-auth.md §2).
func TestPair_ConsumedCode(t *testing.T) {
	f := pendingFixture()
	f.consumeErr = apierr.New(apierr.CodePairingCodeConsumed, "pairing code already consumed")
	h := newHandler(f, nil)

	_, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"CODE123","deviceInfo":{}}`))
	assertCode(t, err, apierr.CodePairingCodeConsumed)
}

// TestPair_ExpiredCode: 409 PAIRING_CODE_EXPIRED.
func TestPair_ExpiredCode(t *testing.T) {
	f := pendingFixture()
	f.consumeErr = apierr.New(apierr.CodePairingCodeExpired, "pairing code expired")
	h := newHandler(f, nil)

	_, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"CODE123","deviceInfo":{}}`))
	assertCode(t, err, apierr.CodePairingCodeExpired)
}

// TestPair_ExpiredIsDecidedByTheStoreNotTheHandler. FindPairingByCode
// returns expired sessions happily; only ConsumePairing's condition rejects
// them. A handler that pre-checked expiry from the read would answer 409 for
// a session that expired between the read and the write and 200 for one that
// expired the other way round — two clocks disagreeing about one fact. This
// pins that an expired-looking session still reaches the store.
func TestPair_ExpiredIsDecidedByTheStoreNotTheHandler(t *testing.T) {
	f := pendingFixture()
	f.sessions["CODE123"].ExpiresAt = fixedNow.Add(-time.Second).Unix()
	f.consumeErr = apierr.New(apierr.CodePairingCodeExpired, "pairing code expired")
	h := newHandler(f, nil)

	_, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"CODE123","deviceInfo":{}}`))
	assertCode(t, err, apierr.CodePairingCodeExpired)
	if len(f.consumed) != 1 {
		t.Errorf("ConsumePairing called %d times, want 1 — expiry must be decided by the condition, not pre-checked", len(f.consumed))
	}
}

// TestPair_MissingCode: an empty pairingCode is a malformed request, not a
// lookup that happens to miss. Passing "" to FindPairingByCode would spend a
// GSI2 query to learn what the shape of the request already said.
func TestPair_MissingCode(t *testing.T) {
	h := newHandler(pendingFixture(), nil)
	for _, body := range []string{`{}`, `{"pairingCode":""}`, `{"pairingCode":"   "}`} {
		_, err := h.Pair(context.Background(), jsonRequest(body))
		assertCode(t, err, apierr.CodeValidation)
	}
}

func TestPair_MalformedBody(t *testing.T) {
	h := newHandler(pendingFixture(), nil)
	_, err := h.Pair(context.Background(), jsonRequest(`{ not json`))
	assertCode(t, err, apierr.CodeValidation)
}

// TestPair_NoSecretsInLogs. This handler is the one place both a pairingCode
// and a deviceSecret exist in plaintext in the same function, and
// docs/05-backend.md §2.5 forbids either reaching a log — where they would
// stay readable for the log group's 30-day retention, far outliving the
// 5-minute QR they came from.
func TestPair_NoSecretsInLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f := pendingFixture()
	h := newHandler(f, logger)

	resp, err := h.Pair(context.Background(), jsonRequest(`{"pairingCode":"CODE123","deviceInfo":{"model":"Pixel 8"}}`))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	body := decodeBody[PairResponse](t, resp)

	logged := buf.String()
	if strings.Contains(logged, "CODE123") {
		t.Errorf("the pairing code appears in the log: %s", logged)
	}
	if strings.Contains(logged, body.DeviceSecret) {
		t.Errorf("the device secret appears in the log: %s", logged)
	}
}

// --- POST /device/token --------------------------------------------------

func pairedFixture(secret string) *fakeStore {
	return &fakeStore{
		devices: map[string]*store.Device{
			"dev-1": {
				DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関",
				Status: store.DeviceStatusPaired, Interval: 5,
				DeviceSecretHash: tokens.HashDeviceSecret(secret),
			},
		},
	}
}

// TestToken_Success pins the token's shape and its 12-hour life
// (docs/06-auth.md §3), and that only the hash is persisted.
func TestToken_Success(t *testing.T) {
	f := pairedFixture("s3cret")
	h := newHandler(f, nil)

	resp, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	body := decodeBody[TokenResponse](t, resp)

	deviceID, _, found := strings.Cut(body.SessionToken, ".")
	if !found || deviceID != "dev-1" {
		t.Errorf("sessionToken = %q, want the form <deviceId>.<random>", body.SessionToken)
	}
	if want := fixedNow.Add(SessionTTL).Unix(); body.ExpiresAt != want {
		t.Errorf("expiresAt = %d, want now+12h = %d", body.ExpiresAt, want)
	}
	if len(f.rotated) != 1 {
		t.Fatalf("RotateSessionToken called %d times, want 1", len(f.rotated))
	}
	got := f.rotated[0]
	if got.hash == body.SessionToken {
		t.Fatal("the raw sessionToken was stored — only its hash may be persisted")
	}
	if want := tokens.HashSessionToken(body.SessionToken); got.hash != want {
		t.Errorf("stored hash = %q, want tokens.HashSessionToken(token) = %q", got.hash, want)
	}
	if got.expiresAt != body.ExpiresAt {
		t.Errorf("stored expiry %d disagrees with the reported %d", got.expiresAt, body.ExpiresAt)
	}
}

// TestToken_RotationInvalidatesThePrevious. "One session per device,
// rotated on every exchange" (docs/06-auth.md §3) is what keeps a stolen
// token from outliving the next refresh. The store enforces it structurally
// by overwriting a single attribute; this pins that the handler actually
// issues a NEW value rather than re-persisting the current one.
func TestToken_RotationInvalidatesThePrevious(t *testing.T) {
	f := pairedFixture("s3cret")
	h := newHandler(f, nil)

	first, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	second, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	a := decodeBody[TokenResponse](t, first).SessionToken
	b := decodeBody[TokenResponse](t, second).SessionToken
	if a == b {
		t.Fatal("two exchanges produced the same token; the old one would stay valid")
	}
	if f.devices["dev-1"].SessionTokenHash != tokens.HashSessionToken(b) {
		t.Error("the stored hash is not the most recently issued token's")
	}
}

// TestToken_WrongSecret and the three that follow all answer 401 rather than
// 404/403. docs/06-auth.md §5 makes 401 the device's single self-repair
// signal: it wipes local credentials and returns to the QR screen. Any other
// status leaves a device that can never recover, and distinguishing
// "unknown device" from "wrong secret" would also let an unauthenticated
// caller enumerate deviceIds.
func TestToken_WrongSecret(t *testing.T) {
	h := newHandler(pairedFixture("s3cret"), nil)
	_, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"wrong"}`))
	assertCode(t, err, apierr.CodeUnauthorized)
}

func TestToken_UnknownDevice(t *testing.T) {
	h := newHandler(pairedFixture("s3cret"), nil)
	_, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"nope","deviceSecret":"s3cret"}`))
	assertCode(t, err, apierr.CodeUnauthorized)
}

// TestToken_ClearedSecretHash is the disconnect path: DisconnectDevice wipes
// deviceSecretHash, and this is where that takes effect for a device that
// still holds its old secret. An empty stored hash must never match; a
// naive compare of two empty strings would hand a session to a device the
// owner just disconnected.
func TestToken_ClearedSecretHash(t *testing.T) {
	f := pairedFixture("s3cret")
	f.devices["dev-1"].DeviceSecretHash = ""
	f.devices["dev-1"].Status = store.DeviceStatusDisconnected
	h := newHandler(f, nil)

	// The device still holds the secret it was issued at pairing time; the
	// server side of that pair is what was wiped.
	_, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	assertCode(t, err, apierr.CodeUnauthorized)
}

// TestToken_NotPaired covers ARCHIVED specifically: a deleted device is a
// logical delete, so its row and its deviceSecretHash both survive
// (docs/06-auth.md §4). Only the status check stops it from refreshing a
// session forever.
func TestToken_NotPaired(t *testing.T) {
	for _, status := range []string{store.DeviceStatusPending, store.DeviceStatusDisconnected, store.DeviceStatusArchived} {
		t.Run(status, func(t *testing.T) {
			f := pairedFixture("s3cret")
			f.devices["dev-1"].Status = status
			h := newHandler(f, nil)

			_, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
			assertCode(t, err, apierr.CodeUnauthorized)
			if len(f.rotated) != 0 {
				t.Errorf("RotateSessionToken was called for a %s device", status)
			}
		})
	}
}

// TestToken_RotateConditionFailurePropagatesAs401: the device can be
// disconnected between our read and our write. The store answers that with
// CodeUnauthorized and the handler must not upgrade it into a 500.
func TestToken_RotateConditionFailure(t *testing.T) {
	f := pairedFixture("s3cret")
	f.rotateErr = apierr.New(apierr.CodeUnauthorized, "device not eligible for a session")
	h := newHandler(f, nil)

	_, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	assertCode(t, err, apierr.CodeUnauthorized)
}

func TestToken_MissingFields(t *testing.T) {
	h := newHandler(pairedFixture("s3cret"), nil)
	for _, body := range []string{`{}`, `{"deviceId":"dev-1"}`, `{"deviceSecret":"s3cret"}`, `{"deviceId":"","deviceSecret":"s3cret"}`} {
		_, err := h.Token(context.Background(), jsonRequest(body))
		assertCode(t, err, apierr.CodeValidation)
	}
}

func TestToken_NoSecretsInLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := newHandler(pairedFixture("s3cret"), logger)

	resp, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	body := decodeBody[TokenResponse](t, resp)

	logged := buf.String()
	if strings.Contains(logged, "s3cret") {
		t.Errorf("the device secret appears in the log: %s", logged)
	}
	if strings.Contains(logged, body.SessionToken) {
		t.Errorf("the session token appears in the log: %s", logged)
	}
}

// --- the cross-package landmine -----------------------------------------

// TestToken_RoundTripsThroughTheAuthorizer is the test the hash-encoding
// contract actually needs (ruling R13). Every other test here and in
// internal/authz builds its fixtures with the same helper, so both suites
// stay self-consistent and would pass unchanged if issuance switched to
// base64 while verification stayed hex. The failure would appear only in
// production, as a blanket 401 across every device, with both packages
// green.
//
// This drives the real issuance path and feeds its output to the real
// authorizer, so the two encodings are compared against each other rather
// than each against itself.
func TestToken_RoundTripsThroughTheAuthorizer(t *testing.T) {
	f := pairedFixture("s3cret")
	h := newHandler(f, nil)

	resp, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	sessionToken := decodeBody[TokenResponse](t, resp).SessionToken

	a := authz.New(f, clock.Fixed{At: fixedNow}, nil)
	authResp, err := a.Authorize(context.Background(), events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": sessionToken},
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !authResp.IsAuthorized {
		t.Fatal("a token this package just issued was rejected by the authorizer")
	}
	if authResp.Context["deviceId"] != "dev-1" || authResp.Context["ownerId"] != "owner-1" {
		t.Errorf("authorizer context = %v, want deviceId dev-1 / ownerId owner-1", authResp.Context)
	}
}

// TestToken_ExpiredSessionIsRejectedByTheAuthorizer completes the round trip
// at the other boundary: the expiry this handler computes must be the one
// the authorizer enforces. If the two disagreed about units (seconds vs
// milliseconds), the token above would still pass and every device would
// either never expire or expire instantly.
func TestToken_ExpiredSessionIsRejectedByTheAuthorizer(t *testing.T) {
	f := pairedFixture("s3cret")
	h := newHandler(f, nil)

	resp, err := h.Token(context.Background(), jsonRequest(`{"deviceId":"dev-1","deviceSecret":"s3cret"}`))
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	sessionToken := decodeBody[TokenResponse](t, resp).SessionToken

	justAfter := fixedNow.Add(SessionTTL).Add(time.Second)
	a := authz.New(f, clock.Fixed{At: justAfter}, nil)
	authResp, err := a.Authorize(context.Background(), events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": sessionToken},
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if authResp.IsAuthorized {
		t.Fatal("a session past its expiry was authorized")
	}
}

// --- wiring --------------------------------------------------------------

// TestRegister pins the route keys against infra/envs/dev/main.tf's
// device_public_routes. A typo here is invisible until deploy, where API
// Gateway routes to this function and the router answers ROUTE_NOT_WIRED.
func TestRegister(t *testing.T) {
	rt := httpx.New(nil)
	newHandler(pendingFixture(), nil).Register(rt)

	for _, routeKey := range []string{"POST /device/pair", "POST /device/token"} {
		resp, err := rt.Route(context.Background(), events.APIGatewayV2HTTPRequest{RouteKey: routeKey, Body: `{}`})
		if err != nil {
			t.Fatalf("Route(%q): %v", routeKey, err)
		}
		if strings.Contains(resp.Body, "ROUTE_NOT_WIRED") {
			t.Errorf("%s is not registered", routeKey)
		}
	}
}

//go:build integration

// Package e2e drives the whole device path across all four Lambdas against
// a real DynamoDB engine.
//
// Every handler package already has unit tests over a fake store, and every
// one of them passes with a fake that agrees with its own assumptions.
// What no unit test can catch is a disagreement BETWEEN packages: web-api
// issuing a pairing code that device-auth's GSI2 lookup cannot resolve,
// device-auth hashing a session one way and the authorizer verifying it
// another, device-api writing a thumbnail key that web-api's dashboard
// never surfaces. Each of those is green on both sides and broken in
// production.
//
// This test walks the exact sequence docs/06-auth.md §2 and §3 describe —
// create, scan, exchange, authorize, upload, observe — with the real store
// against DynamoDB Local, and asserts what the owner sees at the end.
package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ShuMasui/edgewatcher/backend/internal/api"
	"github.com/ShuMasui/edgewatcher/backend/internal/authz"
	"github.com/ShuMasui/edgewatcher/backend/internal/deviceapi"
	"github.com/ShuMasui/edgewatcher/backend/internal/deviceauth"
	"github.com/ShuMasui/edgewatcher/backend/internal/dynamotest"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
	"github.com/ShuMasui/edgewatcher/backend/internal/images"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
	"github.com/ShuMasui/edgewatcher/backend/internal/webapi"
)

// movableClock lets one test advance time across handlers, which is how the
// expiry boundaries get exercised without sleeping.
type movableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *movableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *movableClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// memoryUploader stands in for S3. The bucket is the one dependency this
// test does not run for real — internal/images unit-tests the PutObject
// call and the presigned URL against the actual SDK, and DynamoDB is where
// every cross-package contract in this path actually lives.
type memoryUploader struct {
	mu   sync.Mutex
	objs map[string][]byte
}

func (m *memoryUploader) PutJPEG(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.objs == nil {
		m.objs = map[string][]byte{}
	}
	m.objs[key] = append([]byte(nil), body...)
	return nil
}

type stack struct {
	clock    *movableClock
	web      *webapi.Handler
	auth     *deviceauth.Handler
	authz    *authz.Authorizer
	device   *deviceapi.Handler
	uploader *memoryUploader
}

func newStack(t *testing.T) *stack {
	t.Helper()

	client := dynamotest.NewClient(t)
	table := dynamotest.CreateTable(t, client)
	s := store.New(client, table)

	clk := &movableClock{now: time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC)}

	// A real presign client with dummy credentials: signing is pure
	// computation, so this produces genuine URLs without any network.
	presign := s3.NewPresignClient(s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", ""),
	}))
	mapper := api.NewMapper(images.NewSigner(presign, "ew-images-test", 900*time.Second))
	uploader := &memoryUploader{}

	return &stack{
		clock:    clk,
		web:      webapi.New(s, mapper, clk, webapi.Config{RetentionDays: 1, DeviceLimit: 10}, nil),
		auth:     deviceauth.New(s, clk, nil),
		authz:    authz.New(s, clk, nil),
		device:   deviceapi.New(s, uploader, clk, 1, nil),
		uploader: uploader,
	}
}

// --- request helpers -----------------------------------------------------

func ownerReq(ownerID string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
				JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
					Claims: map[string]string{"sub": ownerID},
				},
			},
		},
	}
}

func body[T any](t *testing.T, resp httpx.Response) T {
	t.Helper()
	raw, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return out
}

// uploadRequest builds one multipart upload the way the device does.
//
// The observationId is a real ULID minted from capturedAt, not a
// hand-written constant, because the day-range query that serves the
// history reads the timestamp OUT of the ULID (ids.DayRange,
// docs/engineering/dynamodb.md §5.2). A fabricated id sorts outside the
// day's bounds and the photo simply never appears — which is exactly what
// the first run of this test showed, and exactly what a device using
// ids.NewULID would never hit.
func uploadRequest(t *testing.T, authContext map[string]interface{}, capturedAt time.Time) (events.APIGatewayV2HTTPRequest, string) {
	t.Helper()
	observationID, err := ids.NewULID(capturedAt)
	if err != nil {
		t.Fatalf("mint observation id: %v", err)
	}
	capturedAtISO := capturedAt.UTC().Format(time.RFC3339)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	parts := map[string][]byte{
		"image":     []byte("full-image-bytes"),
		"thumbnail": []byte("thumb-bytes"),
		"metadata":  []byte(`{"observationId":"` + observationID + `","capturedAt":"` + capturedAtISO + `","lat":35.68,"lng":139.76}`),
	}
	for _, name := range []string{"image", "thumbnail", "metadata"} {
		fw, err := w.CreateFormFile(name, name)
		if err != nil {
			t.Fatalf("CreateFormFile(%q): %v", name, err)
		}
		if _, err := fw.Write(parts[name]); err != nil {
			t.Fatalf("write part %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return events.APIGatewayV2HTTPRequest{
		Headers:         map[string]string{"content-type": w.FormDataContentType()},
		Body:            base64.StdEncoding.EncodeToString(buf.Bytes()),
		IsBase64Encoded: true,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{Lambda: authContext},
		},
	}, observationID
}

// --- the path ------------------------------------------------------------

// TestQRPath_EndToEnd walks the whole sequence an owner and a device
// actually perform.
func TestQRPath_EndToEnd(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	const owner = "owner-e2e"

	// 1. The owner adds a device. Its row exists as PENDING immediately, so
	//    the list shows ペアリング待ち while they walk to the camera
	//    (docs/03-web.md §1.8.1).
	created, err := st.web.CreateDevice(ctx, func() events.APIGatewayV2HTTPRequest {
		r := ownerReq(owner)
		r.Body = `{"name":"玄関"}`
		return r
	}())
	if err != nil {
		t.Fatalf("POST /devices: %v", err)
	}
	create := body[webapi.CreateDeviceResponse](t, created)
	deviceID := create.Device.DeviceID
	pairingCode := create.PairingSession.PairingCode
	if deviceID == "" || pairingCode == "" {
		t.Fatalf("POST /devices returned %+v", create)
	}

	// 2. The modal polls and sees its own PENDING session. This is the
	//    first cross-package contract: the code web-api minted has to come
	//    back through the same GSI1 partition the dashboard reads.
	polled, err := st.web.LatestPairingSession(ctx, withPath(ownerReq(owner), deviceID))
	if err != nil {
		t.Fatalf("GET pairing-sessions/latest: %v", err)
	}
	if got := body[api.PairingSession](t, polled); got.PairingCode != pairingCode || got.Status != store.PairingStatusPending {
		t.Fatalf("poll returned %+v, want the PENDING code just issued", got)
	}

	// 3. The device scans the QR. This resolves the code through GSI2 —
	//    a different index, written by web-api and read by device-auth,
	//    which no unit test on either side exercises together.
	paired, err := st.auth.Pair(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"pairingCode":"` + pairingCode + `","deviceInfo":{"model":"Pixel 8","osVersion":"14","appVersion":"1.0.0"}}`,
	})
	if err != nil {
		t.Fatalf("POST /device/pair: %v", err)
	}
	pair := body[deviceauth.PairResponse](t, paired)
	if pair.DeviceID != deviceID {
		t.Fatalf("pair resolved to %q, want the device web-api created (%q)", pair.DeviceID, deviceID)
	}
	if pair.DeviceSecret == "" {
		t.Fatal("pair returned no deviceSecret")
	}

	// 4. The same QR cannot be scanned twice (docs/06-auth.md §2). If this
	//    passed, a leaked screenshot would pair a second device.
	if _, err := st.auth.Pair(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"pairingCode":"` + pairingCode + `","deviceInfo":{}}`,
	}); err == nil {
		t.Fatal("the pairing code was consumed twice")
	} else if !strings.Contains(err.Error(), "PAIRING_CODE_CONSUMED") {
		t.Fatalf("second scan failed with %v, want PAIRING_CODE_CONSUMED", err)
	}

	// 5. The owner's poll now reports CONSUMED, which is what flips the
	//    modal to 接続しました.
	polled, err = st.web.LatestPairingSession(ctx, withPath(ownerReq(owner), deviceID))
	if err != nil {
		t.Fatalf("GET pairing-sessions/latest after pairing: %v", err)
	}
	if got := body[api.PairingSession](t, polled); got.Status != store.PairingStatusConsumed {
		t.Fatalf("poll status = %q, want CONSUMED", got.Status)
	}

	// 6. The device exchanges its secret for a session.
	tokenResp, err := st.auth.Token(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"deviceId":"` + deviceID + `","deviceSecret":"` + pair.DeviceSecret + `"}`,
	})
	if err != nil {
		t.Fatalf("POST /device/token: %v", err)
	}
	session := body[deviceauth.TokenResponse](t, tokenResp)

	// 7. The authorizer accepts it. This is the hash-encoding landmine:
	//    both sides are self-consistent in their own suites and would only
	//    disagree here.
	authResp, err := st.authz.Authorize(ctx, events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": session.SessionToken},
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !authResp.IsAuthorized {
		t.Fatal("the authorizer rejected a session device-auth just issued")
	}
	if authResp.Context["deviceId"] != deviceID || authResp.Context["ownerId"] != owner {
		t.Fatalf("authorizer context = %v, want deviceId %q / ownerId %q", authResp.Context, deviceID, owner)
	}

	// 8. The device uploads, using the identity the authorizer produced.
	capturedAt := st.clock.Now().Add(-time.Minute)
	uploadReq, observationID := uploadRequest(t, authResp.Context, capturedAt)
	uploaded, err := st.device.Upload(ctx, uploadReq)
	if err != nil {
		t.Fatalf("POST /device/uploads: %v", err)
	}
	if got := body[deviceapi.UploadResponse](t, uploaded); got.NextConfig.IntervalMinutes != 5 {
		t.Errorf("nextConfig.intervalMinutes = %d, want the device's default 5", got.NextConfig.IntervalMinutes)
	}

	// The bytes landed under the authorized device's prefix, keyed by the
	// JST capture date.
	if len(st.uploader.objs) != 2 {
		t.Fatalf("objects written = %v, want the image and its thumbnail", st.uploader.objs)
	}
	for key := range st.uploader.objs {
		if !strings.HasPrefix(key, "observations/"+deviceID+"/") {
			t.Errorf("object %q is outside the device's prefix", key)
		}
	}

	// 9. The dashboard now shows a PAIRED device with a live thumbnail and
	//    no QR. This closes the loop: device-api's thumbnail key has to be
	//    the one web-api signs.
	listed, err := st.web.ListDevices(ctx, ownerReq(owner))
	if err != nil {
		t.Fatalf("GET /devices: %v", err)
	}
	devices := body[[]api.Device](t, listed)
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	dev := devices[0]
	if dev.Status != store.DeviceStatusPaired {
		t.Errorf("status = %q, want PAIRED", dev.Status)
	}
	if dev.ActivePairingSession != nil {
		t.Errorf("activePairingSession = %+v, want null once the code is consumed", dev.ActivePairingSession)
	}
	if dev.LatestThumbnailURL == "" {
		t.Error("latestThumbnailUrl is empty after a successful upload")
	}
	if !strings.Contains(dev.LatestThumbnailURL, "_thumb.jpg") {
		t.Errorf("latestThumbnailUrl = %q, want the thumbnail key, not the full image", dev.LatestThumbnailURL)
	}
	if dev.LastReceivedAt == "" {
		t.Error("lastReceivedAt is empty after a successful upload")
	}

	// 10. And the history for that JST day contains the photo.
	obsResp, err := st.web.ListObservations(ctx, withQuery(withPath(ownerReq(owner), deviceID), "date", "2026-09-08"))
	if err != nil {
		t.Fatalf("GET observations: %v", err)
	}
	obs := body[[]api.Observation](t, obsResp)
	if len(obs) != 1 {
		t.Fatalf("got %d observations, want 1", len(obs))
	}
	if obs[0].ObservationID != observationID {
		t.Errorf("observationId = %q, want the id the device sent (%q)", obs[0].ObservationID, observationID)
	}
	if obs[0].ThumbnailURL == "" {
		t.Error("thumbnailUrl is empty")
	}
	if obs[0].ImageURL != "" {
		t.Errorf("imageUrl = %q, want it omitted from the list", obs[0].ImageURL)
	}
}

// TestQRPath_ReplayedUploadKeepsOneRow is the retry a device performs when
// it never saw our response (docs/05-backend.md §2.6). It is exercised here
// rather than only against a fake because the guarantee is a DynamoDB
// ConditionExpression, and a fake asserting it proves only that the fake
// implements it.
func TestQRPath_ReplayedUploadKeepsOneRow(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	deviceID, authContext := pairAndAuthorize(t, st, "owner-replay")

	req, _ := uploadRequest(t, authContext, st.clock.Now().Add(-time.Minute))
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := st.device.Upload(ctx, req); err != nil {
			t.Fatalf("upload attempt %d: %v", attempt, err)
		}
	}

	obsResp, err := st.web.ListObservations(ctx, withQuery(withPath(ownerReq("owner-replay"), deviceID), "date", "2026-09-08"))
	if err != nil {
		t.Fatalf("GET observations: %v", err)
	}
	if obs := body[[]api.Observation](t, obsResp); len(obs) != 1 {
		t.Fatalf("three identical uploads produced %d rows, want 1", len(obs))
	}
}

// TestQRPath_BackfillDoesNotRewindTheDashboard covers docs/05-backend.md
// §3.3: a device recovering from an outage sends its newest photo first,
// then the backlog oldest-first. The older photos must land in the history
// without dragging the dashboard card back in time — while still proving
// the device is alive.
func TestQRPath_BackfillDoesNotRewindTheDashboard(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	const owner = "owner-backfill"
	deviceID, authContext := pairAndAuthorize(t, st, owner)

	newest := time.Date(2026, 9, 8, 1, 50, 0, 0, time.UTC)
	older := time.Date(2026, 9, 8, 0, 10, 0, 0, time.UTC)

	newestReq, _ := uploadRequest(t, authContext, newest)
	if _, err := st.device.Upload(ctx, newestReq); err != nil {
		t.Fatalf("upload newest: %v", err)
	}
	st.clock.advance(time.Minute)
	olderReq, _ := uploadRequest(t, authContext, older)
	if _, err := st.device.Upload(ctx, olderReq); err != nil {
		t.Fatalf("upload backfilled older: %v", err)
	}

	listed, err := st.web.ListDevices(ctx, ownerReq(owner))
	if err != nil {
		t.Fatalf("GET /devices: %v", err)
	}
	dev := body[[]api.Device](t, listed)[0]
	if want := newest.Format(time.RFC3339); dev.LatestCapturedAt != want {
		t.Errorf("latestCapturedAt = %q, want it pinned to the newest photo (%q)", dev.LatestCapturedAt, want)
	}
	// lastReceivedAt is unconditional: we did hear from the device just now.
	if dev.LastReceivedAt != st.clock.Now().UTC().Format(time.RFC3339) {
		t.Errorf("lastReceivedAt = %q, want the receipt time of the LATEST request", dev.LastReceivedAt)
	}

	obsResp, err := st.web.ListObservations(ctx, withQuery(withPath(ownerReq(owner), deviceID), "date", "2026-09-08"))
	if err != nil {
		t.Fatalf("GET observations: %v", err)
	}
	obs := body[[]api.Observation](t, obsResp)
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want both the newest and the backfilled one", len(obs))
	}
	if want := newest.Format(time.RFC3339); obs[0].CapturedAt != want {
		t.Errorf("history order = %q first, want newest-first (%q)", obs[0].CapturedAt, want)
	}
}

// TestQRPath_ExpiredCodeCannotPair exercises the five-minute window as the
// condition expression sees it, not as a handler pre-check does.
func TestQRPath_ExpiredCodeCannotPair(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()

	created, err := st.web.CreateDevice(ctx, func() events.APIGatewayV2HTTPRequest {
		r := ownerReq("owner-expiry")
		r.Body = `{"name":"玄関"}`
		return r
	}())
	if err != nil {
		t.Fatalf("POST /devices: %v", err)
	}
	create := body[webapi.CreateDeviceResponse](t, created)

	// One second past expiry: ConsumePairing's condition is expiresAt > now.
	st.clock.advance(5*time.Minute + time.Second)

	_, err = st.auth.Pair(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"pairingCode":"` + create.PairingSession.PairingCode + `","deviceInfo":{}}`,
	})
	if err == nil {
		t.Fatal("an expired pairing code was accepted")
	}
	if !strings.Contains(err.Error(), "PAIRING_CODE_EXPIRED") {
		t.Fatalf("expired scan failed with %v, want PAIRING_CODE_EXPIRED", err)
	}
}

// TestQRPath_LogoutRevokesImmediately covers docs/06-auth.md §5: the
// authorizer's per-request GetItem is what makes revocation instant, so a
// session token that was valid a moment ago must stop working with no
// cache to wait out.
func TestQRPath_LogoutRevokesImmediately(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	_, authContext := pairAndAuthorize(t, st, "owner-logout")

	sessionToken := authContext["sessionToken"].(string)
	delete(authContext, "sessionToken")

	if _, err := st.device.Logout(ctx, events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{Lambda: authContext},
		},
	}); err != nil {
		t.Fatalf("POST /device/logout: %v", err)
	}

	authResp, err := st.authz.Authorize(ctx, events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": sessionToken},
	})
	if err != nil {
		t.Fatalf("authorize after logout: %v", err)
	}
	if authResp.IsAuthorized {
		t.Fatal("a logged-out device's session was still authorized")
	}
}

// pairAndAuthorize runs steps 1-7 and returns the deviceId plus the
// authorizer context, with the session token stashed alongside it for tests
// that need to re-present it.
func pairAndAuthorize(t *testing.T, st *stack, owner string) (string, map[string]interface{}) {
	t.Helper()
	ctx := context.Background()

	created, err := st.web.CreateDevice(ctx, func() events.APIGatewayV2HTTPRequest {
		r := ownerReq(owner)
		r.Body = `{"name":"玄関"}`
		return r
	}())
	if err != nil {
		t.Fatalf("POST /devices: %v", err)
	}
	create := body[webapi.CreateDeviceResponse](t, created)

	paired, err := st.auth.Pair(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"pairingCode":"` + create.PairingSession.PairingCode + `","deviceInfo":{"model":"Pixel 8"}}`,
	})
	if err != nil {
		t.Fatalf("POST /device/pair: %v", err)
	}
	pair := body[deviceauth.PairResponse](t, paired)

	tokenResp, err := st.auth.Token(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"deviceId":"` + pair.DeviceID + `","deviceSecret":"` + pair.DeviceSecret + `"}`,
	})
	if err != nil {
		t.Fatalf("POST /device/token: %v", err)
	}
	sessionToken := body[deviceauth.TokenResponse](t, tokenResp).SessionToken

	authResp, err := st.authz.Authorize(ctx, events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": sessionToken},
	})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !authResp.IsAuthorized {
		t.Fatal("authorizer rejected a freshly issued session")
	}
	authContext := authResp.Context
	authContext["sessionToken"] = sessionToken
	authContext["deviceSecret"] = pair.DeviceSecret
	return pair.DeviceID, authContext
}

func withPath(req events.APIGatewayV2HTTPRequest, deviceID string) events.APIGatewayV2HTTPRequest {
	req.PathParameters = map[string]string{"id": deviceID}
	return req
}

func withQuery(req events.APIGatewayV2HTTPRequest, key, value string) events.APIGatewayV2HTTPRequest {
	if req.QueryStringParameters == nil {
		req.QueryStringParameters = map[string]string{}
	}
	req.QueryStringParameters[key] = value
	return req
}

// TestQRPath_DisconnectThenRePairKeepsTheHistory is the claim docs/03-web.md
// §1.8.3 makes to the owner: replacing or factory-resetting the Android
// phone keeps the observation point intact, because the deviceId never
// changes. Nothing short of running the whole loop can show that — the
// credentials are minted twice, by two different pairings, and the history
// has to survive both.
//
// It also walks the self-repair loop of docs/06-auth.md §5 in order:
// disconnect, upload denied, token refresh ALSO denied (which is what tells
// the device to wipe and show the QR again), re-pair, back online.
func TestQRPath_DisconnectThenRePairKeepsTheHistory(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	const owner = "owner-repair"
	deviceID, authContext := pairAndAuthorize(t, st, owner)
	firstSecret := authContext["deviceSecret"].(string)

	// A photo from before the disconnection.
	beforeReq, beforeObsID := uploadRequest(t, authContext, st.clock.Now().Add(-time.Minute))
	if _, err := st.device.Upload(ctx, beforeReq); err != nil {
		t.Fatalf("upload before disconnect: %v", err)
	}

	// The owner disconnects from the web.
	if _, err := st.web.DisconnectDevice(ctx, withPath(ownerReq(owner), deviceID)); err != nil {
		t.Fatalf("POST /devices/{id}/disconnect: %v", err)
	}

	// The session dies on the next request — no cache to wait out.
	authResp, err := st.authz.Authorize(ctx, events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": authContext["sessionToken"].(string)},
	})
	if err != nil {
		t.Fatalf("authorize after disconnect: %v", err)
	}
	if authResp.IsAuthorized {
		t.Fatal("a disconnected device was still authorized")
	}

	// And the refresh it would try next is refused too. This is the step
	// that matters: if the deviceSecret still worked, the device would
	// silently re-obtain a session and the disconnect would not hold
	// (docs/06-auth.md §6).
	if _, err := st.auth.Token(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"deviceId":"` + deviceID + `","deviceSecret":"` + firstSecret + `"}`,
	}); err == nil {
		t.Fatal("a disconnected device refreshed its session with the old deviceSecret")
	}

	// The owner re-issues a QR. No new Device row: same deviceId.
	reissued, err := st.web.CreatePairingSession(ctx, withPath(ownerReq(owner), deviceID))
	if err != nil {
		t.Fatalf("POST /devices/{id}/pairing-sessions: %v", err)
	}
	newCode := body[api.PairingSession](t, reissued).PairingCode

	paired, err := st.auth.Pair(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"pairingCode":"` + newCode + `","deviceInfo":{"model":"Pixel 9"}}`,
	})
	if err != nil {
		t.Fatalf("re-pair: %v", err)
	}
	repair := body[deviceauth.PairResponse](t, paired)
	if repair.DeviceID != deviceID {
		t.Fatalf("re-pairing produced deviceId %q, want the original %q — the history would be orphaned", repair.DeviceID, deviceID)
	}
	if repair.DeviceSecret == firstSecret {
		t.Fatal("re-pairing reissued the same deviceSecret; the disconnected credential must not come back")
	}

	tokenResp, err := st.auth.Token(ctx, events.APIGatewayV2HTTPRequest{
		Body: `{"deviceId":"` + deviceID + `","deviceSecret":"` + repair.DeviceSecret + `"}`,
	})
	if err != nil {
		t.Fatalf("token after re-pair: %v", err)
	}
	newAuth, err := st.authz.Authorize(ctx, events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": body[deviceauth.TokenResponse](t, tokenResp).SessionToken},
	})
	if err != nil || !newAuth.IsAuthorized {
		t.Fatalf("the re-paired device was not authorized (err=%v)", err)
	}

	// A photo from after. Both must be in the same day's history.
	st.clock.advance(time.Minute)
	afterReq, afterObsID := uploadRequest(t, newAuth.Context, st.clock.Now().Add(-30*time.Second))
	if _, err := st.device.Upload(ctx, afterReq); err != nil {
		t.Fatalf("upload after re-pair: %v", err)
	}

	obsResp, err := st.web.ListObservations(ctx, withQuery(withPath(ownerReq(owner), deviceID), "date", "2026-09-08"))
	if err != nil {
		t.Fatalf("GET observations: %v", err)
	}
	seen := map[string]bool{}
	for _, o := range body[[]api.Observation](t, obsResp) {
		seen[o.ObservationID] = true
	}
	if !seen[beforeObsID] {
		t.Error("the photo taken before the disconnection is gone from the history")
	}
	if !seen[afterObsID] {
		t.Error("the photo taken after re-pairing is missing from the history")
	}
}

// TestQRPath_DeleteRemovesTheDeviceAndFreesTheQuota covers the logical
// delete of docs/06-auth.md §6 through the two consequences an owner can
// actually observe: the device leaves the list, and the slot it occupied
// becomes available again (docs/03-web.md §1.9's "3 / 10 台").
//
// The quota half is the one worth running for real: it holds only because
// ArchiveDevice swaps GSI1PK into the ARCHIVED partition, so the row leaves
// the GSI1 query that the limit counts. A FilterExpression, or a status
// check the count forgot, would both pass unit tests over a fake.
func TestQRPath_DeleteRemovesTheDeviceAndFreesTheQuota(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	const owner = "owner-delete"
	deviceID, authContext := pairAndAuthorize(t, st, owner)

	req, _ := uploadRequest(t, authContext, st.clock.Now().Add(-time.Minute))
	if _, err := st.device.Upload(ctx, req); err != nil {
		t.Fatalf("upload: %v", err)
	}

	resp, err := st.web.DeleteDevice(ctx, withPath(ownerReq(owner), deviceID))
	if err != nil {
		t.Fatalf("DELETE /devices/{id}: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}

	listed, err := st.web.ListDevices(ctx, ownerReq(owner))
	if err != nil {
		t.Fatalf("GET /devices: %v", err)
	}
	if devices := body[[]api.Device](t, listed); len(devices) != 0 {
		t.Fatalf("deleted device still in the list: %+v", devices)
	}

	// Its credentials died with it, even though the row survives.
	authResp, err := st.authz.Authorize(ctx, events.APIGatewayV2CustomAuthorizerV2Request{
		Headers: map[string]string{"authorization": authContext["sessionToken"].(string)},
	})
	if err != nil {
		t.Fatalf("authorize after delete: %v", err)
	}
	if authResp.IsAuthorized {
		t.Fatal("a deleted device was still authorized")
	}

	// And the owner can add a device again in the freed slot.
	if _, err := st.web.CreateDevice(ctx, func() events.APIGatewayV2HTTPRequest {
		r := ownerReq(owner)
		r.Body = `{"name":"入れ替え"}`
		return r
	}()); err != nil {
		t.Fatalf("creating a device after a delete freed a slot: %v", err)
	}
}

// TestQRPath_IntervalChangeReachesTheDevice closes the configuration loop of
// docs/05-backend.md §3.4: the server never pushes, so an interval the owner
// picks in the browser only takes effect when the device next uploads and
// reads nextConfig off the response. The value has to survive a PATCH, a
// DynamoDB round trip, and TouchDeviceLatest's ALL_NEW return to get there.
func TestQRPath_IntervalChangeReachesTheDevice(t *testing.T) {
	st := newStack(t)
	ctx := context.Background()
	const owner = "owner-interval"
	deviceID, authContext := pairAndAuthorize(t, st, owner)

	patch := withPath(ownerReq(owner), deviceID)
	patch.Body = `{"interval":15,"name":"玄関(15分)"}`
	updated, err := st.web.UpdateDevice(ctx, patch)
	if err != nil {
		t.Fatalf("PATCH /devices/{id}: %v", err)
	}
	if got := body[api.Device](t, updated); got.Interval != 15 || got.Name != "玄関(15分)" {
		t.Fatalf("PATCH returned %+v, want interval 15 and the new name", got)
	}

	req, _ := uploadRequest(t, authContext, st.clock.Now().Add(-time.Minute))
	uploaded, err := st.device.Upload(ctx, req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if got := body[deviceapi.UploadResponse](t, uploaded); got.NextConfig.IntervalMinutes != 15 {
		t.Errorf("nextConfig.intervalMinutes = %d, want the 15 the owner chose", got.NextConfig.IntervalMinutes)
	}
}

package deviceapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime/multipart"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

var fixedNow = time.Date(2026, 9, 8, 2, 10, 0, 0, time.UTC)

const retentionDays = 1

// --- fakes ---------------------------------------------------------------

// callLog is shared by the S3 and DynamoDB fakes so a test can assert the
// ORDER of writes across the two, not just that both happened.
type callLog struct{ calls []string }

func (c *callLog) record(name string) { c.calls = append(c.calls, name) }

type fakeUploader struct {
	log  *callLog
	objs map[string][]byte
	err  error
}

func (f *fakeUploader) PutJPEG(_ context.Context, key string, body []byte) error {
	f.log.record("s3:" + key)
	if f.err != nil {
		return f.err
	}
	if f.objs == nil {
		f.objs = map[string][]byte{}
	}
	f.objs[key] = append([]byte(nil), body...)
	return nil
}

type fakeStore struct {
	log *callLog

	device *store.Device
	rows   map[string]store.Observation

	putErr        error
	touchErr      error
	disconnectErr error

	touched     []store.TouchDeviceLatestInput
	disconnects []string
}

func (f *fakeStore) PutObservation(_ context.Context, obs store.Observation) error {
	f.log.record("ddb:put")
	if f.putErr != nil {
		return f.putErr
	}
	if f.rows == nil {
		f.rows = map[string]store.Observation{}
	}
	// attribute_not_exists(SK): a replay is a no-op reported as success
	// (docs/05-backend.md §2.6).
	if _, exists := f.rows[obs.SK]; !exists {
		f.rows[obs.SK] = obs
	}
	return nil
}

func (f *fakeStore) TouchDeviceLatest(_ context.Context, in store.TouchDeviceLatestInput) (*store.Device, error) {
	f.log.record("ddb:touch")
	f.touched = append(f.touched, in)
	if f.touchErr != nil {
		return nil, f.touchErr
	}
	return f.device, nil
}

func (f *fakeStore) DisconnectDevice(_ context.Context, deviceID string) (*store.Device, error) {
	f.log.record("ddb:disconnect")
	f.disconnects = append(f.disconnects, deviceID)
	if f.disconnectErr != nil {
		return nil, f.disconnectErr
	}
	return f.device, nil
}

func newFixture() (*Handler, *fakeStore, *fakeUploader) {
	log := &callLog{}
	s := &fakeStore{log: log, device: &store.Device{DeviceID: "dev-1", OwnerID: "owner-1", Interval: 10}}
	u := &fakeUploader{log: log}
	return New(s, u, clock.Fixed{At: fixedNow}, retentionDays, nil), s, u
}

// --- request builders ----------------------------------------------------

type part struct {
	name  string
	value []byte
}

func multipartRequest(t *testing.T, parts []part) events.APIGatewayV2HTTPRequest {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		fw, err := w.CreateFormFile(p.name, p.name)
		if err != nil {
			t.Fatalf("CreateFormFile(%q): %v", p.name, err)
		}
		if _, err := fw.Write(p.value); err != nil {
			t.Fatalf("write part %q: %v", p.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	// API Gateway base64-encodes multipart bodies (they are not in its text
	// content-type list), so the fixture reproduces that rather than the
	// plain-text shape a hand-written test would default to.
	return events.APIGatewayV2HTTPRequest{
		RouteKey:        "POST /device/uploads",
		Headers:         map[string]string{"content-type": w.FormDataContentType()},
		Body:            base64.StdEncoding.EncodeToString(buf.Bytes()),
		IsBase64Encoded: true,
		RequestContext:  authorizedContext("dev-1", "owner-1"),
	}
}

func authorizedContext(deviceID, ownerID string) events.APIGatewayV2HTTPRequestContext {
	return events.APIGatewayV2HTTPRequestContext{
		Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			Lambda: map[string]interface{}{"deviceId": deviceID, "ownerId": ownerID},
		},
	}
}

func validParts(metadata string) []part {
	return []part{
		{"image", []byte{0xFF, 0xD8, 'f', 'u', 'l', 'l'}},
		{"thumbnail", []byte{0xFF, 0xD8, 't', 'h', 'u', 'm', 'b'}},
		{"metadata", []byte(metadata)},
	}
}

const validMetadata = `{"observationId":"01J0OBS","capturedAt":"2026-09-08T02:04:05Z","lat":35.68,"lng":139.76}`

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
		t.Fatalf("marshal response: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return out
}

// --- POST /device/uploads ------------------------------------------------

// TestUpload_Success walks docs/05-backend.md §1.3 end to end.
func TestUpload_Success(t *testing.T) {
	h, s, u := newFixture()

	resp, err := h.Upload(context.Background(), multipartRequest(t, validParts(validMetadata)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	imgKey := "observations/dev-1/2026-09-08/01J0OBS.jpg"
	thumbKey := "observations/dev-1/2026-09-08/01J0OBS_thumb.jpg"
	if !bytes.Equal(u.objs[imgKey], []byte{0xFF, 0xD8, 'f', 'u', 'l', 'l'}) {
		t.Errorf("full image at %q = %v", imgKey, u.objs[imgKey])
	}
	if !bytes.Equal(u.objs[thumbKey], []byte{0xFF, 0xD8, 't', 'h', 'u', 'm', 'b'}) {
		t.Errorf("thumbnail at %q = %v", thumbKey, u.objs[thumbKey])
	}

	row, ok := s.rows["OBS#01J0OBS"]
	if !ok {
		t.Fatalf("no observation written; rows = %v", s.rows)
	}
	if row.DeviceID != "dev-1" || row.CapturedAt != "2026-09-08T02:04:05Z" {
		t.Errorf("observation = %+v, want deviceId dev-1 and the submitted capturedAt", row)
	}
	if row.ImageKey != imgKey || row.ThumbnailKey != thumbKey {
		t.Errorf("observation keys = %q/%q, want %q/%q", row.ImageKey, row.ThumbnailKey, imgKey, thumbKey)
	}
	if row.Lat == nil || *row.Lat != 35.68 || row.Lng == nil || *row.Lng != 139.76 {
		t.Errorf("lat/lng = %v/%v, want the submitted coordinates", row.Lat, row.Lng)
	}

	if got := decodeBody[UploadResponse](t, resp); got.NextConfig.IntervalMinutes != 10 {
		t.Errorf("nextConfig.intervalMinutes = %d, want the device's 10", got.NextConfig.IntervalMinutes)
	}
}

// TestUpload_S3BeforeDynamo pins the ordering docs/05-backend.md §1.3 argues
// for explicitly. Both orders "work" in the happy path, so only an ordering
// assertion can hold it: with DynamoDB first, a crash in between leaves a
// record whose signed URL 404s, where S3-first leaves an orphaned object the
// bucket's lifecycle rule collects on its own.
func TestUpload_S3BeforeDynamo(t *testing.T) {
	h, s, _ := newFixture()
	if _, err := h.Upload(context.Background(), multipartRequest(t, validParts(validMetadata))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	want := []string{
		"s3:observations/dev-1/2026-09-08/01J0OBS.jpg",
		"s3:observations/dev-1/2026-09-08/01J0OBS_thumb.jpg",
		"ddb:put",
		"ddb:touch",
	}
	if !reflect.DeepEqual(s.log.calls, want) {
		t.Errorf("call order = %v, want %v", s.log.calls, want)
	}
}

// TestUpload_S3FailureWritesNoRecord is the other half of that argument: if
// the bytes never landed, the record must not exist either.
func TestUpload_S3FailureWritesNoRecord(t *testing.T) {
	h, s, u := newFixture()
	u.err = errors.New("s3 exploded")

	if _, err := h.Upload(context.Background(), multipartRequest(t, validParts(validMetadata))); err == nil {
		t.Fatal("expected an error when S3 fails")
	}
	if len(s.rows) != 0 {
		t.Errorf("an observation was written despite the S3 failure: %v", s.rows)
	}
	for _, c := range s.log.calls {
		if strings.HasPrefix(c, "ddb:") {
			t.Errorf("DynamoDB was called after an S3 failure: %v", s.log.calls)
		}
	}
}

// TestUpload_ReplayIsIdempotent covers docs/05-backend.md §2.6: a device
// that never saw our 200 retries the same observationId, and the second
// attempt must also succeed while leaving one row. Answering an error would
// make the device retry forever; writing a second row would duplicate the
// history the retry exists to protect.
func TestUpload_ReplayIsIdempotent(t *testing.T) {
	h, s, u := newFixture()
	req := multipartRequest(t, validParts(validMetadata))

	for attempt := 1; attempt <= 2; attempt++ {
		resp, err := h.Upload(context.Background(), req)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if resp.StatusCode != 0 && resp.StatusCode != 200 {
			t.Fatalf("attempt %d: status = %d, want 200", attempt, resp.StatusCode)
		}
	}
	if len(s.rows) != 1 {
		t.Errorf("rows = %d, want exactly 1 after a replay", len(s.rows))
	}
	// The same key is overwritten in S3 rather than accumulating a second
	// object, because the key is derived from the device-assigned id.
	if len(u.objs) != 2 {
		t.Errorf("objects = %d, want 2 (image + thumbnail) after a replay", len(u.objs))
	}
}

// TestUpload_TouchesDeviceWithCaptureTimeAndReceiptTime. The two timestamps
// come from different clocks on purpose (docs/05-backend.md §3.3):
// latestCapturedAt is the device's, and gates the "don't rewind the
// dashboard" condition; lastReceivedAt is ours, and drives the 応答なし
// display. Passing our own clock as both would make a backfilled photo look
// like a live one.
func TestUpload_TouchesDeviceWithCaptureTimeAndReceiptTime(t *testing.T) {
	h, s, _ := newFixture()
	if _, err := h.Upload(context.Background(), multipartRequest(t, validParts(validMetadata))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(s.touched) != 1 {
		t.Fatalf("TouchDeviceLatest called %d times, want 1", len(s.touched))
	}
	got := s.touched[0]
	if !got.ReceivedAt.Equal(fixedNow) {
		t.Errorf("ReceivedAt = %v, want the server clock %v", got.ReceivedAt, fixedNow)
	}
	if want := time.Date(2026, 9, 8, 2, 4, 5, 0, time.UTC); !got.LatestCapturedAt.Equal(want) {
		t.Errorf("LatestCapturedAt = %v, want the device's %v", got.LatestCapturedAt, want)
	}
	if got.LatestThumbnailKey != "observations/dev-1/2026-09-08/01J0OBS_thumb.jpg" {
		t.Errorf("LatestThumbnailKey = %q, want the thumbnail key", got.LatestThumbnailKey)
	}
	if got.DeviceID != "dev-1" {
		t.Errorf("DeviceID = %q, want dev-1", got.DeviceID)
	}
}

// TestUpload_ObservationTTLFromCapturedAt pins that retention is anchored to
// capture time, so a backfill cannot outlive the S3 lifecycle rule.
func TestUpload_ObservationTTLFromCapturedAt(t *testing.T) {
	h, s, _ := newFixture()
	if _, err := h.Upload(context.Background(), multipartRequest(t, validParts(validMetadata))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	captured := time.Date(2026, 9, 8, 2, 4, 5, 0, time.UTC)
	if want := captured.AddDate(0, 0, retentionDays).Unix(); s.rows["OBS#01J0OBS"].ExpiresAt != want {
		t.Errorf("expiresAt = %d, want capturedAt+%dd = %d", s.rows["OBS#01J0OBS"].ExpiresAt, retentionDays, want)
	}
}

// TestUpload_DeviceIDComesFromTheAuthorizerNotTheBody is the security
// property of this route. The authorizer has already proven which device is
// calling (docs/05-backend.md §3.2); anything in the payload is attacker-
// controlled. A handler trusting a body-supplied deviceId would let any
// paired device write observations into any other device's history.
func TestUpload_DeviceIDComesFromTheAuthorizerNotTheBody(t *testing.T) {
	h, s, u := newFixture()
	hostile := `{"observationId":"01J0OBS","capturedAt":"2026-09-08T02:04:05Z","deviceId":"victim","ownerId":"someone-else"}`

	if _, err := h.Upload(context.Background(), multipartRequest(t, validParts(hostile))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got := s.rows["OBS#01J0OBS"]; got.DeviceID != "dev-1" || got.PK != "DEVICE#dev-1" {
		t.Errorf("observation = %+v, want it keyed to the authorized dev-1", got)
	}
	for key := range u.objs {
		if !strings.HasPrefix(key, "observations/dev-1/") {
			t.Errorf("object written outside the authorized device's prefix: %q", key)
		}
	}
	if s.touched[0].DeviceID != "dev-1" {
		t.Errorf("touched %q, want dev-1", s.touched[0].DeviceID)
	}
}

// TestUpload_Unauthenticated: without the authorizer's context there is no
// device to attribute the upload to. This is defense in depth — API Gateway
// will not route here without the authorizer — but the alternative to
// failing is guessing.
func TestUpload_Unauthenticated(t *testing.T) {
	h, _, _ := newFixture()
	req := multipartRequest(t, validParts(validMetadata))
	req.RequestContext = events.APIGatewayV2HTTPRequestContext{}

	_, err := h.Upload(context.Background(), req)
	assertCode(t, err, apierr.CodeUnauthorized)
}

// TestUpload_BadMetadata: every one of these is permanently unfixable by
// retrying, so each must be a 400 (docs/05-backend.md §1.6). A 500 would put
// the device into exponential backoff against a request that can never
// succeed, and it would keep the bad frame in its buffer forever.
func TestUpload_BadMetadata(t *testing.T) {
	cases := map[string]string{
		"malformed json":         `{ not json`,
		"missing observationId":  `{"capturedAt":"2026-09-08T02:04:05Z"}`,
		"empty observationId":    `{"observationId":"","capturedAt":"2026-09-08T02:04:05Z"}`,
		"missing capturedAt":     `{"observationId":"01J0OBS"}`,
		"unparseable capturedAt": `{"observationId":"01J0OBS","capturedAt":"yesterday"}`,
	}
	for name, metadata := range cases {
		t.Run(name, func(t *testing.T) {
			h, s, u := newFixture()
			_, err := h.Upload(context.Background(), multipartRequest(t, validParts(metadata)))
			assertCode(t, err, apierr.CodeValidation)
			if len(u.objs) != 0 || len(s.rows) != 0 {
				t.Errorf("a rejected upload still wrote something: objs=%v rows=%v", u.objs, s.rows)
			}
		})
	}
}

// TestUpload_MissingParts: all three parts are required. Accepting an upload
// without a thumbnail would produce a device row whose latestThumbnailKey
// points at an object that was never written.
func TestUpload_MissingParts(t *testing.T) {
	full := validParts(validMetadata)
	for _, missing := range []string{"image", "thumbnail", "metadata"} {
		t.Run("without "+missing, func(t *testing.T) {
			var parts []part
			for _, p := range full {
				if p.name != missing {
					parts = append(parts, p)
				}
			}
			h, _, _ := newFixture()
			_, err := h.Upload(context.Background(), multipartRequest(t, parts))
			assertCode(t, err, apierr.CodeValidation)
		})
	}
}

// TestUpload_EmptyImagePart: a zero-byte JPEG is not a JPEG. Storing it
// would put a permanently broken image in the history that no retry
// replaces, because the observationId is already consumed.
func TestUpload_EmptyImagePart(t *testing.T) {
	h, _, _ := newFixture()
	parts := []part{{"image", nil}, {"thumbnail", []byte{0xFF}}, {"metadata", []byte(validMetadata)}}
	_, err := h.Upload(context.Background(), multipartRequest(t, parts))
	assertCode(t, err, apierr.CodeValidation)
}

// TestUpload_Oversized enforces the ~4.5MB ceiling of docs/05-backend.md
// §1.3 in the handler as well as at the gateway. It is a 400 because the
// device must drop the frame rather than retry it: no amount of retrying
// shrinks it.
func TestUpload_Oversized(t *testing.T) {
	h, _, _ := newFixture()
	parts := []part{
		{"image", bytes.Repeat([]byte{0xFF}, MaxUploadBytes)},
		{"thumbnail", bytes.Repeat([]byte{0xFF}, 1024)},
		{"metadata", []byte(validMetadata)},
	}
	_, err := h.Upload(context.Background(), multipartRequest(t, parts))
	assertCode(t, err, apierr.CodeValidation)
}

// TestUpload_NotMultipart: a device sending JSON here gets a 400 rather than
// a parser panic surfacing as a 500.
func TestUpload_NotMultipart(t *testing.T) {
	h, _, _ := newFixture()
	req := events.APIGatewayV2HTTPRequest{
		Headers:        map[string]string{"content-type": "application/json"},
		Body:           `{"observationId":"x"}`,
		RequestContext: authorizedContext("dev-1", "owner-1"),
	}
	_, err := h.Upload(context.Background(), req)
	assertCode(t, err, apierr.CodeValidation)
}

// TestUpload_NextConfigComesFromTheWrite is ruling R2's structural form of
// G3. device-api's IAM role grants PutItem and UpdateItem and no read at
// all, so intervalMinutes must come from TouchDeviceLatest's ALL_NEW return
// rather than a GetItem. Asserting the interface's method set is stronger
// than counting calls on a fake: there is no read method to call.
func TestUpload_NextConfigComesFromTheWrite(t *testing.T) {
	iface := reflect.TypeOf((*UploadStore)(nil)).Elem()
	var got []string
	for i := 0; i < iface.NumMethod(); i++ {
		got = append(got, iface.Method(i).Name)
	}
	sort.Strings(got)
	want := []string{"PutObservation", "TouchDeviceLatest"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UploadStore methods = %v, want exactly %v — device-api has no read permission (docs/05-backend.md §2.4)", got, want)
	}
}

// TestUpload_TouchFailurePropagates: if the device row could not be updated,
// we do not know the device's interval, and inventing a default would
// silently override a setting the owner chose.
func TestUpload_TouchFailurePropagates(t *testing.T) {
	h, s, _ := newFixture()
	s.touchErr = apierr.New(apierr.CodeDeviceNotFound, "device not found")

	_, err := h.Upload(context.Background(), multipartRequest(t, validParts(validMetadata)))
	assertCode(t, err, apierr.CodeDeviceNotFound)
}

// --- POST /device/logout -------------------------------------------------

// TestLogout_204 covers docs/04-native.md §1.3: the device drops both
// credentials server-side and returns to the QR screen. 204 with no body is
// what api.ts expects.
func TestLogout_204(t *testing.T) {
	h, s, _ := newFixture()

	resp, err := h.Logout(context.Background(), events.APIGatewayV2HTTPRequest{
		RouteKey:       "POST /device/logout",
		RequestContext: authorizedContext("dev-1", "owner-1"),
	})
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if resp.Body != nil {
		t.Errorf("body = %v, want none", resp.Body)
	}
	if len(s.disconnects) != 1 || s.disconnects[0] != "dev-1" {
		t.Errorf("disconnects = %v, want [dev-1]", s.disconnects)
	}
}

// TestLogout_UsesTheAuthorizedDevice: same property as the upload route.
// A body-supplied deviceId would let one device log another out.
func TestLogout_UsesTheAuthorizedDevice(t *testing.T) {
	h, s, _ := newFixture()

	if _, err := h.Logout(context.Background(), events.APIGatewayV2HTTPRequest{
		Body:           `{"deviceId":"victim"}`,
		RequestContext: authorizedContext("dev-1", "owner-1"),
	}); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if s.disconnects[0] != "dev-1" {
		t.Errorf("disconnected %q, want the authorized dev-1", s.disconnects[0])
	}
}

func TestLogout_Unauthenticated(t *testing.T) {
	h, _, _ := newFixture()
	_, err := h.Logout(context.Background(), events.APIGatewayV2HTTPRequest{})
	assertCode(t, err, apierr.CodeUnauthorized)
}

// --- wiring --------------------------------------------------------------

// TestRegister pins the route keys against infra/envs/dev/main.tf's
// device_routes.
func TestRegister(t *testing.T) {
	rt := httpx.New(nil)
	h, _, _ := newFixture()
	h.Register(rt)

	for _, routeKey := range []string{"POST /device/uploads", "POST /device/logout"} {
		resp, err := rt.Route(context.Background(), events.APIGatewayV2HTTPRequest{
			RouteKey:       routeKey,
			RequestContext: authorizedContext("dev-1", "owner-1"),
		})
		if err != nil {
			t.Fatalf("Route(%q): %v", routeKey, err)
		}
		if strings.Contains(resp.Body, "ROUTE_NOT_WIRED") {
			t.Errorf("%s is not registered", routeKey)
		}
	}
}

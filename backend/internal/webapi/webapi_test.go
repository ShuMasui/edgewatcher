package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/api"
	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/images"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

var fixedNow = time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC)

// --- fakes ---------------------------------------------------------------

type fakeSigner struct{}

func (fakeSigner) SignGetObject(_ context.Context, key string, now time.Time) (images.SignedURL, error) {
	return images.SignedURL{URL: "https://signed.example/" + key, ExpiresAt: now.Add(900 * time.Second).Unix()}, nil
}

type fakeStore struct {
	rows     []store.OwnerListRow
	devices  map[string]*store.Device
	obsByDay map[string][]store.Observation
	obs      map[string]*store.Observation

	listErr error

	created  []store.CreateDeviceWithPairingInput
	sessions []store.CreatePairingSessionInput
}

func (f *fakeStore) ListOwnerDevices(_ context.Context, _ string) ([]store.OwnerListRow, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.rows, nil
}

func (f *fakeStore) GetDeviceForAuth(_ context.Context, deviceID string) (*store.Device, error) {
	dev, ok := f.devices[deviceID]
	if !ok {
		return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
	}
	return dev, nil
}

func (f *fakeStore) QueryObservationsByDay(_ context.Context, deviceID, dateStr string) ([]store.Observation, error) {
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		return nil, apierr.Wrap(apierr.CodeValidation, "invalid date", err)
	}
	return f.obsByDay[deviceID+"/"+dateStr], nil
}

func (f *fakeStore) GetObservation(_ context.Context, deviceID, observationID string) (*store.Observation, error) {
	o, ok := f.obs[deviceID+"/"+observationID]
	if !ok {
		return nil, apierr.New(apierr.CodeObservationNotFound, "observation not found")
	}
	return o, nil
}

func (f *fakeStore) CreateDeviceWithPairing(_ context.Context, in store.CreateDeviceWithPairingInput) (*store.Device, *store.PairingSession, error) {
	f.created = append(f.created, in)
	dev := &store.Device{
		DeviceID: in.DeviceID, OwnerID: in.OwnerID, Name: in.Name,
		Status: store.DeviceStatusPending, Interval: in.Interval, CreatedAt: "2026-09-08T02:00:00Z",
	}
	sess := &store.PairingSession{
		DeviceID: in.DeviceID, OwnerID: in.OwnerID, PairingCode: in.PairingCode,
		Status: store.PairingStatusPending, ExpiresAt: in.Now.Add(5 * time.Minute).Unix(),
	}
	return dev, sess, nil
}

func (f *fakeStore) CreatePairingSession(_ context.Context, in store.CreatePairingSessionInput) (*store.PairingSession, error) {
	f.sessions = append(f.sessions, in)
	return &store.PairingSession{
		DeviceID: in.DeviceID, OwnerID: in.OwnerID, PairingCode: in.PairingCode,
		Status: store.PairingStatusPending, ExpiresAt: in.Now.Add(5 * time.Minute).Unix(),
	}, nil
}

func newHandler(f *fakeStore) *Handler {
	return New(f, api.NewMapper(fakeSigner{}), clock.Fixed{At: fixedNow},
		Config{RetentionDays: 1, DeviceLimit: 10}, nil)
}

// --- request builders ----------------------------------------------------

func ownerRequest(ownerID string) events.APIGatewayV2HTTPRequest {
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

func withPath(req events.APIGatewayV2HTTPRequest, params map[string]string) events.APIGatewayV2HTTPRequest {
	req.PathParameters = params
	return req
}

func withQuery(req events.APIGatewayV2HTTPRequest, params map[string]string) events.APIGatewayV2HTTPRequest {
	req.QueryStringParameters = params
	return req
}

func withBody(req events.APIGatewayV2HTTPRequest, body string) events.APIGatewayV2HTTPRequest {
	req.Body = body
	return req
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
		t.Fatalf("marshal response: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return out
}

func deviceRow(deviceID, name, status string) store.OwnerListRow {
	return store.OwnerListRow{
		GSI1PK: "OWNER#owner-1", GSI1SK: "DEVICE#" + deviceID,
		DeviceID: deviceID, Name: name, Status: status, Interval: 5,
	}
}

func pairingRow(deviceID, code, status string, expiresAt int64) store.OwnerListRow {
	return store.OwnerListRow{
		GSI1PK: "OWNER#owner-1", GSI1SK: "PAIRING#" + deviceID,
		DeviceID: deviceID, PairingCode: code, Status: status, ExpiresAt: expiresAt,
	}
}

// --- GET /app-config -----------------------------------------------------

// TestAppConfig returns the environment constants the web needs to render
// limits and the interval <select> without hardcoding them
// (docs/05-backend.md §3.6). Serving them from the same env vars the write
// paths validate against is what keeps the client's options and the
// server's validation from drifting.
func TestAppConfig(t *testing.T) {
	h := newHandler(&fakeStore{})
	resp, err := h.AppConfig(context.Background(), ownerRequest("owner-1"))
	if err != nil {
		t.Fatalf("AppConfig: %v", err)
	}
	got := decodeBody[api.AppConfig](t, resp)
	if got.RetentionDays != 1 || got.DeviceLimit != 10 {
		t.Errorf("config = %+v, want retentionDays 1 / deviceLimit 10", got)
	}
	if len(got.IntervalOptions) != 3 {
		t.Errorf("intervalOptions = %v, want three choices", got.IntervalOptions)
	}
}

// --- GET /devices --------------------------------------------------------

// TestListDevices_JoinsActivePairing covers the single GSI1 query that
// backs the dashboard (access pattern 1): device rows and pairing rows come
// back together and are stitched here rather than by a per-device fan-out.
func TestListDevices_JoinsActivePairing(t *testing.T) {
	f := &fakeStore{rows: []store.OwnerListRow{
		deviceRow("dev-1", "玄関", store.DeviceStatusPending),
		deviceRow("dev-2", "裏口", store.DeviceStatusPaired),
		pairingRow("dev-1", "CODE1", store.PairingStatusPending, fixedNow.Add(time.Minute).Unix()),
	}}
	h := newHandler(f)

	resp, err := h.ListDevices(context.Background(), ownerRequest("owner-1"))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	devices := decodeBody[[]api.Device](t, resp)
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	byID := map[string]api.Device{}
	for _, d := range devices {
		byID[d.DeviceID] = d
	}
	if byID["dev-1"].ActivePairingSession == nil {
		t.Error("dev-1 has a live PENDING session but activePairingSession is null")
	} else if byID["dev-1"].ActivePairingSession.PairingCode != "CODE1" {
		t.Errorf("dev-1 pairingCode = %q, want CODE1", byID["dev-1"].ActivePairingSession.PairingCode)
	}
	if byID["dev-2"].ActivePairingSession != nil {
		t.Error("dev-2 has no session but activePairingSession is set")
	}
	if byID["dev-1"].OwnerID != "owner-1" {
		t.Errorf("ownerId = %q, want it recovered from GSI1PK", byID["dev-1"].OwnerID)
	}
}

// TestListDevices_ConsumedAndExpiredSessionsAreNull. A consumed code cannot
// be redeemed and an expired one is refused by ConsumePairing's condition,
// so surfacing either would put a QR on screen that silently does nothing.
func TestListDevices_ConsumedAndExpiredSessionsAreNull(t *testing.T) {
	cases := map[string]store.OwnerListRow{
		"consumed": pairingRow("dev-1", "CODE1", store.PairingStatusConsumed, fixedNow.Add(time.Minute).Unix()),
		"expired":  pairingRow("dev-1", "CODE1", store.PairingStatusPending, fixedNow.Add(-time.Second).Unix()),
	}
	for name, row := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeStore{rows: []store.OwnerListRow{deviceRow("dev-1", "玄関", store.DeviceStatusPaired), row}}
			resp, err := newHandler(f).ListDevices(context.Background(), ownerRequest("owner-1"))
			if err != nil {
				t.Fatalf("ListDevices: %v", err)
			}
			devices := decodeBody[[]api.Device](t, resp)
			if devices[0].ActivePairingSession != nil {
				t.Errorf("activePairingSession = %+v, want null for a %s session", devices[0].ActivePairingSession, name)
			}
		})
	}
}

// TestListDevices_PrefersTheNewestSession: re-issuing a QR
// (POST /devices/{id}/pairing-sessions) leaves the older session row in
// place, so a device can have several. The dashboard must show the one the
// owner is currently looking at, not whichever the index happens to return
// first.
func TestListDevices_PrefersTheNewestSession(t *testing.T) {
	f := &fakeStore{rows: []store.OwnerListRow{
		deviceRow("dev-1", "玄関", store.DeviceStatusPending),
		pairingRow("dev-1", "OLD", store.PairingStatusPending, fixedNow.Add(time.Minute).Unix()),
		pairingRow("dev-1", "NEW", store.PairingStatusPending, fixedNow.Add(4*time.Minute).Unix()),
	}}
	resp, err := newHandler(f).ListDevices(context.Background(), ownerRequest("owner-1"))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	devices := decodeBody[[]api.Device](t, resp)
	if devices[0].ActivePairingSession == nil || devices[0].ActivePairingSession.PairingCode != "NEW" {
		t.Errorf("activePairingSession = %+v, want the most recently issued code", devices[0].ActivePairingSession)
	}
}

// TestListDevices_ExcludesArchived. ArchiveDevice repoints GSI1PK to
// "ARCHIVED", so a deleted device is already outside the owner's partition
// — but a row that somehow carried the status must still be filtered, since
// a deleted device reappearing in the list is the one outcome the logical
// delete exists to prevent.
func TestListDevices_ExcludesArchived(t *testing.T) {
	f := &fakeStore{rows: []store.OwnerListRow{
		deviceRow("dev-1", "玄関", store.DeviceStatusPaired),
		deviceRow("dev-2", "削除済み", store.DeviceStatusArchived),
	}}
	resp, err := newHandler(f).ListDevices(context.Background(), ownerRequest("owner-1"))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	devices := decodeBody[[]api.Device](t, resp)
	if len(devices) != 1 || devices[0].DeviceID != "dev-1" {
		t.Errorf("devices = %+v, want only dev-1", devices)
	}
}

// TestListDevices_EmptyIsArray: a new account marshals as [], not null.
func TestListDevices_EmptyIsArray(t *testing.T) {
	resp, err := newHandler(&fakeStore{}).ListDevices(context.Background(), ownerRequest("owner-1"))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	raw, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("body = %s, want []", raw)
	}
}

func TestListDevices_Unauthenticated(t *testing.T) {
	_, err := newHandler(&fakeStore{}).ListDevices(context.Background(), events.APIGatewayV2HTTPRequest{})
	assertCode(t, err, apierr.CodeUnauthorized)
}

// --- ownership -----------------------------------------------------------

// TestOwnership covers every route that takes a device id. docs/06-auth.md
// §7 requires the JWT's sub to be matched against the resource's ownerId on
// every request; without it, a deviceId is a bearer token for someone
// else's camera history.
//
// 403 and 404 are genuinely distinct here even though docs/05-backend.md
// §1.6 has the web render them identically: the difference is what gets
// logged, and an attacker probing for valid deviceIds is exactly what those
// logs are for.
func TestOwnership(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{
			"dev-1":   {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired, Interval: 5},
			"other":   {DeviceID: "other", OwnerID: "owner-2", Status: store.DeviceStatusPaired, Interval: 5},
			"deleted": {DeviceID: "deleted", OwnerID: "owner-1", Status: store.DeviceStatusArchived, Interval: 5},
		},
		obs: map[string]*store.Observation{
			"dev-1/obs-1": {ObservationID: "obs-1", DeviceID: "dev-1", ImageKey: "full/1.jpg"},
		},
	}
	h := newHandler(f)

	routes := map[string]func(context.Context, events.APIGatewayV2HTTPRequest) (httpx.Response, error){
		"observations":   h.ListObservations,
		"latest pairing": h.LatestPairingSession,
		"new pairing":    h.CreatePairingSession,
	}
	for name, handler := range routes {
		t.Run(name+"/someone else's device", func(t *testing.T) {
			req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "other"}), map[string]string{"date": "2026-09-08"})
			_, err := handler(context.Background(), req)
			assertCode(t, err, apierr.CodeForbidden)
		})
		t.Run(name+"/nonexistent device", func(t *testing.T) {
			req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "nope"}), map[string]string{"date": "2026-09-08"})
			_, err := handler(context.Background(), req)
			assertCode(t, err, apierr.CodeDeviceNotFound)
		})
		t.Run(name+"/archived device", func(t *testing.T) {
			req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "deleted"}), map[string]string{"date": "2026-09-08"})
			_, err := handler(context.Background(), req)
			assertCode(t, err, apierr.CodeDeviceNotFound)
		})
	}

	t.Run("image/someone else's device", func(t *testing.T) {
		req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "obs-1"}), map[string]string{"deviceId": "other"})
		_, err := h.ObservationImage(context.Background(), req)
		assertCode(t, err, apierr.CodeForbidden)
	})
}

// --- GET /devices/{id}/observations --------------------------------------

func TestListObservations_ByDay(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired}},
		obsByDay: map[string][]store.Observation{
			"dev-1/2026-09-08": {
				{ObservationID: "obs-2", DeviceID: "dev-1", CapturedAt: "2026-09-08T03:00:00Z", ThumbnailKey: "thumb/2.jpg", ImageKey: "full/2.jpg"},
				{ObservationID: "obs-1", DeviceID: "dev-1", CapturedAt: "2026-09-08T01:00:00Z", ThumbnailKey: "thumb/1.jpg", ImageKey: "full/1.jpg"},
			},
		},
	}
	req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"}), map[string]string{"date": "2026-09-08"})

	resp, err := newHandler(f).ListObservations(context.Background(), req)
	if err != nil {
		t.Fatalf("ListObservations: %v", err)
	}
	obs := decodeBody[[]api.Observation](t, resp)
	if len(obs) != 2 {
		t.Fatalf("got %d observations, want 2", len(obs))
	}
	// The store queries with ScanIndexForward:false, so newest first — the
	// handler must not reorder.
	if obs[0].ObservationID != "obs-2" {
		t.Errorf("first observation = %q, want the newest (obs-2)", obs[0].ObservationID)
	}
	if obs[0].ThumbnailURL == "" {
		t.Error("thumbnailUrl is empty")
	}
	if obs[0].ImageURL != "" {
		t.Errorf("imageUrl = %q, want it omitted from a list response", obs[0].ImageURL)
	}
}

// TestListObservations_DefaultsToToday: the history view opens on today, so
// omitting ?date= is a normal request rather than a malformed one. "Today"
// is JST because that is the day the S3 key layout and the ULID day-range
// both use (docs/engineering/dynamodb.md §5.2).
func TestListObservations_DefaultsToToday(t *testing.T) {
	f := &fakeStore{
		devices:  map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired}},
		obsByDay: map[string][]store.Observation{},
	}
	// fixedNow is 02:00Z = 11:00 JST on the 8th.
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})
	if _, err := newHandler(f).ListObservations(context.Background(), req); err != nil {
		t.Fatalf("ListObservations without ?date=: %v", err)
	}
}

// TestListObservations_BadDate is a 400 rather than an empty list: silently
// returning nothing for "08-09-2026" would look like "no photos that day".
func TestListObservations_BadDate(t *testing.T) {
	f := &fakeStore{devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired}}}
	for _, bad := range []string{"08-09-2026", "2026/09/08", "2026-9-8", "yesterday"} {
		req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"}), map[string]string{"date": bad})
		_, err := newHandler(f).ListObservations(context.Background(), req)
		assertCode(t, err, apierr.CodeValidation)
	}
}

// --- GET /observations/{id}/image ----------------------------------------

// TestObservationImage_RequiresDeviceID pins G11. Observations are keyed by
// PK = DEVICE#<deviceId>, so the id alone cannot address one — and more to
// the point, without a deviceId there is nothing to run the ownership check
// against, so the route would hand out any image to any authenticated user.
func TestObservationImage_RequiresDeviceID(t *testing.T) {
	f := &fakeStore{devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired}}}
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "obs-1"})
	_, err := newHandler(f).ObservationImage(context.Background(), req)
	assertCode(t, err, apierr.CodeValidation)
}

func TestObservationImage_Success(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired}},
		obs: map[string]*store.Observation{
			"dev-1/obs-1": {ObservationID: "obs-1", DeviceID: "dev-1", ImageKey: "full/1.jpg", ThumbnailKey: "thumb/1.jpg"},
		},
	}
	req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "obs-1"}), map[string]string{"deviceId": "dev-1"})

	resp, err := newHandler(f).ObservationImage(context.Background(), req)
	if err != nil {
		t.Fatalf("ObservationImage: %v", err)
	}
	got := decodeBody[ObservationImageResponse](t, resp)
	if got.ObservationID != "obs-1" {
		t.Errorf("observationId = %q, want obs-1", got.ObservationID)
	}
	// The FULL image, not the thumbnail: this route exists precisely
	// because the list response omits it.
	if !strings.Contains(got.ImageURL, "full/1.jpg") {
		t.Errorf("imageUrl = %q, want the full-size key", got.ImageURL)
	}
	if want := fixedNow.Add(900 * time.Second).Unix(); got.ExpiresAt != want {
		t.Errorf("expiresAt = %d, want %d", got.ExpiresAt, want)
	}
}

func TestObservationImage_NotFound(t *testing.T) {
	f := &fakeStore{devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired}}}
	req := withQuery(withPath(ownerRequest("owner-1"), map[string]string{"id": "nope"}), map[string]string{"deviceId": "dev-1"})
	_, err := newHandler(f).ObservationImage(context.Background(), req)
	assertCode(t, err, apierr.CodeObservationNotFound)
}

// --- POST /devices -------------------------------------------------------

// TestCreateDevice_ReturnsDeviceAndSession matches api.ts's
// CreateDeviceResponse. The session is echoed twice — once at the top level
// and once inside device.activePairingSession — because the pairing modal
// reads the first and the device list row reads the second, and having the
// server fill both means the client never has to graft them together.
func TestCreateDevice_ReturnsDeviceAndSession(t *testing.T) {
	f := &fakeStore{}
	resp, err := newHandler(f).CreateDevice(context.Background(), withBody(ownerRequest("owner-1"), `{"name":"玄関"}`))
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	got := decodeBody[CreateDeviceResponse](t, resp)

	if got.Device.Name != "玄関" || got.Device.Status != store.DeviceStatusPending {
		t.Errorf("device = %+v, want a PENDING 玄関", got.Device)
	}
	if got.Device.DeviceID == "" {
		t.Error("deviceId is empty")
	}
	if got.PairingSession.PairingCode == "" {
		t.Error("pairingCode is empty")
	}
	if got.Device.ActivePairingSession == nil {
		t.Fatal("device.activePairingSession is null on a device that was just issued a QR")
	}
	if got.Device.ActivePairingSession.PairingCode != got.PairingSession.PairingCode {
		t.Error("the two copies of the session disagree")
	}
	if got.PairingSession.ExpiresAt != fixedNow.Add(5*time.Minute).Unix() {
		t.Errorf("expiresAt = %d, want now+5m", got.PairingSession.ExpiresAt)
	}
	if got.Device.Interval != 5 {
		t.Errorf("interval = %d, want the 5-minute default", got.Device.Interval)
	}
}

// TestCreateDevice_LimitCountsPendingNotArchived. A PENDING device occupies
// a slot — it is a real row in the list with a name the owner chose — while
// an ARCHIVED one has been deleted and must not. Counting the wrong set
// either locks an owner out of their own quota or lets them exceed it.
func TestCreateDevice_LimitCountsPendingNotArchived(t *testing.T) {
	t.Run("at the limit, counting PENDING", func(t *testing.T) {
		var rows []store.OwnerListRow
		for i := 0; i < 10; i++ {
			rows = append(rows, deviceRow(string(rune('a'+i)), "d", store.DeviceStatusPending))
		}
		f := &fakeStore{rows: rows}
		_, err := newHandler(f).CreateDevice(context.Background(), withBody(ownerRequest("owner-1"), `{"name":"11台目"}`))
		assertCode(t, err, apierr.CodeDeviceLimitExceeded)
		if len(f.created) != 0 {
			t.Error("a device was created despite the limit")
		}
	})

	t.Run("archived rows do not consume the quota", func(t *testing.T) {
		var rows []store.OwnerListRow
		for i := 0; i < 9; i++ {
			rows = append(rows, deviceRow(string(rune('a'+i)), "d", store.DeviceStatusPaired))
		}
		rows = append(rows, deviceRow("z", "削除済み", store.DeviceStatusArchived))
		f := &fakeStore{rows: rows}
		if _, err := newHandler(f).CreateDevice(context.Background(), withBody(ownerRequest("owner-1"), `{"name":"10台目"}`)); err != nil {
			t.Fatalf("CreateDevice: %v", err)
		}
	})

	t.Run("pairing rows are not devices", func(t *testing.T) {
		var rows []store.OwnerListRow
		for i := 0; i < 5; i++ {
			id := string(rune('a' + i))
			rows = append(rows, deviceRow(id, "d", store.DeviceStatusPaired))
			rows = append(rows, pairingRow(id, "CODE", store.PairingStatusPending, fixedNow.Add(time.Minute).Unix()))
		}
		f := &fakeStore{rows: rows}
		if _, err := newHandler(f).CreateDevice(context.Background(), withBody(ownerRequest("owner-1"), `{"name":"6台目"}`)); err != nil {
			t.Fatalf("CreateDevice counted pairing rows as devices: %v", err)
		}
	})
}

// TestCreateDevice_RequiresName: the name is what makes the immediately
// visible PENDING row identifiable (docs/03-web.md §1.8.1). A blank one
// leaves an unlabelled row the owner cannot tell apart from any other.
func TestCreateDevice_RequiresName(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":""}`, `{"name":"   "}`, `{ not json`} {
		_, err := newHandler(&fakeStore{}).CreateDevice(context.Background(), withBody(ownerRequest("owner-1"), body))
		assertCode(t, err, apierr.CodeValidation)
	}
}

// TestCreateDevice_UsesTheCallersOwnerID: the ownerId is the JWT's sub, so
// a request cannot create a device in someone else's account.
func TestCreateDevice_UsesTheCallersOwnerID(t *testing.T) {
	f := &fakeStore{}
	if _, err := newHandler(f).CreateDevice(context.Background(),
		withBody(ownerRequest("owner-1"), `{"name":"玄関","ownerId":"owner-2"}`)); err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if f.created[0].OwnerID != "owner-1" {
		t.Errorf("OwnerID = %q, want the JWT's sub", f.created[0].OwnerID)
	}
}

// --- POST /devices/{id}/pairing-sessions ---------------------------------

// TestCreatePairingSession_ReusesTheDevice covers both re-issuing an
// expired QR and re-pairing a disconnected device (docs/03-web.md §1.8.2,
// §1.8.3). Neither creates a new Device row: the deviceId is immutable, and
// that is what keeps a replaced Android phone attached to the same
// observation history.
func TestCreatePairingSession_ReusesTheDevice(t *testing.T) {
	for _, status := range []string{store.DeviceStatusPending, store.DeviceStatusPaired, store.DeviceStatusDisconnected} {
		t.Run(status, func(t *testing.T) {
			f := &fakeStore{devices: map[string]*store.Device{
				"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Name: "玄関", Status: status, Interval: 5},
			}}
			req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})

			resp, err := newHandler(f).CreatePairingSession(context.Background(), req)
			if err != nil {
				t.Fatalf("CreatePairingSession: %v", err)
			}
			got := decodeBody[api.PairingSession](t, resp)
			if got.PairingCode == "" {
				t.Error("pairingCode is empty")
			}
			if got.Status != store.PairingStatusPending {
				t.Errorf("status = %q, want PENDING", got.Status)
			}
			if got.ExpiresAt != fixedNow.Add(5*time.Minute).Unix() {
				t.Errorf("expiresAt = %d, want now+5m", got.ExpiresAt)
			}
			if len(f.sessions) != 1 || f.sessions[0].DeviceID != "dev-1" {
				t.Errorf("CreatePairingSession input = %+v, want it bound to dev-1", f.sessions)
			}
		})
	}
}

// TestCreatePairingSession_CodesAreNotReused: each issue must mint a fresh
// code, or an expired QR photographed earlier would still be redeemable.
func TestCreatePairingSession_CodesAreNotReused(t *testing.T) {
	f := &fakeStore{devices: map[string]*store.Device{
		"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPending, Interval: 5},
	}}
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})
	h := newHandler(f)

	first, err := h.CreatePairingSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}
	second, err := h.CreatePairingSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}
	if decodeBody[api.PairingSession](t, first).PairingCode == decodeBody[api.PairingSession](t, second).PairingCode {
		t.Fatal("two issues produced the same pairing code")
	}
}

// --- GET /devices/{id}/pairing-sessions/latest ---------------------------

// TestLatestPairingSession_ReportsConsumed is the polling contract the web
// actually implements (web/src/hooks/use-pairing.ts reads
// session.status === 'CONSUMED' off a 200 body). Returning 409 for a
// consumed session — as docs/03-web.md §1.10.4's table suggests — would
// make the modal show "expired, re-issue" at the exact moment pairing
// SUCCEEDED. Expiry is handled client-side by the countdown, so this route
// reports state rather than adjudicating it.
func TestLatestPairingSession_ReportsConsumed(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired, Interval: 5}},
		rows: []store.OwnerListRow{
			deviceRow("dev-1", "玄関", store.DeviceStatusPaired),
			pairingRow("dev-1", "CODE1", store.PairingStatusConsumed, fixedNow.Add(time.Minute).Unix()),
		},
	}
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})

	resp, err := newHandler(f).LatestPairingSession(context.Background(), req)
	if err != nil {
		t.Fatalf("LatestPairingSession: %v", err)
	}
	got := decodeBody[api.PairingSession](t, resp)
	if got.Status != store.PairingStatusConsumed {
		t.Errorf("status = %q, want CONSUMED so the modal can show 接続しました", got.Status)
	}
	if got.PairingCode != "CODE1" {
		t.Errorf("pairingCode = %q, want CODE1", got.PairingCode)
	}
}

func TestLatestPairingSession_ReportsPending(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPending, Interval: 5}},
		rows: []store.OwnerListRow{
			deviceRow("dev-1", "玄関", store.DeviceStatusPending),
			pairingRow("dev-1", "OLD", store.PairingStatusPending, fixedNow.Add(time.Minute).Unix()),
			pairingRow("dev-1", "NEW", store.PairingStatusPending, fixedNow.Add(4*time.Minute).Unix()),
		},
	}
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})

	resp, err := newHandler(f).LatestPairingSession(context.Background(), req)
	if err != nil {
		t.Fatalf("LatestPairingSession: %v", err)
	}
	if got := decodeBody[api.PairingSession](t, resp); got.PairingCode != "NEW" {
		t.Errorf("pairingCode = %q, want the most recent one", got.PairingCode)
	}
}

// TestLatestPairingSession_NoneEver is a 404: a device that has never been
// issued a QR has nothing to report, and an empty 200 body would make the
// polling client parse a session out of nothing.
func TestLatestPairingSession_NoneEver(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired, Interval: 5}},
		rows:    []store.OwnerListRow{deviceRow("dev-1", "玄関", store.DeviceStatusPaired)},
	}
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})
	_, err := newHandler(f).LatestPairingSession(context.Background(), req)
	assertCode(t, err, apierr.CodePairingNotFound)
}

// TestLatestPairingSession_IgnoresOtherDevices: the GSI1 query returns every
// row in the owner's partition, so filtering by deviceId is what keeps one
// device's QR out of another device's modal.
func TestLatestPairingSession_IgnoresOtherDevices(t *testing.T) {
	f := &fakeStore{
		devices: map[string]*store.Device{"dev-1": {DeviceID: "dev-1", OwnerID: "owner-1", Status: store.DeviceStatusPaired, Interval: 5}},
		rows: []store.OwnerListRow{
			deviceRow("dev-1", "玄関", store.DeviceStatusPaired),
			pairingRow("dev-2", "OTHER", store.PairingStatusPending, fixedNow.Add(4*time.Minute).Unix()),
		},
	}
	req := withPath(ownerRequest("owner-1"), map[string]string{"id": "dev-1"})
	_, err := newHandler(f).LatestPairingSession(context.Background(), req)
	assertCode(t, err, apierr.CodePairingNotFound)
}

// --- wiring --------------------------------------------------------------

// TestRegister pins every route key against infra/envs/dev/main.tf's
// web_routes. A mismatch is invisible until deploy, where API Gateway
// forwards a routeKey this router does not know and answers ROUTE_NOT_WIRED.
func TestRegister(t *testing.T) {
	rt := httpx.New(nil)
	newHandler(&fakeStore{}).Register(rt)

	for _, routeKey := range []string{
		"GET /app-config",
		"GET /devices",
		"POST /devices",
		"POST /devices/{id}/pairing-sessions",
		"GET /devices/{id}/pairing-sessions/latest",
		"GET /devices/{id}/observations",
		"GET /observations/{id}/image",
	} {
		req := ownerRequest("owner-1")
		req.RouteKey = routeKey
		resp, err := rt.Route(context.Background(), req)
		if err != nil {
			t.Fatalf("Route(%q): %v", routeKey, err)
		}
		if strings.Contains(resp.Body, "ROUTE_NOT_WIRED") {
			t.Errorf("%s is not registered", routeKey)
		}
	}
}

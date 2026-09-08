// Package webapi implements the routes the browser calls behind API
// Gateway's Cognito JWT authorizer (docs/05-backend.md §1.1).
//
// This function never verifies a token itself — the gateway does, and
// §3.1 explains why adding a Lambda hop to re-check a signature buys
// nothing. What it must do on every single request is check ownership:
// the JWT proves who is calling, not what they may see, and a deviceId is
// otherwise a bearer token for someone else's camera history
// (docs/06-auth.md §7). loadOwnedDevice is the one place that check lives.
package webapi

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/api"
	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

// defaultInterval is the upload interval a newly created device starts on
// (minutes). It is the first of api.IntervalOptions rather than a separate
// literal so the default cannot drift out of the set the web offers.
var defaultInterval = api.IntervalOptions[0]

// Store is the slice of internal/store this package uses.
type Store interface {
	ListOwnerDevices(ctx context.Context, ownerID string) ([]store.OwnerListRow, error)
	GetDeviceForAuth(ctx context.Context, deviceID string) (*store.Device, error)
	QueryObservationsByDay(ctx context.Context, deviceID, dateStr string) ([]store.Observation, error)
	GetObservation(ctx context.Context, deviceID, observationID string) (*store.Observation, error)
	CreateDeviceWithPairing(ctx context.Context, in store.CreateDeviceWithPairingInput) (*store.Device, *store.PairingSession, error)
	CreatePairingSession(ctx context.Context, in store.CreatePairingSessionInput) (*store.PairingSession, error)
	DisconnectDevice(ctx context.Context, deviceID string) (*store.Device, error)
	UpdateDeviceProfile(ctx context.Context, in store.UpdateDeviceProfileInput) (*store.Device, error)
	ArchiveDevice(ctx context.Context, deviceID string, now time.Time) (*store.Device, error)
}

// Config is the environment-derived settings this package needs
// (docs/05-backend.md §3.6).
type Config struct {
	RetentionDays int
	DeviceLimit   int
}

// Handler serves the web routes.
type Handler struct {
	store  Store
	mapper *api.Mapper
	clock  clock.Clock
	cfg    Config
	logger *slog.Logger
}

// New constructs a Handler.
func New(s Store, m *api.Mapper, c clock.Clock, cfg Config, logger *slog.Logger) *Handler {
	return &Handler{store: s, mapper: m, clock: c, cfg: cfg, logger: logger}
}

// Register wires this package's routes. The keys must match
// infra/envs/dev/main.tf's web_routes exactly.
func (h *Handler) Register(rt *httpx.Router) {
	rt.Handle("GET /app-config", h.AppConfig)
	rt.Handle("GET /devices", h.ListDevices)
	rt.Handle("POST /devices", h.CreateDevice)
	rt.Handle("POST /devices/{id}/pairing-sessions", h.CreatePairingSession)
	rt.Handle("GET /devices/{id}/pairing-sessions/latest", h.LatestPairingSession)
	rt.Handle("GET /devices/{id}/observations", h.ListObservations)
	rt.Handle("GET /observations/{id}/image", h.ObservationImage)
	rt.Handle("POST /devices/{id}/disconnect", h.DisconnectDevice)
	rt.Handle("PATCH /devices/{id}", h.UpdateDevice)
	rt.Handle("DELETE /devices/{id}", h.DeleteDevice)
}

// CreateDeviceResponse is POST /devices' body (api.ts:
// CreateDeviceResponse).
//
// The session appears twice — once at the top level and once inside
// Device.ActivePairingSession — because the pairing modal reads one and the
// device list row reads the other. Filling both server-side means the
// client never grafts them together and cannot get the join wrong.
type CreateDeviceResponse struct {
	Device         api.Device         `json:"device"`
	PairingSession api.PairingSession `json:"pairingSession"`
}

// ObservationImageResponse is GET /observations/{id}/image's body (api.ts:
// GetObservationImageResponse).
type ObservationImageResponse struct {
	ObservationID string `json:"observationId"`
	ImageURL      string `json:"imageUrl"`
	ExpiresAt     int64  `json:"expiresAt"`
}

// AppConfig returns the environment constants the web renders against.
func (h *Handler) AppConfig(_ context.Context, _ events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	return httpx.Response{Body: api.AppConfig{
		RetentionDays:   h.cfg.RetentionDays,
		DeviceLimit:     h.cfg.DeviceLimit,
		IntervalOptions: api.IntervalOptions,
	}}, nil
}

// ListDevices serves the dashboard from a single GSI1 query (access pattern
// 1). Device rows and pairing rows arrive together and are joined here, so
// showing "ペアリング待ち" costs no extra read per device.
func (h *Handler) ListDevices(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	ownerID, err := httpx.OwnerID(req)
	if err != nil {
		return httpx.Response{}, err
	}
	rows, err := h.store.ListOwnerDevices(ctx, ownerID)
	if err != nil {
		return httpx.Response{}, err
	}

	now := h.clock.Now()
	latest := latestSessionsByDevice(rows)

	devices := make([]api.Device, 0, len(rows))
	for _, row := range rows {
		if !row.IsDevice() {
			continue
		}
		// ArchiveDevice repoints GSI1PK to "ARCHIVED", so a deleted device
		// is already outside this partition. The filter is the belt to that
		// braces: a deleted device reappearing is the single outcome the
		// logical delete exists to prevent.
		if row.Status == store.DeviceStatusArchived {
			continue
		}
		dev, err := h.mapper.DeviceFromRow(ctx, row, api.ActivePairingSession(latest[row.DeviceID], now), now)
		if err != nil {
			return httpx.Response{}, err
		}
		devices = append(devices, dev)
	}
	return httpx.Response{Body: devices}, nil
}

// CreateDevice creates the Device row and its first PairingSession in one
// transaction (docs/06-auth.md §2).
//
// The device is created BEFORE it is ever scanned, as PENDING, so the row
// appears in the list immediately and the owner can walk to the camera
// knowing the pairing is in flight. The cost — a PENDING row for a QR
// nobody scanned — is accepted and cleaned up by the owner, not by us.
func (h *Handler) CreateDevice(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	ownerID, err := httpx.OwnerID(req)
	if err != nil {
		return httpx.Response{}, err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := httpx.DecodeJSON(req, &body); err != nil {
		return httpx.Response{}, err
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "端末名を入力してください")
	}

	rows, err := h.store.ListOwnerDevices(ctx, ownerID)
	if err != nil {
		return httpx.Response{}, err
	}
	if countDevices(rows) >= h.cfg.DeviceLimit {
		return httpx.Response{}, apierr.New(apierr.CodeDeviceLimitExceeded, "端末の上限に達しています")
	}

	now := h.clock.Now()
	deviceID, err := ids.NewULID(now)
	if err != nil {
		return httpx.Response{}, apierr.Wrap(apierr.CodeInternal, "internal error", err)
	}
	pairingCode, err := ids.NewPairingCode()
	if err != nil {
		return httpx.Response{}, apierr.Wrap(apierr.CodeInternal, "internal error", err)
	}

	dev, session, err := h.store.CreateDeviceWithPairing(ctx, store.CreateDeviceWithPairingInput{
		OwnerID: ownerID, DeviceID: deviceID, Name: name,
		Interval: defaultInterval, PairingCode: pairingCode, Now: now,
	})
	if err != nil {
		return httpx.Response{}, err
	}

	sessionDTO := api.PairingSessionDTO(*session)
	deviceDTO, err := h.mapper.Device(ctx, *dev, &sessionDTO, now)
	if err != nil {
		return httpx.Response{}, err
	}

	// deviceId, not pairingCode: the code is a live credential for the next
	// five minutes and must not reach a log (docs/05-backend.md §2.5).
	h.log(ctx, "webapi: device created", "ownerId", ownerID, "deviceId", deviceID)
	return httpx.Response{Body: CreateDeviceResponse{Device: deviceDTO, PairingSession: sessionDTO}}, nil
}

// CreatePairingSession issues a fresh QR for an existing device: an expired
// code being re-issued (docs/03-web.md §1.8.2) or a disconnected device
// being re-paired (§1.8.3).
//
// It never creates a Device row. The deviceId is immutable, which is what
// keeps a replaced or factory-reset Android phone attached to the same
// observation history.
func (h *Handler) CreatePairingSession(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	ownerID, dev, err := h.loadOwnedDevice(ctx, req)
	if err != nil {
		return httpx.Response{}, err
	}
	pairingCode, err := ids.NewPairingCode()
	if err != nil {
		return httpx.Response{}, apierr.Wrap(apierr.CodeInternal, "internal error", err)
	}
	session, err := h.store.CreatePairingSession(ctx, store.CreatePairingSessionInput{
		DeviceID: dev.DeviceID, OwnerID: ownerID, PairingCode: pairingCode, Now: h.clock.Now(),
	})
	if err != nil {
		return httpx.Response{}, err
	}
	h.log(ctx, "webapi: pairing session issued", "ownerId", ownerID, "deviceId", dev.DeviceID)
	return httpx.Response{Body: api.PairingSessionDTO(*session)}, nil
}

// LatestPairingSession is the 2-second poll the pairing modal runs
// (docs/03-web.md §1.8.1).
//
// It reports the session's state rather than adjudicating it: a CONSUMED
// session comes back as a 200 whose status the client reads to switch the
// modal to 接続しました (web/src/hooks/use-pairing.ts). Answering 409 for a
// consumed session — as docs/03-web.md §1.10.4's table suggests — would show
// "expired, re-issue" at the exact moment pairing SUCCEEDED. Expiry is
// likewise the client's countdown to detect, since it already has expiresAt
// and does not need a round trip to learn the clock advanced.
//
// A device that has never been issued a QR is a 404: an empty 200 would
// make the poller parse a session out of nothing.
func (h *Handler) LatestPairingSession(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	ownerID, dev, err := h.loadOwnedDevice(ctx, req)
	if err != nil {
		return httpx.Response{}, err
	}
	rows, err := h.store.ListOwnerDevices(ctx, ownerID)
	if err != nil {
		return httpx.Response{}, err
	}
	session := latestSessionsByDevice(rows)[dev.DeviceID]
	if session == nil {
		return httpx.Response{}, apierr.New(apierr.CodePairingNotFound, "ペアリングセッションがありません")
	}
	return httpx.Response{Body: api.PairingSessionDTO(*session)}, nil
}

// ListObservations returns one JST day of photos, newest first, each with a
// signed thumbnail URL.
//
// Omitting ?date= means today rather than being an error: the history view
// opens on today, and requiring the client to compute the date would put a
// second definition of "today" — in the browser's timezone, not JST — in
// front of a query whose day boundaries are JST by construction
// (docs/engineering/dynamodb.md §5.2).
func (h *Handler) ListObservations(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	_, dev, err := h.loadOwnedDevice(ctx, req)
	if err != nil {
		return httpx.Response{}, err
	}
	now := h.clock.Now()
	date := req.QueryStringParameters["date"]
	if date == "" {
		date = now.In(ids.JST).Format("2006-01-02")
	}

	obs, err := h.store.QueryObservationsByDay(ctx, dev.DeviceID, date)
	if err != nil {
		return httpx.Response{}, err
	}
	// Order comes from the query (ScanIndexForward:false) and is preserved
	// here — the scrubber depends on newest-first.
	mapped, err := h.mapper.Observations(ctx, obs, now)
	if err != nil {
		return httpx.Response{}, err
	}
	return httpx.Response{Body: mapped}, nil
}

// ObservationImage issues a signed URL for one full-size image.
//
// ?deviceId= is required (G11) for two reasons that point the same way:
// observations are keyed by PK = DEVICE#<deviceId> so the observation id
// alone cannot address one, and without a device there is nothing to run
// the ownership check against — the route would hand any image to any
// authenticated user.
func (h *Handler) ObservationImage(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	observationID := req.PathParameters["id"]
	if observationID == "" {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "observationId が指定されていません")
	}
	deviceID := req.QueryStringParameters["deviceId"]
	if deviceID == "" {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "deviceId クエリパラメータは必須です")
	}
	if _, _, err := h.loadOwnedDeviceByID(ctx, req, deviceID); err != nil {
		return httpx.Response{}, err
	}

	obs, err := h.store.GetObservation(ctx, deviceID, observationID)
	if err != nil {
		return httpx.Response{}, err
	}
	signed, err := h.mapper.SignImage(ctx, obs.ImageKey, h.clock.Now())
	if err != nil {
		return httpx.Response{}, err
	}
	// The URL itself is never logged: it is a working credential for its
	// whole TTL, so a log line would leak the image for the log group's
	// 30-day retention rather than for 15 minutes (docs/05-backend.md §2.5).
	return httpx.Response{Body: ObservationImageResponse{
		ObservationID: obs.ObservationID,
		ImageURL:      signed.URL,
		ExpiresAt:     signed.ExpiresAt,
	}}, nil
}

// DisconnectDeviceResponse is POST /devices/{id}/disconnect's body (api.ts:
// DisconnectDeviceResponse).
//
// It is deliberately narrower than a full Device. The web patches the
// affected row from it, and returning a whole Device would invite a client
// to overwrite fields this operation never touched — a stale interval or
// thumbnail arriving back as if it were fresh.
type DisconnectDeviceResponse struct {
	DeviceID string `json:"deviceId"`
	Status   string `json:"status"`
}

// DisconnectDevice revokes a device's credentials without deleting it
// (docs/06-auth.md §6).
//
// Both hashes are cleared, not just the session: clearing only
// sessionTokenHash would let the device call POST /device/token with the
// deviceSecret it still holds and be back online within seconds
// (§6, "セッション切断が sessionToken だけでは成立しない理由"). The device
// discovers this the next time it uploads, as a 401 it cannot refresh past
// (§5) — nothing pushes the disconnection to it.
//
// The status in the response comes from the write's ALL_NEW return rather
// than from a constant, so what the client renders is what was actually
// stored.
func (h *Handler) DisconnectDevice(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	_, dev, err := h.loadOwnedDevice(ctx, req)
	if err != nil {
		return httpx.Response{}, err
	}
	updated, err := h.store.DisconnectDevice(ctx, dev.DeviceID)
	if err != nil {
		return httpx.Response{}, err
	}
	h.log(ctx, "webapi: device disconnected", "ownerId", updated.OwnerID, "deviceId", updated.DeviceID)
	return httpx.Response{Body: DisconnectDeviceResponse{
		DeviceID: updated.DeviceID,
		Status:   updated.Status,
	}}, nil
}

// updateDeviceRequest is PATCH /devices/{id}'s body (api.ts:
// UpdateDeviceRequest).
//
// Both fields are pointers because PATCH means "change what I sent". With
// plain values there is no way to tell `{"name":"x"}` from
// `{"name":"x","interval":0}`, and the second would write interval = 0 —
// silently stopping the device from ever uploading again.
type updateDeviceRequest struct {
	Name     *string `json:"name"`
	Interval *int    `json:"interval"`
}

// UpdateDevice renames a device and/or changes its upload interval
// (docs/05-backend.md §1.2). It returns the complete Device (api.ts:
// UpdateDeviceResponse = Device) so the list row is replaced wholesale
// rather than merged field by field.
//
// Ownership is proven before the body is even parsed: validating a
// stranger's payload and answering "invalid interval" would confirm the
// device exists to someone with no right to know it does.
func (h *Handler) UpdateDevice(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	_, dev, err := h.loadOwnedDevice(ctx, req)
	if err != nil {
		return httpx.Response{}, err
	}

	var body updateDeviceRequest
	if err := httpx.DecodeJSON(req, &body); err != nil {
		return httpx.Response{}, err
	}
	if body.Name == nil && body.Interval == nil {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "変更する項目がありません")
	}

	in := store.UpdateDeviceProfileInput{DeviceID: dev.DeviceID}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" {
			return httpx.Response{}, apierr.New(apierr.CodeValidation, "端末名を入力してください")
		}
		in.Name = &name
	}
	if body.Interval != nil {
		// Validated against the same list GET /app-config hands the
		// client's <select>, so the offered choices and the accepted ones
		// cannot drift. This value reaches the device through the upload
		// response's nextConfig and becomes its real cadence: an
		// unvalidated 0 stops a camera, an unvalidated 1 hammers the API.
		if !isOfferedInterval(*body.Interval) {
			return httpx.Response{}, apierr.New(apierr.CodeValidation, "送信間隔が不正です")
		}
		in.Interval = body.Interval
	}

	updated, err := h.store.UpdateDeviceProfile(ctx, in)
	if err != nil {
		return httpx.Response{}, err
	}

	now := h.clock.Now()
	// A PATCH never touches pairing, so the session is re-resolved rather
	// than assumed absent: a rename while the QR modal is open must not
	// come back with activePairingSession null and blank the modal.
	session, err := h.activeSessionFor(ctx, updated.OwnerID, updated.DeviceID, now)
	if err != nil {
		return httpx.Response{}, err
	}
	dto, err := h.mapper.Device(ctx, *updated, session, now)
	if err != nil {
		return httpx.Response{}, err
	}
	h.log(ctx, "webapi: device updated", "ownerId", updated.OwnerID, "deviceId", updated.DeviceID)
	return httpx.Response{Body: dto}, nil
}

// DeleteDevice logically deletes a device (docs/06-auth.md §6): status
// ARCHIVED, credentials cleared, and GSI1PK swapped into the ARCHIVED
// partition so it leaves the owner's list and stops counting against the
// device limit.
//
// The row itself survives, which is what keeps the immediate-revocation
// guarantee honest: the authorizer denies on status, not on absence
// (§4), so a deleted device is rejected on its very next request even
// though its record is still there.
//
// 204 with no body. api-client.ts synthesizes {success:true} itself rather
// than reading one from us.
func (h *Handler) DeleteDevice(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	_, dev, err := h.loadOwnedDevice(ctx, req)
	if err != nil {
		return httpx.Response{}, err
	}
	updated, err := h.store.ArchiveDevice(ctx, dev.DeviceID, h.clock.Now())
	if err != nil {
		return httpx.Response{}, err
	}
	h.log(ctx, "webapi: device archived", "ownerId", updated.OwnerID, "deviceId", updated.DeviceID)
	return httpx.Response{StatusCode: http.StatusNoContent}, nil
}

// isOfferedInterval reports whether v is one of the intervals
// GET /app-config advertises.
func isOfferedInterval(v int) bool {
	for _, offered := range api.IntervalOptions {
		if v == offered {
			return true
		}
	}
	return false
}

// activeSessionFor resolves one device's currently-showable pairing session
// through the owner's GSI1 partition — the same query and the same rule
// ListDevices uses, so a device's activePairingSession cannot mean one
// thing in the list and another in a single-device response.
func (h *Handler) activeSessionFor(ctx context.Context, ownerID, deviceID string, now time.Time) (*api.PairingSession, error) {
	rows, err := h.store.ListOwnerDevices(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	return api.ActivePairingSession(latestSessionsByDevice(rows)[deviceID], now), nil
}

// loadOwnedDevice resolves {id} from the path and proves the caller owns it.
func (h *Handler) loadOwnedDevice(ctx context.Context, req events.APIGatewayV2HTTPRequest) (string, *store.Device, error) {
	deviceID := req.PathParameters["id"]
	if deviceID == "" {
		return "", nil, apierr.New(apierr.CodeValidation, "deviceId が指定されていません")
	}
	return h.loadOwnedDeviceByID(ctx, req, deviceID)
}

// loadOwnedDeviceByID is the ownership check every device-scoped route runs
// (docs/06-auth.md §7).
//
// 403 and 404 stay distinct even though docs/05-backend.md §1.6 has the web
// render them identically. The difference is what reaches the log, and an
// attacker walking deviceIds is precisely what those logs exist to reveal —
// collapsing them server-side would erase the signal to avoid an
// information leak the client already absorbs.
//
// An ARCHIVED device is a 404 rather than a 403: it belongs to this owner,
// but they deleted it, and a logical delete that still answered requests
// would be indistinguishable from no delete at all.
//
// The read is store.GetDeviceForAuth, whose ConsistentRead was chosen for
// the authorizer's immediate-revocation guarantee. It is the right read here
// too: the pairing modal polls two seconds after POST /devices, and an
// eventually-consistent read could report "device not found" for a device
// the same client just created.
func (h *Handler) loadOwnedDeviceByID(ctx context.Context, req events.APIGatewayV2HTTPRequest, deviceID string) (string, *store.Device, error) {
	ownerID, err := httpx.OwnerID(req)
	if err != nil {
		return "", nil, err
	}
	dev, err := h.store.GetDeviceForAuth(ctx, deviceID)
	if err != nil {
		return "", nil, err
	}
	if dev.OwnerID != ownerID {
		h.log(ctx, "webapi: ownership mismatch", "ownerId", ownerID, "deviceId", deviceID, "deviceOwnerId", dev.OwnerID)
		return "", nil, apierr.New(apierr.CodeForbidden, "この端末を操作する権限がありません")
	}
	if dev.Status == store.DeviceStatusArchived {
		return "", nil, apierr.New(apierr.CodeDeviceNotFound, "端末が見つかりません")
	}
	return ownerID, dev, nil
}

// latestSessionsByDevice picks, per device, the most recently issued pairing
// session out of a GSI1 result.
//
// A device accumulates sessions: re-issuing a QR writes a new row and leaves
// the old one, since expiry is enforced by condition rather than by deletion
// (docs/06-auth.md §2). "Most recent" is decided by expiresAt because
// createdAt is not in GSI1's projection, and the two are equivalent for
// ordering — every session expires exactly five minutes after it is issued,
// so expiresAt is createdAt shifted by a constant.
//
// Ties are broken by pairingCode so the choice is deterministic rather than
// dependent on the index's return order; two sessions issued in the same
// second are already a pathological case, but a nondeterministic answer to a
// 2-second poll would flicker between two QRs.
func latestSessionsByDevice(rows []store.OwnerListRow) map[string]*store.PairingSession {
	byDevice := map[string]*store.PairingSession{}
	sorted := make([]store.OwnerListRow, 0, len(rows))
	for _, row := range rows {
		if row.IsPairingSession() {
			sorted = append(sorted, row)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ExpiresAt != sorted[j].ExpiresAt {
			return sorted[i].ExpiresAt < sorted[j].ExpiresAt
		}
		return sorted[i].PairingCode < sorted[j].PairingCode
	})
	for _, row := range sorted {
		byDevice[row.DeviceID] = &store.PairingSession{
			DeviceID:    row.DeviceID,
			OwnerID:     row.OwnerID(),
			PairingCode: row.PairingCode,
			Status:      row.Status,
			ExpiresAt:   row.ExpiresAt,
		}
	}
	return byDevice
}

// countDevices counts the rows that occupy a slot against DEVICE_LIMIT.
//
// PENDING counts: it is a real row in the owner's list with a name they
// chose, and not counting it would let an owner queue unlimited unscanned
// QRs. ARCHIVED does not: they deleted it. Pairing rows are not devices at
// all — counting them would halve the effective limit for anyone mid-setup.
func countDevices(rows []store.OwnerListRow) int {
	n := 0
	for _, row := range rows {
		if row.IsDevice() && row.Status != store.DeviceStatusArchived {
			n++
		}
	}
	return n
}

func (h *Handler) log(ctx context.Context, msg string, args ...any) {
	if h.logger == nil {
		return
	}
	h.logger.InfoContext(ctx, msg, args...)
}

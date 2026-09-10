// Package deviceauth implements the two routes that run WITHOUT an
// authorizer in front of them: POST /device/pair and POST /device/token
// (docs/05-backend.md §1.1).
//
// Everything reaching this package is untrusted. That is why it lives in
// its own function and its own package rather than beside the authorized
// handlers: the property "no caller here has been authenticated yet" is a
// fact about the deployment, and keeping it visible in the code layout is
// the point of the split.
//
// The two routes are the only places a long-lived credential is minted
// (pair) or exchanged (token). Both hash through internal/tokens rather
// than computing a digest locally — see that package for why a second
// implementation of "SHA-256" is a landmine rather than a duplication.
package deviceauth

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
	"github.com/ShuMasui/edgewatcher/backend/internal/tokens"
)

// SessionTTL is how long an issued session lives (docs/06-auth.md §3: 12
// hours). A device refreshes roughly twice a day, which is what keeps the
// long-lived deviceSecret off the wire the rest of the time.
const SessionTTL = 12 * time.Hour

// Store is the slice of internal/store this package uses. It is narrow on
// purpose: device-auth's execution role grants Query on GSI2,
// TransactWriteItems, UpdateItem and GetItem and nothing else
// (docs/05-backend.md §2.4), and a method here that the role cannot perform
// would be an AccessDenied at runtime where a compile error belongs.
type Store interface {
	FindPairingByCode(ctx context.Context, pairingCode string) (*store.PairingSession, error)
	ConsumePairing(ctx context.Context, in store.ConsumePairingInput) error
	GetDeviceForAuth(ctx context.Context, deviceID string) (*store.Device, error)
	RotateSessionToken(ctx context.Context, deviceID, sessionTokenHash string, sessionExpiresAt int64) (*store.Device, error)
}

// Handler serves the pre-auth device routes.
type Handler struct {
	store  Store
	clock  clock.Clock
	logger *slog.Logger
}

// New constructs a Handler.
func New(s Store, c clock.Clock, logger *slog.Logger) *Handler {
	return &Handler{store: s, clock: c, logger: logger}
}

// Register wires this package's routes onto a router. The route keys must
// match infra/envs/dev/main.tf's device_public_routes exactly — API Gateway
// passes its own routeKey through, and a mismatch surfaces only after
// deploy as ROUTE_NOT_WIRED.
func (h *Handler) Register(rt *httpx.Router) {
	rt.Handle("POST /device/pair", h.Pair)
	rt.Handle("POST /device/token", h.Token)
}

// PairRequest is POST /device/pair's body (docs/06-auth.md §3).
type PairRequest struct {
	PairingCode string     `json:"pairingCode"`
	DeviceInfo  DeviceInfo `json:"deviceInfo"`
}

// DeviceInfo is what the device reports about itself. This is the only
// point in the system where it is supplied, so it is stored as given and
// treated as display-only data — nothing authorizes on it.
type DeviceInfo struct {
	Model      string `json:"model"`
	OSVersion  string `json:"osVersion"`
	AppVersion string `json:"appVersion"`
}

// PairResponse carries the deviceSecret. This is the one and only response
// in the entire system that contains it in plaintext: the server keeps only
// its SHA-256 (docs/06-auth.md §3), so a device that loses it must pair
// again with a fresh QR.
type PairResponse struct {
	DeviceID     string `json:"deviceId"`
	DeviceSecret string `json:"deviceSecret"`
}

// Pair completes the QR handshake of docs/06-auth.md §2: resolve the code
// through GSI2, mint a deviceSecret, and consume the session and promote
// the device to PAIRED in one transaction.
//
// The three failure codes are deliberately distinguishable because the
// device acts differently on each (docs/04-native.md §1.4): GSI2 is
// eventually consistent, so a freshly issued code can legitimately miss,
// and 404 PAIRING_NOT_FOUND is the ONLY case the device retries. Collapsing
// a consumed or expired code into 404 would have it retry a QR that can
// never work; collapsing a genuine miss into 409 would abandon a pairing
// that would have succeeded a second later.
func (h *Handler) Pair(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	var body PairRequest
	if err := httpx.DecodeJSON(req, &body); err != nil {
		return httpx.Response{}, err
	}
	code := strings.TrimSpace(body.PairingCode)
	if code == "" {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "pairingCode は必須です")
	}

	// Resolve the code to a device. This read cannot decide whether the
	// code is still usable — see below — it only tells us which device row
	// the transaction must target.
	session, err := h.store.FindPairingByCode(ctx, code)
	if err != nil {
		return httpx.Response{}, err
	}

	secret, err := ids.NewDeviceSecret()
	if err != nil {
		return httpx.Response{}, apierr.Wrap(apierr.CodeInternal, "internal error", err)
	}

	// Status and expiry are checked ONLY by the transaction's condition, not
	// here. Re-deriving them from the read above would reopen the race the
	// condition exists to close: two devices scanning the same QR would both
	// read PENDING and both be told they succeeded (docs/06-auth.md §2,
	// "使い捨ての担保").
	err = h.store.ConsumePairing(ctx, store.ConsumePairingInput{
		DeviceID:         session.DeviceID,
		PairingCode:      code,
		DeviceSecretHash: tokens.HashDeviceSecret(secret),
		DeviceInfo: store.DeviceInfo{
			Model:      body.DeviceInfo.Model,
			OSVersion:  body.DeviceInfo.OSVersion,
			AppVersion: body.DeviceInfo.AppVersion,
		},
		Now: h.clock.Now(),
	})
	if err != nil {
		h.log(ctx, "deviceauth: pairing failed", "deviceId", session.DeviceID, "error", err.Error())
		return httpx.Response{}, err
	}

	h.log(ctx, "deviceauth: paired", "deviceId", session.DeviceID, "ownerId", session.OwnerID)
	return httpx.Response{Body: PairResponse{DeviceID: session.DeviceID, DeviceSecret: secret}}, nil
}

// TokenRequest is POST /device/token's body.
type TokenRequest struct {
	DeviceID     string `json:"deviceId"`
	DeviceSecret string `json:"deviceSecret"`
}

// TokenResponse is the issued session (docs/06-auth.md §3).
type TokenResponse struct {
	SessionToken string `json:"sessionToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// Token exchanges a deviceSecret for a 12-hour session.
//
// Every rejection here is 401, never 403 or 404. docs/06-auth.md §5 makes
// 401 the device's entire self-repair vocabulary — it wipes its credentials,
// stops the foreground service and returns to the QR screen — so any other
// status strands a device that can never recover on its own. It also means
// an unauthenticated caller cannot tell "no such device" from "wrong
// secret", so deviceIds are not enumerable through this route.
func (h *Handler) Token(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	var body TokenRequest
	if err := httpx.DecodeJSON(req, &body); err != nil {
		return httpx.Response{}, err
	}
	if body.DeviceID == "" {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "deviceId は必須です")
	}
	if body.DeviceSecret == "" {
		return httpx.Response{}, apierr.New(apierr.CodeValidation, "deviceSecret は必須です")
	}

	dev, err := h.store.GetDeviceForAuth(ctx, body.DeviceID)
	if err != nil {
		// A missing device is not a 404 on this route — see the doc comment.
		// Any other store failure is a genuine fault and keeps its own code
		// so it is not silently reported to the device as "your credentials
		// are dead", which would make it wipe them over a transient outage.
		if isCode(err, apierr.CodeDeviceNotFound) {
			return httpx.Response{}, h.denied(ctx, "unknown_device", body.DeviceID)
		}
		return httpx.Response{}, err
	}
	if dev.Status != store.DeviceStatusPaired {
		return httpx.Response{}, h.denied(ctx, "not_paired", body.DeviceID)
	}
	if !validSecret(body.DeviceSecret, dev.DeviceSecretHash) {
		return httpx.Response{}, h.denied(ctx, "secret_mismatch", body.DeviceID)
	}

	sessionToken, err := ids.NewSessionToken(dev.DeviceID)
	if err != nil {
		return httpx.Response{}, apierr.Wrap(apierr.CodeInternal, "internal error", err)
	}
	expiresAt := h.clock.Now().Add(SessionTTL).Unix()

	// The store's condition re-checks PAIRED, closing the window between the
	// read above and this write in which the owner may have disconnected the
	// device. It reports that as CodeUnauthorized, which is already the
	// right answer, so it propagates untouched.
	if _, err := h.store.RotateSessionToken(ctx, dev.DeviceID, tokens.HashSessionToken(sessionToken), expiresAt); err != nil {
		return httpx.Response{}, err
	}

	h.log(ctx, "deviceauth: session issued", "deviceId", dev.DeviceID, "ownerId", dev.OwnerID, "expiresAt", expiresAt)
	return httpx.Response{Body: TokenResponse{SessionToken: sessionToken, ExpiresAt: expiresAt}}, nil
}

// validSecret compares a presented secret against the stored digest.
//
// The empty-hash guard is the disconnect path made explicit: DisconnectDevice
// wipes deviceSecretHash, and a device still holding its old secret must be
// refused. subtle.ConstantTimeCompare would already refuse on the length
// mismatch, but a caller presenting an empty secret against an empty hash is
// exactly the input where an "obvious" future simplification to == would
// start returning true.
func validSecret(secret, storedHash string) bool {
	if storedHash == "" || secret == "" {
		return false
	}
	got := tokens.HashDeviceSecret(secret)
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}

// denied logs why a token request failed and returns the single 401 that all
// of them share. The reason stays in the log — where it is needed to tell a
// disconnected device from a wrong secret while investigating — and out of
// the response, where it would leak which deviceIds exist.
func (h *Handler) denied(ctx context.Context, reason, deviceID string) error {
	h.log(ctx, "deviceauth: token denied", "reason", reason, "deviceId", deviceID)
	return apierr.New(apierr.CodeUnauthorized, "資格情報が無効です")
}

func isCode(err error, code apierr.Code) bool {
	var apiErr *apierr.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func (h *Handler) log(ctx context.Context, msg string, args ...any) {
	if h.logger == nil {
		return
	}
	h.logger.InfoContext(ctx, msg, args...)
}

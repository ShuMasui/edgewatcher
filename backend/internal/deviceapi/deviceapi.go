// Package deviceapi implements the two routes a paired device calls behind
// the Lambda authorizer: POST /device/uploads and POST /device/logout
// (docs/05-backend.md §1.1).
//
// The authorizer has already established which device is calling and put
// its deviceId/ownerId in the request context (§3.2). That is the single
// source of identity here: nothing in the payload is trusted, because a
// paired device is authenticated but not thereby authorized to write into
// another device's history.
package deviceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/images"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

// MaxUploadBytes is the ceiling on image + thumbnail combined.
//
// docs/05-backend.md §1.3 derives it: API Gateway allows 10MB, but Lambda's
// synchronous invocation payload caps at 6MB and the gateway base64-encodes
// the body on the way in (~1.33x), leaving roughly 4.5MB of actual image.
// The gateway rejects anything past its own limit with a 413 before we run;
// this check covers the band between that and what Lambda can actually
// carry, and makes the limit testable without deploying.
const MaxUploadBytes = 4_500_000

// Part names in the multipart body (docs/05-backend.md §1.3).
const (
	partImage     = "image"
	partThumbnail = "thumbnail"
	partMetadata  = "metadata"
)

// UploadStore is everything POST /device/uploads may do to DynamoDB.
//
// It has no read method, and that absence is the point: device-api's
// execution role grants PutItem and UpdateItem only (docs/05-backend.md
// §2.4, G3), so intervalMinutes in the response has to come out of
// TouchDeviceLatest's ALL_NEW return rather than a GetItem. Enforcing that
// through the interface makes it a compile error rather than a runtime
// AccessDenied — and stronger than a test that counts calls, since there is
// no read method to call.
type UploadStore interface {
	PutObservation(ctx context.Context, obs store.Observation) error
	TouchDeviceLatest(ctx context.Context, in store.TouchDeviceLatestInput) (*store.Device, error)
}

// LogoutStore is everything POST /device/logout may do.
type LogoutStore interface {
	DisconnectDevice(ctx context.Context, deviceID string) (*store.Device, error)
}

// Store is the union the constructed handler holds. It is assembled from
// the two per-route interfaces rather than declared flat so that each
// route's ceiling stays legible on its own.
type Store interface {
	UploadStore
	LogoutStore
}

// ImageUploader is the S3 half: exactly one method, matching the single S3
// action device-api's role grants.
type ImageUploader interface {
	PutJPEG(ctx context.Context, key string, body []byte) error
}

// Handler serves the authorized device routes.
type Handler struct {
	store         Store
	uploader      ImageUploader
	clock         clock.Clock
	retentionDays int
	logger        *slog.Logger
}

// New constructs a Handler. retentionDays is RETENTION_DAYS, used to set
// each observation's TTL.
func New(s Store, u ImageUploader, c clock.Clock, retentionDays int, logger *slog.Logger) *Handler {
	return &Handler{store: s, uploader: u, clock: c, retentionDays: retentionDays, logger: logger}
}

// Register wires this package's routes. The keys must match
// infra/envs/dev/main.tf's device_routes exactly.
func (h *Handler) Register(rt *httpx.Router) {
	rt.Handle("POST /device/uploads", h.Upload)
	rt.Handle("POST /device/logout", h.Logout)
}

// NextConfig is the pull-style configuration a device applies after an
// upload (docs/05-backend.md §3.4). The server never pushes: this is the
// only channel by which an interval change reaches a device.
type NextConfig struct {
	IntervalMinutes int `json:"intervalMinutes"`
}

// UploadResponse is POST /device/uploads' body.
type UploadResponse struct {
	NextConfig NextConfig `json:"nextConfig"`
}

// uploadMetadata is the JSON part of the multipart body.
//
// It deliberately has no deviceId field. The device does send one in some
// builds, and a struct that accepted it would invite a later edit to use it;
// leaving it undeclared means the identity can only come from the
// authorizer.
type uploadMetadata struct {
	ObservationID string   `json:"observationId"`
	CapturedAt    string   `json:"capturedAt"`
	Lat           *float64 `json:"lat"`
	Lng           *float64 `json:"lng"`
}

// Upload receives one observation: the full image, its thumbnail and the
// metadata, in a single request (docs/03-web.md §3.7, docs/05-backend.md
// §1.3).
//
// The write order is S3 first, DynamoDB second, and it is not arbitrary
// (§1.3). Either order can fail in the middle; this one fails toward an
// orphaned object, which the bucket's lifecycle rule collects on its own.
// The reverse fails toward a record whose signed URL 404s, which nothing
// cleans up and which the web surfaces as a broken image.
//
// Every rejection below is a 400 rather than a 500, because none of them
// can be fixed by retrying: a device that receives a 5xx backs off and
// retries the same frame forever (§1.6), while a 400 tells it to drop the
// frame and move on.
func (h *Handler) Upload(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	identity, err := httpx.ExtractDeviceIdentity(req)
	if err != nil {
		return httpx.Response{}, err
	}

	parts, err := parseMultipart(req)
	if err != nil {
		return httpx.Response{}, err
	}

	image, thumbnail, meta, err := validateParts(parts)
	if err != nil {
		return httpx.Response{}, err
	}

	capturedAt, err := time.Parse(time.RFC3339, meta.CapturedAt)
	if err != nil {
		return httpx.Response{}, apierr.Wrap(apierr.CodeValidation, "capturedAt が ISO 8601 ではありません", err)
	}

	// Keys are derived from the AUTHORIZED deviceId, never from anything in
	// the payload, so a device cannot write into another device's prefix.
	imageKey, thumbnailKey := images.ObservationKeys(identity.DeviceID, meta.ObservationID, capturedAt)

	if err := h.uploader.PutJPEG(ctx, imageKey, image); err != nil {
		return httpx.Response{}, err
	}
	if err := h.uploader.PutJPEG(ctx, thumbnailKey, thumbnail); err != nil {
		return httpx.Response{}, err
	}

	// PutObservation treats a replayed observationId as success without
	// writing a second row (§2.6), so a device that never saw our response
	// can retry safely. Re-putting the same S3 keys above is idempotent for
	// the same reason: the key is derived from the device-assigned id.
	obs := store.NewObservation(store.NewObservationInput{
		DeviceID:      identity.DeviceID,
		ObservationID: meta.ObservationID,
		CapturedAt:    capturedAt,
		ImageKey:      imageKey,
		ThumbnailKey:  thumbnailKey,
		Lat:           meta.Lat,
		Lng:           meta.Lng,
		RetentionDays: h.retentionDays,
	})
	if err := h.store.PutObservation(ctx, obs); err != nil {
		return httpx.Response{}, err
	}

	// Two clocks, deliberately (§3.3): the capture time is the device's and
	// gates the "don't rewind the dashboard to an older photo" condition,
	// while the receipt time is ours and drives the 応答なし display.
	dev, err := h.store.TouchDeviceLatest(ctx, store.TouchDeviceLatestInput{
		DeviceID:           identity.DeviceID,
		ReceivedAt:         h.clock.Now(),
		LatestCapturedAt:   capturedAt,
		LatestThumbnailKey: thumbnailKey,
	})
	if err != nil {
		// The interval is only knowable from this call's ALL_NEW return, and
		// substituting a default would silently override the interval the
		// owner chose.
		return httpx.Response{}, err
	}

	h.log(ctx, "deviceapi: observation stored",
		"deviceId", identity.DeviceID, "ownerId", identity.OwnerID, "observationId", meta.ObservationID)

	return httpx.Response{Body: UploadResponse{NextConfig: NextConfig{IntervalMinutes: dev.Interval}}}, nil
}

// Logout drops both credentials server-side and marks the device
// DISCONNECTED (docs/04-native.md §1.3).
//
// The device it logs out is the authorized one, never one named in the
// body: otherwise any paired device could disconnect any other.
//
// 204 with no body, per docs/05-backend.md §1.2.
func (h *Handler) Logout(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
	identity, err := httpx.ExtractDeviceIdentity(req)
	if err != nil {
		return httpx.Response{}, err
	}
	if _, err := h.store.DisconnectDevice(ctx, identity.DeviceID); err != nil {
		return httpx.Response{}, err
	}
	h.log(ctx, "deviceapi: device logged out", "deviceId", identity.DeviceID, "ownerId", identity.OwnerID)
	return httpx.Response{StatusCode: http.StatusNoContent}, nil
}

// parseMultipart reads the request body into its named parts.
//
// The body goes through httpx.RawBody because API Gateway base64-encodes
// multipart payloads; parsing req.Body directly would hand the reader
// base64 text and fail with a misleading boundary error.
func parseMultipart(req events.APIGatewayV2HTTPRequest) (map[string][]byte, error) {
	contentType := headerValue(req.Headers, "content-type")
	if contentType == "" {
		return nil, apierr.New(apierr.CodeValidation, "Content-Type がありません")
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return nil, apierr.Wrap(apierr.CodeValidation, "multipart/form-data で送信してください", err)
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, apierr.New(apierr.CodeValidation, "multipart の boundary がありません")
	}

	raw, err := httpx.RawBody(req)
	if err != nil {
		return nil, err
	}

	parts := make(map[string][]byte, 3)
	reader := multipart.NewReader(bytes.NewReader(raw), boundary)
	total := 0
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, apierr.Wrap(apierr.CodeValidation, "リクエスト本文を解析できません", err)
		}

		// Bounded by MaxUploadBytes+1 so the read itself reveals an
		// oversized part rather than buffering it first: the limit exists
		// to keep a 1024MB function from being handed more than it can
		// return, so discovering the excess after materializing it would
		// defeat the check.
		body, err := io.ReadAll(io.LimitReader(part, int64(MaxUploadBytes)+1))
		_ = part.Close()
		if err != nil {
			return nil, apierr.Wrap(apierr.CodeValidation, "リクエスト本文を読み取れません", err)
		}
		total += len(body)
		if total > MaxUploadBytes {
			return nil, apierr.New(apierr.CodeValidation, "画像のサイズが上限を超えています")
		}
		parts[part.FormName()] = body
	}
	return parts, nil
}

// validateParts checks that all three parts are present and usable.
//
// Each absence is caught here rather than downstream because the failures
// downstream are all silent: a missing thumbnail would leave the device row
// pointing at an object that was never written, and an empty image part
// would store a zero-byte "JPEG" that no retry can replace, since the
// observationId is already consumed.
func validateParts(parts map[string][]byte) (image, thumbnail []byte, meta uploadMetadata, err error) {
	image, ok := parts[partImage]
	if !ok || len(image) == 0 {
		return nil, nil, meta, apierr.New(apierr.CodeValidation, "image パートがありません")
	}
	thumbnail, ok = parts[partThumbnail]
	if !ok || len(thumbnail) == 0 {
		return nil, nil, meta, apierr.New(apierr.CodeValidation, "thumbnail パートがありません")
	}
	rawMeta, ok := parts[partMetadata]
	if !ok || len(rawMeta) == 0 {
		return nil, nil, meta, apierr.New(apierr.CodeValidation, "metadata パートがありません")
	}
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		return nil, nil, meta, apierr.Wrap(apierr.CodeValidation, "metadata の形式が不正です", err)
	}
	if meta.ObservationID == "" {
		return nil, nil, meta, apierr.New(apierr.CodeValidation, "metadata.observationId は必須です")
	}
	if meta.CapturedAt == "" {
		return nil, nil, meta, apierr.New(apierr.CodeValidation, "metadata.capturedAt は必須です")
	}
	return image, thumbnail, meta, nil
}

// headerValue looks a header up case-insensitively. API Gateway lowercases
// header names on the HTTP API payload, but a test or a future payload
// version need not, and the cost of not depending on that is two lines.
func headerValue(headers map[string]string, name string) string {
	if v, ok := headers[name]; ok {
		return v
	}
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func (h *Handler) log(ctx context.Context, msg string, args ...any) {
	if h.logger == nil {
		return
	}
	h.logger.InfoContext(ctx, msg, args...)
}

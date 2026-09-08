// Package api holds the wire types the web client actually receives, and
// the mappers from internal/store's DynamoDB items to them.
//
// The layer exists for two reasons that are easy to lose sight of:
//
//  1. store.Device carries deviceSecretHash and sessionTokenHash. Serializing
//     it directly would put credential material on the wire the moment
//     someone adds a handler that returns a device, and no compiler on
//     either side would object.
//  2. web/src/types/domain.ts is the binding contract and it is TypeScript.
//     Nothing type-checks Go structs against it, so the field names live
//     here in one place with golden tests over the serialized bytes, rather
//     than being reconstructed ad hoc in each handler.
package api

import (
	"context"
	"time"

	"github.com/ShuMasui/edgewatcher/backend/internal/images"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

// IntervalOptions is the set of upload intervals (minutes) the web offers
// and PATCH /devices/{id} accepts (docs/03-web.md). GET /app-config returns
// it so the client's <select> and the server's validation cannot drift.
var IntervalOptions = []int{5, 10, 15}

// AppConfig is GET /app-config's body (domain.ts: AppConfig).
type AppConfig struct {
	RetentionDays   int   `json:"retentionDays"`
	DeviceLimit     int   `json:"deviceLimit"`
	IntervalOptions []int `json:"intervalOptions"`
}

// PairingSession is the QR-side view of a pairing session (domain.ts:
// PairingSession).
//
// It carries pairingCode because the web draws the QR from it — this is the
// one response where the code is deliberately exposed, to the owner who
// created it. It must never be logged (docs/05-backend.md §2.5).
type PairingSession struct {
	PairingCode string `json:"pairingCode"`
	ExpiresAt   int64  `json:"expiresAt"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt,omitempty"`
	ConsumedAt  string `json:"consumedAt,omitempty"`
}

// Device is the web's view of a device (domain.ts: Device).
//
// ActivePairingSession is a pointer WITHOUT omitempty, so it always
// serializes — as an object or as an explicit null. Every other optional
// field is omitted when absent. The asymmetry is deliberate: the dashboard
// switches a device between "ペアリング待ち" and "接続中" on this field, and
// null (the QR is gone) has to be distinguishable from "the server didn't
// say", which is what an omitted field means to a client that merges
// responses into cached state.
type Device struct {
	DeviceID           string `json:"deviceId"`
	OwnerID            string `json:"ownerId"`
	Name               string `json:"name"`
	Status             string `json:"status"`
	Interval           int    `json:"interval"`
	LastReceivedAt     string `json:"lastReceivedAt,omitempty"`
	LatestThumbnailURL string `json:"latestThumbnailUrl,omitempty"`
	LatestCapturedAt   string `json:"latestCapturedAt,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	ArchivedAt         string `json:"archivedAt,omitempty"`

	ActivePairingSession *PairingSession `json:"activePairingSession"`
}

// Observation is one photo as the web sees it (domain.ts: Observation).
//
// ImageURL is populated only by GET /observations/{id}/image. A list
// response leaves it empty and omitempty drops it: signing a full-size URL
// for every observation in a day would hand the browser a page of live
// credentials for images it will mostly never open (docs/05-backend.md
// §1.5).
type Observation struct {
	ObservationID string   `json:"observationId"`
	DeviceID      string   `json:"deviceId"`
	CapturedAt    string   `json:"capturedAt"`
	ThumbnailURL  string   `json:"thumbnailUrl"`
	ImageURL      string   `json:"imageUrl,omitempty"`
	Lat           *float64 `json:"lat,omitempty"`
	Lng           *float64 `json:"lng,omitempty"`
	ExpiresAt     int64    `json:"expiresAt,omitempty"`
}

// URLSigner is the mapper's view of images.Signer: turning an S3 key into a
// URL the browser may fetch. Narrowed to an interface so this package's
// tests need no AWS types and so device-api — which has no signing
// permission at all — cannot accidentally acquire one through this package.
type URLSigner interface {
	SignGetObject(ctx context.Context, key string, now time.Time) (images.SignedURL, error)
}

// Mapper converts store items into wire types, signing image keys as it
// goes.
type Mapper struct {
	signer URLSigner
}

// NewMapper constructs a Mapper over a signer.
func NewMapper(signer URLSigner) *Mapper {
	return &Mapper{signer: signer}
}

// ActivePairingSession reports the session the web should render a QR for,
// or nil.
//
// A session qualifies only while it could still actually be redeemed:
// PENDING, and strictly before expiresAt. The boundary is `>` rather than
// `>=` to match store.ConsumePairing's `expiresAt > :nowEpoch` condition
// exactly — at the boundary instant the server already refuses the code, so
// showing it would be a QR that scans and then fails.
//
// Both rejections collapse to nil rather than to distinct states because
// the web has one thing to do with either: stop showing the QR and offer
// re-issuing it.
func ActivePairingSession(session *store.PairingSession, now time.Time) *PairingSession {
	if session == nil {
		return nil
	}
	if session.Status != store.PairingStatusPending {
		return nil
	}
	if session.ExpiresAt <= now.Unix() {
		return nil
	}
	dto := PairingSessionDTO(*session)
	return &dto
}

// PairingSessionDTO converts a stored session to its wire form. It performs
// no expiry or status filtering — POST /devices and POST
// /devices/{id}/pairing-sessions return the session they just created, which
// is PENDING by construction, while GET /devices decides visibility through
// ActivePairingSession.
func PairingSessionDTO(s store.PairingSession) PairingSession {
	return PairingSession{
		PairingCode: s.PairingCode,
		ExpiresAt:   s.ExpiresAt,
		Status:      s.Status,
		CreatedAt:   s.CreatedAt,
		ConsumedAt:  s.ConsumedAt,
	}
}

// Device converts a full store.Device (a base-table read) to its wire form,
// signing latestThumbnailKey if there is one.
//
// session is the caller's already-resolved active session, or nil. It is a
// parameter rather than something this mapper fetches because the two
// callers get it from different places: GET /devices has it from the same
// GSI1 query, while POST /devices has it from the transaction it just ran.
func (m *Mapper) Device(ctx context.Context, dev store.Device, session *PairingSession, now time.Time) (Device, error) {
	thumbURL, err := m.signThumbnail(ctx, dev.LatestThumbnailKey, now)
	if err != nil {
		return Device{}, err
	}
	return Device{
		DeviceID:             dev.DeviceID,
		OwnerID:              dev.OwnerID,
		Name:                 dev.Name,
		Status:               dev.Status,
		Interval:             dev.Interval,
		LastReceivedAt:       dev.LastReceivedAt,
		LatestThumbnailURL:   thumbURL,
		LatestCapturedAt:     dev.LatestCapturedAt,
		CreatedAt:            dev.CreatedAt,
		ArchivedAt:           dev.ArchivedAt,
		ActivePairingSession: session,
	}, nil
}

// DeviceFromRow converts a GSI1 row (access pattern 1) to its wire form.
//
// It exists separately from Device because a GSI1 row is not a Device and
// cannot be treated as one: docs/engineering/dynamodb.md §4's INCLUDE
// projection omits ownerId, so it is recovered from GSI1PK via
// store.OwnerListRow.OwnerID. createdAt is not projected either and is
// simply absent here — legal, since domain.ts marks it optional.
func (m *Mapper) DeviceFromRow(ctx context.Context, row store.OwnerListRow, session *PairingSession, now time.Time) (Device, error) {
	thumbURL, err := m.signThumbnail(ctx, row.LatestThumbnailKey, now)
	if err != nil {
		return Device{}, err
	}
	return Device{
		DeviceID:             row.DeviceID,
		OwnerID:              row.OwnerID(),
		Name:                 row.Name,
		Status:               row.Status,
		Interval:             row.Interval,
		LastReceivedAt:       row.LastReceivedAt,
		LatestThumbnailURL:   thumbURL,
		LatestCapturedAt:     row.LatestCapturedAt,
		ActivePairingSession: session,
	}, nil
}

// Observations converts a day's observations, signing each thumbnail.
//
// The slice is always non-nil so an empty day marshals as [] rather than
// null; the web maps over it without a guard.
func (m *Mapper) Observations(ctx context.Context, obs []store.Observation, now time.Time) ([]Observation, error) {
	out := make([]Observation, 0, len(obs))
	for _, o := range obs {
		thumbURL, err := m.signThumbnail(ctx, o.ThumbnailKey, now)
		if err != nil {
			return nil, err
		}
		out = append(out, Observation{
			ObservationID: o.ObservationID,
			DeviceID:      o.DeviceID,
			CapturedAt:    o.CapturedAt,
			ThumbnailURL:  thumbURL,
			Lat:           o.Lat,
			Lng:           o.Lng,
			ExpiresAt:     o.ExpiresAt,
		})
	}
	return out, nil
}

// SignImage issues the full-size URL for GET /observations/{id}/image,
// returning the expiry alongside it because that response reports both
// (api.ts: GetObservationImageResponse).
func (m *Mapper) SignImage(ctx context.Context, key string, now time.Time) (images.SignedURL, error) {
	return m.signer.SignGetObject(ctx, key, now)
}

// signThumbnail signs key, or returns "" when there is no key.
//
// The empty-key short circuit is not just an optimization: presigning ""
// yields a perfectly valid URL to a nonexistent object, so a device that has
// never uploaded would get a latestThumbnailUrl that renders as a broken
// image instead of the dashboard's "no image yet" placeholder.
func (m *Mapper) signThumbnail(ctx context.Context, key string, now time.Time) (string, error) {
	if key == "" {
		return "", nil
	}
	signed, err := m.signer.SignGetObject(ctx, key, now)
	if err != nil {
		return "", err
	}
	return signed.URL, nil
}

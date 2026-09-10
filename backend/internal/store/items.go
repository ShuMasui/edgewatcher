package store

import "strings"

// Device.Status values (docs/engineering/dynamodb.md §3, docs/06-auth.md
// §6). Task 7's write methods are the only place these transitions happen;
// no other package should hardcode these strings.
const (
	DeviceStatusPending      = "PENDING"
	DeviceStatusPaired       = "PAIRED"
	DeviceStatusDisconnected = "DISCONNECTED"
	DeviceStatusArchived     = "ARCHIVED"
)

// PairingSession.Status values (docs/engineering/dynamodb.md §3).
const (
	PairingStatusPending  = "PENDING"
	PairingStatusConsumed = "CONSUMED"
)

// Device is the full base-table item for a device (docs/engineering/dynamodb.md
// §3). PK == SK == "DEVICE#<deviceId>" so the authorizer can GetItem from
// deviceId alone.
type Device struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	EntityType string `dynamodbav:"entityType"`

	DeviceID string `dynamodbav:"deviceId"`
	OwnerID  string `dynamodbav:"ownerId"`
	Name     string `dynamodbav:"name"`
	Status   string `dynamodbav:"status"`
	Interval int    `dynamodbav:"interval"`

	// DeviceSecretHash and SessionTokenHash are removed on disconnect
	// (docs/06-auth.md §6), so both are optional. json:"-" on both: these
	// are SHA-256 hashes of long-lived credentials, returned to this
	// package's callers via ALL_NEW on several write paths, and must never
	// be reachable by an accidental json.Marshal(dev) in a future handler
	// (G7 — never leak credentials/pairingCode/signed URLs).
	DeviceSecretHash string `dynamodbav:"deviceSecretHash,omitempty" json:"-"`
	SessionTokenHash string `dynamodbav:"sessionTokenHash,omitempty" json:"-"`
	// SessionExpiresAt is epoch seconds. It is a session deadline, not the
	// table's TTL attribute — see doc.go.
	SessionExpiresAt int64 `dynamodbav:"sessionExpiresAt,omitempty"`

	LastReceivedAt     string `dynamodbav:"lastReceivedAt,omitempty"`
	LatestThumbnailKey string `dynamodbav:"latestThumbnailKey,omitempty"`
	LatestCapturedAt   string `dynamodbav:"latestCapturedAt,omitempty"`

	CreatedAt  string `dynamodbav:"createdAt,omitempty"`
	ArchivedAt string `dynamodbav:"archivedAt,omitempty"`

	// GSI1PK/GSI1SK are absent only in the theoretical case of a Device
	// item written without them; in practice every Device row carries
	// them (either OWNER#<ownerId> or, once ARCHIVED, the "ARCHIVED"
	// partition — Task 7's ArchiveDevice).
	GSI1PK string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK string `dynamodbav:"GSI1SK,omitempty"`

	// DeviceInfo is the pairing device's self-reported client details
	// (docs/06-auth.md §2's { model, osVersion, appVersion }), written by
	// Task 7's ConsumePairing. Note: docs/engineering/dynamodb.md §3's
	// Device attribute table does not enumerate this field — a documented
	// gap in that table — but §6's pairing transaction and the task
	// brief's expression B both name it explicitly, so it is treated as
	// authoritative here.
	DeviceInfo *DeviceInfo `dynamodbav:"deviceInfo,omitempty"`
}

// DeviceInfo is the nested map attribute described above.
type DeviceInfo struct {
	Model      string `dynamodbav:"model,omitempty"`
	OSVersion  string `dynamodbav:"osVersion,omitempty"`
	AppVersion string `dynamodbav:"appVersion,omitempty"`
}

// PairingSession is the full base-table item for a device's pairing session
// (docs/engineering/dynamodb.md §3). PK = "DEVICE#<deviceId>",
// SK = "PAIRING#<pairingCode>".
type PairingSession struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	EntityType string `dynamodbav:"entityType"`

	DeviceID    string `dynamodbav:"deviceId"`
	OwnerID     string `dynamodbav:"ownerId"`
	PairingCode string `dynamodbav:"pairingCode"`
	Status      string `dynamodbav:"status"`
	// ExpiresAt is epoch seconds, and doubles as this item's TTL value.
	// Never trust it alone to decide expiry — see doc.go.
	ExpiresAt int64 `dynamodbav:"expiresAt"`

	CreatedAt  string `dynamodbav:"createdAt,omitempty"`
	ConsumedAt string `dynamodbav:"consumedAt,omitempty"`

	GSI1PK string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK string `dynamodbav:"GSI1SK,omitempty"`
	GSI2PK string `dynamodbav:"GSI2PK,omitempty"`
	GSI2SK string `dynamodbav:"GSI2SK,omitempty"`
}

// Observation is the full base-table item for one captured image
// (docs/engineering/dynamodb.md §3). PK = "DEVICE#<deviceId>",
// SK = "OBS#<observationId>". Observations are never indexed by a GSI — the
// highest-volume item type, so indexing it would double storage and write
// cost.
type Observation struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	EntityType string `dynamodbav:"entityType"`

	ObservationID string `dynamodbav:"observationId"`
	DeviceID      string `dynamodbav:"deviceId"`
	CapturedAt    string `dynamodbav:"capturedAt"`
	ImageKey      string `dynamodbav:"imageKey"`
	ThumbnailKey  string `dynamodbav:"thumbnailKey"`

	Lat *float64 `dynamodbav:"lat,omitempty"`
	Lng *float64 `dynamodbav:"lng,omitempty"`

	// ExpiresAt is epoch seconds: capturedAt + retention_days. It drives
	// the table's TTL sweep, but reads must still bound their own query by
	// the requested day (docs/engineering/dynamodb.md §5.2) rather than
	// trust TTL to have removed anything expired.
	ExpiresAt int64 `dynamodbav:"expiresAt"`
}

// OwnerListRow is one row of a GSI1 query keyed by GSI1PK = "OWNER#<ownerId>"
// (access pattern 1). GSI1's projection is INCLUDE and deliberately does not
// carry ownerId (docs/engineering/dynamodb.md §4) — unmarshalling a GSI1 row
// into a full Device would silently produce OwnerID == "", which is wrong in
// a way nothing would catch. OwnerListRow exists so that mistake cannot be
// made: there is no OwnerID field to leave zero, only a method that derives
// it from GSI1PK.
//
// A single query mixes two item kinds — Device rows (GSI1SK begins with
// "DEVICE#") and PairingSession rows (GSI1SK begins with "PAIRING#") — sorted
// together by GSI1SK, which puts all Device rows before all PairingSession
// rows. IsDevice/IsPairingSession tell them apart; fields that don't apply
// to a given kind are simply zero on that row.
type OwnerListRow struct {
	GSI1PK string `dynamodbav:"GSI1PK"`
	GSI1SK string `dynamodbav:"GSI1SK"`

	DeviceID string `dynamodbav:"deviceId"`

	// Device-only fields.
	Name               string `dynamodbav:"name,omitempty"`
	Interval           int    `dynamodbav:"interval,omitempty"`
	LastReceivedAt     string `dynamodbav:"lastReceivedAt,omitempty"`
	LatestThumbnailKey string `dynamodbav:"latestThumbnailKey,omitempty"`
	LatestCapturedAt   string `dynamodbav:"latestCapturedAt,omitempty"`

	// Shared / PairingSession-only fields.
	Status      string `dynamodbav:"status,omitempty"`
	PairingCode string `dynamodbav:"pairingCode,omitempty"`
	ExpiresAt   int64  `dynamodbav:"expiresAt,omitempty"`

	// createdAt is not in this GSI's projection list at all, so it is
	// simply unavailable on this path — legal per web/src/types/domain.ts,
	// which marks Device.createdAt optional.
}

// OwnerID recovers the owning Cognito sub from GSI1PK. It is never read as
// a projected attribute because GSI1 does not project ownerId.
func (r OwnerListRow) OwnerID() string {
	return ownerIDFromGSI1PK(r.GSI1PK)
}

// IsDevice reports whether this row is a Device rather than a
// PairingSession.
func (r OwnerListRow) IsDevice() bool {
	return strings.HasPrefix(r.GSI1SK, devicePrefix)
}

// IsPairingSession reports whether this row is a PairingSession rather than
// a Device.
func (r OwnerListRow) IsPairingSession() bool {
	return strings.HasPrefix(r.GSI1SK, pairingPrefix)
}

package store

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
	"github.com/ShuMasui/edgewatcher/backend/internal/ids"
)

// QueryObservationsByDay implements access pattern 3
// (docs/engineering/dynamodb.md §5.2): one device's observation history for
// one JST calendar day, newest first. dateStr must be "YYYY-MM-DD"
// (web/src/utils/date.ts's formatDateParam shape); internal/ids.DayRange
// computes the inclusive SK bounds so the day boundary math lives in exactly
// one place.
//
// The time range is always part of the KeyConditionExpression, never left
// to a full-partition read: TTL deletion lags up to 48 hours, so a
// partition scan could return observations past their retention window
// (docs/engineering/dynamodb.md §5.2, §7).
//
// ScanIndexForward is false so ULID sort order (== capture-time order,
// since a ULID's leading bits are a millisecond timestamp) comes back
// descending without a client-side sort.
func (s *Store) QueryObservationsByDay(ctx context.Context, deviceID, dateStr string) ([]Observation, error) {
	lower, upper, err := ids.DayRange(dateStr)
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeValidation, "invalid date", err)
	}

	pk := deviceKey(deviceID)

	// No LastEvaluatedKey pagination loop here: docs/engineering/dynamodb.md
	// §5.2 bounds one JST day at 288 observations (5-minute interval), which
	// is far under a single Query page (1 MB). If the minimum interval or
	// the day-based history unit ever changes, revisit this.
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &s.table,
		KeyConditionExpression: strPtr("PK = :pk AND SK BETWEEN :lo AND :hi"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": stringAV(pk),
			":lo": stringAV(lower),
			":hi": stringAV(upper),
		},
		ScanIndexForward: boolPtr(false),
	})
	if err != nil {
		return nil, wrapInternal("QueryObservationsByDay", err)
	}

	obs := make([]Observation, 0, len(out.Items))
	for _, item := range out.Items {
		var o Observation
		if err := attributevalue.UnmarshalMap(item, &o); err != nil {
			return nil, wrapInternal("QueryObservationsByDay: unmarshal", err)
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// GetObservation implements access pattern 7
// (docs/engineering/dynamodb.md §5): fetching a single observation so
// web-api can issue a signed URL for its full-resolution image. Not found
// returns an *apierr.Error with CodeObservationNotFound.
func (s *Store) GetObservation(ctx context.Context, deviceID, observationID string) (*Observation, error) {
	pk := deviceKey(deviceID)
	sk := observationSK(observationID)

	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.table,
		Key:       map[string]types.AttributeValue{"PK": stringAV(pk), "SK": stringAV(sk)},
	})
	if err != nil {
		return nil, wrapInternal("GetObservation", err)
	}
	if out.Item == nil {
		return nil, apierr.New(apierr.CodeObservationNotFound, "observation not found")
	}

	var o Observation
	if err := attributevalue.UnmarshalMap(out.Item, &o); err != nil {
		return nil, wrapInternal("GetObservation: unmarshal", err)
	}
	return &o, nil
}

// PutObservation implements the observation-write half of POST
// /device/uploads (docs/05-backend.md §1.3, §2.6). It is idempotent by
// construction: observationId is a ULID the device itself generates and
// resends unchanged on retry (the device, not the server, owns the ID —
// otherwise a retry after a lost response would mint a new ID and record a
// duplicate). ConditionExpression: attribute_not_exists(SK) makes a
// duplicate Put a no-op.
//
// A ConditionalCheckFailedException here is SUCCESS, not an error: it means
// this exact observationId was already recorded — a replay of a request
// whose response the device never received — so PutObservation returns nil
// rather than propagating the SDK error. Any other error is genuinely
// unexpected and is wrapped as CodeInternal.
func (s *Store) PutObservation(ctx context.Context, obs Observation) error {
	item, err := attributevalue.MarshalMap(obs)
	if err != nil {
		return wrapInternal("PutObservation: marshal", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &s.table,
		Item:                item,
		ConditionExpression: strPtr("attribute_not_exists(SK)"),
	})
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil
		}
		return wrapInternal("PutObservation", err)
	}
	return nil
}

// NewObservationInput is the domain-level description of one captured
// image, as device-api receives it.
type NewObservationInput struct {
	DeviceID      string
	ObservationID string
	CapturedAt    time.Time
	ImageKey      string
	ThumbnailKey  string
	Lat           *float64
	Lng           *float64

	// RetentionDays is RETENTION_DAYS (dev 1 / prod 7). It sets the item's
	// TTL so DynamoDB's sweep and the image bucket's lifecycle rule expire
	// the metadata and the bytes together
	// (docs/engineering/dynamodb.md §7).
	RetentionDays int
}

// NewObservation builds the base-table item for one observation.
//
// It exists so that device-api — the only writer of observations — never
// constructs a "DEVICE#"/"OBS#" key itself. Key construction lives in
// keys.go and is unexported precisely so there is one place it can be
// wrong; a handler assembling PK by string concatenation would be a second
// place, and the two would diverge silently the first time a prefix
// changed.
//
// ExpiresAt is derived from capturedAt rather than from the receipt time.
// A device backfilling a week of offline photos (docs/04-native.md §3.5)
// must not be able to extend their retention past the window the bucket's
// lifecycle rule will delete their bytes in, which would leave records
// pointing at objects that no longer exist.
func NewObservation(in NewObservationInput) Observation {
	return Observation{
		PK:            deviceKey(in.DeviceID),
		SK:            observationSK(in.ObservationID),
		EntityType:    "Observation",
		ObservationID: in.ObservationID,
		DeviceID:      in.DeviceID,
		CapturedAt:    formatISO(in.CapturedAt),
		ImageKey:      in.ImageKey,
		ThumbnailKey:  in.ThumbnailKey,
		Lat:           in.Lat,
		Lng:           in.Lng,
		ExpiresAt:     in.CapturedAt.AddDate(0, 0, in.RetentionDays).Unix(),
	}
}

package store

import (
	"context"

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

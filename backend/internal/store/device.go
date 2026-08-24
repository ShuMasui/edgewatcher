package store

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// GetDeviceForAuth implements access pattern 4
// (docs/engineering/dynamodb.md §5): the Lambda Authorizer's single GetItem
// against PK = SK = "DEVICE#<deviceId>". It uses ConsistentRead so a device
// disconnected microseconds earlier is rejected immediately (docs/06-auth.md
// §4) — DynamoDB's default eventually-consistent read could otherwise still
// return the pre-disconnect item for a short window.
//
// This is the only read method a strict authorizer-only IAM policy
// (GetItem, no Query) can call — see G3 in the task brief.
//
// The caller (Task 8's authorizer) still must check Status/SessionTokenHash/
// SessionExpiresAt itself: this method only reports whether an item exists
// at that key, not whether it is a currently-valid session. Not found
// returns an *apierr.Error with CodeDeviceNotFound; callers that render an
// HTTP body of their own (rather than the authorizer's simple response)
// can rely on that code, but the authorizer is expected to treat any
// non-nil error as "deny" regardless of code.
func (s *Store) GetDeviceForAuth(ctx context.Context, deviceID string) (*Device, error) {
	key := deviceKey(deviceID)

	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      &s.table,
		Key:            map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		ConsistentRead: boolPtr(true),
	})
	if err != nil {
		return nil, wrapInternal("GetDeviceForAuth", err)
	}
	if out.Item == nil {
		return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
	}

	var dev Device
	if err := attributevalue.UnmarshalMap(out.Item, &dev); err != nil {
		return nil, wrapInternal("GetDeviceForAuth: unmarshal", err)
	}
	return &dev, nil
}

func stringAV(v string) *types.AttributeValueMemberS {
	return &types.AttributeValueMemberS{Value: v}
}

func boolPtr(b bool) *bool { return &b }

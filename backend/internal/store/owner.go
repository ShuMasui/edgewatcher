package store

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// ListOwnerDevices implements access patterns 1 and 2
// (docs/engineering/dynamodb.md §5, §5.1): a single GSI1 Query for
// GSI1PK = "OWNER#<ownerId>", returning every non-archived Device and every
// PairingSession the owner has — sorted by GSI1SK, so Device rows ("DEVICE#")
// sort before PairingSession rows ("PAIRING#"). The dashboard's "latest
// snapshot per device" and the devices page's "pairing pending" indicator
// both read off this one query; neither needs its own request.
//
// GSI1's projection is INCLUDE and does not carry ownerId, so rows come back
// as OwnerListRow, not Device — see that type's doc comment for why.
//
// An owner with no devices is not an error: it returns an empty slice.
func (s *Store) ListOwnerDevices(ctx context.Context, ownerID string) ([]OwnerListRow, error) {
	pk := ownerGSI1PK(ownerID)

	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &s.table,
		IndexName:              strPtr("GSI1"),
		KeyConditionExpression: strPtr("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": stringAV(pk),
		},
	})
	if err != nil {
		return nil, wrapInternal("ListOwnerDevices", err)
	}

	rows := make([]OwnerListRow, 0, len(out.Items))
	for _, item := range out.Items {
		var row OwnerListRow
		if err := attributevalue.UnmarshalMap(item, &row); err != nil {
			return nil, wrapInternal("ListOwnerDevices: unmarshal", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func strPtr(s string) *string { return &s }

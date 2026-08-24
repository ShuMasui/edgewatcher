package store

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// FindPairingByCode implements access pattern 5
// (docs/engineering/dynamodb.md §5): a device presents only a pairingCode
// (scanned from the QR), so the lookup goes GSI2 -> base table.
//
// GSI2 is eventually consistent, and a genuine race is expected: the code
// can be scanned within roughly a second of being issued, faster than GSI2
// propagation typically completes. docs/06-auth.md §2 and
// docs/engineering/dynamodb.md §4 both require this to be a distinct,
// retryable case, not conflated with an invalid code — the Android client
// retries a GSI2 miss up to 3 times at 1-second intervals before declaring
// the QR invalid. So a GSI2 miss here returns CodePairingNotFound (404),
// deliberately different from the 409s (CodePairingCodeExpired /
// CodePairingCodeConsumed) that Task 7's transactional consume path returns
// once it has reached the base table and found the code genuinely used up.
// This method itself never inspects Status/ExpiresAt to distinguish
// expired/consumed — it only answers "does this code resolve to a session
// at all"; the caller decides validity from the returned item.
func (s *Store) FindPairingByCode(ctx context.Context, pairingCode string) (*PairingSession, error) {
	gsi2Key := pairingGSI2Key(pairingCode)

	idxOut, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &s.table,
		IndexName:              strPtr("GSI2"),
		KeyConditionExpression: strPtr("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": stringAV(gsi2Key),
		},
		Limit: int32Ptr(1),
	})
	if err != nil {
		return nil, wrapInternal("FindPairingByCode: GSI2 query", err)
	}
	if len(idxOut.Items) == 0 {
		return nil, apierr.New(apierr.CodePairingNotFound, "pairing code not found")
	}

	pkAV, ok := idxOut.Items[0]["PK"]
	if !ok {
		return nil, wrapInternal("FindPairingByCode", errMissingKeyAttr("PK"))
	}
	skAV, ok := idxOut.Items[0]["SK"]
	if !ok {
		return nil, wrapInternal("FindPairingByCode", errMissingKeyAttr("SK"))
	}

	baseOut, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.table,
		Key:       map[string]types.AttributeValue{"PK": pkAV, "SK": skAV},
	})
	if err != nil {
		return nil, wrapInternal("FindPairingByCode: base GetItem", err)
	}
	if baseOut.Item == nil {
		// GSI2 found a key that the base table no longer has. GSI2 is
		// eventually consistent, so a very recent delete of the base item
		// (or, out of MVP scope, the DATA-02 cleanup process) can produce
		// this window; treat it as internal rather than PairingNotFound,
		// since a genuine "code never existed" case is already handled
		// above, before we trusted anything from GSI2.
		return nil, wrapInternal("FindPairingByCode", errStaleGSI2Entry)
	}

	var session PairingSession
	if err := attributevalue.UnmarshalMap(baseOut.Item, &session); err != nil {
		return nil, wrapInternal("FindPairingByCode: unmarshal", err)
	}
	return &session, nil
}

func int32Ptr(i int32) *int32 { return &i }

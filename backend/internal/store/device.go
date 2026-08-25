package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

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

// formatISO renders t as a UTC RFC3339 timestamp. It is the single
// chokepoint every ISO-8601 timestamp this package writes goes through,
// and normalizing to UTC before formatting is mandatory, not cosmetic:
// latestCapturedAt is compared lexicographically inside a
// ConditionExpression (TouchDeviceLatest), and a lexicographic compare of
// RFC3339 strings is monotone with wall-clock time only if every write
// uses the same UTC offset. A caller-supplied time.Time in a non-UTC
// location (e.g. JST, "+09:00") would otherwise sort incorrectly against
// a "Z" timestamp written by another call — silently breaking the "reject
// an older photo" guarantee this package exists to enforce.
func formatISO(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// isConditionalCheckFailed reports whether err is a single-request
// (non-transactional) DynamoDB ConditionalCheckFailedException — the
// UpdateItem/PutItem equivalent of a TransactWriteItems cancellation.
func isConditionalCheckFailed(err error) bool {
	var ccf *types.ConditionalCheckFailedException
	return errors.As(err, &ccf)
}

// RotateSessionToken implements POST /device/token
// (docs/06-auth.md §3): the caller (device-auth) has already verified
// deviceSecret against DeviceSecretHash (via a prior GetDeviceForAuth-style
// read — device-auth's IAM ceiling includes GetItem) and generated a fresh
// sessionToken; this method persists its hash and expiry.
//
// "Session per device, rotated on every exchange" (docs/06-auth.md §3) is
// satisfied structurally: sessionTokenHash is a single scalar attribute, so
// SET overwrites it in place rather than adding a second valid token.
//
// ConditionExpression requires the device to still exist and be PAIRED —
// a device that has been disconnected or archived between the caller's
// read and this write must not be handed a live session.
func (s *Store) RotateSessionToken(ctx context.Context, deviceID, sessionTokenHash string, sessionExpiresAt int64) (*Device, error) {
	key := deviceKey(deviceID)

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &s.table,
		Key:                 map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		UpdateExpression:    strPtr("SET sessionTokenHash = :hash, sessionExpiresAt = :exp"),
		ConditionExpression: strPtr("attribute_exists(PK) AND #status = :paired"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":hash":   stringAV(sessionTokenHash),
			":exp":    numberAV(sessionExpiresAt),
			":paired": stringAV(DeviceStatusPaired),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, apierr.New(apierr.CodeUnauthorized, "device not eligible for a session")
		}
		return nil, wrapInternal("RotateSessionToken", err)
	}

	var dev Device
	if err := attributevalue.UnmarshalMap(out.Attributes, &dev); err != nil {
		return nil, wrapInternal("RotateSessionToken: unmarshal", err)
	}
	return &dev, nil
}

// DisconnectDevice implements POST /devices/{id}/disconnect (Web-initiated)
// and POST /device/logout (device-initiated) — docs/06-auth.md §6 defines
// these as the same server-side operation, differing only in caller.
//
// It removes BOTH deviceSecretHash and sessionTokenHash. Removing only the
// session token is not a disconnect: the device still holds deviceSecret
// and would silently re-obtain a new session within seconds via
// POST /device/token (docs/06-auth.md §6's "セッション切断が
// sessionToken だけでは成立しない理由").
//
// ConditionExpression requires the device to exist and not already be
// ARCHIVED: a deleted device's credentials are already invalidated by
// ArchiveDevice, and disconnecting it again would be a no-op at best, a
// confusing status regression (ARCHIVED -> DISCONNECTED) at worst.
func (s *Store) DisconnectDevice(ctx context.Context, deviceID string) (*Device, error) {
	key := deviceKey(deviceID)

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &s.table,
		Key:                 map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		UpdateExpression:    strPtr("SET #status = :disconnected REMOVE deviceSecretHash, sessionTokenHash"),
		ConditionExpression: strPtr("attribute_exists(PK) AND #status <> :archived"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":disconnected": stringAV(DeviceStatusDisconnected),
			":archived":     stringAV(DeviceStatusArchived),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
		}
		return nil, wrapInternal("DisconnectDevice", err)
	}

	var dev Device
	if err := attributevalue.UnmarshalMap(out.Attributes, &dev); err != nil {
		return nil, wrapInternal("DisconnectDevice: unmarshal", err)
	}
	return &dev, nil
}

// ArchiveDevice implements DELETE /devices/{id} (docs/06-auth.md §6): a
// logical delete. It sets status = ARCHIVED, records archivedAt, and — the
// structural piece the rest of the system depends on — swaps GSI1PK from
// "OWNER#<ownerId>" to the "ARCHIVED" partition
// (docs/engineering/dynamodb.md §4). That swap, not a FilterExpression, is
// what makes an archived device vanish from ListOwnerDevices, which in turn
// is what makes "GET /devices, count the rows" a correct device-limit check
// (docs/03-web.md §1.9, Task 11's 429).
//
// It also invalidates the credentials (deviceSecretHash, sessionTokenHash),
// same as DisconnectDevice: docs/06-auth.md §6 lists ARCHIVED as a terminal
// state reachable from any other status, and a deleted device must not be
// able to keep uploading with credentials issued before deletion.
//
// Any prior status may transition to ARCHIVED (docs/06-auth.md §6: "どの
// 状態からも ARCHIVED に落ちる"), so the only condition is that the device
// exists at all.
func (s *Store) ArchiveDevice(ctx context.Context, deviceID string, now time.Time) (*Device, error) {
	key := deviceKey(deviceID)
	archivedAt := formatISO(now)

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &s.table,
		Key:                 map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		UpdateExpression:    strPtr("SET #status = :archived, archivedAt = :at, GSI1PK = :gsi1pk REMOVE deviceSecretHash, sessionTokenHash"),
		ConditionExpression: strPtr("attribute_exists(PK)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":archived": stringAV(DeviceStatusArchived),
			":at":       stringAV(archivedAt),
			":gsi1pk":   stringAV(archivedGSI1PK),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
		}
		return nil, wrapInternal("ArchiveDevice", err)
	}

	var dev Device
	if err := attributevalue.UnmarshalMap(out.Attributes, &dev); err != nil {
		return nil, wrapInternal("ArchiveDevice: unmarshal", err)
	}
	return &dev, nil
}

// UpdateDeviceProfileInput is the input to UpdateDeviceProfile. Name and
// Interval are independently optional (PATCH semantics: only supplied
// fields change) but at least one must be set.
type UpdateDeviceProfileInput struct {
	DeviceID string
	Name     *string
	Interval *int
}

// UpdateDeviceProfile implements PATCH /devices/{id} (docs/05-backend.md
// §1.2): renaming a device and/or changing its upload interval. Both
// "name" and "interval" are DynamoDB reserved words (doc.go), so both need
// ExpressionAttributeNames placeholders whenever referenced — including
// #status here, used only in the condition guarding against editing an
// ARCHIVED (deleted) device.
func (s *Store) UpdateDeviceProfile(ctx context.Context, in UpdateDeviceProfileInput) (*Device, error) {
	if in.Name == nil && in.Interval == nil {
		return nil, apierr.New(apierr.CodeValidation, "no fields to update")
	}
	key := deviceKey(in.DeviceID)

	var setClauses []string
	names := map[string]string{"#status": "status"}
	values := map[string]types.AttributeValue{":archived": stringAV(DeviceStatusArchived)}

	if in.Name != nil {
		setClauses = append(setClauses, "#name = :name")
		names["#name"] = "name"
		values[":name"] = stringAV(*in.Name)
	}
	if in.Interval != nil {
		setClauses = append(setClauses, "#interval = :interval")
		names["#interval"] = "interval"
		values[":interval"] = &types.AttributeValueMemberN{Value: strconv.Itoa(*in.Interval)}
	}

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &s.table,
		Key:                       map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		UpdateExpression:          strPtr("SET " + strings.Join(setClauses, ", ")),
		ConditionExpression:       strPtr("attribute_exists(PK) AND #status <> :archived"),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ReturnValues:              types.ReturnValueAllNew,
	})
	if err != nil {
		if isConditionalCheckFailed(err) {
			return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
		}
		return nil, wrapInternal("UpdateDeviceProfile", err)
	}

	var dev Device
	if err := attributevalue.UnmarshalMap(out.Attributes, &dev); err != nil {
		return nil, wrapInternal("UpdateDeviceProfile: unmarshal", err)
	}
	return &dev, nil
}

// TouchDeviceLatestInput is the input to TouchDeviceLatest.
type TouchDeviceLatestInput struct {
	DeviceID           string
	ReceivedAt         time.Time
	LatestCapturedAt   time.Time
	LatestThumbnailKey string
}

// TouchDeviceLatest implements the Device-side half of upload receipt
// (docs/engineering/dynamodb.md §5.1, docs/05-backend.md §3.3): after
// PutObservation succeeds, denormalize lastReceivedAt/latest* onto the
// Device row so the dashboard's "latest snapshot per device" is a single
// GSI1 query with no per-device fan-out.
//
// Primary attempt updates all three fields together, conditioned on the
// incoming photo being newer than whatever latestCapturedAt already holds
// (or there being none yet). attribute_exists(PK) is mandatory, not
// optional hardening: UpdateItem is an upsert, and device-api — the only
// caller — has no read permission at all (G3), so without this condition a
// hard-deleted (ARCHIVED-then-physically-removed) device would be silently
// resurrected as a bare attribute fragment carrying none of its history.
//
// If that condition fails — an offline device backfilling older photos
// after already sending its newest one first (docs/04-native.md §3.5) —
// lastReceivedAt must still advance unconditionally: "we heard from this
// device just now" is true regardless of the photo's age, and that value
// drives the dashboard's 応答なし (no response) display. The fallback
// UpdateItem does exactly that, still gated on attribute_exists(PK) alone
// so a nonexistent device fails both paths rather than being created by
// either.
//
// Both paths use ReturnValues: ALL_NEW so device-api can read back
// Interval for the upload response's nextConfig without a GetItem it is
// not permitted to make.
//
// Both paths also require status <> ARCHIVED, the same defense-in-depth
// already applied to ConsumePairing: the authorizer is what actually keeps
// an archived device from reaching this code path (its credentials are
// invalidated by ArchiveDevice), but ArchiveDevice leaves the row itself
// in place, so this guard is nearly free and stops a write to an archived
// device's row from succeeding by any other means.
//
// recvIso/capIso are formatted via formatISO (UTC, fixed-width RFC3339),
// never time.RFC3339Nano: latestCapturedAt is compared lexicographically
// as an index value, and RFC3339Nano's variable-width fractional seconds
// would make that comparison non-monotone ("...:00.5Z" sorts before
// "...:00Z"). Two uploads landing in the same second could in principle
// collide/truncate to the same value; at a 5-minute upload interval this
// is an acceptable, deliberate trade for keeping the ordering exact.
func (s *Store) TouchDeviceLatest(ctx context.Context, in TouchDeviceLatestInput) (*Device, error) {
	key := deviceKey(in.DeviceID)
	recvIso := formatISO(in.ReceivedAt)
	capIso := formatISO(in.LatestCapturedAt)

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &s.table,
		Key:                 map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		UpdateExpression:    strPtr("SET lastReceivedAt = :recv, latestThumbnailKey = :thumb, latestCapturedAt = :cap"),
		ConditionExpression: strPtr("attribute_exists(PK) AND #status <> :archived AND (attribute_not_exists(latestCapturedAt) OR latestCapturedAt < :cap)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":recv":     stringAV(recvIso),
			":thumb":    stringAV(in.LatestThumbnailKey),
			":cap":      stringAV(capIso),
			":archived": stringAV(DeviceStatusArchived),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err == nil {
		var dev Device
		if uerr := attributevalue.UnmarshalMap(out.Attributes, &dev); uerr != nil {
			return nil, wrapInternal("TouchDeviceLatest: unmarshal", uerr)
		}
		return &dev, nil
	}
	if !isConditionalCheckFailed(err) {
		return nil, wrapInternal("TouchDeviceLatest", err)
	}

	// Fallback: advance lastReceivedAt only. Still requires the device to
	// exist and not be ARCHIVED — see doc comment above.
	fallbackOut, ferr := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &s.table,
		Key:                 map[string]types.AttributeValue{"PK": stringAV(key), "SK": stringAV(key)},
		UpdateExpression:    strPtr("SET lastReceivedAt = :recv"),
		ConditionExpression: strPtr("attribute_exists(PK) AND #status <> :archived"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":recv":     stringAV(recvIso),
			":archived": stringAV(DeviceStatusArchived),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if ferr != nil {
		if isConditionalCheckFailed(ferr) {
			return nil, apierr.New(apierr.CodeDeviceNotFound, "device not found")
		}
		return nil, wrapInternal("TouchDeviceLatest: fallback", ferr)
	}

	var dev Device
	if uerr := attributevalue.UnmarshalMap(fallbackOut.Attributes, &dev); uerr != nil {
		return nil, wrapInternal("TouchDeviceLatest: fallback unmarshal", uerr)
	}
	return &dev, nil
}

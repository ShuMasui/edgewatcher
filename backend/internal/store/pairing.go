package store

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// pairingSessionTTL is the pairing QR's lifetime — 5 minutes, fixed by
// docs/engineering/dynamodb.md §3/§6 and docs/06-auth.md §2. It doubles as
// this item's TTL attribute value, but expiry is always decided by
// comparing ExpiresAt to the current time in code (or in a
// ConditionExpression) — never by assuming the TTL sweep has already
// removed the item (doc.go).
const pairingSessionTTL = 5 * time.Minute

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

// CreateDeviceWithPairingInput is the input to CreateDeviceWithPairing. The
// caller (a Task 9 handler) generates DeviceID and PairingCode via
// internal/ids and supplies Now via internal/clock — this package never
// calls time.Now() or ids.NewULID itself, so every write here is a pure,
// deterministic translation an integration test can pin exactly.
type CreateDeviceWithPairingInput struct {
	OwnerID     string
	DeviceID    string
	Name        string
	Interval    int
	PairingCode string
	Now         time.Time
}

// CreateDeviceWithPairing implements POST /devices
// (docs/engineering/dynamodb.md §6, docs/06-auth.md §2): one
// TransactWriteItems call that Puts a new Device (status PENDING) and its
// first PairingSession together. Both items land in the same partition
// (PK = "DEVICE#<deviceId>"), so this never spans partitions.
//
// Each Put carries its own attribute_not_exists condition — on the
// Device's PK, on the PairingSession's SK — guarding against a deviceId
// collision (vanishingly unlikely for a freshly generated ULID, but an
// unconditional TransactWriteItems::Put is an upsert, and this package
// never leaves an upsert unconditioned against a name it doesn't own).
func (s *Store) CreateDeviceWithPairing(ctx context.Context, in CreateDeviceWithPairingInput) (*Device, *PairingSession, error) {
	key := deviceKey(in.DeviceID)
	createdAt := formatISO(in.Now)
	expiresAt := in.Now.Add(pairingSessionTTL).Unix()

	dev := Device{
		PK: key, SK: key, EntityType: "Device",
		DeviceID: in.DeviceID, OwnerID: in.OwnerID, Name: in.Name,
		Status: DeviceStatusPending, Interval: in.Interval,
		CreatedAt: createdAt,
		GSI1PK:    ownerGSI1PK(in.OwnerID), GSI1SK: deviceGSI1SK(in.DeviceID),
	}
	devItem, err := attributevalue.MarshalMap(dev)
	if err != nil {
		return nil, nil, wrapInternal("CreateDeviceWithPairing: marshal device", err)
	}

	session := PairingSession{
		PK: key, SK: pairingSK(in.PairingCode), EntityType: "PairingSession",
		DeviceID: in.DeviceID, OwnerID: in.OwnerID, PairingCode: in.PairingCode,
		Status: PairingStatusPending, ExpiresAt: expiresAt, CreatedAt: createdAt,
		GSI1PK: ownerGSI1PK(in.OwnerID), GSI1SK: pairingGSI1SK(in.DeviceID),
		GSI2PK: pairingGSI2Key(in.PairingCode), GSI2SK: pairingGSI2Key(in.PairingCode),
	}
	sessionItem, err := attributevalue.MarshalMap(session)
	if err != nil {
		return nil, nil, wrapInternal("CreateDeviceWithPairing: marshal pairing session", err)
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           &s.table,
				Item:                devItem,
				ConditionExpression: strPtr("attribute_not_exists(PK)"),
			}},
			{Put: &types.Put{
				TableName:           &s.table,
				Item:                sessionItem,
				ConditionExpression: strPtr("attribute_not_exists(SK)"),
			}},
		},
	})
	if err != nil {
		return nil, nil, wrapInternal("CreateDeviceWithPairing", err)
	}
	return &dev, &session, nil
}

// CreatePairingSessionInput is the input to CreatePairingSession.
type CreatePairingSessionInput struct {
	DeviceID    string
	OwnerID     string
	PairingCode string
	Now         time.Time
}

// CreatePairingSession implements POST /devices/{id}/pairing-sessions
// (docs/03-web.md §1.8.2, docs/05-backend.md §1.2): re-issuing a QR for an
// existing device without recreating the Device row (deviceId is
// immutable). A single PutItem, not a transaction: it writes exactly one
// new item (a fresh PairingCode means a fresh SK).
//
// ConditionExpression: attribute_not_exists(PK) is a ULID/pairingCode
// collision guard only (symmetric to the same check in
// CreateDeviceWithPairing) — vanishingly unlikely to ever fire, but an
// unconditional Put is an upsert and this package never leaves one
// unconditioned. It does NOT check that the Device row exists or is
// non-ARCHIVED: ConsumePairing's own "attribute_exists(PK) AND status <>
// ARCHIVED" condition is the real, load-bearing gate against pairing an
// archived or nonexistent device, and it holds regardless of what this
// method does.
//
// A Device-row check was deliberately NOT added here as a
// TransactWriteItems ConditionCheck, even though it would reject a
// re-issue against a deleted/nonexistent deviceId up front rather than
// producing a QR that can only ever fail. ConditionCheck is governed by
// the IAM action dynamodb:ConditionCheckItem, which is DISTINCT from
// dynamodb:TransactWriteItems (holding the latter does not grant the
// former) and which the deployed web-api role
// (infra/envs/dev/iam.tf) does not grant — adding one here would make
// every call to this method fail closed with AccessDeniedException in the
// real environment, which no test running against DynamoDB Local (which
// does not enforce IAM) would catch. If a device-existence check is wanted
// here in the future, it requires either an IAM change (out of this
// package's scope, G4) or an UpdateItem in place of the Put — the latter
// was also rejected: every attribute a no-op Update could SET on the
// PairingSession row is otherwise supplied by this same Put, so turning it
// into an Update would mean crafting an item-shape workaround rather than
// a real fix.
func (s *Store) CreatePairingSession(ctx context.Context, in CreatePairingSessionInput) (*PairingSession, error) {
	createdAt := formatISO(in.Now)
	expiresAt := in.Now.Add(pairingSessionTTL).Unix()

	session := PairingSession{
		PK: deviceKey(in.DeviceID), SK: pairingSK(in.PairingCode), EntityType: "PairingSession",
		DeviceID: in.DeviceID, OwnerID: in.OwnerID, PairingCode: in.PairingCode,
		Status: PairingStatusPending, ExpiresAt: expiresAt, CreatedAt: createdAt,
		GSI1PK: ownerGSI1PK(in.OwnerID), GSI1SK: pairingGSI1SK(in.DeviceID),
		GSI2PK: pairingGSI2Key(in.PairingCode), GSI2SK: pairingGSI2Key(in.PairingCode),
	}
	item, err := attributevalue.MarshalMap(session)
	if err != nil {
		return nil, wrapInternal("CreatePairingSession: marshal", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &s.table,
		Item:                item,
		ConditionExpression: strPtr("attribute_not_exists(PK)"),
	})
	if err != nil {
		return nil, wrapInternal("CreatePairingSession", err)
	}
	return &session, nil
}

// ConsumePairingInput is the input to ConsumePairing.
type ConsumePairingInput struct {
	DeviceID         string
	PairingCode      string
	DeviceSecretHash string
	DeviceInfo       DeviceInfo
	Now              time.Time
}

// ConsumePairing implements POST /device/pair
// (docs/engineering/dynamodb.md §6, docs/06-auth.md §2): the exactly-once
// consumption of a pairing code. The caller must already have resolved
// PairingCode to DeviceID via FindPairingByCode (GSI2 -> base table); this
// method only performs the transactional state change against the base
// table's PK = SK = "DEVICE#<deviceId>" partition — it never itself
// queries GSI2, so "not found in GSI2 at all" (CodePairingNotFound, a
// distinct and retryable case per docs/engineering/dynamodb.md §4) can
// never come from this method.
//
// One TransactWriteItems call, two Updates in the same partition:
//
//  1. PairingSession: PENDING -> CONSUMED, REMOVE GSI2PK/GSI2SK so the
//     code becomes unresolvable through GSI2 the instant it's consumed
//     (a replayed code can't even be looked up, let alone re-consumed).
//     Condition: status = PENDING AND expiresAt > now. Carries
//     ReturnValuesOnConditionCheckFailure: ALL_OLD so a failure here can
//     be classified into PAIRING_CODE_EXPIRED vs PAIRING_CODE_CONSUMED
//     without a second round trip.
//  2. Device: status -> PAIRED, deviceSecretHash and deviceInfo set.
//     Condition: attribute_exists(PK) AND status <> ARCHIVED — the second
//     clause is an addition beyond docs/06-auth.md's condition (that
//     document only requires the record to exist): without it, a deleted
//     (ARCHIVED) device could be re-paired by whoever still holds the QR.
//
// On success there is nothing further to report: the caller already holds
// the deviceSecret it hashed before calling this method.
func (s *Store) ConsumePairing(ctx context.Context, in ConsumePairingInput) error {
	key := deviceKey(in.DeviceID)
	nowIso := formatISO(in.Now)
	nowEpoch := in.Now.Unix()

	deviceInfoAV, err := attributevalue.MarshalMap(in.DeviceInfo)
	if err != nil {
		return wrapInternal("ConsumePairing: marshal deviceInfo", err)
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Update: &types.Update{
				TableName: &s.table,
				Key: map[string]types.AttributeValue{
					"PK": stringAV(key), "SK": stringAV(pairingSK(in.PairingCode)),
				},
				UpdateExpression:    strPtr("SET #status = :consumed, consumedAt = :nowIso REMOVE GSI2PK, GSI2SK"),
				ConditionExpression: strPtr("#status = :pending AND expiresAt > :nowEpoch"),
				ExpressionAttributeNames: map[string]string{
					"#status": "status",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":consumed": stringAV(PairingStatusConsumed),
					":nowIso":   stringAV(nowIso),
					":pending":  stringAV(PairingStatusPending),
					":nowEpoch": numberAV(nowEpoch),
				},
				ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
			}},
			{Update: &types.Update{
				TableName: &s.table,
				Key: map[string]types.AttributeValue{
					"PK": stringAV(key), "SK": stringAV(key),
				},
				UpdateExpression:    strPtr("SET #status = :paired, deviceSecretHash = :hash, deviceInfo = :info"),
				ConditionExpression: strPtr("attribute_exists(PK) AND #status <> :archived"),
				ExpressionAttributeNames: map[string]string{
					"#status": "status",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":paired":   stringAV(DeviceStatusPaired),
					":hash":     stringAV(in.DeviceSecretHash),
					":info":     &types.AttributeValueMemberM{Value: deviceInfoAV},
					":archived": stringAV(DeviceStatusArchived),
				},
			}},
		},
	})
	if err != nil {
		return classifyConsumePairingError(err, in.Now)
	}
	return nil
}

// classifyConsumePairingError turns a TransactWriteItems failure from
// ConsumePairing into the specific *apierr.Error docs/05-backend.md §1.6
// and docs/04-native.md §1.4 require the caller to distinguish.
//
// DynamoDB reports a canceled transaction as
// *types.TransactionCanceledException with one CancellationReason per
// TransactItems entry, in request order, "None" for any item that did not
// itself fail. Index 0 is the PairingSession update, index 1 is the Device
// update (matching TransactItems' order above).
func classifyConsumePairingError(err error, now time.Time) error {
	var tce *types.TransactionCanceledException
	if !errors.As(err, &tce) {
		return wrapInternal("ConsumePairing", err)
	}

	reasons := tce.CancellationReasons
	if len(reasons) > 0 && cancellationCodeIs(reasons[0], "ConditionalCheckFailed") {
		return classifyPairingConditionFailure(reasons[0].Item, now)
	}
	if len(reasons) > 1 && cancellationCodeIs(reasons[1], "ConditionalCheckFailed") {
		return apierr.New(apierr.CodeDeviceNotFound, "device not found")
	}
	return wrapInternal("ConsumePairing: transaction canceled", err)
}

// classifyPairingConditionFailure decides PAIRING_CODE_EXPIRED vs
// PAIRING_CODE_CONSUMED from the PairingSession row DynamoDB returned via
// ReturnValuesOnConditionCheckFailure: ALL_OLD — the only way to make this
// distinction in a single round trip. Expiry is checked first and wins:
// docs/engineering/dynamodb.md §7 requires expiry itself to always be
// decided by comparing expiresAt to now, in code, never by trusting that a
// CONSUMED status implies "not expired" or vice versa.
//
// An empty item means the row itself no longer exists at all — most likely
// a race against the TTL sweep (FindPairingByCode resolved it moments ago,
// but TTL deletion can run at any point after expiresAt passes; docs/
// engineering/dynamodb.md §7's up-to-48h lag is an upper bound, not a
// floor). This is not a case of trusting TTL to decide expiry — the
// #status = :pending AND expiresAt > :nowEpoch condition already decided,
// authoritatively, that the update must fail; this is only choosing how to
// *report* an already-failed condition, and PAIRING_CODE_EXPIRED is the
// honest answer for a row that no longer exists to be resolved from GSI2
// either, since a row only ever leaves PENDING existence by expiring
// (whether this method catches it via the condition or TTL beats it to
// physical deletion). Note this empty-item branch, and this function
// entirely, depend on ConsumePairing's Update carrying
// ReturnValuesOnConditionCheckFailure: ALL_OLD (pinned by
// TestConsumePairing_TransactItemsShape) — without it, every condition
// failure would arrive here as an empty item, and a genuine CONSUMED
// replay would quietly misreport as PAIRING_CODE_EXPIRED instead of
// PAIRING_CODE_CONSUMED.
//
// One remaining, accepted imprecision: a code that was CONSUMED and then
// later TTL-swept (the base row physically deleted) also arrives here with
// an empty item, and is reported PAIRING_CODE_EXPIRED rather than the
// technically-more-accurate PAIRING_CODE_CONSUMED. Both map to 409 and
// both are "this code cannot be used" to the caller, so this is treated as
// cosmetic rather than fixed.
func classifyPairingConditionFailure(item map[string]types.AttributeValue, now time.Time) error {
	if len(item) == 0 {
		return apierr.New(apierr.CodePairingCodeExpired, "pairing code expired")
	}
	var session PairingSession
	if err := attributevalue.UnmarshalMap(item, &session); err != nil {
		return wrapInternal("ConsumePairing: unmarshal cancellation item", err)
	}
	if session.ExpiresAt <= now.Unix() {
		return apierr.New(apierr.CodePairingCodeExpired, "pairing code expired")
	}
	return apierr.New(apierr.CodePairingCodeConsumed, "pairing code already consumed")
}

func cancellationCodeIs(r types.CancellationReason, code string) bool {
	return r.Code != nil && *r.Code == code
}

func numberAV(n int64) *types.AttributeValueMemberN {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(n, 10)}
}

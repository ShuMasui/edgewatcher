package store

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// stubDynamoDBAPI is a fake DynamoDBAPI that records every input it's
// called with, so tests can pin exact request shapes — ConsistentRead,
// ScanIndexForward, IndexName, Limit, ReturnValues,
// ReturnValuesOnConditionCheckFailure — that DynamoDB Local cannot
// distinguish from their absence (it serves every read strongly consistent
// and doesn't care which GSI a Query names as long as the keys resolve, and
// it does return ALL_NEW/ALL_OLD correctly, but nothing fails if the flag
// is silently dropped from the Go call site — only a request-shape
// assertion catches that). The integration suite
// (store_integration_test.go) proves these methods produce correct
// *results* against a real engine; this file proves the *request* going
// out is the one the docs actually require, so dropping a flag in a future
// refactor fails a fast unit test instead of only mattering in production.
type stubDynamoDBAPI struct {
	getItemInput  *dynamodb.GetItemInput
	getItemOutput *dynamodb.GetItemOutput
	getItemErr    error

	queryInputs  []*dynamodb.QueryInput
	queryOutputs []*dynamodb.QueryOutput
	queryErr     error

	putItemInputs []*dynamodb.PutItemInput
	putItemErr    error

	// updateItemErrs/updateItemOutputs are indexed by call order, so a test
	// can make e.g. the first UpdateItem call (TouchDeviceLatest's primary
	// attempt) fail and the second (its fallback) succeed.
	updateItemInputs  []*dynamodb.UpdateItemInput
	updateItemOutputs []*dynamodb.UpdateItemOutput
	updateItemErrs    []error

	transactWriteItemsInput *dynamodb.TransactWriteItemsInput
	transactWriteItemsErr   error
}

func (f *stubDynamoDBAPI) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getItemInput = params
	if f.getItemErr != nil {
		return nil, f.getItemErr
	}
	if f.getItemOutput != nil {
		return f.getItemOutput, nil
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (f *stubDynamoDBAPI) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.queryInputs = append(f.queryInputs, params)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	idx := len(f.queryInputs) - 1
	if idx < len(f.queryOutputs) {
		return f.queryOutputs[idx], nil
	}
	return &dynamodb.QueryOutput{}, nil
}

func (f *stubDynamoDBAPI) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.putItemInputs = append(f.putItemInputs, params)
	if f.putItemErr != nil {
		return nil, f.putItemErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (f *stubDynamoDBAPI) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	idx := len(f.updateItemInputs)
	f.updateItemInputs = append(f.updateItemInputs, params)
	if idx < len(f.updateItemErrs) && f.updateItemErrs[idx] != nil {
		return nil, f.updateItemErrs[idx]
	}
	if idx < len(f.updateItemOutputs) {
		return f.updateItemOutputs[idx], nil
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func (f *stubDynamoDBAPI) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.transactWriteItemsInput = params
	if f.transactWriteItemsErr != nil {
		return nil, f.transactWriteItemsErr
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func mustMarshalMap(t *testing.T, v any) map[string]types.AttributeValue {
	t.Helper()
	item, err := attributevalue.MarshalMap(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return item
}

func TestGetDeviceForAuth_UsesConsistentRead(t *testing.T) {
	deviceItem := mustMarshalMap(t, Device{
		PK: deviceKey("d1"), SK: deviceKey("d1"),
		EntityType: "Device", DeviceID: "d1", OwnerID: "o1",
		Name: "n", Status: "PAIRED", Interval: 5,
	})
	stub := &stubDynamoDBAPI{getItemOutput: &dynamodb.GetItemOutput{Item: deviceItem}}
	s := New(stub, "test-table")

	if _, err := s.GetDeviceForAuth(context.Background(), "d1"); err != nil {
		t.Fatalf("GetDeviceForAuth: %v", err)
	}

	if stub.getItemInput == nil {
		t.Fatal("GetItem was never called")
	}
	if stub.getItemInput.ConsistentRead == nil || !*stub.getItemInput.ConsistentRead {
		t.Fatal("GetDeviceForAuth must set ConsistentRead: true — docs/06-auth.md §4 requires a just-disconnected device to be rejected on its very next request, which an eventually-consistent read cannot guarantee")
	}
}

func TestQueryObservationsByDay_ScanIndexForwardFalse(t *testing.T) {
	stub := &stubDynamoDBAPI{}
	s := New(stub, "test-table")

	if _, err := s.QueryObservationsByDay(context.Background(), "d1", "2026-06-15"); err != nil {
		t.Fatalf("QueryObservationsByDay: %v", err)
	}

	if len(stub.queryInputs) != 1 {
		t.Fatalf("expected 1 Query call, got %d", len(stub.queryInputs))
	}
	in := stub.queryInputs[0]
	if in.ScanIndexForward == nil || *in.ScanIndexForward {
		t.Fatal("QueryObservationsByDay must set ScanIndexForward: false — without it the history page renders oldest-first")
	}
}

func TestListOwnerDevices_QueriesGSI1(t *testing.T) {
	stub := &stubDynamoDBAPI{}
	s := New(stub, "test-table")

	if _, err := s.ListOwnerDevices(context.Background(), "o1"); err != nil {
		t.Fatalf("ListOwnerDevices: %v", err)
	}

	if len(stub.queryInputs) != 1 {
		t.Fatalf("expected 1 Query call, got %d", len(stub.queryInputs))
	}
	in := stub.queryInputs[0]
	if in.IndexName == nil || *in.IndexName != "GSI1" {
		t.Fatalf("ListOwnerDevices must query IndexName \"GSI1\", got %v", in.IndexName)
	}
}

func TestFindPairingByCode_QueriesGSI2WithLimit1(t *testing.T) {
	sessionItem := mustMarshalMap(t, PairingSession{
		PK: deviceKey("d1"), SK: pairingSK("CODE1"),
		EntityType: "PairingSession", DeviceID: "d1", OwnerID: "o1",
		PairingCode: "CODE1", Status: "PENDING", ExpiresAt: 999999,
	})
	stub := &stubDynamoDBAPI{
		queryOutputs: []*dynamodb.QueryOutput{
			{Items: []map[string]types.AttributeValue{
				{"PK": stringAV(deviceKey("d1")), "SK": stringAV(pairingSK("CODE1"))},
			}},
		},
		getItemOutput: &dynamodb.GetItemOutput{Item: sessionItem},
	}
	s := New(stub, "test-table")

	if _, err := s.FindPairingByCode(context.Background(), "CODE1"); err != nil {
		t.Fatalf("FindPairingByCode: %v", err)
	}

	if len(stub.queryInputs) != 1 {
		t.Fatalf("expected 1 Query call, got %d", len(stub.queryInputs))
	}
	in := stub.queryInputs[0]
	if in.IndexName == nil || *in.IndexName != "GSI2" {
		t.Fatalf("FindPairingByCode must query IndexName \"GSI2\", got %v", in.IndexName)
	}
	if in.Limit == nil || *in.Limit != 1 {
		t.Fatalf("FindPairingByCode must set Limit: 1 on the GSI2 query, got %v", in.Limit)
	}

	if stub.getItemInput == nil {
		t.Fatal("expected a follow-up base-table GetItem after the GSI2 hit")
	}
}

func TestCreateDeviceWithPairing_TransactItemsShape(t *testing.T) {
	stub := &stubDynamoDBAPI{}
	s := New(stub, "test-table")

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	_, _, err := s.CreateDeviceWithPairing(context.Background(), CreateDeviceWithPairingInput{
		OwnerID: "o1", DeviceID: "d1", Name: "n", Interval: 5,
		PairingCode: "CODE1", Now: now,
	})
	if err != nil {
		t.Fatalf("CreateDeviceWithPairing: %v", err)
	}

	if stub.transactWriteItemsInput == nil {
		t.Fatal("expected a TransactWriteItems call")
	}
	items := stub.transactWriteItemsInput.TransactItems
	if len(items) != 2 {
		t.Fatalf("expected 2 transact items, got %d", len(items))
	}
	if items[0].Put == nil || items[0].Put.ConditionExpression == nil || *items[0].Put.ConditionExpression != "attribute_not_exists(PK)" {
		t.Fatalf("Device Put must have ConditionExpression attribute_not_exists(PK), got %+v", items[0].Put)
	}
	if items[1].Put == nil || items[1].Put.ConditionExpression == nil || *items[1].Put.ConditionExpression != "attribute_not_exists(SK)" {
		t.Fatalf("PairingSession Put must have ConditionExpression attribute_not_exists(SK), got %+v", items[1].Put)
	}
}

func TestConsumePairing_TransactItemsShape(t *testing.T) {
	stub := &stubDynamoDBAPI{}
	s := New(stub, "test-table")

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	err := s.ConsumePairing(context.Background(), ConsumePairingInput{
		DeviceID: "d1", PairingCode: "CODE1", DeviceSecretHash: "hash",
		DeviceInfo: DeviceInfo{Model: "Pixel", OSVersion: "14", AppVersion: "1.0"},
		Now:        now,
	})
	if err != nil {
		t.Fatalf("ConsumePairing: %v", err)
	}

	if stub.transactWriteItemsInput == nil {
		t.Fatal("expected a TransactWriteItems call")
	}
	items := stub.transactWriteItemsInput.TransactItems
	if len(items) != 2 {
		t.Fatalf("expected 2 transact items, got %d", len(items))
	}

	pairingUpdate := items[0].Update
	if pairingUpdate == nil {
		t.Fatal("index 0 must be an Update (PairingSession)")
	}
	if pairingUpdate.ReturnValuesOnConditionCheckFailure != types.ReturnValuesOnConditionCheckFailureAllOld {
		t.Fatalf("PairingSession Update must set ReturnValuesOnConditionCheckFailure: ALL_OLD (needed to distinguish PAIRING_CODE_EXPIRED from PAIRING_CODE_CONSUMED), got %v", pairingUpdate.ReturnValuesOnConditionCheckFailure)
	}
	if pairingUpdate.ConditionExpression == nil || *pairingUpdate.ConditionExpression != "#status = :pending AND expiresAt > :nowEpoch" {
		t.Fatalf("unexpected PairingSession ConditionExpression: %v", pairingUpdate.ConditionExpression)
	}
	if pairingUpdate.UpdateExpression == nil || *pairingUpdate.UpdateExpression != "SET #status = :consumed, consumedAt = :nowIso REMOVE GSI2PK, GSI2SK" {
		t.Fatalf("unexpected PairingSession UpdateExpression: %v", pairingUpdate.UpdateExpression)
	}
	if pairingUpdate.ExpressionAttributeNames["#status"] != "status" {
		t.Fatalf("PairingSession Update must escape #status -> status, got %v", pairingUpdate.ExpressionAttributeNames)
	}

	deviceUpdate := items[1].Update
	if deviceUpdate == nil {
		t.Fatal("index 1 must be an Update (Device)")
	}
	if deviceUpdate.ReturnValuesOnConditionCheckFailure == types.ReturnValuesOnConditionCheckFailureAllOld {
		t.Fatal("Device Update should not need ReturnValuesOnConditionCheckFailure: ALL_OLD — its failure always maps to DEVICE_NOT_FOUND without inspecting the item")
	}
	if deviceUpdate.ConditionExpression == nil || *deviceUpdate.ConditionExpression != "attribute_exists(PK) AND #status <> :archived" {
		t.Fatalf("unexpected Device ConditionExpression: %v — must reject re-pairing an ARCHIVED device", deviceUpdate.ConditionExpression)
	}
	if deviceUpdate.ExpressionAttributeNames["#status"] != "status" {
		t.Fatalf("Device Update must escape #status -> status, got %v", deviceUpdate.ExpressionAttributeNames)
	}
}

func TestClassifyConsumePairingError_ExpiredVsConsumed(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)

	expiredSession := mustMarshalMap(t, PairingSession{
		PK: deviceKey("d1"), SK: pairingSK("CODE1"), Status: "PENDING",
		ExpiresAt: now.Add(-1 * time.Minute).Unix(),
	})
	consumedSession := mustMarshalMap(t, PairingSession{
		PK: deviceKey("d1"), SK: pairingSK("CODE1"), Status: "CONSUMED",
		ExpiresAt: now.Add(1 * time.Minute).Unix(),
	})
	condFailedCode := "ConditionalCheckFailed"
	noneCode := "None"

	t.Run("expired", func(t *testing.T) {
		err := classifyConsumePairingError(&types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: &condFailedCode, Item: expiredSession},
				{Code: &noneCode},
			},
		}, now)
		if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodePairingCodeExpired {
			t.Fatalf("expected CodePairingCodeExpired, got %s", apiErr.Code)
		}
	})

	t.Run("consumed", func(t *testing.T) {
		err := classifyConsumePairingError(&types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: &condFailedCode, Item: consumedSession},
				{Code: &noneCode},
			},
		}, now)
		if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodePairingCodeConsumed {
			t.Fatalf("expected CodePairingCodeConsumed, got %s", apiErr.Code)
		}
	})

	t.Run("device not found", func(t *testing.T) {
		err := classifyConsumePairingError(&types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: &noneCode},
				{Code: &condFailedCode},
			},
		}, now)
		if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeDeviceNotFound {
			t.Fatalf("expected CodeDeviceNotFound, got %s", apiErr.Code)
		}
	})
}

func TestTouchDeviceLatest_ReturnValuesAllNewOnBothPaths(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)

	t.Run("primary succeeds", func(t *testing.T) {
		stub := &stubDynamoDBAPI{
			updateItemOutputs: []*dynamodb.UpdateItemOutput{
				{Attributes: mustMarshalMap(t, Device{PK: deviceKey("d1"), SK: deviceKey("d1"), Interval: 5})},
			},
		}
		s := New(stub, "test-table")

		dev, err := s.TouchDeviceLatest(context.Background(), TouchDeviceLatestInput{
			DeviceID: "d1", ReceivedAt: now, LatestCapturedAt: now, LatestThumbnailKey: "thumb/1",
		})
		if err != nil {
			t.Fatalf("TouchDeviceLatest: %v", err)
		}
		if len(stub.updateItemInputs) != 1 {
			t.Fatalf("expected exactly 1 UpdateItem call on the happy path, got %d", len(stub.updateItemInputs))
		}
		if stub.updateItemInputs[0].ReturnValues != types.ReturnValueAllNew {
			t.Fatalf("primary UpdateItem must set ReturnValues: ALL_NEW, got %v", stub.updateItemInputs[0].ReturnValues)
		}
		wantCond := "attribute_exists(PK) AND #status <> :archived AND (attribute_not_exists(latestCapturedAt) OR latestCapturedAt < :cap)"
		if stub.updateItemInputs[0].ConditionExpression == nil || *stub.updateItemInputs[0].ConditionExpression != wantCond {
			t.Fatalf("unexpected primary ConditionExpression: %v, want %q", stub.updateItemInputs[0].ConditionExpression, wantCond)
		}
		if dev.Interval != 5 {
			t.Fatalf("expected Interval to come back via ALL_NEW, got %d", dev.Interval)
		}
	})

	t.Run("fallback on older capturedAt", func(t *testing.T) {
		stub := &stubDynamoDBAPI{
			updateItemErrs: []error{&types.ConditionalCheckFailedException{}, nil},
			updateItemOutputs: []*dynamodb.UpdateItemOutput{
				nil,
				{Attributes: mustMarshalMap(t, Device{PK: deviceKey("d1"), SK: deviceKey("d1"), Interval: 10})},
			},
		}
		s := New(stub, "test-table")

		dev, err := s.TouchDeviceLatest(context.Background(), TouchDeviceLatestInput{
			DeviceID: "d1", ReceivedAt: now, LatestCapturedAt: now, LatestThumbnailKey: "thumb/old",
		})
		if err != nil {
			t.Fatalf("TouchDeviceLatest: %v", err)
		}
		if len(stub.updateItemInputs) != 2 {
			t.Fatalf("expected primary + fallback UpdateItem calls, got %d", len(stub.updateItemInputs))
		}
		if stub.updateItemInputs[1].ReturnValues != types.ReturnValueAllNew {
			t.Fatalf("fallback UpdateItem must also set ReturnValues: ALL_NEW (this is how device-api gets interval without a GetItem it can't make), got %v", stub.updateItemInputs[1].ReturnValues)
		}
		wantFallbackCond := "attribute_exists(PK) AND #status <> :archived"
		if stub.updateItemInputs[1].ConditionExpression == nil || *stub.updateItemInputs[1].ConditionExpression != wantFallbackCond {
			t.Fatalf("fallback UpdateItem must require %q — UpdateItem is an upsert and device-api has no read permission to guard against resurrecting a deleted or archived device, got %v", wantFallbackCond, stub.updateItemInputs[1].ConditionExpression)
		}
		if dev.Interval != 10 {
			t.Fatalf("expected Interval from the fallback's ALL_NEW, got %d", dev.Interval)
		}
	})

	t.Run("device does not exist on either path", func(t *testing.T) {
		stub := &stubDynamoDBAPI{
			updateItemErrs: []error{&types.ConditionalCheckFailedException{}, &types.ConditionalCheckFailedException{}},
		}
		s := New(stub, "test-table")

		_, err := s.TouchDeviceLatest(context.Background(), TouchDeviceLatestInput{
			DeviceID: "ghost", ReceivedAt: now, LatestCapturedAt: now, LatestThumbnailKey: "thumb/x",
		})
		if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodeDeviceNotFound {
			t.Fatalf("expected CodeDeviceNotFound for a nonexistent device on both paths, got %s", apiErr.Code)
		}
	})
}

func TestPutObservation_ReplayIsSuccess(t *testing.T) {
	stub := &stubDynamoDBAPI{putItemErr: &types.ConditionalCheckFailedException{}}
	s := New(stub, "test-table")

	err := s.PutObservation(context.Background(), Observation{
		PK: deviceKey("d1"), SK: observationSK("01ARZ3NDEKTSV4RRFFQ69G5FAV"),
		EntityType: "Observation", ObservationID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", DeviceID: "d1",
		CapturedAt: "2026-06-15T09:00:00Z", ImageKey: "img/1", ThumbnailKey: "thumb/1",
	})
	if err != nil {
		t.Fatalf("PutObservation replay must return nil (success), got %v", err)
	}
	if len(stub.putItemInputs) != 1 {
		t.Fatalf("expected 1 PutItem call, got %d", len(stub.putItemInputs))
	}
	if stub.putItemInputs[0].ConditionExpression == nil || *stub.putItemInputs[0].ConditionExpression != "attribute_not_exists(SK)" {
		t.Fatalf("PutObservation must set ConditionExpression attribute_not_exists(SK), got %v", stub.putItemInputs[0].ConditionExpression)
	}
}

func TestDisconnectDevice_RemovesBothHashes(t *testing.T) {
	stub := &stubDynamoDBAPI{
		updateItemOutputs: []*dynamodb.UpdateItemOutput{
			{Attributes: mustMarshalMap(t, Device{PK: deviceKey("d1"), SK: deviceKey("d1"), Status: "DISCONNECTED"})},
		},
	}
	s := New(stub, "test-table")

	if _, err := s.DisconnectDevice(context.Background(), "d1"); err != nil {
		t.Fatalf("DisconnectDevice: %v", err)
	}

	in := stub.updateItemInputs[0]
	if in.UpdateExpression == nil || *in.UpdateExpression != "SET #status = :disconnected REMOVE deviceSecretHash, sessionTokenHash" {
		t.Fatalf("DisconnectDevice must REMOVE both deviceSecretHash and sessionTokenHash, got %v", in.UpdateExpression)
	}
}

func TestArchiveDevice_SwapsGSI1PK(t *testing.T) {
	stub := &stubDynamoDBAPI{
		updateItemOutputs: []*dynamodb.UpdateItemOutput{
			{Attributes: mustMarshalMap(t, Device{PK: deviceKey("d1"), SK: deviceKey("d1"), Status: "ARCHIVED", GSI1PK: "ARCHIVED"})},
		},
	}
	s := New(stub, "test-table")

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := s.ArchiveDevice(context.Background(), "d1", now); err != nil {
		t.Fatalf("ArchiveDevice: %v", err)
	}

	in := stub.updateItemInputs[0]
	if in.UpdateExpression == nil || *in.UpdateExpression != "SET #status = :archived, archivedAt = :at, GSI1PK = :gsi1pk REMOVE deviceSecretHash, sessionTokenHash" {
		t.Fatalf("unexpected ArchiveDevice UpdateExpression: %v", in.UpdateExpression)
	}
	gsi1pk, ok := in.ExpressionAttributeValues[":gsi1pk"].(*types.AttributeValueMemberS)
	if !ok || gsi1pk.Value != "ARCHIVED" {
		t.Fatalf("ArchiveDevice must swap GSI1PK to the literal \"ARCHIVED\" partition, got %v", in.ExpressionAttributeValues[":gsi1pk"])
	}
}

// TestUpdateDeviceProfile_RejectsArchivedDevice_ExpressionShape pins the
// exact ConditionExpression string (I2): deleting the "AND #status <>
// :archived" clause must fail this test even though the happy-path
// integration test wouldn't notice, since it never exercises an ARCHIVED
// device.
func TestUpdateDeviceProfile_RejectsArchivedDevice_ExpressionShape(t *testing.T) {
	stub := &stubDynamoDBAPI{
		updateItemOutputs: []*dynamodb.UpdateItemOutput{
			{Attributes: mustMarshalMap(t, Device{PK: deviceKey("d1"), SK: deviceKey("d1"), Name: "New Name", Interval: 15})},
		},
	}
	s := New(stub, "test-table")

	newName := "New Name"
	newInterval := 15
	if _, err := s.UpdateDeviceProfile(context.Background(), UpdateDeviceProfileInput{
		DeviceID: "d1", Name: &newName, Interval: &newInterval,
	}); err != nil {
		t.Fatalf("UpdateDeviceProfile: %v", err)
	}

	in := stub.updateItemInputs[0]
	wantCond := "attribute_exists(PK) AND #status <> :archived"
	if in.ConditionExpression == nil || *in.ConditionExpression != wantCond {
		t.Fatalf("UpdateDeviceProfile must set ConditionExpression %q (rejecting an ARCHIVED device), got %v", wantCond, in.ConditionExpression)
	}
	if in.ExpressionAttributeNames["#status"] != "status" {
		t.Fatalf("UpdateDeviceProfile must escape #status -> status, got %v", in.ExpressionAttributeNames)
	}
	archivedVal, ok := in.ExpressionAttributeValues[":archived"].(*types.AttributeValueMemberS)
	if !ok || archivedVal.Value != "ARCHIVED" {
		t.Fatalf("UpdateDeviceProfile must bind :archived to \"ARCHIVED\", got %v", in.ExpressionAttributeValues[":archived"])
	}
}

// TestCreatePairingSession_PutItemShape pins the reverted I3 fix (R11): a
// single PutItem — not a transaction, and in particular no ConditionCheck
// (that would require dynamodb:ConditionCheckItem, which the deployed
// web-api role does not grant) — guarded only by attribute_not_exists(PK),
// the ULID/pairingCode collision guard. Deleting this ConditionExpression
// would make the Put an unconditioned upsert and this test would catch it.
func TestCreatePairingSession_PutItemShape(t *testing.T) {
	stub := &stubDynamoDBAPI{}
	s := New(stub, "test-table")

	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if _, err := s.CreatePairingSession(context.Background(), CreatePairingSessionInput{
		DeviceID: "d1", OwnerID: "o1", PairingCode: "NEWCODE", Now: now,
	}); err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}

	if stub.transactWriteItemsInput != nil {
		t.Fatal("CreatePairingSession must not use TransactWriteItems — ConditionCheck requires dynamodb:ConditionCheckItem, which web-api's IAM role does not grant (infra/envs/dev/iam.tf)")
	}
	if len(stub.putItemInputs) != 1 {
		t.Fatalf("expected exactly 1 PutItem call, got %d", len(stub.putItemInputs))
	}
	in := stub.putItemInputs[0]
	if in.ConditionExpression == nil || *in.ConditionExpression != "attribute_not_exists(PK)" {
		t.Fatalf("CreatePairingSession must set ConditionExpression attribute_not_exists(PK), got %v", in.ConditionExpression)
	}
}

// TestClassifyPairingConditionFailure_EmptyItemIsExpired pins Minor 4: a
// TTL-swept row (empty ALL_OLD item) must classify as
// PAIRING_CODE_EXPIRED, not CodeInternal.
func TestClassifyPairingConditionFailure_EmptyItemIsExpired(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	err := classifyPairingConditionFailure(map[string]types.AttributeValue{}, now)
	if apiErr := apierr.AsError(err); apiErr.Code != apierr.CodePairingCodeExpired {
		t.Fatalf("expected CodePairingCodeExpired for an empty ALL_OLD item, got %s", apiErr.Code)
	}
}

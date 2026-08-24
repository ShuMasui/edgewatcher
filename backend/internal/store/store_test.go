package store

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// stubDynamoDBAPI is a fake DynamoDBAPI that records every input it's
// called with, so tests can pin exact request shapes — ConsistentRead,
// ScanIndexForward, IndexName, Limit — that DynamoDB Local cannot
// distinguish from their absence (it serves every read strongly consistent
// and doesn't care which GSI a Query names as long as the keys resolve).
// The integration suite (store_integration_test.go) proves these methods
// produce correct *results* against a real engine; this file proves the
// *request* going out is the one the docs actually require, so dropping a
// flag in a future refactor fails a fast unit test instead of only
// mattering in production.
type stubDynamoDBAPI struct {
	getItemInput  *dynamodb.GetItemInput
	getItemOutput *dynamodb.GetItemOutput
	getItemErr    error

	queryInputs  []*dynamodb.QueryInput
	queryOutputs []*dynamodb.QueryOutput
	queryErr     error
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

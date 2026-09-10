//go:build integration

// Package dynamotest provides the DynamoDB Local harness shared by every
// integration test in this module.
//
// It is guarded by the `integration` build tag, so it is invisible to a
// normal build and never reaches a deployed binary despite importing
// "testing". It lives outside internal/store because more than one package
// needs a real table now: internal/store's own tests, and internal/e2e,
// which drives all four Lambdas against one.
//
// It never touches a real AWS account. The region and credentials are
// static dummy values and the endpoint is always overridden.
package dynamotest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// defaultEndpoint is the address `docker run ... -p 8000:8000
// amazon/dynamodb-local` binds to, used when DYNAMO_ENDPOINT is unset.
const defaultEndpoint = "http://localhost:8000"

// GSI1NonKeyAttributes is GSI1's INCLUDE projection, mirroring
// infra/modules/data/main.tf and docs/engineering/dynamodb.md §4.
//
// It is a single exported definition rather than a literal inside
// CreateTable so that store's TestGSI1ProjectionExcludesOwnerID can assert
// against the same list the fixture builds from. ownerId is deliberately
// absent: that absence is the whole reason store.OwnerListRow exists, and a
// silent widening of the Terraform projection must break a test rather than
// change the shape of a row in production.
var GSI1NonKeyAttributes = []string{
	"deviceId",
	"name",
	"status",
	"interval",
	"lastReceivedAt",
	"latestThumbnailKey",
	"latestCapturedAt",
	"pairingCode",
	"expiresAt",
}

// NewClient builds a *dynamodb.Client pointed at DynamoDB Local.
func NewClient(t *testing.T) *dynamodb.Client {
	t.Helper()

	endpoint := os.Getenv("DYNAMO_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("local"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// CreateTable creates a table matching infra/modules/data/main.tf's key
// schema and indexes: PK/SK, GSI1 with its INCLUDE projection, and GSI2 as
// KEYS_ONLY. The table name is random per call so parallel and repeated
// runs against one DynamoDB Local instance never collide; cleanup is
// registered on t.
//
// It deliberately does NOT enable the expiresAt TTL that Terraform enables
// in production. See the comment below the CreateTable call.
func CreateTable(t *testing.T, client *dynamodb.Client) string {
	t.Helper()
	ctx := context.Background()

	table := "edgewatcher-test-" + sanitizeTableName(t.Name()) + "-" + randomSuffix()

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   &table,
		BillingMode: types.BillingModePayPerRequest,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI2PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI2SK"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("GSI1"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI1PK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSI1SK"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{
					ProjectionType:   types.ProjectionTypeInclude,
					NonKeyAttributes: GSI1NonKeyAttributes,
				},
			},
			{
				IndexName: aws.String("GSI2"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI2PK"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("GSI2SK"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeKeysOnly,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create test table: %v", err)
	}

	// The expiresAt TTL is NOT enabled here, even though
	// infra/modules/data/main.tf enables it in production.
	//
	// DynamoDB Local runs a real reaper on real wall-clock time — measured
	// at roughly a 5-second sweep — while the tests run on a fixed fake
	// clock (internal/e2e's movableClock starts at 2026-09-08). Every row
	// those tests write is therefore born already expired, and whether a
	// sweep lands between a write and the read that follows it is pure
	// timing. That surfaced as pairing codes intermittently resolving to
	// PAIRING_NOT_FOUND, and as consumed codes reporting EXPIRED instead of
	// CONSUMED (the reaper removes the row that
	// ReturnValuesOnConditionCheckFailure would otherwise have returned).
	//
	// Advancing the fake clock past today only postpones this: the date is
	// a literal, so it becomes the past again as wall-clock time moves. The
	// fake clock and a real-time reaper cannot both be right.
	//
	// Nothing is lost by leaving it off. TTL affects only background
	// deletion, never reads or writes, and no test asserts that a row is
	// eventually deleted — none could, since real DynamoDB only promises
	// removal "within 48 hours". What the tests do check is that expiresAt
	// is written with the right value, which is an attribute assertion and
	// works the same either way.

	t.Cleanup(func() {
		_, _ = client.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: &table})
	})

	return table
}

// sanitizeTableName replaces characters DynamoDB table names disallow
// (subtest names contain "/") with underscores.
func sanitizeTableName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

// randomSuffix keeps concurrent and repeated runs from colliding.
func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

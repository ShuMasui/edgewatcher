//go:build integration

package store

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

// defaultDynamoEndpoint is used when DYNAMO_ENDPOINT isn't set — the address
// the task brief's `docker run ... -p 8000:8000 amazon/dynamodb-local`
// binds to.
const defaultDynamoEndpoint = "http://localhost:8000"

// newIntegrationClient builds a *dynamodb.Client pointed at DynamoDB Local.
// It never touches a real AWS account: the region and credentials are
// static dummy values, and the endpoint is always overridden — either by
// DYNAMO_ENDPOINT or defaultDynamoEndpoint.
func newIntegrationClient(t *testing.T) *dynamodb.Client {
	t.Helper()

	endpoint := os.Getenv("DYNAMO_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultDynamoEndpoint
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

// createTestTable creates a table matching infra/modules/data/main.tf
// exactly: PK/SK, GSI1 with its INCLUDE projection and nine non-key
// attributes, GSI2 as KEYS_ONLY, and the expiresAt TTL. Duplicating the
// projection list here (rather than sharing it with any Terraform-derived
// constant, since none exists in Go) is deliberate: TestGSI1ProjectionExcludesOwnerID
// asserts against exactly this list, so if someone widens the Terraform
// projection without updating this helper, that test breaks loudly instead
// of production silently returning a wider (and wrong-shaped) row.
//
// The table name is random per call so parallel test runs and repeated
// local runs don't collide; the caller registers cleanup via t.Cleanup.
func createTestTable(t *testing.T, client *dynamodb.Client) string {
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
					ProjectionType: types.ProjectionTypeInclude,
					NonKeyAttributes: []string{
						"deviceId",
						"name",
						"status",
						"interval",
						"lastReceivedAt",
						"latestThumbnailKey",
						"latestCapturedAt",
						"pairingCode",
						"expiresAt",
					},
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

	_, err = client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: &table,
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("expiresAt"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		t.Fatalf("enable ttl: %v", err)
	}

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

// randomSuffix returns a short random hex string so concurrent/repeated
// test runs against the same DynamoDB Local instance never collide on a
// table name.
func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

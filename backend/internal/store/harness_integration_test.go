//go:build integration

package store

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/ShuMasui/edgewatcher/backend/internal/dynamotest"
)

// The DynamoDB Local harness lives in internal/dynamotest because
// internal/e2e needs the same table. These two shims keep this package's
// integration tests reading the way they did when it was local.
//
// dynamotest.GSI1NonKeyAttributes is the single definition of GSI1's
// projection, mirroring infra/modules/data/main.tf:
// TestGSI1ProjectionExcludesOwnerID asserts against the same list the
// fixture builds from, so widening the Terraform projection without
// updating it breaks a test loudly instead of changing a row's shape in
// production silently.

func newIntegrationClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	return dynamotest.NewClient(t)
}

func createTestTable(t *testing.T, client *dynamodb.Client) string {
	t.Helper()
	return dynamotest.CreateTable(t, client)
}

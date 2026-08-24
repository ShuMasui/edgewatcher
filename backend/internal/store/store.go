package store

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDBAPI is the subset of *dynamodb.Client this package calls. Defining
// it lets tests substitute a stub or a client pointed at DynamoDB Local
// without this package importing any HTTP concerns of its own; production
// code always passes a real *dynamodb.Client, which satisfies this
// interface. Task 6 only reads, so only GetItem/Query are declared —
// widening this for Task 7's writes is a small diff in a file Task 7 is
// already touching, and until then every stub implementing this interface
// only has to provide the two methods actually exercised.
type DynamoDBAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// Store is the DynamoDB access layer. It holds no state beyond a client and
// the single table name — every method is a thin, documented translation
// from a domain-level read (or, from Task 7, write) into the exact
// DynamoDB call docs/engineering/dynamodb.md specifies.
type Store struct {
	client DynamoDBAPI
	table  string
}

// New constructs a Store over an existing DynamoDB client and table name.
// The caller owns the client's lifecycle (region, credentials, retries);
// this package never constructs one itself in production code.
func New(client DynamoDBAPI, table string) *Store {
	return &Store{client: client, table: table}
}

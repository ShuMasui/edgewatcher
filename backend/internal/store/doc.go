// Package store is the DynamoDB access layer for EdgeWatcher, and the only
// package in this codebase that may import the DynamoDB SDK. It speaks in
// domain structs (Device, PairingSession, Observation, ...) and in
// internal/apierr codes — never in HTTP. Handlers (internal/httpx, cmd/*)
// call this package; this package never imports them.
//
// Key design is fixed by docs/engineering/dynamodb.md, which is the
// deciding document whenever anything else disagrees. keys.go is the only
// place that may concatenate PK/SK/GSI strings — every other file (and every
// caller in later tasks) goes through those helpers.
//
// # Reserved words
//
// "name", "status" and "interval" are DynamoDB reserved words. Any
// expression (KeyConditionExpression, ConditionExpression, FilterExpression,
// UpdateExpression, ProjectionExpression) that references those attributes
// by name must use ExpressionAttributeNames placeholders — "#name",
// "#status", "#interval" — or DynamoDB rejects the request at call time,
// not at compile time. This package's own read methods happen not to filter
// on those attributes (GSI1/GSI2 partition and sort keys are never reserved
// words), but Task 7's writes and conditional updates do touch them
// constantly; this is the single most likely first-day bug in this package,
// so it is called out here rather than left to be rediscovered.
//
// # TTL is never used to decide expiry
//
// The table's only TTL attribute is expiresAt. DynamoDB's TTL sweep lags up
// to 48 hours behind the deadline, so an expired item can still be returned
// by GetItem/Query long after expiresAt has passed. Expiry is always decided
// in code (by comparing expiresAt to the current time) or by a
// ConditionExpression on a write — never by assuming TTL has already
// removed the item. Device.sessionExpiresAt is a session deadline, not a
// TTL attribute, and Device carries no TTL at all: it must never disappear
// on its own.
//
// # Marshalling vs. expressions
//
// Struct <-> item marshalling uses
// github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue via struct
// tags (items.go). Expressions are hand-written strings, not built with the
// expression package: they are short, specified verbatim in
// docs/engineering/dynamodb.md, and a builder would obscure them in review.
package store

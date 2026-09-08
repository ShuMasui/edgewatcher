package store

import (
	"errors"
	"fmt"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// wrapInternal normalizes an unexpected DynamoDB SDK error (network,
// throttling, marshalling, ...) into an *apierr.Error the rest of the
// system can log and render without ever leaking the SDK error into an
// HTTP response body.
func wrapInternal(op string, err error) error {
	return apierr.Wrap(apierr.CodeInternal, "store: "+op+" failed", err)
}

// errStaleGSI2Entry is the cause wrapped when a GSI2 query returns a key
// the base table no longer has — see FindPairingByCode.
var errStaleGSI2Entry = errors.New("store: GSI2 key not present on base table")

// errMissingKeyAttr is the cause wrapped when a query result item is
// missing an expected key attribute (would indicate a schema mismatch
// against docs/engineering/dynamodb.md).
func errMissingKeyAttr(name string) error {
	return fmt.Errorf("store: query result missing key attribute %q", name)
}

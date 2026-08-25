// Package tokens holds the session-token hashing contract shared by every
// package that either issues or verifies a device session
// (docs/06-auth.md §3). It exists as its own neutral package — rather than
// living inside internal/authz — specifically so that the code issuing a
// session (device-auth's POST /device/token, a later task) has no reason
// to import the authorizer to get it right. A helper tucked inside
// internal/authz is exactly the kind of thing a sibling package "forgets"
// to import and reimplements slightly differently, which is how a session
// silently stops validating.
package tokens

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashSessionToken renders token as the lowercase-hex SHA-256 digest
// stored in Device.SessionTokenHash (docs/06-auth.md §3: "ハッシュは
// SHA-256 で足りる" — a 256-bit random token has no offline-guessing
// surface, so a slow KDF buys nothing here, and lowercase-hex is the
// encoding docs/06-auth.md §3 now states explicitly).
//
// Whichever code path issues a session (POST /device/token) MUST hash the
// token with this exact function before calling
// store.RotateSessionToken — any other encoding (base64, raw bytes, ...)
// makes every session internal/authz is asked to validate mismatch and
// deny, which surfaces only as a blanket 401 across every device, not as
// a test failure.
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

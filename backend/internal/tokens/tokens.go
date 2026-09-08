// Package tokens holds the credential-hashing contract shared by every
// package that either issues or verifies a device credential
// (docs/06-auth.md §3): the long-lived deviceSecret and the short-lived
// sessionToken both hash the same way — lowercase-hex SHA-256 — and both
// are stored that way (deviceSecretHash, sessionTokenHash). This package
// exists on its own — rather than living inside internal/authz — so that
// whichever code issues either credential (device-auth's POST /device/pair
// and POST /device/token, later tasks) has an obviously-correct function
// to reach for instead of writing its own sha256+hex and risking a subtly
// different encoding. A helper tucked inside internal/authz is exactly the
// kind of thing a sibling package "forgets" to import and reimplements
// slightly differently, which is how a credential silently stops
// validating.
package tokens

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex is the shared primitive: the lowercase-hex SHA-256 digest of
// v (docs/06-auth.md §3: "ハッシュは SHA-256 で足りる" — a 256-bit random
// value has no offline-guessing surface, so a slow KDF buys nothing here,
// and lowercase-hex is the encoding docs/06-auth.md §3 states explicitly
// for both deviceSecretHash and sessionTokenHash).
//
// HashSessionToken and HashDeviceSecret are named wrappers over this, not
// because the computation differs, but so call sites read as "hashing a
// session token" / "hashing a device secret" rather than a bare
// SHA256Hex(x) that gives no hint which credential it's meant for.
func SHA256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// HashSessionToken renders a sessionToken as the digest stored in
// Device.SessionTokenHash. Whichever code path issues a session
// (POST /device/token) MUST hash the token with this exact function
// before calling store.RotateSessionToken — any other encoding (base64,
// raw bytes, ...) makes every session internal/authz is asked to validate
// mismatch and deny, which surfaces only as a blanket 401 across every
// device, not as a test failure.
func HashSessionToken(token string) string {
	return SHA256Hex(token)
}

// HashDeviceSecret renders a deviceSecret as the digest stored in
// Device.DeviceSecretHash. Whichever code path issues or verifies a
// deviceSecret (POST /device/pair, POST /device/token) MUST hash it with
// this exact function — the same landmine HashSessionToken's doc comment
// warns about applies identically one layer down.
func HashDeviceSecret(secret string) string {
	return SHA256Hex(secret)
}

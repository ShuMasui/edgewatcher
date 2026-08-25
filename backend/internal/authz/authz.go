// Package authz implements the Lambda REQUEST authorizer for the device
// API (docs/06-auth.md §4, docs/05-backend.md §3.2). API Gateway invokes
// it in front of every Lambda-authorized device route
// (POST /device/uploads, POST /device/logout) with
// enable_simple_responses = true and authorizer_result_ttl = 0: every
// single request re-checks DynamoDB, with no caching, so that revoking a
// device's session (disconnect, delete) takes effect on the device's very
// next request rather than after some cache TTL elapses.
//
// The whole security property rests on a single fact: store.Store's
// GetDeviceForAuth issues a ConsistentRead GetItem. This package must
// never introduce a path that reads the Device item any other way.
//
// Failure handling is deliberately uniform: every rejection path —
// missing header, malformed header, unknown device, wrong status, hash
// mismatch, expired session, or even a DynamoDB error — returns
// isAuthorized:false. Authorize never returns a non-nil error, so nothing
// upstream of it (API Gateway, the Lambda runtime) can mistake an
// authorizer bug for an "fail open" allow. See Authorize's doc comment.
package authz

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
	"github.com/ShuMasui/edgewatcher/backend/internal/tokens"
)

// bearerPrefix is an optional, tolerated prefix. docs/06-auth.md §3 and
// docs/05-backend.md §1.1 both specify the device session header as a
// bare token — "Authorization: <sessionToken>" — with no scheme. The
// "Bearer " convention belongs to the separate Cognito/JWT path used by
// the web client (web/src/services/api-client.ts), not this one. Stripping
// an optional "Bearer " prefix, rather than rejecting it, costs nothing:
// a sessionToken is "<deviceId>.<random>" with a ULID deviceId, which can
// never collide with the literal string "Bearer".
const bearerPrefix = "Bearer "

// DeviceGetter is the subset of *store.Store this package depends on. It
// exists so tests can substitute an in-memory fake instead of a real
// DynamoDB-backed Store, without this package importing the AWS SDK.
//
// Deliberately narrow: the deployed authorizer IAM role has
// dynamodb:GetItem only (infra/envs/dev/iam.tf, G3) — there must be no
// method on this interface a strict reading of that policy wouldn't
// allow.
type DeviceGetter interface {
	GetDeviceForAuth(ctx context.Context, deviceID string) (*store.Device, error)
}

// Authorizer evaluates device session tokens.
type Authorizer struct {
	store  DeviceGetter
	clock  clock.Clock
	logger *slog.Logger
}

// New constructs an Authorizer. logger may be nil (Authorize still works;
// nothing gets logged).
func New(s DeviceGetter, c clock.Clock, logger *slog.Logger) *Authorizer {
	return &Authorizer{store: s, clock: c, logger: logger}
}

// deny is the single not-authorized value every rejection path returns.
// Named so every "return deny(...)" call site reads as an explicit,
// deliberate denial rather than an easily-overlooked zero value.
func deny() events.APIGatewayV2CustomAuthorizerSimpleResponse {
	return events.APIGatewayV2CustomAuthorizerSimpleResponse{IsAuthorized: false}
}

// Authorize evaluates an API Gateway HTTP API v2 REQUEST-authorizer event
// and returns a simple response. It never returns a non-nil error: every
// failure mode — including an unexpected store error — is mapped to
// isAuthorized:false so a bug here can only ever deny traffic, never let
// it through fail-open, and API Gateway never has to guess how to treat a
// Go error from an authorizer Lambda.
//
// Authorization requires all of:
//  1. the lowercase "authorization" header is present and holds a
//     non-empty token — the bare token per docs/06-auth.md §3, with an
//     optional "Bearer " prefix tolerated (payload format 2.0 always
//     lowercases header names; there is no "Authorization" key to fall
//     back to on a real request)
//  2. the token's deviceId prefix (docs/06-auth.md §3: sessionToken is
//     "<deviceId>.<random>") resolves to a Device via a single
//     ConsistentRead GetItem
//  3. that Device's status is exactly PAIRED (PENDING/DISCONNECTED/
//     ARCHIVED are all rejected)
//  4. the token's SHA-256 hash matches SessionTokenHash, compared in
//     constant time
//  5. SessionExpiresAt (epoch seconds) is strictly after the current
//     time
func (a *Authorizer) Authorize(ctx context.Context, req events.APIGatewayV2CustomAuthorizerV2Request) (events.APIGatewayV2CustomAuthorizerSimpleResponse, error) {
	token, deviceID, ok := extractToken(req.Headers)
	if !ok {
		a.log(ctx, "authz: rejected", "reason", "missing_or_malformed_header")
		return deny(), nil
	}

	dev, err := a.store.GetDeviceForAuth(ctx, deviceID)
	if err != nil {
		a.log(ctx, "authz: rejected", "reason", "device_lookup_failed", "deviceId", deviceID, "error", err.Error())
		return deny(), nil
	}
	if dev == nil {
		// DeviceGetter is a public interface; store.GetDeviceForAuth never
		// returns (nil, nil) today, but nothing stops another
		// implementation (or a future refactor of this one) from doing
		// so. Without this guard that shape would panic on dev.Status
		// below, turning a deny into a 500 instead.
		a.log(ctx, "authz: rejected", "reason", "device_lookup_nil", "deviceId", deviceID)
		return deny(), nil
	}

	if dev.Status != store.DeviceStatusPaired {
		a.log(ctx, "authz: rejected", "reason", "not_paired", "deviceId", deviceID, "status", dev.Status)
		return deny(), nil
	}

	if !validHash(token, dev.SessionTokenHash) {
		a.log(ctx, "authz: rejected", "reason", "hash_mismatch", "deviceId", deviceID)
		return deny(), nil
	}

	now := a.clock.Now()
	expiresAt := timeFromEpochSeconds(dev.SessionExpiresAt)
	if !expiresAt.After(now) {
		a.log(ctx, "authz: rejected", "reason", "session_expired", "deviceId", deviceID)
		return deny(), nil
	}

	a.log(ctx, "authz: authorized", "deviceId", deviceID, "ownerId", dev.OwnerID)
	return events.APIGatewayV2CustomAuthorizerSimpleResponse{
		IsAuthorized: true,
		Context: map[string]interface{}{
			"deviceId": deviceID,
			"ownerId":  dev.OwnerID,
		},
	}, nil
}

// extractToken reads the session token from the lowercase "authorization"
// header and splits it into (token, deviceId). The primary, documented
// wire format (docs/06-auth.md §3, docs/05-backend.md §1.1) is a bare
// token with no scheme; an optional "Bearer " prefix is tolerated and
// stripped if present, since a real sessionToken ("<deviceId>.<random>"
// with a ULID deviceId) can never equal or start with that literal
// string.
//
// It reports ok=false for a nil header map, a missing key, an empty
// token (after stripping any "Bearer " prefix), or a token with no
// "<deviceId>." prefix to key the GetItem on.
func extractToken(headers map[string]string) (token, deviceID string, ok bool) {
	if headers == nil {
		return "", "", false
	}
	raw, present := headers["authorization"]
	if !present {
		return "", "", false
	}
	token = strings.TrimPrefix(raw, bearerPrefix)
	if token == "" {
		return "", "", false
	}

	deviceID, _, found := strings.Cut(token, ".")
	if !found || deviceID == "" {
		return "", "", false
	}
	return token, deviceID, true
}

// validHash reports whether token hashes to storedHash, compared in
// constant time so response-timing cannot be used to recover the hash
// (and, transitively, brute-force the token) one byte at a time.
// subtle.ConstantTimeCompare reports unequal on a length mismatch too,
// which is output-equivalent to bytes.Equal/== for every input here —
// the timing difference between the two is real but is not something a
// functional test can observe (see authz_test.go's note on this).
//
// The storedHash == "" guard is defensive rather than load-bearing today
// (a disconnected/archived device has no hash and already fails the
// PAIRED check above), but it is pinned by
// TestAuthorize_DisconnectedDeviceEmptyHash: an empty stored hash must
// never validate against anything, including another empty string,
// regardless of what future refactors do to the caller's ordering of
// checks.
func validHash(token, storedHash string) bool {
	if storedHash == "" {
		return false
	}
	got := tokens.HashSessionToken(token)
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}

// timeFromEpochSeconds converts Device.SessionExpiresAt (epoch seconds) to
// a time.Time so expiry is decided by comparing instants, never by string
// comparison — SessionExpiresAt is stored as a number precisely to avoid
// the RFC3339 lexicographic trap docs/engineering/dynamodb.md warns about
// elsewhere in this codebase (formatISO's doc comment in
// internal/store/device.go).
func timeFromEpochSeconds(sec int64) time.Time {
	return time.Unix(sec, 0)
}

// log writes a structured line through the shared logger, which is safe
// by construction: internal/logging.New's ReplaceAttr redacts
// deviceSecret/sessionToken/pairingCode/signedUrl by key, and this
// package's call sites never pass the raw token or hash under any key —
// only deviceId, ownerId, status, and a fixed reason string. A nil logger
// (used by some tests) is a silent no-op.
func (a *Authorizer) log(ctx context.Context, msg string, args ...any) {
	if a.logger == nil {
		return
	}
	a.logger.InfoContext(ctx, msg, args...)
}

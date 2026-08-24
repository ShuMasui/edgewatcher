// Package ids provides ID and secret generation, plus the JST day-boundary
// math used by the observation-history query (docs/engineering/dynamodb.md
// §5.2).
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// JST is Asia/Tokyo, used for observation-day boundaries. It's a fixed
// UTC+9 offset rather than a tzdata-backed *time.Location: Japan has no
// DST, and provided.al2023 Lambdas aren't guaranteed to ship system
// zoneinfo, so a fixed zone avoids depending on it.
//
// web/src/utils/date.ts's formatDateParam() builds the `date=` query
// parameter from the browser's local year/month/day; computing day
// boundaries in UTC here would push observations captured 00:00–09:00 JST
// onto the previous day's page.
var JST = time.FixedZone("Asia/Tokyo", 9*60*60)

const obsPrefix = "OBS#"

// entropyLen is the ULID entropy portion: 80 bits = 16 Crockford base32
// characters.
const entropyLen = 16

// DayRange returns the inclusive SK bounds (each prefixed "OBS#") covering
// one full JST calendar day for dateStr, formatted "YYYY-MM-DD" (the shape
// produced by formatDateParam).
//
// The upper bound is that day's 23:59:59.999 JST, not the next day's
// midnight: DynamoDB's SK BETWEEN is inclusive, so using next-midnight as
// the upper timestamp would admit the next day's first millisecond too.
func DayRange(dateStr string) (lower, upper string, err error) {
	day, err := time.ParseInLocation("2006-01-02", dateStr, JST)
	if err != nil {
		return "", "", fmt.Errorf("ids: invalid date %q: %w", dateStr, err)
	}

	start := day
	end := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 999_000_000, JST)

	lower = obsPrefix + ulidBound(start, 0x00)
	upper = obsPrefix + ulidBound(end, 0xFF)
	return lower, upper, nil
}

// ulidBound encodes a ULID string whose timestamp component is t and
// whose entropy component is entropyLen bytes of fill repeated — 0x00
// yields the all-'0' minimum, 0xFF the all-'Z' maximum, since each base32
// character is 5 bits and 0xFF's low 5 bits are all set.
func ulidBound(t time.Time, fill byte) string {
	var id ulid.ULID
	// SetTime/SetEntropy only fail if the timestamp overflows 48 bits or
	// the entropy slice isn't exactly 10 bytes; neither can happen here.
	_ = id.SetTime(ulid.Timestamp(t))

	entropy := make([]byte, 10)
	for i := range entropy {
		entropy[i] = fill
	}
	_ = id.SetEntropy(entropy)

	return id.String()
}

// NewULID generates a new ULID string for the given time, using
// crypto/rand for its entropy. Used for deviceId and observationId, which
// docs/engineering/dynamodb.md §5.2 requires to be the same ID scheme so
// their string order matches creation order.
func NewULID(t time.Time) (string, error) {
	id, err := ulid.New(ulid.Timestamp(t), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("ids: generate ulid: %w", err)
	}
	return id.String(), nil
}

// pairingCodeBytes is 256 bits of randomness (docs/engineering/dynamodb.md
// §3, the deciding document over docs/06-auth.md §2's 128-bit claim).
const pairingCodeBytes = 32

var pairingCodeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewPairingCode returns 256 bits of crypto/rand randomness, base32
// encoded without padding. Base32 fits QR alphanumeric mode.
func NewPairingCode() (string, error) {
	buf := make([]byte, pairingCodeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ids: generate pairing code: %w", err)
	}
	return pairingCodeEncoding.EncodeToString(buf), nil
}

// deviceSecretBytes matches pairingCodeBytes: 256 bits, no documented
// reason to differ.
const deviceSecretBytes = 32

// NewDeviceSecret returns 256 bits of crypto/rand randomness, base32
// encoded without padding, for use as a device's long-lived credential.
func NewDeviceSecret() (string, error) {
	buf := make([]byte, deviceSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ids: generate device secret: %w", err)
	}
	return pairingCodeEncoding.EncodeToString(buf), nil
}

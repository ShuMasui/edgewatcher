package ids

import (
	"strings"
	"testing"
	"time"
)

func TestDayRangeCoversJSTDayWithMinAndMaxEntropy(t *testing.T) {
	lower, upper, err := DayRange("2026-08-25")
	if err != nil {
		t.Fatalf("DayRange returned error: %v", err)
	}

	if !strings.HasPrefix(lower, obsPrefix) || !strings.HasPrefix(upper, obsPrefix) {
		t.Fatalf("bounds must be prefixed %q: lower=%q upper=%q", obsPrefix, lower, upper)
	}

	lowerTail := lower[len(lower)-entropyLen:]
	upperTail := upper[len(upper)-entropyLen:]

	if lowerTail != strings.Repeat("0", entropyLen) {
		t.Errorf("lower bound entropy = %q, want 16 zeros", lowerTail)
	}
	if upperTail != strings.Repeat("Z", entropyLen) {
		t.Errorf("upper bound entropy = %q, want 16 'Z's", upperTail)
	}

	// Full ULID = "OBS#" + 26 chars (10 timestamp + 16 entropy).
	if len(lower) != len(obsPrefix)+26 {
		t.Errorf("lower length = %d, want %d", len(lower), len(obsPrefix)+26)
	}
	if len(upper) != len(obsPrefix)+26 {
		t.Errorf("upper length = %d, want %d", len(upper), len(obsPrefix)+26)
	}

	if lower >= upper {
		t.Errorf("lower bound must sort before upper bound: lower=%q upper=%q", lower, upper)
	}
}

func TestDayRangeUpperBoundIsSameDay235959999NotNextMidnight(t *testing.T) {
	_, upper, err := DayRange("2026-08-25")
	if err != nil {
		t.Fatalf("DayRange returned error: %v", err)
	}

	endOfDay := time.Date(2026, 8, 25, 23, 59, 59, 999_000_000, JST)
	nextMidnight := time.Date(2026, 8, 26, 0, 0, 0, 0, JST)

	wantUpper := obsPrefix + ulidBound(endOfDay, 0xFF)
	nextMidnightBound := obsPrefix + ulidBound(nextMidnight, 0xFF)

	if upper != wantUpper {
		t.Errorf("upper = %q, want %q (this day's 23:59:59.999)", upper, wantUpper)
	}
	if upper == nextMidnightBound {
		t.Errorf("upper bound must not equal next day's midnight bound")
	}
	if upper >= nextMidnightBound {
		t.Errorf("upper bound %q must sort strictly before next midnight's bound %q, else the next day's first millisecond leaks in", upper, nextMidnightBound)
	}
}

func TestDayRangeRejectsMalformedDate(t *testing.T) {
	cases := []string{"", "2026/08/25", "not-a-date", "2026-13-01"}
	for _, s := range cases {
		if _, _, err := DayRange(s); err == nil {
			t.Errorf("DayRange(%q) should have returned an error", s)
		}
	}
}

func TestDayRangeUsesJSTNotUTC(t *testing.T) {
	// 2026-08-25 00:30 JST is 2026-08-24 15:30 UTC. A UTC-based
	// implementation would compute the wrong day's bounds here.
	lower, _, err := DayRange("2026-08-25")
	if err != nil {
		t.Fatalf("DayRange returned error: %v", err)
	}

	wantStart := time.Date(2026, 8, 25, 0, 0, 0, 0, JST)
	wantLower := obsPrefix + ulidBound(wantStart, 0x00)
	if lower != wantLower {
		t.Errorf("lower = %q, want %q", lower, wantLower)
	}

	if wantStart.UTC().Hour() != 15 || wantStart.UTC().Day() != 24 {
		t.Fatalf("test assumption about JST offset is wrong: %v", wantStart.UTC())
	}
}

func TestNewULIDIsSortableByCreationTime(t *testing.T) {
	t1 := time.Now()
	id1, err := NewULID(t1)
	if err != nil {
		t.Fatalf("NewULID returned error: %v", err)
	}

	t2 := t1.Add(time.Second)
	id2, err := NewULID(t2)
	if err != nil {
		t.Fatalf("NewULID returned error: %v", err)
	}

	if len(id1) != 26 || len(id2) != 26 {
		t.Fatalf("ULID length = %d/%d, want 26", len(id1), len(id2))
	}
	if id1 >= id2 {
		t.Errorf("ULID string order should match creation order: id1=%q id2=%q", id1, id2)
	}
}

func TestNewULIDIsNotDeterministic(t *testing.T) {
	now := time.Now()
	id1, err := NewULID(now)
	if err != nil {
		t.Fatalf("NewULID returned error: %v", err)
	}
	id2, err := NewULID(now)
	if err != nil {
		t.Fatalf("NewULID returned error: %v", err)
	}
	if id1 == id2 {
		t.Errorf("two ULIDs generated at the same instant should differ in entropy, got identical %q", id1)
	}
}

func TestNewPairingCodeIs256BitsBase32NoPadding(t *testing.T) {
	code, err := NewPairingCode()
	if err != nil {
		t.Fatalf("NewPairingCode returned error: %v", err)
	}
	if strings.Contains(code, "=") {
		t.Errorf("pairing code must not be padded: %q", code)
	}
	// 256 bits / 5 bits-per-base32-char = 51.2 -> 52 chars unpadded.
	if len(code) != 52 {
		t.Errorf("len(code) = %d, want 52 (256 bits base32-encoded)", len(code))
	}
}

func TestNewPairingCodeIsRandomEachCall(t *testing.T) {
	a, err := NewPairingCode()
	if err != nil {
		t.Fatalf("NewPairingCode returned error: %v", err)
	}
	b, err := NewPairingCode()
	if err != nil {
		t.Fatalf("NewPairingCode returned error: %v", err)
	}
	if a == b {
		t.Errorf("two pairing codes should not collide: %q", a)
	}
}

// TestNewSessionToken_Shape pins the format the Lambda Authorizer parses:
// the deviceId, one ".", then the random half. The authorizer splits on the
// FIRST "." and treats everything before it as the deviceId, so the random
// half must not introduce another one — base32 guarantees that, and this
// test would catch a switch to an encoding (base64url's "-_" is fine,
// standard base64's "+/=" is not) that changed the alphabet.
func TestNewSessionToken_Shape(t *testing.T) {
	got, err := NewSessionToken("01J0DEVICE")
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	deviceID, random, found := strings.Cut(got, ".")
	if !found {
		t.Fatalf("token %q has no separator", got)
	}
	if deviceID != "01J0DEVICE" {
		t.Errorf("deviceId = %q, want %q", deviceID, "01J0DEVICE")
	}
	if strings.Contains(random, ".") {
		t.Errorf("random half %q contains a second separator", random)
	}
	// 16 bytes of base32 without padding is ceil(128/5) = 26 characters.
	if len(random) != 26 {
		t.Errorf("random half = %q (%d chars), want 26 (128 bits of base32)", random, len(random))
	}
}

// TestNewSessionToken_Unique guards the one property that makes rotation
// meaningful: two calls must not produce the same token, or a "rotated"
// session would still accept the value it was supposed to invalidate.
func TestNewSessionToken_Unique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		tok, err := NewSessionToken("01J0DEVICE")
		if err != nil {
			t.Fatalf("NewSessionToken: %v", err)
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q after %d draws", tok, i)
		}
		seen[tok] = true
	}
}

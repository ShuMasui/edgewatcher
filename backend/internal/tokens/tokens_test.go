package tokens

import "testing"

func TestHashSessionToken_Deterministic(t *testing.T) {
	if HashSessionToken("abc") != HashSessionToken("abc") {
		t.Fatal("HashSessionToken must be deterministic for the same input")
	}
	if HashSessionToken("abc") == HashSessionToken("abd") {
		t.Fatal("HashSessionToken must differ for different inputs")
	}
}

func TestHashDeviceSecret_Deterministic(t *testing.T) {
	if HashDeviceSecret("abc") != HashDeviceSecret("abc") {
		t.Fatal("HashDeviceSecret must be deterministic for the same input")
	}
	if HashDeviceSecret("abc") == HashDeviceSecret("abd") {
		t.Fatal("HashDeviceSecret must differ for different inputs")
	}
}

// TestHashSessionToken_LowercaseHex and TestHashDeviceSecret_LowercaseHex
// pin the encoding contract docs/06-auth.md §3 states for both
// credentials: a 64-character lowercase-hex SHA-256 digest.
func TestHashSessionToken_LowercaseHex(t *testing.T) {
	assertLowercaseHexDigest(t, "HashSessionToken", HashSessionToken("abc"))
}

func TestHashDeviceSecret_LowercaseHex(t *testing.T) {
	assertLowercaseHexDigest(t, "HashDeviceSecret", HashDeviceSecret("abc"))
}

func TestSHA256Hex_LowercaseHex(t *testing.T) {
	assertLowercaseHexDigest(t, "SHA256Hex", SHA256Hex("abc"))
}

// TestWrappers_MatchSHA256Hex pins that HashSessionToken and
// HashDeviceSecret are exactly SHA256Hex under different names, not an
// independently-drifting computation.
func TestWrappers_MatchSHA256Hex(t *testing.T) {
	if HashSessionToken("some-value") != SHA256Hex("some-value") {
		t.Fatal("HashSessionToken must equal SHA256Hex for the same input")
	}
	if HashDeviceSecret("some-value") != SHA256Hex("some-value") {
		t.Fatal("HashDeviceSecret must equal SHA256Hex for the same input")
	}
}

func assertLowercaseHexDigest(t *testing.T, fn, got string) {
	t.Helper()
	if len(got) != 64 {
		t.Fatalf("%s: expected a 64-character hex digest, got %d chars: %q", fn, len(got), got)
	}
	for _, r := range got {
		isLowerHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isLowerHex {
			t.Fatalf("%s: expected lowercase hex only, got char %q in %q", fn, r, got)
		}
	}
}

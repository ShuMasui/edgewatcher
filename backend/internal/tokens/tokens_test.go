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

func TestHashSessionToken_LowercaseHex(t *testing.T) {
	got := HashSessionToken("abc")
	if len(got) != 64 {
		t.Fatalf("expected a 64-character hex digest, got %d chars: %q", len(got), got)
	}
	for _, r := range got {
		isLowerHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isLowerHex {
			t.Fatalf("expected lowercase hex only, got char %q in %q", r, got)
		}
	}
}

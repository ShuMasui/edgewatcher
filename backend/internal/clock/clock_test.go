package clock

import (
	"testing"
	"time"
)

func TestFixedClockAlwaysReturnsSameInstant(t *testing.T) {
	at := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	c := Fixed{At: at}

	if got := c.Now(); !got.Equal(at) {
		t.Errorf("Now() = %v, want %v", got, at)
	}
	if got := c.Now(); !got.Equal(at) {
		t.Errorf("second call Now() = %v, want %v", got, at)
	}
}

func TestRealClockReturnsCurrentTime(t *testing.T) {
	before := time.Now()
	got := Real{}.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, want between %v and %v", got, before, after)
	}
}

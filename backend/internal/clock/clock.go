// Package clock provides an injectable source of "now" so handlers and
// their tests never call time.Now() directly. That matters most for the
// conditional-update logic in later tasks (e.g. the latestCapturedAt
// comparison in docs/engineering/dynamodb.md §5.1), which needs a fixed
// clock to test deterministically.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// Real is the production Clock, backed by time.Now.
type Real struct{}

func (Real) Now() time.Time { return time.Now() }

// Fixed is a Clock that always returns the same instant. Useful in tests
// that need deterministic timestamps.
type Fixed struct {
	At time.Time
}

func (f Fixed) Now() time.Time { return f.At }

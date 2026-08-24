// Package logging configures the structured JSON logger every Lambda uses.
//
// Never log deviceSecret, sessionToken, pairingCode, or signed URLs — a
// signed URL is itself a time-limited authorization, so logging one leaks
// it for the log group's retention period (30 days). Log lines should
// instead carry requestId, deviceId, ownerId, and the error code.
//
// That discipline is mostly a call-site concern: a signed URL passed as
// slog.String("url", u) can't be caught structurally, since nothing marks
// the value itself as sensitive. What New does guard against is the more
// mechanical mistake of logging one of these fields under its own name —
// redactedKeys below is checked against every attribute key, regardless of
// call site.
package logging

import (
	"log/slog"
	"os"
)

// redactedKeys are attribute keys whose values are replaced with
// "[REDACTED]" instead of being written out, no matter which call site
// produced them.
var redactedKeys = map[string]bool{
	"deviceSecret": true,
	"sessionToken": true,
	"pairingCode":  true,
	"signedUrl":    true,
}

// New returns a slog.Logger writing structured JSON to stdout, which
// CloudWatch Logs picks up as one log group per function
// (infra/modules/lambda creates it with a 30-day retention).
func New() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: redact,
	})
	return slog.New(handler)
}

func redact(groups []string, a slog.Attr) slog.Attr {
	if redactedKeys[a.Key] {
		return slog.String(a.Key, "[REDACTED]")
	}
	return a
}

// Package logging configures the structured JSON logger every Lambda uses.
//
// Never log deviceSecret, sessionToken, pairingCode, or signed URLs — a
// signed URL is itself a time-limited authorization, so logging one leaks
// it for the log group's retention period (30 days). Log lines should
// instead carry requestId, deviceId, ownerId, and the error code.
package logging

import (
	"log/slog"
	"os"
)

// New returns a slog.Logger writing structured JSON to stdout, which
// CloudWatch Logs picks up as one log group per function
// (infra/modules/lambda creates it with a 30-day retention).
func New() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// newTestLogger builds the same JSON handler New does, but writing to buf
// instead of os.Stdout, so the test can inspect the emitted line.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: redact,
	})
	return slog.New(handler)
}

func TestRedactsKnownSensitiveKeys(t *testing.T) {
	secrets := map[string]string{
		"deviceSecret": "top-secret-device-value",
		"sessionToken": "top-secret-session-value",
		"pairingCode":  "top-secret-pairing-value",
		"signedUrl":    "https://example.com/secret?X-Amz-Signature=abc",
	}

	for key, value := range secrets {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf)
			logger.Info("something happened", key, value)

			line := buf.String()
			if strings.Contains(line, value) {
				t.Fatalf("log line leaked sensitive value for key %q: %s", key, line)
			}

			var parsed map[string]any
			if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
				t.Fatalf("log line is not valid JSON: %v\nline=%s", err, line)
			}
			if parsed[key] != "[REDACTED]" {
				t.Errorf("parsed[%q] = %v, want [REDACTED]", key, parsed[key])
			}
		})
	}
}

func TestDoesNotRedactSafeKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("request handled", "requestId", "req-1", "deviceId", "dev-1", "ownerId", "owner-1")

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	if parsed["requestId"] != "req-1" || parsed["deviceId"] != "dev-1" || parsed["ownerId"] != "owner-1" {
		t.Errorf("safe keys should pass through unredacted, got %v", parsed)
	}
}

func TestNewProducesAWorkingLogger(t *testing.T) {
	if logger := New(); logger == nil {
		t.Fatal("New returned nil")
	}
}

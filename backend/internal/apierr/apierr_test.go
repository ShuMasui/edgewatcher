package apierr

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestStatusRegistry(t *testing.T) {
	cases := map[Code]int{
		CodeValidation:          http.StatusBadRequest,
		CodeUnauthorized:        http.StatusUnauthorized,
		CodeForbidden:           http.StatusForbidden,
		CodeDeviceNotFound:      http.StatusNotFound,
		CodeObservationNotFound: http.StatusNotFound,
		CodePairingNotFound:     http.StatusNotFound,
		CodePairingCodeExpired:  http.StatusConflict,
		CodePairingCodeConsumed: http.StatusConflict,
		CodeDeviceLimitExceeded: http.StatusTooManyRequests,
		CodeInternal:            http.StatusInternalServerError,
		CodeRouteNotWired:       http.StatusInternalServerError,
	}
	for code, want := range cases {
		if got := Status(code); got != want {
			t.Errorf("Status(%s) = %d, want %d", code, got, want)
		}
	}
}

func TestStatusUnknownCodeDefaultsTo500(t *testing.T) {
	if got := Status(Code("SOMETHING_MADE_UP")); got != http.StatusInternalServerError {
		t.Errorf("Status(unknown) = %d, want 500", got)
	}
}

func TestWriteErrorBodyExactShape(t *testing.T) {
	status, body := Body(New(CodeDeviceLimitExceeded, "端末の上限に達しています"))

	if status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", status)
	}

	want := `{"error":{"code":"DEVICE_LIMIT_EXCEEDED","message":"端末の上限に達しています"}}`
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}

	// Round-trip through the same struct the web client parses to be sure
	// the shape (not just these particular bytes) is right.
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("body did not parse as ApiErrorPayload: %v", err)
	}
	if parsed.Error.Code != "DEVICE_LIMIT_EXCEEDED" {
		t.Errorf("parsed code = %s", parsed.Error.Code)
	}
}

func TestUnknownErrorBecomes500WithoutLeakingCause(t *testing.T) {
	secret := "arn:aws:s3:::edgewatcher-images/very-secret-signed-url?X-Amz-Signature=abcdef"
	cause := errors.New("dynamodb: ConditionalCheckFailedException: " + secret)

	status, body := Body(cause)

	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if strings.Contains(string(body), secret) {
		t.Fatalf("response body leaked the underlying cause: %s", body)
	}
	if strings.Contains(string(body), "dynamodb") {
		t.Fatalf("response body leaked implementation detail: %s", body)
	}

	want := `{"error":{"code":"INTERNAL_ERROR","message":"internal error"}}`
	if string(body) != want {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestAsErrorPassesThroughKnownErrors(t *testing.T) {
	original := New(CodeForbidden, "許可されていません")
	wrapped := AsError(original)
	if wrapped != original {
		t.Fatalf("AsError should return the same *Error unchanged")
	}
}

func TestWrapPreservesCauseForLoggingOnly(t *testing.T) {
	cause := errors.New("boom")
	err := Wrap(CodeInternal, "internal error", cause)

	if !errors.Is(err, cause) && errors.Unwrap(err) != cause {
		t.Fatalf("Wrap should preserve cause via Unwrap")
	}

	_, body := Body(err)
	if strings.Contains(string(body), "boom") {
		t.Fatalf("Body leaked wrapped cause: %s", body)
	}
}

func TestWriteErrorProducesJSONContentType(t *testing.T) {
	resp := WriteError(New(CodePairingCodeExpired, "QRコードの有効期限が切れています"))

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("StatusCode = %d, want 409", resp.StatusCode)
	}
	if ct := resp.Headers["content-type"]; ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	// No CORS headers: the HTTP API's cors_configuration already injects
	// them, and duplicates make browsers reject the response.
	for k := range resp.Headers {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			t.Errorf("unexpected CORS header set by WriteError: %s", k)
		}
	}
}

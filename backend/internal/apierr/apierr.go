// Package apierr defines the error contract shared with the web client.
//
// The wire shape is fixed by web/src/types/api.ts (ApiErrorPayload) and
// parsed by ApiClient.request in web/src/services/api-client.ts:
//
//	{"error":{"code":"...","message":"..."}}
//
// Every HTTP route in this backend must produce exactly this body on
// failure. Handlers should never return a bare error to the Lambda
// runtime — that yields a headerless 502 the web client cannot parse
// (internal/httpx.Router exists specifically to prevent that).
package apierr

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
)

// Code is the machine-readable identifier the web client branches on.
type Code string

// Registry fixed by docs/05-backend.md §1.6 and the task brief.
const (
	CodeValidation          Code = "VALIDATION_ERROR"
	CodeUnauthorized        Code = "UNAUTHORIZED"
	CodeForbidden           Code = "FORBIDDEN"
	CodeDeviceNotFound      Code = "DEVICE_NOT_FOUND"
	CodeObservationNotFound Code = "OBSERVATION_NOT_FOUND"
	CodePairingNotFound     Code = "PAIRING_NOT_FOUND"
	CodePairingCodeExpired  Code = "PAIRING_CODE_EXPIRED"
	CodePairingCodeConsumed Code = "PAIRING_CODE_CONSUMED"
	CodeDeviceLimitExceeded Code = "DEVICE_LIMIT_EXCEEDED"
	CodeInternal            Code = "INTERNAL_ERROR"

	// CodeRouteNotWired means req.RouteKey matched no registered handler.
	// That can only happen if Terraform's route table and the Go router
	// have drifted apart — it is a deploy-time bug, not a client error,
	// so it maps to 500 like everything else unexpected.
	CodeRouteNotWired Code = "ROUTE_NOT_WIRED"
)

var statusByCode = map[Code]int{
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

// Status returns the HTTP status for code. Unknown codes — which should
// never occur if callers only construct *Error via New/Wrap with the
// constants above — default to 500 rather than panicking or leaking 0.
func Status(code Code) int {
	if s, ok := statusByCode[code]; ok {
		return s
	}
	return http.StatusInternalServerError
}

// Error is the type every handler should return on failure. cause is the
// real underlying error (if any); it is available to callers via Unwrap
// for logging, but it is never serialized into the response body.
type Error struct {
	Code    Code
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return string(e.Code) + ": " + e.Message + ": " + e.cause.Error()
	}
	return string(e.Code) + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.cause }

// New constructs an *Error with no wrapped cause.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap constructs an *Error carrying cause for logging. cause never
// reaches the client — only code and message are serialized.
func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}

// AsError normalizes any error into *Error. Errors already of that type
// (including wrapped ones, via errors.As) pass through unchanged; every
// other error becomes CodeInternal with a generic message, so an
// unrecognized cause never leaks into the response body — only the log
// line written by the caller should carry err.Error().
func AsError(err error) *Error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return &Error{Code: CodeInternal, Message: "internal error", cause: err}
}

// payload mirrors web/src/types/api.ts's ApiErrorPayload byte for byte.
type payload struct {
	Error payloadBody `json:"error"`
}

type payloadBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const fallbackBody = `{"error":{"code":"INTERNAL_ERROR","message":"internal error"}}`

// Body renders err as (status, JSON body). Any error not already an
// *Error is normalized to CodeInternal first, so the cause never appears
// in the returned bytes.
func Body(err error) (int, []byte) {
	apiErr := AsError(err)
	status := Status(apiErr.Code)

	b, marshalErr := json.Marshal(payload{Error: payloadBody{
		Code:    string(apiErr.Code),
		Message: apiErr.Message,
	}})
	if marshalErr != nil {
		// The payload shape is static; this should be unreachable. Fall
		// back to a literal so a response body is always well-formed.
		return http.StatusInternalServerError, []byte(fallbackBody)
	}
	return status, b
}

// WriteError renders err into a complete API Gateway v2 HTTP response.
// No CORS headers are set — the HTTP API's cors_configuration already
// injects them, and duplicates make browsers reject the response.
func WriteError(err error) events.APIGatewayV2HTTPResponse {
	status, body := Body(err)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"content-type": "application/json"},
		Body:       string(body),
	}
}

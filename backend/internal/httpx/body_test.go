package httpx

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

func TestRawBody_Plain(t *testing.T) {
	got, err := RawBody(events.APIGatewayV2HTTPRequest{Body: `{"a":1}`})
	if err != nil {
		t.Fatalf("RawBody: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("body = %q, want %q", got, `{"a":1}`)
	}
}

// TestRawBody_Base64 is the case that actually matters. API Gateway
// base64-encodes any body whose content type is not in its text list —
// which is every multipart upload device-api receives (docs/05-backend.md
// §1.3). Reading req.Body directly there yields base64 gibberish that a
// multipart parser rejects with a confusing "no boundary" error.
func TestRawBody_Base64(t *testing.T) {
	raw := []byte{0xFF, 0xD8, 0xFF, 0xE0, 'h', 'i'}
	got, err := RawBody(events.APIGatewayV2HTTPRequest{
		Body:            base64.StdEncoding.EncodeToString(raw),
		IsBase64Encoded: true,
	})
	if err != nil {
		t.Fatalf("RawBody: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("body = %v, want %v", got, raw)
	}
}

// TestRawBody_BadBase64 must be a 400, not a 500: the encoding is chosen by
// the caller's content type, so a body that claims base64 and isn't one is
// a malformed request.
func TestRawBody_BadBase64(t *testing.T) {
	_, err := RawBody(events.APIGatewayV2HTTPRequest{Body: "!!!not base64!!!", IsBase64Encoded: true})
	assertCode(t, err, apierr.CodeValidation)
}

func TestDecodeJSON_Valid(t *testing.T) {
	var body struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(events.APIGatewayV2HTTPRequest{Body: `{"name":"玄関"}`}, &body); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if body.Name != "玄関" {
		t.Errorf("Name = %q, want %q", body.Name, "玄関")
	}
}

// TestDecodeJSON_Malformed: a body that is not JSON is the client's fault.
// Letting json.Unmarshal's error escape as an unclassified error would make
// the router log it as an internal error and answer 500, telling the client
// to retry something that can never succeed (docs/05-backend.md §1.6).
func TestDecodeJSON_Malformed(t *testing.T) {
	var body struct{}
	err := DecodeJSON(events.APIGatewayV2HTTPRequest{Body: `{ not json`}, &body)
	assertCode(t, err, apierr.CodeValidation)
}

// TestDecodeJSON_Empty is called out separately because an empty body is
// the shape a client sends when it forgot the payload entirely, and
// json.Unmarshal("") produces an "unexpected end of JSON input" that reads
// as a parse bug rather than a missing body.
func TestDecodeJSON_Empty(t *testing.T) {
	var body struct{}
	err := DecodeJSON(events.APIGatewayV2HTTPRequest{Body: ""}, &body)
	assertCode(t, err, apierr.CodeValidation)
}

func assertCode(t *testing.T, err error, want apierr.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with code %s, got nil", want)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *apierr.Error", err)
	}
	if apiErr.Code != want {
		t.Fatalf("code = %s, want %s", apiErr.Code, want)
	}
}

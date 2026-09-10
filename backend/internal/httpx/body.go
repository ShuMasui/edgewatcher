package httpx

import (
	"encoding/base64"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
)

// RawBody returns the request body's actual bytes.
//
// API Gateway hands Lambda the body base64-encoded whenever the content
// type is not one it considers text — which covers every multipart upload
// device-api receives (docs/05-backend.md §1.3) — and sets IsBase64Encoded
// to say so. Reading req.Body directly is correct for JSON and silently
// wrong for uploads, which is exactly the kind of difference that survives
// unit tests and fails in the deployed function, so every body in this
// codebase goes through here.
//
// A body that claims to be base64 and is not is the caller's error, not
// ours: it is reported as a 400 rather than escaping unclassified into the
// router's 500 path.
func RawBody(req events.APIGatewayV2HTTPRequest) ([]byte, error) {
	if !req.IsBase64Encoded {
		return []byte(req.Body), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		return nil, badRequest("リクエスト本文を読み取れません", err)
	}
	return decoded, nil
}

// DecodeJSON unmarshals the request body into v.
//
// Both failure modes it classifies — a malformed body and an empty one —
// are permanently unfixable by retrying, so they must reach the client as
// 400 (docs/05-backend.md §1.6). An unclassified error would instead be
// logged as an internal fault and answered 500, which tells a device to
// back off and retry forever.
func DecodeJSON(req events.APIGatewayV2HTTPRequest, v any) error {
	raw, err := RawBody(req)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return badRequest("リクエスト本文が空です", nil)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return badRequest("リクエスト本文の形式が不正です", err)
	}
	return nil
}

package httpx

import (
	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// OwnerID extracts the Cognito sub from the JWT authorizer context
// (web-api routes only — API Gateway attaches this after validating the
// token, per infra/modules/api's JWT authorizer).
//
// A missing sub can only mean misconfiguration, but it must never
// silently become an empty ownerId: that would either match nothing, or
// worse, match rows written with an empty owner. So this errors instead
// of returning "".
func OwnerID(req events.APIGatewayV2HTTPRequest) (string, error) {
	authorizer := req.RequestContext.Authorizer
	if authorizer == nil || authorizer.JWT == nil {
		return "", apierr.New(apierr.CodeUnauthorized, "認証されていません")
	}
	sub, ok := authorizer.JWT.Claims["sub"]
	if !ok || sub == "" {
		return "", apierr.New(apierr.CodeUnauthorized, "認証されていません")
	}
	return sub, nil
}

// DeviceIdentity is what the Lambda authorizer places in the request
// context for device-api routes (infra's authorizer module sets
// enable_simple_responses = true, which is what makes this context map
// available here instead of only in an IAM policy).
type DeviceIdentity struct {
	DeviceID string
	OwnerID  string
}

// ExtractDeviceIdentity reads deviceId/ownerId out of
// RequestContext.Authorizer.Lambda, both delivered as `any` and requiring
// a type assertion.
func ExtractDeviceIdentity(req events.APIGatewayV2HTTPRequest) (DeviceIdentity, error) {
	authorizer := req.RequestContext.Authorizer
	if authorizer == nil || authorizer.Lambda == nil {
		return DeviceIdentity{}, apierr.New(apierr.CodeUnauthorized, "認証されていません")
	}

	deviceID, ok := authorizer.Lambda["deviceId"].(string)
	if !ok || deviceID == "" {
		return DeviceIdentity{}, apierr.New(apierr.CodeUnauthorized, "認証されていません")
	}

	ownerID, ok := authorizer.Lambda["ownerId"].(string)
	if !ok || ownerID == "" {
		return DeviceIdentity{}, apierr.New(apierr.CodeUnauthorized, "認証されていません")
	}

	return DeviceIdentity{DeviceID: deviceID, OwnerID: ownerID}, nil
}

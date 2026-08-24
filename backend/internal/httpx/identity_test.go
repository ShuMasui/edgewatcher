package httpx

import (
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

func TestOwnerIDReadsSubFromJWTClaims(t *testing.T) {
	req := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
				JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
					Claims: map[string]string{"sub": "user-123"},
				},
			},
		},
	}

	got, err := OwnerID(req)
	if err != nil {
		t.Fatalf("OwnerID returned error: %v", err)
	}
	if got != "user-123" {
		t.Errorf("OwnerID = %q, want user-123", got)
	}
}

func TestOwnerIDErrorsWhenSubMissing(t *testing.T) {
	cases := []struct {
		name string
		req  events.APIGatewayV2HTTPRequest
	}{
		{"no authorizer", events.APIGatewayV2HTTPRequest{}},
		{"no jwt block", events.APIGatewayV2HTTPRequest{
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{},
			},
		}},
		{"claims without sub", events.APIGatewayV2HTTPRequest{
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
					JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
						Claims: map[string]string{"email": "a@example.com"},
					},
				},
			},
		}},
		{"empty sub", events.APIGatewayV2HTTPRequest{
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
					JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
						Claims: map[string]string{"sub": ""},
					},
				},
			},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OwnerID(tc.req)
			if err == nil {
				t.Fatalf("expected error, got ownerId=%q", got)
			}
			if got != "" {
				t.Fatalf("OwnerID must return empty string alongside an error, got %q", got)
			}
			var apiErr *apierr.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *apierr.Error, got %T", err)
			}
			if apiErr.Code != apierr.CodeUnauthorized {
				t.Errorf("code = %s, want UNAUTHORIZED", apiErr.Code)
			}
		})
	}
}

func TestExtractDeviceIdentityReadsLambdaAuthorizerContext(t *testing.T) {
	req := events.APIGatewayV2HTTPRequest{
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
				Lambda: map[string]interface{}{
					"deviceId": "dev-abc",
					"ownerId":  "owner-xyz",
				},
			},
		},
	}

	got, err := ExtractDeviceIdentity(req)
	if err != nil {
		t.Fatalf("ExtractDeviceIdentity returned error: %v", err)
	}
	if got.DeviceID != "dev-abc" || got.OwnerID != "owner-xyz" {
		t.Errorf("got %+v", got)
	}
}

func TestExtractDeviceIdentityErrorsOnMissingOrWrongType(t *testing.T) {
	cases := []struct {
		name string
		req  events.APIGatewayV2HTTPRequest
	}{
		{"no authorizer", events.APIGatewayV2HTTPRequest{}},
		{"no lambda block", events.APIGatewayV2HTTPRequest{
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{},
			},
		}},
		{"deviceId wrong type", events.APIGatewayV2HTTPRequest{
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
					Lambda: map[string]interface{}{
						"deviceId": 123,
						"ownerId":  "owner-xyz",
					},
				},
			},
		}},
		{"ownerId missing", events.APIGatewayV2HTTPRequest{
			RequestContext: events.APIGatewayV2HTTPRequestContext{
				Authorizer: &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
					Lambda: map[string]interface{}{
						"deviceId": "dev-abc",
					},
				},
			},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractDeviceIdentity(tc.req)
			if err == nil {
				t.Fatalf("expected error, got %+v", got)
			}
			var apiErr *apierr.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *apierr.Error, got %T", err)
			}
			if apiErr.Code != apierr.CodeUnauthorized {
				t.Errorf("code = %s, want UNAUTHORIZED", apiErr.Code)
			}
		})
	}
}

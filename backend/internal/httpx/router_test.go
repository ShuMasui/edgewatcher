package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

func newReq(routeKey string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		RouteKey: routeKey,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			RouteKey:  routeKey,
			RequestID: "req-1",
		},
	}
}

func decodeBody(t *testing.T, resp events.APIGatewayV2HTTPResponse) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(resp.Body), &m); err != nil {
		t.Fatalf("response body is not valid JSON: %v\nbody=%s", err, resp.Body)
	}
	return m
}

func TestRouteDispatchesByExactRouteKey(t *testing.T) {
	r := New(nil)
	r.Handle("GET /devices", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error) {
		return Response{Body: map[string]string{"ok": "yes"}}, nil
	})
	r.Handle("POST /devices", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error) {
		return Response{StatusCode: 201, Body: map[string]string{"ok": "created"}}, nil
	})

	resp, err := r.Route(context.Background(), newReq("GET /devices"))
	if err != nil {
		t.Fatalf("Route returned non-nil error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if resp.Headers["content-type"] != "application/json" {
		t.Errorf("content-type = %q", resp.Headers["content-type"])
	}

	resp2, _ := r.Route(context.Background(), newReq("POST /devices"))
	if resp2.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", resp2.StatusCode)
	}
}

func TestRouteUnknownRouteKeyReturns500RouteNotWired(t *testing.T) {
	r := New(nil)
	r.Handle("GET /devices", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error) {
		return Response{}, nil
	})

	resp, err := r.Route(context.Background(), newReq("DELETE /nonexistent"))
	if err != nil {
		t.Fatalf("Route returned non-nil error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("StatusCode = %d, want 500", resp.StatusCode)
	}

	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "ROUTE_NOT_WIRED" {
		t.Errorf("code = %v, want ROUTE_NOT_WIRED", errObj["code"])
	}
}

func TestRouteNeverReturnsGoError(t *testing.T) {
	// Binding constraint: handlers never propagate a non-nil error to the
	// Lambda runtime. That would produce a bare 502 with no parseable body.
	r := New(nil)
	r.Handle("GET /boom", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error) {
		return Response{}, errors.New("some internal failure detail")
	})

	resp, err := r.Route(context.Background(), newReq("GET /boom"))
	if err != nil {
		t.Fatalf("Route must never return a non-nil error, got %v", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("StatusCode = %d, want 500", resp.StatusCode)
	}
	if strings.Contains(resp.Body, "some internal failure detail") {
		t.Fatalf("response leaked handler error detail: %s", resp.Body)
	}
}

func TestRoutePropagatesKnownApiErrCodeAndStatus(t *testing.T) {
	r := New(nil)
	r.Handle("GET /devices/1", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error) {
		return Response{}, apierr.New(apierr.CodeDeviceNotFound, "端末が見つかりません")
	})

	resp, _ := r.Route(context.Background(), newReq("GET /devices/1"))
	if resp.StatusCode != 404 {
		t.Fatalf("StatusCode = %d, want 404", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "DEVICE_NOT_FOUND" {
		t.Errorf("code = %v", errObj["code"])
	}
	if errObj["message"] != "端末が見つかりません" {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestRouteRecoversPanicsIntoWellFormedResponse(t *testing.T) {
	r := New(nil)
	r.Handle("GET /panics", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error) {
		panic("deviceSecret=super-secret-value should never be logged or returned")
	})

	resp, err := r.Route(context.Background(), newReq("GET /panics"))
	if err != nil {
		t.Fatalf("Route returned non-nil error after panic: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("StatusCode = %d, want 500", resp.StatusCode)
	}
	if strings.Contains(resp.Body, "super-secret-value") {
		t.Fatalf("panic value leaked into response body: %s", resp.Body)
	}
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "INTERNAL_ERROR" {
		t.Errorf("code = %v, want INTERNAL_ERROR", errObj["code"])
	}
}

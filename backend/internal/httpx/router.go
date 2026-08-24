// Package httpx wires an API Gateway v2 HTTP API payload to Go handlers.
//
// Routing is exact-match on req.RouteKey. Payload format 2.0 delivers the
// route key verbatim (e.g. "GET /devices/{id}/observations"), matching the
// Terraform route strings character for character, so there is no path
// splitting or regex here. An unmatched key can only mean Terraform and Go
// have drifted apart.
package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/apierr"
)

// Response is what a HandlerFunc returns on success. Body is marshaled to
// JSON. StatusCode of 0 means 200.
type Response struct {
	StatusCode int
	Body       any
}

// HandlerFunc handles one route. Returning a non-nil error is fine and
// expected — the Router converts it into the apierr wire format. What a
// HandlerFunc must never do is panic without the Router around it, which
// is why Router.Route always recovers.
type HandlerFunc func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (Response, error)

// Router dispatches API Gateway v2 requests to registered HandlerFuncs by
// exact RouteKey match.
type Router struct {
	logger *slog.Logger
	routes map[string]HandlerFunc
}

// New creates a Router. logger may be nil (routing still works; nothing
// gets logged).
func New(logger *slog.Logger) *Router {
	return &Router{logger: logger, routes: make(map[string]HandlerFunc)}
}

// Handle registers h for routeKey, e.g. "GET /devices".
func (rt *Router) Handle(routeKey string, h HandlerFunc) {
	rt.routes[routeKey] = h
}

// Route is the Lambda entry point: pass it directly to lambda.Start. It
// never returns a non-nil error — a bare error return from the top-level
// handler produces a headerless 502 that the web client cannot parse, so
// every failure (unmatched route, handler error, panic) is converted into
// a well-formed apierr response instead.
func (rt *Router) Route(ctx context.Context, req events.APIGatewayV2HTTPRequest) (resp events.APIGatewayV2HTTPResponse, _ error) {
	defer func() {
		if p := recover(); p != nil {
			rt.logError(req, "panic recovered", "panic", fmt.Sprintf("%v", p), "stack", string(debug.Stack()))
			resp = apierr.WriteError(apierr.New(apierr.CodeInternal, "internal error"))
		}
	}()

	handler, ok := rt.routes[req.RouteKey]
	if !ok {
		rt.logError(req, "route not wired", "routeKey", req.RouteKey)
		return apierr.WriteError(apierr.New(apierr.CodeRouteNotWired, "internal error")), nil
	}

	out, err := handler(ctx, req)
	if err != nil {
		apiErr := apierr.AsError(err)
		if apiErr.Code == apierr.CodeInternal {
			rt.logError(req, "handler returned an unrecognized error", "error", err.Error())
		}
		return apierr.WriteError(apiErr), nil
	}

	status := out.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	body, marshalErr := json.Marshal(out.Body)
	if marshalErr != nil {
		rt.logError(req, "failed to marshal handler response", "error", marshalErr.Error())
		return apierr.WriteError(apierr.New(apierr.CodeInternal, "internal error")), nil
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"content-type": "application/json"},
		Body:       string(body),
	}, nil
}

// logError writes a structured line carrying requestId, never the request
// body or headers (which may carry Authorization / session tokens).
func (rt *Router) logError(req events.APIGatewayV2HTTPRequest, msg string, kv ...any) {
	if rt.logger == nil {
		return
	}
	args := append([]any{"requestId", req.RequestContext.RequestID, "routeKey", req.RouteKey}, kv...)
	rt.logger.Error(msg, args...)
}

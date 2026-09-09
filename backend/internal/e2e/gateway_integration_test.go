//go:build integration

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
)

// This file models the one component of the deployed system that has no Go
// code of its own: API Gateway.
//
// Without it, every e2e test calls a handler directly with a hand-built
// events.APIGatewayV2HTTPRequest — which means the tests supply the very
// things the gateway is responsible for producing. Three consequences, all
// of them silent:
//
//   - RouteKey is set by the test, so a handler registered under the wrong
//     key passes every e2e test and fails only once deployed.
//   - PathParameters are set by the test, so nothing checks that "{id}" in
//     a route template actually reaches the handler as a path parameter.
//   - The return value is inspected as an httpx.Response struct, so no test
//     ever sees an HTTP status code. The apierr.Code -> status mapping is
//     unit-tested in isolation and never end to end.
//
// The model here is deliberately narrow: route matching, path-parameter
// extraction, the authorizer hand-off, and body encoding. It does NOT
// verify JWTs — signature verification is API Gateway's job and stays out
// of scope (there is no Cognito in this test). A web request's bearer token
// is taken at face value as the caller's sub.

// routeClass decides which authorizer, if any, runs in front of a route.
type routeClass int

const (
	// classWeb: API Gateway's Cognito JWT authorizer
	// (infra/envs/dev/main.tf web_routes).
	classWeb routeClass = iota
	// classDevicePublic: no authorizer at all — device-auth's two routes
	// (device_public_routes). Every input is untrusted.
	classDevicePublic
	// classDevice: the Lambda authorizer (device_routes).
	classDevice
)

// route is one entry of the API Gateway route table.
type route struct {
	key      string // "POST /devices/{id}/pairing-sessions"
	method   string
	segments []string
	class    routeClass
	router   *httpx.Router
}

// apiGateway dispatches HTTP requests the way the deployed API Gateway
// does, then hands off to the same httpx.Router the Lambda uses.
type apiGateway struct {
	routes []route
	stack  *stack
}

// newGatewayServer wires the four functions behind one httptest server.
//
// The route templates are written out here rather than read from the
// routers because that is the direction the real dependency runs: API
// Gateway owns the table (infra/envs/dev/main.tf) and passes routeKey to
// the Lambda, which looks it up. TestRouteTableMatchesTerraform keeps this
// list honest against the Terraform that actually defines it.
func newGatewayServer(t *testing.T, st *stack) *httptest.Server {
	t.Helper()

	webRouter := httpx.New(nil)
	st.web.Register(webRouter)

	publicRouter := httpx.New(nil)
	st.auth.Register(publicRouter)

	deviceRouter := httpx.New(nil)
	st.device.Register(deviceRouter)

	gw := &apiGateway{stack: st}
	add := func(class routeClass, router *httpx.Router, keys ...string) {
		for _, key := range keys {
			method, path, found := strings.Cut(key, " ")
			if !found {
				t.Fatalf("malformed route key %q", key)
			}
			gw.routes = append(gw.routes, route{
				key:      key,
				method:   method,
				segments: splitPath(path),
				class:    class,
				router:   router,
			})
		}
	}

	add(classWeb, webRouter, webRouteKeys...)
	add(classDevicePublic, publicRouter, devicePublicRouteKeys...)
	add(classDevice, deviceRouter, deviceRouteKeys...)

	server := httptest.NewServer(gw)
	t.Cleanup(server.Close)
	return server
}

var webRouteKeys = []string{
	"GET /app-config",
	"GET /devices",
	"POST /devices",
	"POST /devices/{id}/pairing-sessions",
	"GET /devices/{id}/pairing-sessions/latest",
	"POST /devices/{id}/disconnect",
	"PATCH /devices/{id}",
	"DELETE /devices/{id}",
	"GET /devices/{id}/observations",
	"GET /observations/{id}/image",
}

var devicePublicRouteKeys = []string{
	"POST /device/pair",
	"POST /device/token",
}

var deviceRouteKeys = []string{
	"POST /device/uploads",
	"POST /device/logout",
}

func splitPath(p string) []string {
	return strings.Split(strings.Trim(p, "/"), "/")
}

func (gw *apiGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	matched, params, ok := gw.match(r.Method, r.URL.Path)
	if !ok {
		// API Gateway's own 404 for an unmatched route — note it is NOT
		// the application's {"error":{...}} envelope, because the request
		// never reached a Lambda.
		writeGatewayError(w, http.StatusNotFound, "Not Found")
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, "Bad Request")
		return
	}

	req := events.APIGatewayV2HTTPRequest{
		RouteKey:              matched.key,
		RawPath:               r.URL.Path,
		Headers:               lowercaseHeaders(r),
		PathParameters:        params,
		QueryStringParameters: flattenQuery(r),
	}

	// API Gateway base64-encodes any body whose content type it does not
	// consider text — which is every multipart upload. Getting this wrong
	// is invisible to a handler test that hands over plain bytes.
	if isTextContentType(r.Header.Get("Content-Type")) {
		req.Body = string(raw)
	} else if len(raw) > 0 {
		req.Body = base64.StdEncoding.EncodeToString(raw)
		req.IsBase64Encoded = true
	}

	if !gw.authorize(matched, &req, w) {
		return
	}

	resp, err := matched.router.Route(r.Context(), req)
	if err != nil {
		writeGatewayError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.StatusCode)
	if resp.Body != "" {
		_, _ = io.WriteString(w, resp.Body)
	}
}

// authorize runs the route's authorizer and populates the request context
// the handlers read their identity from. It reports whether the request may
// continue; when it may not, it has already written the response.
func (gw *apiGateway) authorize(matched route, req *events.APIGatewayV2HTTPRequest, w http.ResponseWriter) bool {
	switch matched.class {
	case classDevicePublic:
		return true

	case classWeb:
		// Same identity-source rule as above: no Authorization header means
		// 401 before any verification happens.
		//
		// No signature verification either: that is API Gateway's job and
		// there is no Cognito here. The bearer token is taken as the
		// caller's sub, which is all the handlers read (httpx.OwnerID).
		sub := strings.TrimPrefix(req.Headers["authorization"], "Bearer ")
		if sub == "" {
			writeGatewayError(w, http.StatusUnauthorized, "Unauthorized")
			return false
		}
		req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{
				Claims: map[string]string{"sub": sub},
			},
		}
		return true

	case classDevice:
		// Both authorizers declare
		// identity_sources = ["$request.header.Authorization"]
		// (infra/modules/api/main.tf). When that header is absent API
		// Gateway answers 401 WITHOUT invoking the authorizer at all — the
		// Lambda never runs. Only a request that carried a token and was
		// then denied gets 403. Collapsing the two would misreport the
		// missing-header case, which is the state a device is in right
		// after it wipes its credentials.
		if req.Headers["authorization"] == "" {
			writeGatewayError(w, http.StatusUnauthorized, "Unauthorized")
			return false
		}
		// The REAL authorizer runs here, against the real store.
		decision, err := gw.stack.authz.Authorize(context.Background(),
			events.APIGatewayV2CustomAuthorizerV2Request{Headers: req.Headers})
		if err != nil || !decision.IsAuthorized {
			// A denied Lambda authorizer produces 403, not 401
			// (docs/05-backend.md §1.6). The device's self-repair loop keys
			// off this, so the status is part of the contract.
			writeGatewayError(w, http.StatusForbidden, "Forbidden")
			return false
		}
		req.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			Lambda: decision.Context,
		}
		return true
	}
	return false
}

// match finds the route template for a method and path, preferring literal
// segments over "{param}" ones — the same precedence API Gateway applies.
func (gw *apiGateway) match(method, path string) (route, map[string]string, bool) {
	segments := splitPath(path)

	best := -1
	bestLiterals := -1
	var bestParams map[string]string

	for i, candidate := range gw.routes {
		if candidate.method != method || len(candidate.segments) != len(segments) {
			continue
		}
		params := map[string]string{}
		literals := 0
		ok := true
		for j, want := range candidate.segments {
			switch {
			case strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}"):
				params[strings.Trim(want, "{}")] = segments[j]
			case want == segments[j]:
				literals++
			default:
				ok = false
			}
			if !ok {
				break
			}
		}
		if ok && literals > bestLiterals {
			best, bestLiterals, bestParams = i, literals, params
		}
	}
	if best < 0 {
		return route{}, nil, false
	}
	return gw.routes[best], bestParams, true
}

func lowercaseHeaders(r *http.Request) map[string]string {
	// API Gateway's HTTP API payload lowercases header names; internal/authz
	// looks up "authorization" and would miss a capitalized key.
	out := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) > 0 {
			out[strings.ToLower(k)] = v[0]
		}
	}
	return out
}

func flattenQuery(r *http.Request) map[string]string {
	q := r.URL.Query()
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]string, len(q))
	for k := range q {
		out[k] = q.Get(k)
	}
	return out
}

func isTextContentType(ct string) bool {
	return ct == "" || strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "text/")
}

// writeGatewayError emits API Gateway's own error shape, which is a bare
// {"message": "..."} and deliberately NOT the application's
// {"error":{"code","message"}} envelope — a client that sees the former
// knows the request never reached the function.
func writeGatewayError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

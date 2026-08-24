// Command authorizer is the Lambda authorizer for device-session routes
// (docs/05-backend.md §1.1, §3.2). It is invoked by API Gateway directly —
// not through internal/httpx's RouteKey router, since its wire contract is
// an authorization decision (APIGatewayV2CustomAuthorizerSimpleResponse),
// not an HTTP response.
//
// The Terraform authorizer module sets enable_simple_responses = true and
// authorizer_result_ttl = 0 (05-backend.md §3.2): every invocation must
// check DynamoDB fresh, no caching, so that revoking a device's pairing
// takes effect on the very next request.
//
// The actual DynamoDB session check is implemented in a later task. This
// handler only proves the module compiles and answers a well-formed
// response; deny-by-default is the correct behavior for that unimplemented
// state.
package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/ShuMasui/edgewatcher/backend/internal/config"
	"github.com/ShuMasui/edgewatcher/backend/internal/logging"
)

func main() {
	logger := logging.New()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err.Error())
		os.Exit(1)
	}
	_ = cfg // consumed by the later task implementing the session check.

	lambda.Start(handle)
}

func handle(ctx context.Context, req events.APIGatewayV2CustomAuthorizerV2Request) (events.APIGatewayV2CustomAuthorizerSimpleResponse, error) {
	// Deny-by-default placeholder: session validation lands in a later
	// task. Returning a well-formed "not authorized" response, rather than
	// a Go error, keeps API Gateway's behavior predictable in the meantime.
	return events.APIGatewayV2CustomAuthorizerSimpleResponse{IsAuthorized: false}, nil
}

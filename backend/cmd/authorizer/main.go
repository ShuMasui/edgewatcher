// Command authorizer is the Lambda authorizer for device-session routes
// (docs/05-backend.md §1.1, §3.2, docs/06-auth.md §4). It is invoked by
// API Gateway directly — not through internal/httpx's RouteKey router,
// since its wire contract is an authorization decision
// (APIGatewayV2CustomAuthorizerSimpleResponse), not an HTTP response.
//
// The Terraform authorizer module sets enable_simple_responses = true and
// authorizer_result_ttl = 0 (05-backend.md §3.2): every invocation checks
// DynamoDB fresh, no caching, so that revoking a device's pairing takes
// effect on the very next request.
//
// The deployed IAM role for this function grants dynamodb:GetItem only
// (infra/envs/dev/iam.tf) — internal/authz.Authorizer's only store
// dependency is store.Store.GetDeviceForAuth, a single ConsistentRead
// GetItem, matching that ceiling exactly.
package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/ShuMasui/edgewatcher/backend/internal/authz"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/config"
	"github.com/ShuMasui/edgewatcher/backend/internal/logging"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
)

func main() {
	logger := logging.New()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err.Error())
		os.Exit(1)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Error("failed to load aws config", "error", err.Error())
		os.Exit(1)
	}

	client := dynamodb.NewFromConfig(awsCfg)
	s := store.New(client, cfg.TableName)
	a := authz.New(s, clock.Real{}, logger)

	lambda.Start(func(ctx context.Context, req events.APIGatewayV2CustomAuthorizerV2Request) (events.APIGatewayV2CustomAuthorizerSimpleResponse, error) {
		return a.Authorize(ctx, req)
	})
}

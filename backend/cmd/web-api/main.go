// Command web-api is the Lambda entry point for the JWT-authorized routes
// (docs/05-backend.md §1.1): /app-config, /devices/*, /observations/*.
//
// Business routes are wired in later tasks (6-9). This file only proves
// the module compiles, config loads, and the router answers a request —
// the one route registered below is a smoke target for the router's own
// unit tests, not part of the API Gateway route table
// (infra/envs/dev/main.tf's web_routes).
package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/ShuMasui/edgewatcher/backend/internal/config"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/logging"
)

func main() {
	logger := logging.New()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err.Error())
		os.Exit(1)
	}
	_ = cfg // consumed by later tasks wiring DynamoDB/S3 clients.

	router := httpx.New(logger)
	router.Handle("GET /_internal/health", func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (httpx.Response, error) {
		return httpx.Response{Body: map[string]string{"status": "ok"}}, nil
	})

	lambda.Start(router.Route)
}

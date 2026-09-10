// Command device-auth is the Lambda entry point for the two pre-auth
// routes (docs/05-backend.md §1.1): POST /device/pair and POST
// /device/token. It is the only one of the four functions with no
// authorizer in front of it — every input is untrusted until
// internal/deviceauth validates it.
//
// Its IAM role grants Query on GSI2, GetItem, UpdateItem and
// TransactWriteItems (infra/envs/dev/iam.tf, docs/05-backend.md §2.4),
// which is exactly the set internal/deviceauth.Store declares. It holds no
// S3 permission at all: nothing on these routes touches an image.
package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/config"
	"github.com/ShuMasui/edgewatcher/backend/internal/deviceauth"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
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

	s := store.New(dynamodb.NewFromConfig(awsCfg), cfg.TableName)

	router := httpx.New(logger)
	deviceauth.New(s, clock.Real{}, logger).Register(router)

	lambda.Start(router.Route)
}

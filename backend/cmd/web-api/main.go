// Command web-api is the Lambda entry point for the browser-facing routes
// (docs/05-backend.md §1.1). API Gateway's Cognito JWT authorizer runs in
// front of every one of them, so this function never verifies a token —
// but it checks ownership on every device-scoped request, because the JWT
// says who is calling and nothing about what they may see
// (docs/06-auth.md §7).
//
// Its IAM role reaches the table and GSI1 for reads and writes and holds
// s3:GetObject but not PutObject (infra/envs/dev/iam.tf,
// docs/05-backend.md §2.4). GetObject is needed even though this function
// never calls it: a presigned URL inherits the signer's permissions, so
// without it every URL it issues would be signed by a principal that may
// not read the object.
package main

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ShuMasui/edgewatcher/backend/internal/api"
	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/config"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/images"
	"github.com/ShuMasui/edgewatcher/backend/internal/logging"
	"github.com/ShuMasui/edgewatcher/backend/internal/store"
	"github.com/ShuMasui/edgewatcher/backend/internal/webapi"
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
	presign := s3.NewPresignClient(s3.NewFromConfig(awsCfg))
	signer := images.NewSigner(presign, cfg.ImagesBucket, time.Duration(cfg.SignedURLTTL)*time.Second)

	router := httpx.New(logger)
	webapi.New(s, api.NewMapper(signer), clock.Real{}, webapi.Config{
		RetentionDays: cfg.RetentionDays,
		DeviceLimit:   cfg.DeviceLimit,
	}, logger).Register(router)

	lambda.Start(router.Route)
}

// Command device-api is the Lambda entry point for the two authorized
// device routes (docs/05-backend.md §1.1): POST /device/uploads and POST
// /device/logout. The Lambda authorizer runs in front of both and puts the
// caller's deviceId/ownerId in the request context (§3.2); this function
// never re-establishes identity of its own.
//
// Its IAM role grants dynamodb:PutItem/UpdateItem and s3:PutObject and
// nothing else (infra/envs/dev/iam.tf, docs/05-backend.md §2.4). No read
// permission at all, which is why internal/deviceapi.UploadStore has no
// read method and the upload response's intervalMinutes comes out of
// TouchDeviceLatest's ALL_NEW return.
//
// It is the function configured with the most memory (1024MB, §2.2) —
// memory being Lambda's proxy for CPU, so the two S3 transfers finish
// sooner rather than because the payload needs the room.
package main

import (
	"context"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/ShuMasui/edgewatcher/backend/internal/clock"
	"github.com/ShuMasui/edgewatcher/backend/internal/config"
	"github.com/ShuMasui/edgewatcher/backend/internal/deviceapi"
	"github.com/ShuMasui/edgewatcher/backend/internal/httpx"
	"github.com/ShuMasui/edgewatcher/backend/internal/images"
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
	uploader := images.NewUploader(s3.NewFromConfig(awsCfg), cfg.ImagesBucket)

	router := httpx.New(logger)
	deviceapi.New(s, uploader, clock.Real{}, cfg.RetentionDays, logger).Register(router)

	lambda.Start(router.Route)
}

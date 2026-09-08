// Package config loads the environment variables Terraform injects into
// every Lambda function (infra/envs/dev/main.tf local.common_env). None of
// these are secrets — no secret ever travels through Lambda environment
// variables in this system.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the values common to all four functions, plus the one
// variable web-api alone receives (COGNITO_USER_POOL_ID, left empty for
// the other three).
type Config struct {
	// TableName is the single DynamoDB table (TABLE_NAME).
	TableName string
	// ImagesBucket is the S3 bucket for full images and thumbnails
	// (IMAGES_BUCKET).
	ImagesBucket string
	// RetentionDays is how long observations/images live before TTL /
	// lifecycle expiry (RETENTION_DAYS).
	RetentionDays int
	// DeviceLimit is the max paired devices per owner (DEVICE_LIMIT).
	DeviceLimit int
	// SignedURLTTL is the lifetime, in seconds, of S3 signed URLs
	// (SIGNED_URL_TTL).
	SignedURLTTL int
	// Env is the deployment environment name, e.g. "dev" (ENV).
	Env string
	// CognitoUserPoolID is set only for web-api (COGNITO_USER_POOL_ID);
	// empty for the other three functions, which don't declare it.
	CognitoUserPoolID string
}

// Load reads Config from the process environment, rejecting it outright if
// any of the variables Terraform is documented to inject on every function
// are missing or non-numeric. COGNITO_USER_POOL_ID is read as-is and never
// required, since only web-api's Terraform module sets it.
func Load() (Config, error) {
	table, err := requireEnv("TABLE_NAME")
	if err != nil {
		return Config{}, err
	}
	bucket, err := requireEnv("IMAGES_BUCKET")
	if err != nil {
		return Config{}, err
	}
	env, err := requireEnv("ENV")
	if err != nil {
		return Config{}, err
	}

	retentionDays, err := requireIntEnv("RETENTION_DAYS")
	if err != nil {
		return Config{}, err
	}
	deviceLimit, err := requireIntEnv("DEVICE_LIMIT")
	if err != nil {
		return Config{}, err
	}
	signedURLTTL, err := requireIntEnv("SIGNED_URL_TTL")
	if err != nil {
		return Config{}, err
	}

	return Config{
		TableName:         table,
		ImagesBucket:      bucket,
		RetentionDays:     retentionDays,
		DeviceLimit:       deviceLimit,
		SignedURLTTL:      signedURLTTL,
		Env:               env,
		CognitoUserPoolID: os.Getenv("COGNITO_USER_POOL_ID"),
	}, nil
}

func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("config: %s is required but not set", name)
	}
	return v, nil
}

func requireIntEnv(name string) (int, error) {
	raw, err := requireEnv(name)
	if err != nil {
		return 0, err
	}
	v, convErr := strconv.Atoi(raw)
	if convErr != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q: %w", name, raw, convErr)
	}
	return v, nil
}

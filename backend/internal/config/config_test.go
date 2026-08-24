package config

import (
	"os"
	"testing"
)

// clearEnv removes every variable Load reads, so each test starts from a
// clean slate regardless of what the surrounding shell/CI happens to export.
func clearEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"TABLE_NAME", "IMAGES_BUCKET", "RETENTION_DAYS", "DEVICE_LIMIT",
		"SIGNED_URL_TTL", "ENV", "COGNITO_USER_POOL_ID",
	}
	for _, v := range vars {
		old, had := os.LookupEnv(v)
		os.Unsetenv(v)
		t.Cleanup(func() {
			if had {
				os.Setenv(v, old)
			} else {
				os.Unsetenv(v)
			}
		})
	}
}

func setFullValidEnv(t *testing.T) {
	t.Helper()
	env := map[string]string{
		"TABLE_NAME":     "edgewatcher-dev",
		"IMAGES_BUCKET":  "edgewatcher-dev-images",
		"RETENTION_DAYS": "1",
		"DEVICE_LIMIT":   "10",
		"SIGNED_URL_TTL": "900",
		"ENV":            "dev",
	}
	for k, v := range env {
		os.Setenv(k, v)
	}
}

func TestLoadRejectsMissingTableName(t *testing.T) {
	clearEnv(t)
	setFullValidEnv(t)
	os.Unsetenv("TABLE_NAME")

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load to reject a missing TABLE_NAME, got nil error")
	}
}

func TestLoadRejectsMissingRequiredVars(t *testing.T) {
	required := []string{"IMAGES_BUCKET", "RETENTION_DAYS", "DEVICE_LIMIT", "SIGNED_URL_TTL", "ENV"}
	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			clearEnv(t)
			setFullValidEnv(t)
			os.Unsetenv(missing)

			if _, err := Load(); err == nil {
				t.Fatalf("expected Load to reject missing %s", missing)
			}
		})
	}
}

func TestLoadRejectsNonIntegerNumericVars(t *testing.T) {
	clearEnv(t)
	setFullValidEnv(t)
	os.Setenv("DEVICE_LIMIT", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to reject a non-integer DEVICE_LIMIT")
	}
}

func TestLoadSucceedsWithFullEnvAndParsesValues(t *testing.T) {
	clearEnv(t)
	setFullValidEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error with a fully populated env: %v", err)
	}

	if cfg.TableName != "edgewatcher-dev" {
		t.Errorf("TableName = %q", cfg.TableName)
	}
	if cfg.ImagesBucket != "edgewatcher-dev-images" {
		t.Errorf("ImagesBucket = %q", cfg.ImagesBucket)
	}
	if cfg.RetentionDays != 1 {
		t.Errorf("RetentionDays = %d, want 1", cfg.RetentionDays)
	}
	if cfg.DeviceLimit != 10 {
		t.Errorf("DeviceLimit = %d, want 10", cfg.DeviceLimit)
	}
	if cfg.SignedURLTTL != 900 {
		t.Errorf("SignedURLTTL = %d, want 900", cfg.SignedURLTTL)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.CognitoUserPoolID != "" {
		t.Errorf("CognitoUserPoolID = %q, want empty when unset", cfg.CognitoUserPoolID)
	}
}

func TestLoadReadsOptionalCognitoUserPoolID(t *testing.T) {
	clearEnv(t)
	setFullValidEnv(t)
	os.Setenv("COGNITO_USER_POOL_ID", "us-east-1_abc123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.CognitoUserPoolID != "us-east-1_abc123" {
		t.Errorf("CognitoUserPoolID = %q", cfg.CognitoUserPoolID)
	}
}

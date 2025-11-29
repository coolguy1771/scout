package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.API.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.API.Port)
	}

	if cfg.Database.Host != "localhost" {
		t.Errorf("Expected default host 'localhost', got '%s'", cfg.Database.Host)
	}
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	os.Setenv("API_PORT", "9000")
	os.Setenv("DB_HOST", "test-host")
	defer os.Unsetenv("API_PORT")
	defer os.Unsetenv("DB_HOST")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.API.Port != 9000 {
		t.Errorf("Expected port 9000 from env, got %d", cfg.API.Port)
	}

	if cfg.Database.Host != "test-host" {
		t.Errorf("Expected host 'test-host' from env, got '%s'", cfg.Database.Host)
	}
}

func TestValidate_Production(t *testing.T) {
	cfg := &Config{
		API: APIConfig{
			Env: "production",
		},
		JWT: JWTConfig{
			Secret: "change-me-in-production",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected validation error for default JWT secret in production")
	}
}

func TestValidate_Development(t *testing.T) {
	cfg := &Config{
		API: APIConfig{
			Env: "development",
		},
		JWT: JWTConfig{
			Secret: "change-me-in-production",
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected no validation error in development, got: %v", err)
	}
}




package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Database     DatabaseConfig
	Valkey       ValkeyConfig
	API          APIConfig
	JWT          JWTConfig
	S3           S3Config
	Search       SearchConfig
	LogLevel     string
	LogFormat    string
	FeatureFlags FeatureFlags
	LayerUpload  LayerUploadConfig
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}

type ValkeyConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type APIConfig struct {
	Port int
	Env  string
	CORS CORSConfig
}

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

type JWTConfig struct {
	Secret  string
	Issuer  string
	Expires int // in hours
}

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
}

type SearchConfig struct {
	Enabled  bool
	Host     string
	Port     int
	User     string
	Password string
}

type FeatureFlags struct {
	EnableDynamicTiles  bool
	MaxSearchCandidates int
}

type LayerUploadConfig struct {
	MaxFileSize    int64    // Maximum file size in bytes (default 500MB)
	AllowedFormats []string // Allowed file formats (default: ["geojson"])
}

// Load loads configuration from files and environment variables.
// If configPath is provided, it will load that specific file.
// If configPath is empty, it will try to load config.yaml or config.json in that order.
// Environment variables always override file values.
func Load(configPath ...string) (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	setDefaults(k)

	// Load from config file
	var configFile string
	if len(configPath) > 0 && configPath[0] != "" {
		// Use provided config file path
		configFile = configPath[0]
	} else {
		// Try default config files: config.yaml first, then config.json
		defaultFiles := []string{"config.yaml", "config.json"}
		for _, f := range defaultFiles {
			if _, err := os.Stat(f); err == nil {
				configFile = f
				break
			}
		}
	}

	if configFile != "" {
		// Determine parser based on file extension
		var parser koanf.Parser
		if strings.HasSuffix(configFile, ".yaml") || strings.HasSuffix(configFile, ".yml") {
			parser = yaml.Parser()
		} else if strings.HasSuffix(configFile, ".json") {
			parser = json.Parser()
		} else {
			// Try to detect by reading first few bytes
			// Default to YAML if extension is unknown
			parser = yaml.Parser()
		}

		if err := k.Load(file.Provider(configFile), parser); err != nil {
			return nil, fmt.Errorf("failed to load config file %s: %w", configFile, err)
		}
	}

	// Load environment variables (overrides file config)
	// Transform env vars: DB_HOST -> db.host, VALKEY_PORT -> valkey.port
	// This allows both DB_HOST and db.host to work
	envProvider := env.Provider("", ".", func(s string) string {
		// Convert UPPER_SNAKE_CASE to lower.snake.case
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "_", ".")
		return s
	})

	if err := k.Load(envProvider, nil); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	cfg := &Config{
		Database: DatabaseConfig{
			Host:     k.String("db.host"),
			Port:     k.Int("db.port"),
			User:     k.String("db.user"),
			Password: k.String("db.password"),
			Name:     k.String("db.name"),
			SSLMode:  k.String("db.sslmode"),
		},
		Valkey: ValkeyConfig{
			Host:     k.String("valkey.host"),
			Port:     k.Int("valkey.port"),
			Password: k.String("valkey.password"),
			DB:       k.Int("valkey.db"),
		},
		API: APIConfig{
			Port: k.Int("api.port"),
			Env:  k.String("api.env"),
			CORS: CORSConfig{
				AllowedOrigins:   k.Strings("api.cors.allowed_origins"),
				AllowedMethods:   k.Strings("api.cors.allowed_methods"),
				AllowedHeaders:   k.Strings("api.cors.allowed_headers"),
				ExposedHeaders:   k.Strings("api.cors.exposed_headers"),
				AllowCredentials: k.Bool("api.cors.allow_credentials"),
				MaxAge:           k.Int("api.cors.max_age"),
			},
		},
		JWT: JWTConfig{
			Secret:  k.String("jwt.secret"),
			Issuer:  k.String("jwt.issuer"),
			Expires: k.Int("jwt.expires"),
		},
		S3: S3Config{
			Endpoint:        k.String("s3.endpoint"),
			Region:          k.String("s3.region"),
			Bucket:          k.String("s3.bucket"),
			AccessKeyID:     k.String("s3.access_key_id"),
			SecretAccessKey: k.String("s3.secret_access_key"),
			UseSSL:          k.Bool("s3.use_ssl"),
		},
		Search: SearchConfig{
			Enabled:  k.Bool("search.enabled"),
			Host:     k.String("search.host"),
			Port:     k.Int("search.port"),
			User:     k.String("search.user"),
			Password: k.String("search.password"),
		},
		LogLevel:  k.String("log.level"),
		LogFormat: k.String("log.format"),
		FeatureFlags: FeatureFlags{
			EnableDynamicTiles:  k.Bool("feature_flags.enable_dynamic_tiles"),
			MaxSearchCandidates: k.Int("feature_flags.max_search_candidates"),
		},
		LayerUpload: LayerUploadConfig{
			MaxFileSize:    k.Int64("layer_upload.max_file_size"),
			AllowedFormats: k.Strings("layer_upload.allowed_formats"),
		},
	}

	return cfg, nil
}

func setDefaults(k *koanf.Koanf) {
	// Database defaults
	k.Set("db.host", "localhost")
	k.Set("db.port", 5432)
	k.Set("db.user", "scout")
	k.Set("db.password", "scout")
	k.Set("db.name", "scout")
	k.Set("db.sslmode", "disable")

	// Valkey defaults
	k.Set("valkey.host", "localhost")
	k.Set("valkey.port", 6379)
	k.Set("valkey.password", "")
	k.Set("valkey.db", 0)

	// API defaults
	k.Set("api.port", 8080)
	k.Set("api.env", "development")

	// CORS defaults
	k.Set("api.cors.allowed_origins", []string{"*"})
	k.Set("api.cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	k.Set("api.cors.allowed_headers", []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-IP"})
	k.Set("api.cors.exposed_headers", []string{"Link"})
	k.Set("api.cors.allow_credentials", true)
	k.Set("api.cors.max_age", 300)

	// JWT defaults
	k.Set("jwt.secret", "change-me-in-production")
	k.Set("jwt.issuer", "scout")
	k.Set("jwt.expires", 24)

	// S3 defaults
	k.Set("s3.endpoint", "")
	k.Set("s3.region", "us-east-1")
	k.Set("s3.bucket", "scout-tiles")
	k.Set("s3.access_key_id", "")
	k.Set("s3.secret_access_key", "")
	k.Set("s3.use_ssl", true)

	// Search defaults
	k.Set("search.enabled", false)
	k.Set("search.host", "localhost")
	k.Set("search.port", 9200)
	k.Set("search.user", "")
	k.Set("search.password", "")

	// Logging defaults
	k.Set("log.level", "info")
	k.Set("log.format", "json")

	// Feature flags defaults
	k.Set("feature_flags.enable_dynamic_tiles", false)
	k.Set("feature_flags.max_search_candidates", 50000)

	// Layer upload defaults
	k.Set("layer_upload.max_file_size", 500*1024*1024) // 500MB
	k.Set("layer_upload.allowed_formats", []string{"geojson"})
}

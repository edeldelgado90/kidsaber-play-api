package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all application configuration loaded from environment variables.
// All timeouts are stored as both raw integers (for env parsing) and derived durations.
type Config struct {
	Server ServerConfig
	LLM    LLMConfig
	DB     DBConfig
	Auth   AuthConfig
	Job    JobConfig
	Notify NotifyConfig
	CORS   CORSConfig
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port     int `env:"PORT"             envDefault:"8080"`
	TimeoutS int `env:"SERVER_TIMEOUT_S" envDefault:"30"`

	Timeout time.Duration // derived: TimeoutS * time.Second
}

// LLMConfig holds LLM provider settings.
type LLMConfig struct {
	Provider   string `env:"LLM_PROVIDER"    envDefault:"gemini"`
	APIKey     string `env:"LLM_API_KEY"`
	Model      string `env:"LLM_MODEL"       envDefault:"gemini-2.0-flash-lite"`
	BaseURL    string `env:"LLM_BASE_URL"    envDefault:"https://generativelanguage.googleapis.com/v1beta/openai/"`
	TimeoutS   int    `env:"LLM_TIMEOUT_S"   envDefault:"20"`
	MaxRetries int    `env:"LLM_MAX_RETRIES" envDefault:"2"`

	Timeout time.Duration // derived: TimeoutS * time.Second
}

// DBConfig holds PostgreSQL database settings.
type DBConfig struct {
	URL string `env:"DATABASE_URL,required"`
}

// AuthConfig holds API authentication settings.
type AuthConfig struct {
	Enabled bool   `env:"AUTH_ENABLED" envDefault:"false"`
	APIKey  string `env:"API_KEY"`

	// TokenSecret is the HMAC-SHA256 signing key for device tokens issued by
	// POST /auth/token. If left empty it is automatically derived from API_KEY
	// using HKDF-lite so no extra configuration is required for basic deployments.
	// Set TOKEN_SECRET explicitly to rotate tokens independently of the API key.
	TokenSecret string `env:"TOKEN_SECRET"`

	// TokenTTLS is the device token lifetime in seconds (default 86400 = 24 h).
	TokenTTLS int `env:"TOKEN_TTL_S" envDefault:"86400"`

	// TokenTTL is derived from TokenTTLS at load time.
	TokenTTL time.Duration
}

// JobConfig holds question generator job settings.
// The schedule is now managed externally by Cloud Scheduler — not configured here.
type JobConfig struct {
	BatchSize            int `env:"JOB_BATCH_SIZE"              envDefault:"10"`
	MaxPerCombination    int `env:"JOB_MAX_PER_COMBINATION"     envDefault:"100"`
	SeedIterations       int `env:"JOB_SEED_ITERATIONS"         envDefault:"10"`
	CombinationDelayS    int `env:"JOB_COMBINATION_DELAY_S"     envDefault:"4"`

	CombinationDelay time.Duration // derived: CombinationDelayS * time.Second
}

// NotifyConfig holds notification settings for webhook and SMTP.
type NotifyConfig struct {
	WebhookURL string `env:"NOTIFY_WEBHOOK_URL"`
	SMTPHost   string `env:"NOTIFY_SMTP_HOST"`
	SMTPPort   int    `env:"NOTIFY_SMTP_PORT"     envDefault:"587"`
	SMTPUser   string `env:"NOTIFY_SMTP_USER"`
	SMTPPass   string `env:"NOTIFY_SMTP_PASSWORD"`
	SMTPFrom   string `env:"NOTIFY_SMTP_FROM"`
	SMTPTo     string `env:"NOTIFY_SMTP_TO"` // comma-separated
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins string `env:"CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:3000"`
}

// Load parses all environment variables into a Config struct.
// Fails fast at startup if required variables are missing or auth is misconfigured.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Derive duration fields
	cfg.Server.Timeout = time.Duration(cfg.Server.TimeoutS) * time.Second
	cfg.LLM.Timeout = time.Duration(cfg.LLM.TimeoutS) * time.Second
	cfg.Auth.TokenTTL = time.Duration(cfg.Auth.TokenTTLS) * time.Second
	cfg.Job.CombinationDelay = time.Duration(cfg.Job.CombinationDelayS) * time.Second

	// Validate: auth enabled requires an API key
	if cfg.Auth.Enabled && cfg.Auth.APIKey == "" {
		return nil, fmt.Errorf("AUTH_ENABLED=true but API_KEY is not set")
	}

	// Derive TOKEN_SECRET from API_KEY if not explicitly configured.
	// Uses HMAC-SHA256 as a simple KDF so the token signing key is
	// cryptographically independent of (but derived from) the static key.
	// Falls back to a dev-only placeholder when both are empty.
	if cfg.Auth.TokenSecret == "" {
		seed := cfg.Auth.APIKey
		if seed == "" {
			seed = "kidsaber-dev-placeholder-not-for-production"
		}
		mac := hmac.New(sha256.New, []byte(seed))
		mac.Write([]byte("kidsaber-device-token-secret-v1"))
		cfg.Auth.TokenSecret = hex.EncodeToString(mac.Sum(nil))
	}

	return cfg, nil
}

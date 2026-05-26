package config

import (
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
}

// JobConfig holds background job settings.
type JobConfig struct {
	Enabled           bool   `env:"JOB_ENABLED"             envDefault:"true"`
	Schedule          string `env:"JOB_SCHEDULE"            envDefault:"0 3 * * *"`
	BatchSize         int    `env:"JOB_BATCH_SIZE"          envDefault:"10"`
	MaxPerCombination int    `env:"JOB_MAX_PER_COMBINATION" envDefault:"100"`
	SeedIterations    int    `env:"JOB_SEED_ITERATIONS"     envDefault:"10"`
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

	// Validate: auth enabled requires an API key
	if cfg.Auth.Enabled && cfg.Auth.APIKey == "" {
		return nil, fmt.Errorf("AUTH_ENABLED=true but API_KEY is not set")
	}

	return cfg, nil
}

package config

import (
	"fmt"
	"log/slog"
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
	Log    LogConfig
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level slog.Level `env:"LOG_LEVEL" envDefault:"INFO"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port     int `env:"PORT"             envDefault:"8080"`
	TimeoutS int `env:"SERVER_TIMEOUT_S" envDefault:"30"`

	Timeout time.Duration // derived: TimeoutS * time.Second
}

// LLMConfig holds Claude API settings.
//
// BaseURL is empty by default so the SDK targets the official endpoint; it
// exists to point tests or a proxy elsewhere. The timeout is generous because
// generation streams with adaptive thinking and runs for minutes, not seconds.
type LLMConfig struct {
	APIKey      string `env:"LLM_API_KEY"`
	Model       string `env:"LLM_MODEL"          envDefault:"claude-opus-5"`
	Effort      string `env:"LLM_EFFORT"         envDefault:"high"`
	BaseURL     string `env:"LLM_BASE_URL"`
	TimeoutS    int    `env:"LLM_TIMEOUT_S"      envDefault:"300"`
	MaxRetries  int    `env:"LLM_MAX_RETRIES"    envDefault:"2"`
	RetryDelayS int    `env:"LLM_RETRY_DELAY_S"  envDefault:"2"`

	Timeout    time.Duration // derived: TimeoutS * time.Second
	RetryDelay time.Duration // derived: RetryDelayS * time.Second
}

// DBConfig holds PostgreSQL database settings.
type DBConfig struct {
	URL string `env:"DATABASE_URL,required"`
}

// AuthConfig holds API authentication settings.
type AuthConfig struct {
	Enabled bool   `env:"AUTH_ENABLED"          envDefault:"false"`
	APIKey  string `env:"API_KEY"`

	// FirebaseProjectID enables Firebase App Check validation when set.
	// Mobile and web clients must include a valid App Check token in the
	// X-Firebase-AppCheck header. When empty, only the static API key is
	// accepted (useful for local development and CI).
	FirebaseProjectID string `env:"FIREBASE_PROJECT_ID"`
}

// JobConfig holds question generator job settings.
// The schedule is now managed externally by Cloud Scheduler — not configured here.
type JobConfig struct {
	BatchSize         int `env:"JOB_BATCH_SIZE"              envDefault:"10"`
	MaxPerCombination int `env:"JOB_MAX_PER_COMBINATION"     envDefault:"100"`
	SeedIterations    int `env:"JOB_SEED_ITERATIONS"         envDefault:"10"`
	CombinationDelayS int `env:"JOB_COMBINATION_DELAY_S"     envDefault:"4"`

	CombinationDelay time.Duration // derived: CombinationDelayS * time.Second
}

// NotifyConfig holds notification settings for webhook and SMTP.
type NotifyConfig struct {
	WebhookURL     string `env:"NOTIFY_WEBHOOK_URL"`
	SMTPHost       string `env:"NOTIFY_SMTP_HOST"`
	SMTPPort       int    `env:"NOTIFY_SMTP_PORT"            envDefault:"587"`
	SMTPUser       string `env:"NOTIFY_SMTP_USER"`
	SMTPPass       string `env:"NOTIFY_SMTP_PASSWORD"`
	SMTPFrom       string `env:"NOTIFY_SMTP_FROM"`
	SMTPTo         string `env:"NOTIFY_SMTP_TO"`                              // comma-separated
	SMTPDailyLimit int    `env:"NOTIFY_SMTP_DAILY_LIMIT"     envDefault:"50"` // 0 = unlimited
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
	cfg.LLM.RetryDelay = time.Duration(cfg.LLM.RetryDelayS) * time.Second
	cfg.Job.CombinationDelay = time.Duration(cfg.Job.CombinationDelayS) * time.Second

	// Validate: auth enabled requires an API key
	if cfg.Auth.Enabled && cfg.Auth.APIKey == "" {
		return nil, fmt.Errorf("AUTH_ENABLED=true but API_KEY is not set")
	}

	return cfg, nil
}

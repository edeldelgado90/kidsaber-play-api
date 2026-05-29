package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/config"
)

// setRequiredEnv sets the minimum env vars needed for config.Load() to succeed.
// DATABASE_URL is the only required field without a default.
// AUTH_ENABLED is pinned to false to avoid the "enabled but no API_KEY" guard.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("AUTH_ENABLED", "false")
}

func TestConfig_LLMRetryDelay_DerivedFromEnvVar(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("LLM_RETRY_DELAY_S", "5")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, 5, cfg.LLM.RetryDelayS)
	assert.Equal(t, 5*time.Second, cfg.LLM.RetryDelay,
		"RetryDelay must be LLM_RETRY_DELAY_S converted to time.Duration")
}

func TestConfig_LLMRetryDelay_DefaultIs2Seconds(t *testing.T) {
	setRequiredEnv(t)

	// Explicitly unset the env var so the envDefault tag takes effect.
	prev, set := os.LookupEnv("LLM_RETRY_DELAY_S")
	os.Unsetenv("LLM_RETRY_DELAY_S")
	t.Cleanup(func() {
		if set {
			os.Setenv("LLM_RETRY_DELAY_S", prev)
		}
	})

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, 2, cfg.LLM.RetryDelayS, "default RetryDelayS must be 2")
	assert.Equal(t, 2*time.Second, cfg.LLM.RetryDelay, "default RetryDelay must be 2s")
}

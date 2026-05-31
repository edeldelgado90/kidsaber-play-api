// cmd/seed pre-populates the question bank by running the generator job N times.
// Usage: make seed   (calls this binary with JOB_SEED_ITERATIONS iterations)
// This is not the production cron runner — it exits after completing all iterations.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/adapter/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/adapter/repository/postgres"
	"github.com/kidsaber/kidsaber-play-api/internal/config"
	"github.com/kidsaber/kidsaber-play-api/internal/job"
	"github.com/kidsaber/kidsaber-play-api/pkg/logger"
	"github.com/kidsaber/kidsaber-play-api/pkg/validator"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	appEnv := os.Getenv("APP_ENV")
	log := logger.New(appEnv, cfg.Log.Level)
	slog.SetDefault(log)

	ctx := context.Background()

	repo, err := postgres.New(ctx, cfg.DB.URL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()

	qValidator, err := validator.NewQuestionValidator()
	if err != nil {
		return fmt.Errorf("initialising validator: %w", err)
	}

	llmClient := llm.NewLLMClient(
		cfg.LLM.BaseURL,
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.Timeout,
	)
	topicPicker := llm.NewTopicPicker()
	llmGen := llm.NewLLMGenerator(llmClient, qValidator, topicPicker, cfg.LLM.MaxRetries, cfg.LLM.RetryDelay, log)

	noopNotifier := notify.NewNoopNotifier()

	generatorJob := job.NewQuestionGeneratorJob(
		llmGen,
		repo,
		repo,
		noopNotifier,
		topicPicker,
		job.Config{
			BatchSize:         cfg.Job.BatchSize,
			MaxPerCombination: cfg.Job.MaxPerCombination,
			SeedIterations:    cfg.Job.SeedIterations,
			CombinationDelay:  cfg.Job.CombinationDelay,
		},
		log,
	)

	iterations := cfg.Job.SeedIterations
	if iterations <= 0 {
		iterations = 10
	}

	log.Info("starting database seed",
		"iterations", iterations,
		"batch_size", cfg.Job.BatchSize,
		"combinations", 72,
	)

	for i := 1; i <= iterations; i++ {
		log.Info("seed iteration", "iteration", i, "of", iterations)
		if err := generatorJob.Run(ctx); err != nil {
			log.Warn("iteration completed with failures", "iteration", i, "error", err)
		}
	}

	log.Info("database seed complete",
		"iterations", iterations,
		"estimated_questions", iterations*cfg.Job.BatchSize*72,
	)

	return nil
}

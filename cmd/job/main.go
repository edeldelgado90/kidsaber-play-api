// cmd/job is the production Cloud Run Job entrypoint.
// It runs one full pass of the question generator over all 72 combinations and exits.
// Triggered externally by Cloud Scheduler — no HTTP server, no cron scheduler inside.
// Exit 0 → all combinations succeeded. Exit 1 → one or more combinations failed.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

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
		fmt.Fprintln(os.Stderr, "job error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	appEnv := os.Getenv("APP_ENV")
	log := logger.New(appEnv)
	slog.SetDefault(log)
	log.Info("starting question generator job")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	repo, err := postgres.New(ctx, cfg.DB.URL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()
	log.Info("connected to database")

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
	llmGen := llm.NewLLMGenerator(llmClient, qValidator, topicPicker, cfg.LLM.MaxRetries, log)

	webhookNotifier := notify.NewWebhookNotifier(cfg.Notify.WebhookURL)
	smtpNotifier := notify.NewSMTPNotifier(
		cfg.Notify.SMTPHost, cfg.Notify.SMTPPort,
		cfg.Notify.SMTPUser, cfg.Notify.SMTPPass,
		cfg.Notify.SMTPFrom, cfg.Notify.SMTPTo,
		cfg.Notify.SMTPDailyLimit,
	)
	notifier := notify.NewMultiNotifier(log, webhookNotifier, smtpNotifier)

	generatorJob := job.NewQuestionGeneratorJob(
		llmGen,
		repo,
		repo,
		notifier,
		topicPicker,
		job.Config{
			BatchSize:         cfg.Job.BatchSize,
			MaxPerCombination: cfg.Job.MaxPerCombination,
			CombinationDelay:  cfg.Job.CombinationDelay,
		},
		log,
	)

	if err := generatorJob.Run(ctx); err != nil {
		log.Error("question generator job failed", "error", err)
		return err
	}

	log.Info("question generator job completed successfully")
	return nil
}

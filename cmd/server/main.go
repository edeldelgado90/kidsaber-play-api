package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/procedural"
	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
	"github.com/kidsaber/kidsaber-play-api/internal/adapter/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/adapter/repository/postgres"
	"github.com/kidsaber/kidsaber-play-api/internal/config"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
	"github.com/kidsaber/kidsaber-play-api/pkg/logger"
	"github.com/kidsaber/kidsaber-play-api/pkg/validator"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// ── Configuration ──────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// ── Logger ─────────────────────────────────────────────────────────────────
	appEnv := os.Getenv("APP_ENV")
	log := logger.New(appEnv)
	slog.SetDefault(log)
	log.Info("starting KidSaber Play API", "port", cfg.Server.Port)

	// ── Database ───────────────────────────────────────────────────────────────
	ctx := context.Background()
	repo, err := postgres.New(ctx, cfg.DB.URL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer repo.Close()
	log.Info("connected to database")

	// ── Validator ──────────────────────────────────────────────────────────────
	qValidator, err := validator.NewQuestionValidator()
	if err != nil {
		return fmt.Errorf("initialising question validator: %w", err)
	}

	// ── LLM Client & Generator ─────────────────────────────────────────────────
	llmClient := llm.NewLLMClient(
		cfg.LLM.BaseURL,
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.Timeout,
	)
	topicPicker := llm.NewTopicPicker()
	llmGen := llm.NewLLMGenerator(llmClient, qValidator, topicPicker, cfg.LLM.MaxRetries, log)

	// ── Procedural Generator ───────────────────────────────────────────────────
	calcGen := procedural.NewCalculatorGenerator()

	// ── Notification Service ───────────────────────────────────────────────────
	webhookNotifier := notify.NewWebhookNotifier(cfg.Notify.WebhookURL)
	smtpNotifier := notify.NewSMTPNotifier(
		cfg.Notify.SMTPHost, cfg.Notify.SMTPPort,
		cfg.Notify.SMTPUser, cfg.Notify.SMTPPass,
		cfg.Notify.SMTPFrom, cfg.Notify.SMTPTo,
		cfg.Notify.SMTPDailyLimit,
	)
	notifier := notify.NewMultiNotifier(log, webhookNotifier, smtpNotifier)

	// ── Use Case ───────────────────────────────────────────────────────────────
	uc := questions.NewGetQuestionsUseCase(calcGen, llmGen, repo, notifier, log)

	// ── HTTP Handlers ──────────────────────────────────────────────────────────
	questionsHandler := httpAdapter.NewQuestionsHandler(uc, log)
	adminHandler := httpAdapter.NewAdminHandler(repo, log)

	// TokenService signs and validates device tokens for POST /auth/token.
	// The signing secret is read from TOKEN_SECRET env var; falls back to a value
	// derived from API_KEY if TOKEN_SECRET is not explicitly set (see config.Load).
	tokenService := httpAdapter.NewTokenService([]byte(cfg.Auth.TokenSecret), cfg.Auth.TokenTTL)
	tokenHandler := httpAdapter.NewTokenHandler(tokenService, log)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: questionsHandler,
		AdminHandler:     adminHandler,
		TokenHandler:     tokenHandler,
		TokenValidator:   tokenService,
		Logger:           log,
		AuthEnabled:      cfg.Auth.Enabled,
		APIKey:           cfg.Auth.APIKey,
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		RequestTimeout:   cfg.Server.Timeout,
		RateLimitEnabled: true,
	})

	// ── HTTP Server ────────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.Timeout,
		WriteTimeout: cfg.Server.Timeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		log.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// ── Graceful Shutdown ──────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		log.Info("shutting down", "signal", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	log.Info("server stopped cleanly")
	return nil
}

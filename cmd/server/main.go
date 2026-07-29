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
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/reports"
	"github.com/kidsaber/kidsaber-play-api/pkg/appcheck"
	"github.com/kidsaber/kidsaber-play-api/pkg/idtoken"
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
	log := logger.New(appEnv, cfg.Log.Level)
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
		cfg.LLM.Effort,
		cfg.LLM.Timeout,
	)
	topicPicker := llm.NewTopicPicker()
	llmGen := llm.NewLLMGenerator(llmClient, qValidator, topicPicker, cfg.LLM.MaxRetries, cfg.LLM.RetryDelay, log)

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

	// Player question reports go to their own Discord channel and never to email
	// — they are content to review, not an incident to page anyone about. The
	// notifier is built directly instead of through MultiNotifier so the SMTP
	// channel cannot pick them up.
	reviewWebhookURL := cfg.Notify.ReviewWebhookURL
	if reviewWebhookURL == "" {
		reviewWebhookURL = cfg.Notify.WebhookURL
	}
	reviewNotifier := notify.NewWebhookNotifier(reviewWebhookURL)

	// ── Use Cases ──────────────────────────────────────────────────────────────
	uc := questions.NewGetQuestionsUseCase(calcGen, llmGen, repo, notifier, log)
	reportUC := reports.NewReportQuestionUseCase(repo, reviewNotifier, log)

	// ── HTTP Handlers ──────────────────────────────────────────────────────────
	questionsHandler := httpAdapter.NewQuestionsHandler(uc, log)
	reportsHandler := httpAdapter.NewReportsHandler(reportUC, log)
	adminHandler := httpAdapter.NewAdminHandler(repo, log)

	// ── Firebase credentials ───────────────────────────────────────────────────
	// When FIREBASE_PROJECT_ID is set, app clients can authenticate without the
	// static API key, either with a Firebase Auth ID token (Authorization:
	// Bearer, what the web and mobile clients send after anonymous sign-in) or
	// with an App Check token (X-Firebase-AppCheck). On Cloud Run, ADC are
	// available automatically.
	var (
		appCheckValidator httpAdapter.AppCheckValidator
		idTokenVerifier   httpAdapter.IDTokenVerifier
	)
	if cfg.Auth.FirebaseProjectID != "" {
		v, err := appcheck.NewValidator(ctx, cfg.Auth.FirebaseProjectID)
		if err != nil {
			return fmt.Errorf("initialising firebase app check: %w", err)
		}
		appCheckValidator = v

		iv, err := idtoken.NewVerifier(ctx, cfg.Auth.FirebaseProjectID)
		if err != nil {
			return fmt.Errorf("initialising firebase id token verifier: %w", err)
		}
		idTokenVerifier = iv

		log.Info("firebase auth enabled", "project", cfg.Auth.FirebaseProjectID,
			"credentials", "id_token+app_check")
	} else {
		log.Info("firebase auth disabled — API key only (FIREBASE_PROJECT_ID not set)")
	}

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler:       questionsHandler,
		ReportsHandler:         reportsHandler,
		AdminHandler:           adminHandler,
		AppCheck:               appCheckValidator,
		IDToken:                idTokenVerifier,
		Logger:                 log,
		AuthEnabled:            cfg.Auth.Enabled,
		APIKey:                 cfg.Auth.APIKey,
		AllowedOrigins:         cfg.CORS.AllowedOrigins,
		RequestTimeout:         cfg.Server.Timeout,
		RateLimitEnabled:       true,
		ReportsRequireAppCheck: cfg.Auth.ReportsRequireAppCheck,
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

package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// RouterConfig carries all dependencies needed to build the chi router.
type RouterConfig struct {
	QuestionsHandler *QuestionsHandler
	ReportsHandler   *ReportsHandler
	AdminHandler     *AdminHandler
	Logger           *slog.Logger
	AuthEnabled      bool
	APIKey           string
	AllowedOrigins   string
	RequestTimeout   time.Duration
	// RateLimitEnabled activates the IP-based rate limiter (60 req/min per IP).
	// Should be true in production; can be disabled in tests to avoid flakiness.
	RateLimitEnabled bool
	// AppCheck validates Firebase App Check tokens from mobile/web clients
	// (X-Firebase-AppCheck header). Pass nil to accept only the static API key.
	AppCheck AppCheckValidator
	// IDToken validates Firebase Auth ID tokens sent by app clients as a bearer
	// token. Pass nil to reject them. Never wired into admin routes.
	IDToken IDTokenVerifier
	// ReportsRequireAppCheck drops the Firebase ID token as an accepted
	// credential on the report route, leaving App Check (or the static API key).
	// Only switch this on once every client platform actually sends an App Check
	// token — today only web does, so enabling it makes reporting fail on native.
	ReportsRequireAppCheck bool
}

const (
	rateLimitMaxReq = 60
	rateLimitWindow = time.Minute

	// Reporting a question is a rare, deliberate act — a child taps it once and
	// moves on. The window is far tighter than the global limit because this
	// route writes to the database and can page a human through Discord.
	//
	// Like the global limiter this is in-process, so on Cloud Run the effective
	// ceiling multiplies by the instance count. The primary-key dedupe in
	// question_reports is what actually bounds the damage; this only slows a
	// single-source flood down.
	reportRateLimitMaxReq = 5
	reportRateLimitWindow = time.Hour
)

// NewRouter builds and returns a configured chi.Router.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Global middleware (applied to all routes)
	r.Use(RecoveryMiddleware(cfg.Logger))
	r.Use(LoggingMiddleware(cfg.Logger))
	r.Use(SecurityHeadersMiddleware)
	r.Use(CORSMiddleware(cfg.AllowedOrigins))
	r.Use(TimeoutMiddleware(cfg.RequestTimeout))

	// IP-based rate limiting — applied before auth to limit unauthenticated probing.
	if cfg.RateLimitEnabled {
		limiter := NewIPRateLimiter(rateLimitMaxReq, rateLimitWindow)
		r.Use(RateLimitMiddleware(limiter))
	}

	// Public routes — no auth required
	r.Get("/health", HealthHandler)

	// Protected routes — static API key OR Firebase ID token OR App Check token
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(AuthConfig{
			Enabled:  cfg.AuthEnabled,
			APIKey:   cfg.APIKey,
			AppCheck: cfg.AppCheck,
			IDToken:  cfg.IDToken,
		}))
		r.Get("/questions", cfg.QuestionsHandler.GetQuestions)
	})

	// Report route — a write endpoint reachable by any player, so it is fenced
	// more tightly than the read routes: its own much stricter rate limiter, and
	// optionally App Check instead of the ID token. An anonymous ID token proves
	// nothing (anyone can mint one), which is why it can be dropped here.
	if cfg.ReportsHandler != nil {
		r.Group(func(r chi.Router) {
			reportLimiter := NewIPRateLimiter(reportRateLimitMaxReq, reportRateLimitWindow)
			r.Use(RateLimitMiddleware(reportLimiter))

			idToken := cfg.IDToken
			if cfg.ReportsRequireAppCheck {
				idToken = nil
			}
			r.Use(AuthMiddleware(AuthConfig{
				Enabled:  cfg.AuthEnabled,
				APIKey:   cfg.APIKey,
				AppCheck: cfg.AppCheck,
				IDToken:  idToken,
			}))
			r.Post("/questions/{id}/report", cfg.ReportsHandler.ReportQuestion)
		})
	}

	// Admin routes — static API key only. App Check and ID tokens are deliberately
	// left nil: anyone can obtain an anonymous ID token, so neither may pass here.
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(AuthConfig{
			Enabled: cfg.APIKey != "",
			APIKey:  cfg.APIKey,
		}))
		r.Get("/admin/jobs", cfg.AdminHandler.GetJobRuns)
	})

	return r
}

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
}

const (
	rateLimitMaxReq = 60
	rateLimitWindow = time.Minute
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

	// Protected routes — accepts static API key OR Firebase App Check token
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(cfg.AuthEnabled, cfg.APIKey, cfg.AppCheck))
		r.Get("/questions", cfg.QuestionsHandler.GetQuestions)
	})

	// Admin routes — static API key only (App Check not accepted)
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(cfg.APIKey != "", cfg.APIKey, nil))
		r.Get("/admin/jobs", cfg.AdminHandler.GetJobRuns)
	})

	return r
}

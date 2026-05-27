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
	// TokenHandler handles POST /auth/token. If nil the endpoint is not registered.
	TokenHandler *TokenHandler
	// TokenValidator validates device tokens issued by POST /auth/token.
	// When set, GET /questions accepts both a static API key and a valid device token.
	// Pass nil to accept only the static API key.
	TokenValidator TokenValidator
	Logger         *slog.Logger
	AuthEnabled    bool
	APIKey         string
	AllowedOrigins string
	RequestTimeout time.Duration
	// RateLimitEnabled activates the IP-based rate limiter (60 req/min per IP).
	// Should be true in production; can be disabled in tests to avoid flakiness.
	RateLimitEnabled bool
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

	// POST /auth/token — issues short-lived device tokens for mobile clients.
	// Public: no prior credential needed. Rate-limited by the global limiter.
	if cfg.TokenHandler != nil {
		r.Post("/auth/token", cfg.TokenHandler.Issue)
	}

	// Protected routes — accepts static API key OR valid device token
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(cfg.AuthEnabled, cfg.APIKey, cfg.TokenValidator))
		r.Get("/questions", cfg.QuestionsHandler.GetQuestions)
	})

	// Admin routes — static API key only (no device token accepted)
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(cfg.APIKey != "", cfg.APIKey, nil))
		r.Get("/admin/jobs", cfg.AdminHandler.GetJobRuns)
	})

	return r
}

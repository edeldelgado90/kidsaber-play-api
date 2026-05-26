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
}

// NewRouter builds and returns a configured chi.Router.
func NewRouter(cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	// Global middleware (applied to all routes)
	r.Use(RecoveryMiddleware(cfg.Logger))
	r.Use(LoggingMiddleware(cfg.Logger))
	r.Use(SecurityHeadersMiddleware)
	r.Use(CORSMiddleware(cfg.AllowedOrigins))
	r.Use(TimeoutMiddleware(cfg.RequestTimeout))

	// Public health endpoint — no auth
	r.Get("/health", HealthHandler)

	// Protected routes — API key auth when enabled
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(cfg.AuthEnabled, cfg.APIKey))
		r.Get("/questions", cfg.QuestionsHandler.GetQuestions)
	})

	// Admin routes — always require auth if API key is configured
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(cfg.APIKey != "", cfg.APIKey))
		r.Get("/admin/jobs", cfg.AdminHandler.GetJobRuns)
	})

	return r
}

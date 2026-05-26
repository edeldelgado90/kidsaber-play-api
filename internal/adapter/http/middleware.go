package http

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture the status code for logging.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware logs method, path, status code, and duration.
// Query param values are never logged (security: they may contain sensitive data).
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// RecoveryMiddleware catches panics, logs them, and returns 500.
func RecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "panic", rec, "path", r.URL.Path)
					writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware aborts requests that exceed the configured timeout.
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CORSMiddleware adds CORS headers. Only origins in the allowlist are accepted.
// Never uses wildcard (*) to prevent cross-origin data leaks.
func CORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	allowed := parseOrigins(allowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-API-Key, Content-Type")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware sets security headers on every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// AuthMiddleware validates the API key when auth is enabled.
// Uses constant-time comparison to prevent timing attacks.
// Returns 401 with a generic message — never reveals key format details.
func AuthMiddleware(enabled bool, apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			key := extractAPIKey(r)
			if key == "" || subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractAPIKey reads the API key from Authorization: Bearer <key> or X-API-Key header.
func extractAPIKey(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}
	return r.Header.Get("X-API-Key")
}

// parseOrigins splits a comma-separated list of origins into a lookup map.
func parseOrigins(s string) map[string]bool {
	m := make(map[string]bool)
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			m[o] = true
		}
	}
	return m
}

package http

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

// corsAllowHeaders lists the request headers a browser may send cross-origin.
// Must include every header the auth middleware reads, otherwise the preflight
// fails and the browser blocks the request before it ever reaches the handler.
const corsAllowHeaders = "Authorization, X-API-Key, X-Firebase-AppCheck, Content-Type"

// corsMaxAge is how long a browser may cache the preflight result, in seconds.
const corsMaxAge = "3600"

// CORSMiddleware adds CORS headers. Only origins matching the allowlist are
// accepted and the matched origin is echoed back — never a wildcard (*), which
// would let any site read question data.
func CORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	allowed := parseOrigins(allowedOrigins)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Origin-dependent response: caches must key on the Origin header.
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			if allowed.matches(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
				w.Header().Set("Access-Control-Max-Age", corsMaxAge)
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

// AppCheckValidator validates Firebase App Check tokens sent by mobile/web clients.
// The token is expected in the X-Firebase-AppCheck request header.
type AppCheckValidator interface {
	VerifyToken(token string) error
}

// AuthMiddleware validates credentials on protected routes.
//
// Accepted credentials (when enabled=true):
//  1. Static API key — Authorization: Bearer <key> or X-API-Key header;
//     constant-time compared; for server-to-server calls and admin tooling.
//  2. Firebase App Check token — X-Firebase-AppCheck header; issued by Google
//     for genuine app instances (iOS/Android/Web). Pass nil to disable.
//
// Uses constant-time comparison for the static key to prevent timing attacks.
// Returns 401 with a generic message — never reveals which check failed.
func AuthMiddleware(enabled bool, apiKey string, appCheck AppCheckValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			// 1. Accept static API key (constant-time comparison).
			if key := extractAPIKey(r); key != "" && apiKey != "" {
				if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 2. Accept a valid Firebase App Check token.
			if appCheck != nil {
				if tok := r.Header.Get("X-Firebase-AppCheck"); tok != "" {
					if appCheck.VerifyToken(tok) == nil {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing credentials")
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

// ── Rate Limiter ──────────────────────────────────────────────────────────────

// clientState tracks the request count within the current window for one IP.
type clientState struct {
	count   int
	resetAt time.Time
}

// ipRateLimiter implements a fixed-window in-process rate limiter keyed by client IP.
// It is safe for concurrent use.
type ipRateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientState
	maxReq  int
	window  time.Duration
}

// NewIPRateLimiter creates a rate limiter that allows maxReq requests per window per IP.
// A background goroutine evicts expired entries every window interval.
func NewIPRateLimiter(maxReq int, window time.Duration) *ipRateLimiter {
	rl := &ipRateLimiter{
		clients: make(map[string]*clientState),
		maxReq:  maxReq,
		window:  window,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *ipRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, s := range rl.clients {
			if now.After(s.resetAt) {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow returns true if the IP is within its rate limit for the current window.
func (rl *ipRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	s, ok := rl.clients[ip]
	if !ok || now.After(s.resetAt) {
		rl.clients[ip] = &clientState{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if s.count >= rl.maxReq {
		return false
	}
	s.count++
	return true
}

// RateLimitMiddleware rejects requests from IPs that exceed the limiter's threshold.
// Returns 429 Too Many Requests when the limit is hit.
func RateLimitMiddleware(limiter *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(clientIP(r)) {
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded",
					"Too many requests. Please slow down and try again.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the real client IP, respecting X-Forwarded-For from Cloud Run.
// Falls back to the direct remote address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may contain a comma-separated list; take the first entry.
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = xff[:idx]
		}
		ip := strings.TrimSpace(xff)
		if ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// wildcardOrigin is an allowlist entry of the form "https://*.pages.dev",
// split into the scheme and the host suffix the wildcard expands against.
type wildcardOrigin struct {
	scheme string // e.g. "https"
	suffix string // e.g. ".pages.dev" (leading dot included)
}

// originMatcher decides whether a request Origin is allowed to read responses.
type originMatcher struct {
	exact    map[string]bool
	wildcard []wildcardOrigin
}

// parseOrigins splits a comma-separated allowlist into exact origins plus
// subdomain wildcard patterns.
//
// An entry is either an exact origin ("https://kidsaber.app") or carries a
// single leading "*." in the host ("https://*.pages.dev"), which matches any
// subdomain — needed for Cloudflare Pages, where every deployment gets its own
// preview hostname. The wildcard requires at least one label in front of the
// suffix, so "https://*.pages.dev" accepts "https://foo.pages.dev" and
// "https://a.foo.pages.dev" but never the bare "https://pages.dev". Scheme and
// port must still match exactly.
func parseOrigins(s string) *originMatcher {
	m := &originMatcher{exact: make(map[string]bool)}
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if scheme, host, ok := strings.Cut(o, "://*."); ok && host != "" {
			m.wildcard = append(m.wildcard, wildcardOrigin{scheme: scheme, suffix: "." + host})
			continue
		}
		m.exact[o] = true
	}
	return m
}

// matches reports whether origin is in the allowlist. An empty origin (a
// non-browser or same-origin request) never matches.
func (m *originMatcher) matches(origin string) bool {
	if origin == "" {
		return false
	}
	if m.exact[origin] {
		return true
	}
	if len(m.wildcard) == 0 {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	for _, w := range m.wildcard {
		// len check guarantees a non-empty label before the suffix, so the
		// registrable domain itself is not accepted by its own wildcard.
		if u.Scheme == w.scheme && len(u.Host) > len(w.suffix) && strings.HasSuffix(u.Host, w.suffix) {
			return true
		}
	}
	return false
}

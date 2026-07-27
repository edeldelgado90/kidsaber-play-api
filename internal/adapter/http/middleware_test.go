package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 100}))
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// ─── LoggingMiddleware ────────────────────────────────────────────────────────

func TestLoggingMiddleware_CapturesStatusCode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	wrapped := httpAdapter.LoggingMiddleware(discardLogger())(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestLoggingMiddleware_DefaultsTo200(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no WriteHeader call
		_, _ = w.Write([]byte("ok"))
	})
	wrapped := httpAdapter.LoggingMiddleware(discardLogger())(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── RecoveryMiddleware ───────────────────────────────────────────────────────

func TestRecoveryMiddleware_Panic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
	wrapped := httpAdapter.RecoveryMiddleware(discardLogger())(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() { wrapped.ServeHTTP(rec, req) })
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var body httpAdapter.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, "internal_error", body.Error)
}

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	wrapped := httpAdapter.RecoveryMiddleware(discardLogger())(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── TimeoutMiddleware ────────────────────────────────────────────────────────

func TestTimeoutMiddleware_SetsDeadline(t *testing.T) {
	var hasDeadline bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasDeadline = r.Context().Deadline()
		w.WriteHeader(http.StatusOK)
	})
	wrapped := httpAdapter.TimeoutMiddleware(5 * time.Second)(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.True(t, hasDeadline)
}

// ─── CORSMiddleware ───────────────────────────────────────────────────────────

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("http://allowed.com")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://allowed.com")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, "http://allowed.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("http://allowed.com")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("http://allowed.com")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCORSMiddleware_OPTIONSPreflight(t *testing.T) {
	calledNext := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledNext = true
	})
	wrapped := httpAdapter.CORSMiddleware("http://allowed.com")(handler)

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://allowed.com")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, calledNext, "next handler should not be called for OPTIONS preflight")
}

func TestCORSMiddleware_PreflightAllowsAppCheckHeader(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("https://kidsaber-play.pages.dev")(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/questions", nil)
	req.Header.Set("Origin", "https://kidsaber-play.pages.dev")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-Firebase-AppCheck")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	// The auth middleware reads these; a preflight that omits them blocks the request.
	assert.Contains(t, allowHeaders, "X-Firebase-AppCheck")
	assert.Contains(t, allowHeaders, "X-API-Key")
	assert.Contains(t, allowHeaders, "Authorization")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Equal(t, "3600", rec.Header().Get("Access-Control-Max-Age"))
}

func TestCORSMiddleware_SetsVaryOrigin(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("https://kidsaber.app")(okHandler())

	for _, origin := range []string{"https://kidsaber.app", "https://evil.com", ""} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		assert.Contains(t, rec.Header().Values("Vary"), "Origin", "origin=%q", origin)
	}
}

func TestCORSMiddleware_WildcardPagesDevOrigin(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("https://*.pages.dev")(okHandler())

	// Cloudflare Pages assigns a fresh hostname to every deployment.
	origin := "https://a1b2c3d4.kidsaber-play.pages.dev"
	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, origin, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEqual(t, "*", rec.Header().Get("Access-Control-Allow-Origin"), "must echo the origin, never a wildcard")
}

func TestCORSMiddleware_WildcardDoesNotLeakToOtherDomains(t *testing.T) {
	wrapped := httpAdapter.CORSMiddleware("https://*.pages.dev")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Origin", "https://kidsaber-play.pages.dev.evil.com")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

// ─── SecurityHeadersMiddleware ────────────────────────────────────────────────

func TestSecurityHeadersMiddleware(t *testing.T) {
	wrapped := httpAdapter.SecurityHeadersMiddleware(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
}

// ─── AuthMiddleware ───────────────────────────────────────────────────────────

func authMiddlewareWrapped(enabled bool, key string, appCheck httpAdapter.AppCheckValidator) http.Handler {
	return httpAdapter.AuthMiddleware(httpAdapter.AuthConfig{
		Enabled:  enabled,
		APIKey:   key,
		AppCheck: appCheck,
	})(okHandler())
}

// mockIDToken accepts exactly one ID token and records the tokens it was asked
// to verify, so tests can assert what reached the verifier.
type mockIDToken struct {
	validToken string
	seen       []string
}

func (m *mockIDToken) VerifyIDToken(_ context.Context, token string) error {
	m.seen = append(m.seen, token)
	if token == m.validToken {
		return nil
	}
	return errors.New("invalid id token")
}

func authWithIDToken(key string, idToken httpAdapter.IDTokenVerifier) http.Handler {
	return httpAdapter.AuthMiddleware(httpAdapter.AuthConfig{
		Enabled: true,
		APIKey:  key,
		IDToken: idToken,
	})(okHandler())
}

func TestAuthMiddleware_Disabled(t *testing.T) {
	wrapped := authMiddlewareWrapped(false, "secret", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	wrapped := authMiddlewareWrapped(true, "my-secret-key", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer my-secret-key")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_ValidXAPIKey(t *testing.T) {
	wrapped := authMiddlewareWrapped(true, "my-secret-key", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "my-secret-key")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_WrongBearerToken(t *testing.T) {
	wrapped := authMiddlewareWrapped(true, "my-secret-key", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_ValidAppCheckToken(t *testing.T) {
	// Use the shared mockAppCheck from appcheck_test.go (same package http_test)
	appCheck := &mockAppCheck{validToken: "valid-token"}
	wrapped := authMiddlewareWrapped(true, "", appCheck)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Firebase-AppCheck", "valid-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_InvalidAppCheckToken(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-token"}
	wrapped := authMiddlewareWrapped(true, "key", appCheck)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Firebase-AppCheck", "bad-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_NoCredentials(t *testing.T) {
	wrapped := authMiddlewareWrapped(true, "secret", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_NilAppCheckNoAPIKey(t *testing.T) {
	wrapped := authMiddlewareWrapped(true, "key", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Firebase-AppCheck", "some-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ─── AuthMiddleware: Firebase ID tokens ───────────────────────────────────────

func TestAuthMiddleware_ValidIDToken(t *testing.T) {
	wrapped := authWithIDToken("static-key", &mockIDToken{validToken: "good-id-token"})

	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Authorization", "Bearer good-id-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_InvalidIDToken(t *testing.T) {
	wrapped := authWithIDToken("static-key", &mockIDToken{validToken: "good-id-token"})

	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Authorization", "Bearer forged-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_IDTokenNotAcceptedWhenVerifierNil(t *testing.T) {
	wrapped := authWithIDToken("static-key", nil)

	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Authorization", "Bearer some-id-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_StaticKeyStillWinsBearerSlot(t *testing.T) {
	verifier := &mockIDToken{validToken: "good-id-token"}
	wrapped := authWithIDToken("static-key", verifier)

	// Server-to-server callers keep sending the API key as a bearer token; it
	// must be accepted without ever reaching the ID token verifier.
	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Authorization", "Bearer static-key")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, verifier.seen, "API key must short-circuit before ID token verification")
}

func TestAuthMiddleware_IDTokenWorksWithoutStaticKey(t *testing.T) {
	wrapped := authWithIDToken("", &mockIDToken{validToken: "good-id-token"})

	req := httptest.NewRequest(http.MethodGet, "/questions", nil)
	req.Header.Set("Authorization", "Bearer good-id-token")
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_MalformedAuthorizationHeader(t *testing.T) {
	verifier := &mockIDToken{validToken: "good-id-token"}
	wrapped := authWithIDToken("static-key", verifier)

	for _, header := range []string{"good-id-token", "Basic good-id-token", "Bearer", "bearer good-id-token"} {
		req := httptest.NewRequest(http.MethodGet, "/questions", nil)
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Authorization: %q", header)
	}
	assert.Empty(t, verifier.seen, "no malformed header should reach the verifier")
}

func TestAuthMiddleware_AllThreeCredentialsCoexist(t *testing.T) {
	wrapped := httpAdapter.AuthMiddleware(httpAdapter.AuthConfig{
		Enabled:  true,
		APIKey:   "static-key",
		AppCheck: &mockAppCheck{validToken: "good-appcheck"},
		IDToken:  &mockIDToken{validToken: "good-id-token"},
	})(okHandler())

	tests := []struct {
		name   string
		header string
		value  string
	}{
		{"static key via X-API-Key", "X-API-Key", "static-key"},
		{"static key via bearer", "Authorization", "Bearer static-key"},
		{"firebase id token", "Authorization", "Bearer good-id-token"},
		{"app check token", "X-Firebase-AppCheck", "good-appcheck"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/questions", nil)
			req.Header.Set(tc.header, tc.value)
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// ─── RateLimitMiddleware ──────────────────────────────────────────────────────

func TestRateLimitMiddleware_AllowsUnderLimit(t *testing.T) {
	limiter := httpAdapter.NewIPRateLimiter(5, time.Minute)
	wrapped := httpAdapter.RateLimitMiddleware(limiter)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimitMiddleware_BlocksOverLimit(t *testing.T) {
	limiter := httpAdapter.NewIPRateLimiter(2, time.Minute)
	wrapped := httpAdapter.RateLimitMiddleware(limiter)(okHandler())

	ip := "5.6.7.8:9999"
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should pass", i+1)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var body httpAdapter.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Equal(t, "rate_limit_exceeded", body.Error)
}

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

const testTokenSecret = "test-secret-32-bytes-long-enough!"

// buildRouterWithToken creates a router wired with a TokenService.
func buildRouterWithToken(authEnabled bool, apiKey string) (http.Handler, *httpAdapter.TokenService) {
	tokenSvc := httpAdapter.NewTokenService([]byte(testTokenSecret), 24*time.Hour)
	tokenHandler := httpAdapter.NewTokenHandler(tokenSvc, newLogger())

	logger := newLogger()
	nopRepo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	uc := questions.NewGetQuestionsUseCase(nopGen(), nopGen(), nopRepo, noopNotifier{}, logger)
	qh := httpAdapter.NewQuestionsHandler(uc, logger)
	ah := httpAdapter.NewAdminHandler(&noopJobRepo{}, logger)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: qh,
		AdminHandler:     ah,
		TokenHandler:     tokenHandler,
		TokenValidator:   tokenSvc,
		Logger:           logger,
		AuthEnabled:      authEnabled,
		APIKey:           apiKey,
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})

	return router, tokenSvc
}

// postToken is a test helper that calls POST /auth/token and returns the response.
func postToken(t *testing.T, router http.Handler, deviceID string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"deviceId": deviceID})
	req := httptest.NewRequest(http.MethodPost, "/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ── POST /auth/token happy path ───────────────────────────────────────────────

func TestAuthToken_200_ValidDeviceID(t *testing.T) {
	router, _ := buildRouterWithToken(false, "")

	rec := postToken(t, router, "550e8400-e29b-41d4-a716-446655440000")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp httpAdapter.TokenResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotEmpty(t, resp.Token)
	assert.Greater(t, resp.ExpiresAt, time.Now().Unix())
}

// ── POST /auth/token error cases ─────────────────────────────────────────────

func TestAuthToken_400_MissingDeviceID(t *testing.T) {
	router, _ := buildRouterWithToken(false, "")

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var errResp httpAdapter.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "invalid_request", errResp.Error)
}

func TestAuthToken_400_EmptyBody(t *testing.T) {
	router, _ := buildRouterWithToken(false, "")

	req := httptest.NewRequest(http.MethodPost, "/auth/token", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuthToken_400_DeviceIDTooLong(t *testing.T) {
	router, _ := buildRouterWithToken(false, "")

	longID := make([]byte, 129)
	for i := range longID {
		longID[i] = 'a'
	}
	rec := postToken(t, router, string(longID))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ── POST /auth/token is public ────────────────────────────────────────────────

func TestAuthToken_IsPublic_WhenAuthEnabled(t *testing.T) {
	// POST /auth/token must be reachable without any credential even when
	// AUTH_ENABLED=true — it's the endpoint the app calls to GET credentials.
	router, _ := buildRouterWithToken(true, "my-api-key")

	// No Authorization header
	rec := postToken(t, router, "device-123")

	assert.Equal(t, http.StatusOK, rec.Code,
		"POST /auth/token must be public even with AUTH_ENABLED=true")
}

// ── Device token grants access to /questions ─────────────────────────────────

func TestDeviceToken_GrantsAccessToQuestions(t *testing.T) {
	// Full auth flow: obtain a device token, then use it on /questions.
	router, _ := buildRouterWithToken(true, "static-api-key")

	// Step 1: get token
	tokenRec := postToken(t, router, "device-abc")
	require.Equal(t, http.StatusOK, tokenRec.Code)

	var tokenResp httpAdapter.TokenResponse
	require.NoError(t, json.NewDecoder(tokenRec.Body).Decode(&tokenResp))
	require.NotEmpty(t, tokenResp.Token)

	// Step 2: call /questions with the device token
	// Pool is empty → use case returns error → 503, but critically NOT 401.
	questReq := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	questReq.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	questRec := httptest.NewRecorder()
	router.ServeHTTP(questRec, questReq)

	assert.NotEqual(t, http.StatusUnauthorized, questRec.Code,
		"valid device token must pass auth; got HTTP %d", questRec.Code)
}

// ── Static API key still works alongside device tokens ────────────────────────

func TestStaticAPIKey_StillAcceptedAlongsideTokenValidator(t *testing.T) {
	router, _ := buildRouterWithToken(true, "static-api-key")

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	req.Header.Set("X-API-Key", "static-api-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code,
		"static API key must still be accepted; got HTTP %d", rec.Code)
}

func TestStaticAPIKey_WrongKey_Returns401(t *testing.T) {
	router, _ := buildRouterWithToken(true, "static-api-key")

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── Admin route: device token is NOT accepted ─────────────────────────────────

func TestAdminRoute_RejectsDeviceToken(t *testing.T) {
	// Admin /admin/jobs must only accept the static API key, not device tokens.
	router, _ := buildRouterWithToken(true, "static-api-key")

	// Get a device token
	tokenRec := postToken(t, router, "device-xyz")
	require.Equal(t, http.StatusOK, tokenRec.Code)
	var tokenResp httpAdapter.TokenResponse
	require.NoError(t, json.NewDecoder(tokenRec.Body).Decode(&tokenResp))

	// Try to access admin endpoint with device token
	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"/admin/jobs must reject device tokens and require the static API key")
}

// ── Ensure existing tests still compile with updated RouterConfig ─────────────

func TestBuildRouter_BackwardCompat_NoTokenHandler(t *testing.T) {
	// Existing tests pass nil for TokenHandler; router must still work.
	logger := slog.Default()
	nopRepo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	uc := questions.NewGetQuestionsUseCase(nopGen(), nopGen(), nopRepo, noopNotifier{}, logger)
	qh := httpAdapter.NewQuestionsHandler(uc, logger)
	ah := httpAdapter.NewAdminHandler(&noopJobRepo{}, logger)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: qh,
		AdminHandler:     ah,
		// TokenHandler and TokenValidator intentionally omitted
		Logger:         logger,
		AuthEnabled:    false,
		AllowedOrigins: "http://localhost:3000",
		RequestTimeout: 30 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

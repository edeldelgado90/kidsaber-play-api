package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// mockAppCheck is a test double for AppCheckValidator.
type mockAppCheck struct {
	validToken string
}

func (m *mockAppCheck) VerifyToken(token string) error {
	if token == m.validToken {
		return nil
	}
	return errors.New("invalid app check token")
}

// buildRouterWithAppCheck wires a router with a given App Check validator.
func buildRouterWithAppCheck(authEnabled bool, apiKey string, appCheck httpAdapter.AppCheckValidator) http.Handler {
	logger := newLogger()
	nopRepo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	uc := questions.NewGetQuestionsUseCase(nopGen(), nopGen(), nopRepo, noopNotifier{}, logger)
	qh := httpAdapter.NewQuestionsHandler(uc, logger)
	ah := httpAdapter.NewAdminHandler(&noopJobRepo{}, logger)

	return httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: qh,
		AdminHandler:     ah,
		AppCheck:         appCheck,
		Logger:           logger,
		AuthEnabled:      authEnabled,
		APIKey:           apiKey,
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})
}

// ── Firebase App Check happy path ─────────────────────────────────────────────

func TestAppCheck_ValidToken_GrantsAccessToQuestions(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-app-check-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	req.Header.Set("X-Firebase-AppCheck", "valid-app-check-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code,
		"valid App Check token must pass auth; got HTTP %d", rec.Code)
}

// ── Firebase App Check error cases ────────────────────────────────────────────

func TestAppCheck_InvalidToken_Returns401(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-app-check-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	req.Header.Set("X-Firebase-AppCheck", "tampered-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAppCheck_NoCredentials_Returns401(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── Static API key still works alongside App Check ───────────────────────────

func TestStaticAPIKey_AcceptedAlongsideAppCheck(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	req.Header.Set("X-API-Key", "static-api-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code,
		"static API key must be accepted; got HTTP %d", rec.Code)
}

func TestStaticAPIKey_WrongKey_Returns401(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=quick_calculation", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── Admin route: App Check token is NOT accepted ──────────────────────────────

func TestAdminRoute_RejectsAppCheckToken(t *testing.T) {
	// /admin/jobs must only accept the static API key, never an App Check token.
	appCheck := &mockAppCheck{validToken: "valid-app-check-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	req.Header.Set("X-Firebase-AppCheck", "valid-app-check-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"/admin/jobs must reject App Check tokens and require the static API key")
}

func TestAdminRoute_AcceptsStaticAPIKey(t *testing.T) {
	appCheck := &mockAppCheck{validToken: "valid-token"}
	router := buildRouterWithAppCheck(true, "static-api-key", appCheck)

	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	req.Header.Set("X-API-Key", "static-api-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code,
		"static API key must be accepted on admin route; got HTTP %d", rec.Code)
}

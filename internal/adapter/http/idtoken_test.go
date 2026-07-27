package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// buildRouterWithIDToken wires a router with a given ID token verifier.
func buildRouterWithIDToken(apiKey string, idToken httpAdapter.IDTokenVerifier) http.Handler {
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
		IDToken:          idToken,
		Logger:           logger,
		AuthEnabled:      true,
		APIKey:           apiKey,
		AllowedOrigins:   "https://kidsaber-play.pages.dev",
		RequestTimeout:   30 * time.Second,
	})
}

const questionsPath = "/questions?subject=mathematics&grade=3&type=option_multiple"

func TestRouter_IDTokenGrantsQuestions(t *testing.T) {
	router := buildRouterWithIDToken("static-key", &mockIDToken{validToken: "good-id-token"})

	req := httptest.NewRequest(http.MethodGet, questionsPath, nil)
	req.Header.Set("Authorization", "Bearer good-id-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
}

func TestRouter_ForgedIDTokenRejected(t *testing.T) {
	router := buildRouterWithIDToken("static-key", &mockIDToken{validToken: "good-id-token"})

	req := httptest.NewRequest(http.MethodGet, questionsPath, nil)
	req.Header.Set("Authorization", "Bearer forged")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// Anyone can obtain an anonymous Firebase ID token, so it must never reach the
// admin surface — those routes are static-API-key only.
func TestRouter_IDTokenNeverUnlocksAdmin(t *testing.T) {
	verifier := &mockIDToken{validToken: "good-id-token"}
	router := buildRouterWithIDToken("static-key", verifier)

	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	req.Header.Set("Authorization", "Bearer good-id-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRouter_AppCheckNeverUnlocksAdmin(t *testing.T) {
	router := buildRouterWithAppCheck(true, "static-key", &mockAppCheck{validToken: "good-appcheck"})

	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	req.Header.Set("X-Firebase-AppCheck", "good-appcheck")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRouter_StaticKeyStillUnlocksAdmin(t *testing.T) {
	router := buildRouterWithIDToken("static-key", &mockIDToken{validToken: "good-id-token"})

	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	req.Header.Set("X-API-Key", "static-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// The web client sends its ID token cross-origin, so the preflight must pass
// and the actual request must carry the CORS header back.
func TestRouter_WebClientCORSAndIDTokenTogether(t *testing.T) {
	router := buildRouterWithIDToken("static-key", &mockIDToken{validToken: "good-id-token"})
	origin := "https://kidsaber-play.pages.dev"

	preflight := httptest.NewRequest(http.MethodOptions, "/questions", nil)
	preflight.Header.Set("Origin", origin)
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	preflight.Header.Set("Access-Control-Request-Headers", "Authorization")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, preflight)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, origin, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Authorization")

	actual := httptest.NewRequest(http.MethodGet, questionsPath, nil)
	actual.Header.Set("Origin", origin)
	actual.Header.Set("Authorization", "Bearer good-id-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, actual)

	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, origin, rec.Header().Get("Access-Control-Allow-Origin"))
}

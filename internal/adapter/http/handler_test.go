package http_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// ─── Mocks ───────────────────────────────────────────────────────────────────

type mockGen struct {
	fn func(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error)
}

func (m *mockGen) Generate(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error) {
	return m.fn(ctx, params)
}

type mockQRepo struct {
	findFn func(ctx context.Context, params questions.FindParams, count int) ([]domain.Question, error)
}

func (m *mockQRepo) FindRandom(ctx context.Context, params questions.FindParams, count int) ([]domain.Question, error) {
	return m.findFn(ctx, params, count)
}
func (m *mockQRepo) Save(_ context.Context, _ []domain.Question) error               { return nil }
func (m *mockQRepo) InsertBatch(_ context.Context, _ []domain.Question) error        { return nil }
func (m *mockQRepo) Count(_ context.Context, _ questions.FindParams) (int, error)    { return 0, nil }
func (m *mockQRepo) DeleteMostUsed(_ context.Context, _ questions.FindParams, _ int) error {
	return nil
}
func (m *mockQRepo) IncrementUsageCount(_ context.Context, _ []string) error { return nil }

type noopJobRepo struct{}

func (n *noopJobRepo) Insert(_ context.Context, _ *domain.JobRun) error              { return nil }
func (n *noopJobRepo) Update(_ context.Context, _ *domain.JobRun) error              { return nil }
func (n *noopJobRepo) FindRecent(_ context.Context, _ int) ([]domain.JobRun, error)  { return nil, nil }

type noopNotifier struct{}

func (noopNotifier) Alert(_ context.Context, _ unotify.NotificationEvent) error { return nil }

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func nopGen() *mockGen {
	return &mockGen{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, nil
	}}
}

func makeTestQuestion() domain.Question {
	ca, _ := json.Marshal([]string{"B"})
	return domain.Question{
		ID:        "test-id-001",
		Type:      domain.GameTypeOptionMultiple,
		Subject:   domain.SubjectMathematics,
		Grade:     3,
		Topic:     "multiplication_tables_complete",
		Statement: "¿Cuánto es 4 × 6?",
		Options: []domain.Option{
			{ID: "A", Text: "20"},
			{ID: "B", Text: "24"},
			{ID: "C", Text: "26"},
			{ID: "D", Text: "16"},
		},
		CorrectAnswers: json.RawMessage(ca),
		Meta: domain.QuestionMeta{
			Difficulty:  domain.DifficultyEasy,
			TimeLimitMs: 15000,
			Tags:        []string{"multiplication"},
		},
	}
}

func buildRouter(calcGen, llmGen questions.QuestionGenerator, repo questions.QuestionRepository) http.Handler {
	logger := newLogger()
	uc := questions.NewGetQuestionsUseCase(calcGen, llmGen, repo, noopNotifier{}, logger)
	qh := httpAdapter.NewQuestionsHandler(uc, logger)
	ah := httpAdapter.NewAdminHandler(&noopJobRepo{}, logger)

	return httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: qh,
		AdminHandler:     ah,
		Logger:           logger,
		AuthEnabled:      false,
		APIKey:           "",
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestGetQuestions_200_ServedFromPool(t *testing.T) {
	expected := makeTestQuestion()

	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, count int) ([]domain.Question, error) {
		qs := make([]domain.Question, count)
		for i := range qs {
			qs[i] = expected
		}
		return qs, nil
	}}

	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=option_multiple&count=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp httpAdapter.QuestionsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Len(t, resp.Questions, 5)
}

func TestGetQuestions_400_MissingSubject(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet, "/questions?grade=3&type=option_multiple", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp httpAdapter.ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "invalid_params", errResp.Error)
}

func TestGetQuestions_400_InvalidGrade(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=9&type=option_multiple", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetQuestions_400_InvalidType(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=unknown_type", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetQuestions_503_OnLLMFailure(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil // empty pool
	}}
	failingLLM := &mockGen{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, domain.ErrNoValidQuestions
	}}

	router := buildRouter(nopGen(), failingLLM, repo)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=language&grade=4&type=fill_in_the_blanks&count=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestGetQuestions_429_OnRateLimit(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	rateLimitedLLM := &mockGen{fn: func(_ context.Context, _ questions.GenerateParams) ([]domain.Question, error) {
		return nil, domain.ErrRateLimit
	}}

	router := buildRouter(nopGen(), rateLimitedLLM, repo)

	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=science&grade=5&type=matching&count=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestHealthEndpoint(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAdminJobsEndpoint_200(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/jobs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp httpAdapter.JobRunsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotNil(t, resp.Runs) // empty slice is fine
}

func TestSecurityHeaders(t *testing.T) {
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	router := buildRouter(nopGen(), nopGen(), repo)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
}

func TestAuthMiddleware_401_WhenEnabled(t *testing.T) {
	logger := newLogger()
	repo := &mockQRepo{findFn: func(_ context.Context, _ questions.FindParams, _ int) ([]domain.Question, error) {
		return nil, nil
	}}
	uc := questions.NewGetQuestionsUseCase(nopGen(), nopGen(), repo, noopNotifier{}, logger)
	qh := httpAdapter.NewQuestionsHandler(uc, logger)
	ah := httpAdapter.NewAdminHandler(&noopJobRepo{}, logger)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: qh,
		AdminHandler:     ah,
		Logger:           logger,
		AuthEnabled:      true,
		APIKey:           "secret-key",
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})

	// No key provided
	req := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=option_multiple", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Correct key
	req2 := httptest.NewRequest(http.MethodGet,
		"/questions?subject=mathematics&grade=3&type=option_multiple", nil)
	req2.Header.Set("X-API-Key", "secret-key")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	// Pool is empty, LLM returns nothing, but not 401
	assert.NotEqual(t, http.StatusUnauthorized, rec2.Code)
}

package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpAdapter "github.com/kidsaber/kidsaber-play-api/internal/adapter/http"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/reports"
)

const reportedQuestionID = "11111111-2222-3333-4444-555555555555"

// ─── Mocks ───────────────────────────────────────────────────────────────────

type mockReportRepo struct {
	findErr   error
	outcome   domain.ReportOutcome
	recordErr error
	calls     int
}

func (m *mockReportRepo) FindQuestionSummary(_ context.Context, id string) (domain.QuestionSummary, error) {
	if m.findErr != nil {
		return domain.QuestionSummary{}, m.findErr
	}
	return domain.QuestionSummary{
		ID:        id,
		Subject:   domain.SubjectMathematics,
		Grade:     3,
		Type:      domain.GameTypeOptionMultiple,
		Statement: "¿Cuánto es 7 × 8?",
	}, nil
}

func (m *mockReportRepo) RecordReport(_ context.Context, _ domain.QuestionSummary) (domain.ReportOutcome, error) {
	m.calls++
	if m.recordErr != nil {
		return domain.ReportOutcome{}, m.recordErr
	}
	return m.outcome, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// newReportRouter builds a router with only the report route wired, with the
// rate limiter off so a test can issue several requests without tripping it.
func newReportRouter(repo *mockReportRepo) http.Handler {
	logger := newLogger()
	uc := reports.NewReportQuestionUseCase(repo, noopNotifier{}, logger)

	return httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: httpAdapter.NewQuestionsHandler(nil, logger),
		ReportsHandler:   httpAdapter.NewReportsHandler(uc, logger),
		AdminHandler:     httpAdapter.NewAdminHandler(&noopJobRepo{}, logger),
		Logger:           logger,
		AuthEnabled:      false,
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})
}

func postReport(router http.Handler, id string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/questions/"+id+"/report", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestReportQuestion_202_OnFirstReport(t *testing.T) {
	repo := &mockReportRepo{outcome: domain.ReportOutcome{Created: true, Count: 1}}

	rec := postReport(newReportRouter(repo), reportedQuestionID)

	require.Equal(t, http.StatusAccepted, rec.Code)

	var body httpAdapter.ReportQuestionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "received", body.Status)
	assert.Equal(t, 1, repo.calls)
}

// The response must not leak how many people flagged the question — that would
// hand an abuser a progress meter.
func TestReportQuestion_202_DoesNotLeakReportCount(t *testing.T) {
	repo := &mockReportRepo{outcome: domain.ReportOutcome{Created: false, Count: 42}}

	rec := postReport(newReportRouter(repo), reportedQuestionID)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.NotContains(t, rec.Body.String(), "42")
}

func TestReportQuestion_400_OnMalformedID(t *testing.T) {
	for _, id := range []string{
		"not-a-uuid",
		"1111111122223333444455555555555",       // right length, no hyphens
		"1111111g-2222-3333-4444-555555555555",  // non-hex character
		"11111111-2222-3333-4444-5555555555555", // too long
	} {
		repo := &mockReportRepo{}

		rec := postReport(newReportRouter(repo), id)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "id %q", id)
		assert.Zero(t, repo.calls, "a malformed id must never reach the database")
	}
}

func TestReportQuestion_404_OnUnknownQuestion(t *testing.T) {
	repo := &mockReportRepo{findErr: domain.ErrNotFound}

	rec := postReport(newReportRouter(repo), reportedQuestionID)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Zero(t, repo.calls)
}

func TestReportQuestion_500_OnRepositoryError(t *testing.T) {
	repo := &mockReportRepo{recordErr: errors.New("db down")}

	rec := postReport(newReportRouter(repo), reportedQuestionID)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "db down", "internal errors must not reach the client")
}

// Browsers preflight a cross-origin POST; without POST in Allow-Methods the
// report never leaves the web build.
func TestReportQuestion_CORSAllowsPost(t *testing.T) {
	router := newReportRouter(&mockReportRepo{})

	req := httptest.NewRequest(http.MethodOptions, "/questions/"+reportedQuestionID+"/report", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
}

// An anonymous ID token proves nothing — anyone can mint one — so the hardened
// configuration must reject it and accept only App Check.
func TestReportQuestion_RequireAppCheck_RejectsIDTokenAcceptsAppCheck(t *testing.T) {
	logger := newLogger()
	repo := &mockReportRepo{outcome: domain.ReportOutcome{Created: true, Count: 1}}
	uc := reports.NewReportQuestionUseCase(repo, noopNotifier{}, logger)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler:       httpAdapter.NewQuestionsHandler(nil, logger),
		ReportsHandler:         httpAdapter.NewReportsHandler(uc, logger),
		AdminHandler:           httpAdapter.NewAdminHandler(&noopJobRepo{}, logger),
		Logger:                 logger,
		AuthEnabled:            true,
		APIKey:                 "secret-key",
		AppCheck:               stubAppCheck{},
		IDToken:                stubIDToken{},
		AllowedOrigins:         "http://localhost:3000",
		RequestTimeout:         30 * time.Second,
		ReportsRequireAppCheck: true,
	})

	path := "/questions/" + reportedQuestionID + "/report"

	withIDToken := httptest.NewRequest(http.MethodPost, path, nil)
	withIDToken.Header.Set("Authorization", "Bearer valid-id-token")
	recID := httptest.NewRecorder()
	router.ServeHTTP(recID, withIDToken)
	assert.Equal(t, http.StatusUnauthorized, recID.Code)

	withAppCheck := httptest.NewRequest(http.MethodPost, path, nil)
	withAppCheck.Header.Set("X-Firebase-AppCheck", "valid-app-check")
	recAC := httptest.NewRecorder()
	router.ServeHTTP(recAC, withAppCheck)
	assert.Equal(t, http.StatusAccepted, recAC.Code)
}

// With the flag off — the shipping default, because App Check is web-only for
// now — a native client's ID token must still be accepted.
func TestReportQuestion_WithoutRequireAppCheck_AcceptsIDToken(t *testing.T) {
	logger := newLogger()
	repo := &mockReportRepo{outcome: domain.ReportOutcome{Created: true, Count: 1}}
	uc := reports.NewReportQuestionUseCase(repo, noopNotifier{}, logger)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: httpAdapter.NewQuestionsHandler(nil, logger),
		ReportsHandler:   httpAdapter.NewReportsHandler(uc, logger),
		AdminHandler:     httpAdapter.NewAdminHandler(&noopJobRepo{}, logger),
		Logger:           logger,
		AuthEnabled:      true,
		APIKey:           "secret-key",
		AppCheck:         stubAppCheck{},
		IDToken:          stubIDToken{},
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})

	req := httptest.NewRequest(http.MethodPost, "/questions/"+reportedQuestionID+"/report", nil)
	req.Header.Set("Authorization", "Bearer valid-id-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
}

// The report route carries its own, far tighter limiter than the global one.
func TestReportQuestion_429_AfterRouteRateLimit(t *testing.T) {
	logger := newLogger()
	repo := &mockReportRepo{outcome: domain.ReportOutcome{Created: true, Count: 1}}
	uc := reports.NewReportQuestionUseCase(repo, noopNotifier{}, logger)

	router := httpAdapter.NewRouter(httpAdapter.RouterConfig{
		QuestionsHandler: httpAdapter.NewQuestionsHandler(nil, logger),
		ReportsHandler:   httpAdapter.NewReportsHandler(uc, logger),
		AdminHandler:     httpAdapter.NewAdminHandler(&noopJobRepo{}, logger),
		Logger:           logger,
		AuthEnabled:      false,
		AllowedOrigins:   "http://localhost:3000",
		RequestTimeout:   30 * time.Second,
	})

	var lastCode int
	for i := 0; i < 8; i++ {
		lastCode = postReport(router, reportedQuestionID).Code
	}

	assert.Equal(t, http.StatusTooManyRequests, lastCode)
}

// ─── Auth stubs ──────────────────────────────────────────────────────────────

type stubAppCheck struct{}

func (stubAppCheck) VerifyToken(token string) error {
	if token == "valid-app-check" {
		return nil
	}
	return errors.New("invalid app check token")
}

type stubIDToken struct{}

func (stubIDToken) VerifyIDToken(_ context.Context, token string) error {
	if token == "valid-id-token" {
		return nil
	}
	return errors.New("invalid id token")
}

var _ unotify.NotificationService = noopNotifier{}

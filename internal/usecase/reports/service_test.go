package reports_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/reports"
)

// ─── Mocks ───────────────────────────────────────────────────────────────────

type mockReportRepo struct {
	summary   domain.QuestionSummary
	findErr   error
	outcome   domain.ReportOutcome
	recordErr error

	mu       sync.Mutex
	recorded []domain.QuestionSummary
}

func (m *mockReportRepo) FindQuestionSummary(_ context.Context, id string) (domain.QuestionSummary, error) {
	if m.findErr != nil {
		return domain.QuestionSummary{}, m.findErr
	}
	s := m.summary
	s.ID = id
	return s, nil
}

func (m *mockReportRepo) RecordReport(_ context.Context, s domain.QuestionSummary) (domain.ReportOutcome, error) {
	m.mu.Lock()
	m.recorded = append(m.recorded, s)
	m.mu.Unlock()
	if m.recordErr != nil {
		return domain.ReportOutcome{}, m.recordErr
	}
	return m.outcome, nil
}

// captureNotifier records events on a buffered channel so the test can wait for
// the use case's detached notification goroutine instead of sleeping.
type captureNotifier struct {
	events chan unotify.NotificationEvent
	err    error
}

func newCaptureNotifier() *captureNotifier {
	return &captureNotifier{events: make(chan unotify.NotificationEvent, 4)}
}

func (c *captureNotifier) Alert(_ context.Context, e unotify.NotificationEvent) error {
	c.events <- e
	return c.err
}

// waitForEvent returns the next event, failing the test if none arrives.
func (c *captureNotifier) waitForEvent(t *testing.T) unotify.NotificationEvent {
	t.Helper()
	select {
	case e := <-c.events:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification, none was sent")
		return unotify.NotificationEvent{}
	}
}

// expectNoEvent asserts nothing is notified within a short grace period.
func (c *captureNotifier) expectNoEvent(t *testing.T) {
	t.Helper()
	select {
	case e := <-c.events:
		t.Fatalf("expected no notification, got %q", e.Type)
	case <-time.After(150 * time.Millisecond):
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func validSummary() domain.QuestionSummary {
	return domain.QuestionSummary{
		Subject:   domain.SubjectMathematics,
		Grade:     3,
		Type:      domain.GameTypeOptionMultiple,
		Statement: "¿Cuánto es 7 × 8?",
	}
}

const testID = "11111111-2222-3333-4444-555555555555"

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestExecute_FirstReport_RecordsAndNotifies(t *testing.T) {
	repo := &mockReportRepo{summary: validSummary(), outcome: domain.ReportOutcome{Created: true, Count: 1}}
	notifier := newCaptureNotifier()
	uc := reports.NewReportQuestionUseCase(repo, notifier, newLogger())

	outcome, err := uc.Execute(context.Background(), testID)

	require.NoError(t, err)
	assert.True(t, outcome.Created)
	assert.Equal(t, 1, outcome.Count)

	event := notifier.waitForEvent(t)
	assert.Equal(t, unotify.EventQuestionReport, event.Type)
	require.NotNil(t, event.Report)
	assert.Equal(t, testID, event.Report.QuestionID)
	assert.Equal(t, "mathematics", event.Report.Subject)
	assert.Equal(t, 3, event.Report.Grade)
	assert.Equal(t, "¿Cuánto es 7 × 8?", event.Report.Statement)
}

// A repeat report must stay silent, otherwise tapping the link becomes a way to
// spam the Discord channel.
func TestExecute_RepeatReport_RecordsWithoutNotifying(t *testing.T) {
	repo := &mockReportRepo{summary: validSummary(), outcome: domain.ReportOutcome{Created: false, Count: 7}}
	notifier := newCaptureNotifier()
	uc := reports.NewReportQuestionUseCase(repo, notifier, newLogger())

	outcome, err := uc.Execute(context.Background(), testID)

	require.NoError(t, err)
	assert.False(t, outcome.Created)
	assert.Equal(t, 7, outcome.Count)
	notifier.expectNoEvent(t)
}

func TestExecute_UnknownQuestion_ReturnsNotFoundAndStoresNothing(t *testing.T) {
	repo := &mockReportRepo{findErr: domain.ErrNotFound}
	notifier := newCaptureNotifier()
	uc := reports.NewReportQuestionUseCase(repo, notifier, newLogger())

	_, err := uc.Execute(context.Background(), testID)

	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, repo.recorded, "no report may be stored for a question that does not exist")
	notifier.expectNoEvent(t)
}

// Everything written must come from the DB lookup, never from the caller.
func TestExecute_StoresSummaryFromRepositoryLookup(t *testing.T) {
	repo := &mockReportRepo{summary: validSummary(), outcome: domain.ReportOutcome{Created: true, Count: 1}}
	uc := reports.NewReportQuestionUseCase(repo, newCaptureNotifier(), newLogger())

	_, err := uc.Execute(context.Background(), testID)

	require.NoError(t, err)
	require.Len(t, repo.recorded, 1)
	assert.Equal(t, domain.SubjectMathematics, repo.recorded[0].Subject)
	assert.Equal(t, 3, repo.recorded[0].Grade)
	assert.Equal(t, "¿Cuánto es 7 × 8?", repo.recorded[0].Statement)
}

func TestExecute_RecordFailure_PropagatesError(t *testing.T) {
	boom := errors.New("db down")
	repo := &mockReportRepo{summary: validSummary(), recordErr: boom}
	notifier := newCaptureNotifier()
	uc := reports.NewReportQuestionUseCase(repo, notifier, newLogger())

	_, err := uc.Execute(context.Background(), testID)

	assert.ErrorIs(t, err, boom)
	notifier.expectNoEvent(t)
}

// A missing webhook must not stop reports from being recorded.
func TestExecute_NilNotifier_StillRecords(t *testing.T) {
	repo := &mockReportRepo{summary: validSummary(), outcome: domain.ReportOutcome{Created: true, Count: 1}}
	uc := reports.NewReportQuestionUseCase(repo, nil, newLogger())

	outcome, err := uc.Execute(context.Background(), testID)

	require.NoError(t, err)
	assert.True(t, outcome.Created)
	assert.Len(t, repo.recorded, 1)
}

// The notification runs detached from the request, so a webhook failure must
// never surface to the player.
func TestExecute_NotifierFailure_DoesNotFailRequest(t *testing.T) {
	repo := &mockReportRepo{summary: validSummary(), outcome: domain.ReportOutcome{Created: true, Count: 1}}
	notifier := newCaptureNotifier()
	notifier.err = errors.New("discord down")
	uc := reports.NewReportQuestionUseCase(repo, notifier, newLogger())

	_, err := uc.Execute(context.Background(), testID)

	require.NoError(t, err)
	notifier.waitForEvent(t)
}

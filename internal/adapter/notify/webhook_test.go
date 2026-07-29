package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/notify"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

func TestNewWebhookNotifier_EmptyURL(t *testing.T) {
	assert.Nil(t, notify.NewWebhookNotifier(""))
}

func TestNewWebhookNotifier_NonEmpty(t *testing.T) {
	n := notify.NewWebhookNotifier("http://example.com/webhook")
	assert.NotNil(t, n)
}

func TestWebhookNotifier_Alert_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	err := n.Alert(context.Background(), unotify.NotificationEvent{Type: "job_success"})
	assert.NoError(t, err)
}

func TestWebhookNotifier_Alert_HTTP400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	err := n.Alert(context.Background(), unotify.NotificationEvent{Type: "job_failure"})
	assert.ErrorContains(t, err, "400")
}

func TestWebhookNotifier_Alert_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	err := n.Alert(context.Background(), unotify.NotificationEvent{Type: "job_failure"})
	assert.Error(t, err)
}

func TestWebhookNotifier_Alert_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n := notify.NewWebhookNotifier(srv.URL)
	err := n.Alert(ctx, unotify.NotificationEvent{Type: "job_failure"})
	assert.Error(t, err)
}

// captureBody captures the request body as a discord-style payload.
type discordMsg struct {
	Content string `json:"content"`
}

func captureServer(t *testing.T) (*httptest.Server, *discordMsg) {
	t.Helper()
	captured := &discordMsg{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, captured))
		w.WriteHeader(http.StatusOK)
	}))
	return srv, captured
}

func TestWebhookNotifier_Payload_JobFailure(t *testing.T) {
	srv, msg := captureServer(t)
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	require.NoError(t, n.Alert(context.Background(), unotify.NotificationEvent{
		Type:        "job_failure",
		JobRunID:    "run-123",
		FailedCount: 2,
		Details: []unotify.FailureDetail{
			{Subject: "mathematics", Grade: 3, Type: "option_multiple", Error: "timeout"},
		},
	}))

	assert.Contains(t, msg.Content, "⚠️")
	assert.Contains(t, msg.Content, "failure")
	assert.Contains(t, msg.Content, "2")
	assert.Contains(t, msg.Content, "run-123")
	assert.Contains(t, msg.Content, "mathematics")
}

func TestWebhookNotifier_Payload_PoolLow(t *testing.T) {
	srv, msg := captureServer(t)
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	require.NoError(t, n.Alert(context.Background(), unotify.NotificationEvent{Type: "pool_low"}))
	assert.Contains(t, msg.Content, "🔶")
	assert.Contains(t, msg.Content, "pool")
}

func TestWebhookNotifier_Payload_JobSuccess(t *testing.T) {
	srv, msg := captureServer(t)
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	require.NoError(t, n.Alert(context.Background(), unotify.NotificationEvent{Type: "job_success"}))
	assert.Contains(t, msg.Content, "✅")
	assert.Contains(t, msg.Content, "completed")
}

func TestWebhookNotifier_Payload_UnknownType(t *testing.T) {
	srv, msg := captureServer(t)
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	require.NoError(t, n.Alert(context.Background(), unotify.NotificationEvent{Type: "info"}))
	assert.Contains(t, msg.Content, "ℹ️")
}

func TestWebhookNotifier_Payload_WithJobRunID(t *testing.T) {
	srv, msg := captureServer(t)
	defer srv.Close()

	n := notify.NewWebhookNotifier(srv.URL)
	require.NoError(t, n.Alert(context.Background(), unotify.NotificationEvent{
		Type:     "job_success",
		JobRunID: "abc-456",
	}))
	assert.Contains(t, msg.Content, "abc-456")
}

// ─── Question report payload ─────────────────────────────────────────────────

// capturePayload posts to a test server and returns the Discord content field.
func capturePayload(t *testing.T, event unotify.NotificationEvent) string {
	t.Helper()

	var content string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload struct {
			Content string `json:"content"`
		}
		require.NoError(t, json.Unmarshal(body, &payload))
		content = payload.Content
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, notify.NewWebhookNotifier(srv.URL).Alert(context.Background(), event))
	return content
}

func reportEvent(r *unotify.ReportDetail) unotify.NotificationEvent {
	return unotify.NotificationEvent{Type: unotify.EventQuestionReport, Report: r}
}

func TestWebhookNotifier_QuestionReport_IncludesQuestionDetails(t *testing.T) {
	content := capturePayload(t, reportEvent(&unotify.ReportDetail{
		QuestionID:  "11111111-2222-3333-4444-555555555555",
		Subject:     "mathematics",
		Grade:       3,
		Type:        "option_multiple",
		Statement:   "¿Cuánto es 7 × 8?",
		ReportCount: 1,
	}))

	assert.Contains(t, content, "Pregunta reportada")
	assert.Contains(t, content, "mathematics")
	assert.Contains(t, content, "3.º")
	assert.Contains(t, content, "¿Cuánto es 7 × 8?")
	assert.Contains(t, content, "11111111-2222-3333-4444-555555555555")
}

// A statement that reaches a channel full of humans must not be able to ping
// them, nor to break out of the code fence it is rendered inside.
func TestWebhookNotifier_QuestionReport_DefusesMentionsAndFences(t *testing.T) {
	content := capturePayload(t, reportEvent(&unotify.ReportDetail{
		QuestionID:  "11111111-2222-3333-4444-555555555555",
		Subject:     "language",
		Grade:       2,
		Type:        "matching",
		Statement:   "@everyone ``` fake alert",
		ReportCount: 1,
	}))

	assert.NotContains(t, content, "@everyone", "the mention must be defused")
	assert.Contains(t, content, "@​everyone", "text stays readable with a zero-width space")
	assert.NotContains(t, content, "``` fake alert", "backticks must not close the fence")
}

func TestWebhookNotifier_QuestionReport_TruncatesLongStatement(t *testing.T) {
	content := capturePayload(t, reportEvent(&unotify.ReportDetail{
		QuestionID:  "11111111-2222-3333-4444-555555555555",
		Subject:     "science",
		Grade:       6,
		Type:        "fill_in_the_blanks",
		Statement:   strings.Repeat("a", 500),
		ReportCount: 1,
	}))

	assert.Contains(t, content, "…")
	assert.NotContains(t, content, strings.Repeat("a", 400))
}

func TestWebhookNotifier_QuestionReport_ShowsCountOnlyWhenRepeated(t *testing.T) {
	base := unotify.ReportDetail{
		QuestionID: "11111111-2222-3333-4444-555555555555",
		Subject:    "english",
		Grade:      1,
		Type:       "matching",
		Statement:  "Match the words",
	}

	first := base
	first.ReportCount = 1
	assert.NotContains(t, capturePayload(t, reportEvent(&first)), "reportes")

	repeat := base
	repeat.ReportCount = 4
	assert.Contains(t, capturePayload(t, reportEvent(&repeat)), "4 reportes")
}

// A malformed event must fall through to the generic renderer, not panic.
func TestWebhookNotifier_QuestionReport_NilReportFallsBack(t *testing.T) {
	content := capturePayload(t, unotify.NotificationEvent{Type: unotify.EventQuestionReport})
	assert.Contains(t, content, "KidSaber API notification")
}

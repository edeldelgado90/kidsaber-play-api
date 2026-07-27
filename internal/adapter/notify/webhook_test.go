package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

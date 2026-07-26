package notify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

// ─── parseRecipients ─────────────────────────────────────────────────────────

func TestParseRecipients_Empty(t *testing.T) {
	assert.Empty(t, parseRecipients(""))
}

func TestParseRecipients_Single(t *testing.T) {
	assert.Equal(t, []string{"a@example.com"}, parseRecipients("a@example.com"))
}

func TestParseRecipients_Multiple(t *testing.T) {
	result := parseRecipients("a@example.com, b@example.com , c@example.com")
	assert.Equal(t, []string{"a@example.com", "b@example.com", "c@example.com"}, result)
}

// ─── buildEmailContent ────────────────────────────────────────────────────────

func TestBuildEmailContent_JobFailure(t *testing.T) {
	event := unotify.NotificationEvent{
		Type:        "job_failure",
		FailedCount: 3,
		JobRunID:    "run-abc",
		Timestamp:   time.Now().UTC(),
		Details: []unotify.FailureDetail{
			{Subject: "mathematics", Grade: 3, Type: "option_multiple", Error: "timeout"},
		},
	}
	subject, plain, html := buildEmailContent(event)
	assert.Contains(t, subject, "failure")
	assert.Contains(t, subject, "3")
	assert.Contains(t, plain, "job_failure")
	assert.Contains(t, plain, "run-abc")
	assert.Contains(t, plain, "mathematics")
	assert.Contains(t, html, "<html>")
	assert.Contains(t, html, "run-abc")
}

func TestBuildEmailContent_PoolLow(t *testing.T) {
	event := unotify.NotificationEvent{Type: "pool_low", Timestamp: time.Now().UTC()}
	subject, _, _ := buildEmailContent(event)
	assert.Contains(t, subject, "pool")
}

func TestBuildEmailContent_JobSuccess(t *testing.T) {
	event := unotify.NotificationEvent{Type: "job_success", Timestamp: time.Now().UTC()}
	subject, _, _ := buildEmailContent(event)
	assert.Contains(t, subject, "success")
}

func TestBuildEmailContent_UnknownType(t *testing.T) {
	event := unotify.NotificationEvent{Type: "unknown", Timestamp: time.Now().UTC()}
	subject, _, _ := buildEmailContent(event)
	assert.Contains(t, subject, "Notification")
}

func TestBuildEmailContent_NoJobRunID(t *testing.T) {
	event := unotify.NotificationEvent{Type: "job_success", JobRunID: "", Timestamp: time.Now().UTC()}
	_, plain, html := buildEmailContent(event)
	assert.NotContains(t, plain, "Job:")
	// HTML: the {{if .JobRunID}} block should not render
	assert.NotContains(t, html, "Job ID:")
}

// ─── buildMIMEMessage ─────────────────────────────────────────────────────────

func TestBuildMIMEMessage_ContainsHeaders(t *testing.T) {
	msg := buildMIMEMessage("from@example.com", []string{"to@example.com"}, "Subject!", "plain body", "<html>html</html>")
	s := string(msg)
	assert.Contains(t, s, "From: from@example.com")
	assert.Contains(t, s, "To: to@example.com")
	assert.Contains(t, s, "Subject: Subject!")
	assert.Contains(t, s, "MIME-Version: 1.0")
	assert.Contains(t, s, "plain body")
	assert.Contains(t, s, "<html>html</html>")
}

func TestBuildMIMEMessage_NoHTMLPart(t *testing.T) {
	msg := buildMIMEMessage("from@example.com", []string{"to@example.com"}, "Sub", "body", "")
	s := string(msg)
	assert.Contains(t, s, "body")
	// Without HTML, there should be no HTML content-type
	assert.False(t, strings.Contains(s, "text/html"), "no html part expected")
}

func TestBuildMIMEMessage_MultipleRecipients(t *testing.T) {
	msg := buildMIMEMessage("from@example.com", []string{"a@example.com", "b@example.com"}, "Sub", "body", "")
	s := string(msg)
	assert.Contains(t, s, "a@example.com, b@example.com")
}

// ─── Alert (error paths, no real SMTP needed) ─────────────────────────────────

func TestSMTPNotifier_Alert_CancelledContext(t *testing.T) {
	n := &SMTPNotifier{
		host:       "smtp.example.com",
		port:       587,
		dailyLimit: 0,
		lastDay:    today(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := n.Alert(ctx, unotify.NotificationEvent{Type: "job_success"})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSMTPNotifier_Alert_DailyLimitExceeded(t *testing.T) {
	n := &SMTPNotifier{
		host:       "smtp.example.com",
		port:       587,
		dailyLimit: 1,
		sentToday:  1, // already at limit
		lastDay:    today(),
	}
	err := n.Alert(context.Background(), unotify.NotificationEvent{Type: "job_success"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daily limit")
}

func TestSMTPNotifier_Alert_SMTPSendFails(t *testing.T) {
	// Point to localhost:1 — guaranteed to fail connection quickly.
	n := &SMTPNotifier{
		host:       "127.0.0.1",
		port:       1,
		from:       "from@example.com",
		to:         []string{"to@example.com"},
		dailyLimit: 0,
		lastDay:    today(),
	}
	err := n.Alert(context.Background(), unotify.NotificationEvent{
		Type:      "job_failure",
		Timestamp: time.Now().UTC(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP")
}

// NewSMTPNotifier with empty host returns nil.
func TestNewSMTPNotifier_EmptyHost(t *testing.T) {
	assert.Nil(t, NewSMTPNotifier("", 587, "u", "p", "f@x.com", "t@x.com", 5))
}

// NewSMTPNotifier with valid host returns non-nil.
func TestNewSMTPNotifier_ValidHost(t *testing.T) {
	n := NewSMTPNotifier("smtp.example.com", 587, "u", "p", "f@x.com", "t@x.com", 5)
	assert.NotNil(t, n)
}

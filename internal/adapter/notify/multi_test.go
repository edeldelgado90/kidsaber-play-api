package notify_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/notify"
	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

type countingNotifier struct {
	calls  int
	events []unotify.NotificationEvent
	err    error
}

func (c *countingNotifier) Alert(_ context.Context, event unotify.NotificationEvent) error {
	c.calls++
	c.events = append(c.events, event)
	return c.err
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestMultiNotifier_CallsAllNotifiers(t *testing.T) {
	n1 := &countingNotifier{}
	n2 := &countingNotifier{}

	multi := notify.NewMultiNotifier(newTestLogger(), n1, n2)

	event := unotify.NotificationEvent{
		Type:      "job_failure",
		Timestamp: time.Now(),
	}

	err := multi.Alert(context.Background(), event)
	require.NoError(t, err)

	assert.Equal(t, 1, n1.calls)
	assert.Equal(t, 1, n2.calls)
}

func TestMultiNotifier_ContinuesAfterIndividualFailure(t *testing.T) {
	n1 := &countingNotifier{err: errors.New("webhook down")}
	n2 := &countingNotifier{}

	multi := notify.NewMultiNotifier(newTestLogger(), n1, n2)

	event := unotify.NotificationEvent{Type: "pool_low", Timestamp: time.Now()}
	err := multi.Alert(context.Background(), event)

	// MultiNotifier always returns nil — individual failures are logged, not propagated
	require.NoError(t, err)
	assert.Equal(t, 1, n1.calls)
	assert.Equal(t, 1, n2.calls, "n2 should be called even if n1 fails")
}

func TestMultiNotifier_FiltersNilNotifiers(t *testing.T) {
	n1 := &countingNotifier{}

	// Pass nil as second notifier (e.g. disabled webhook)
	multi := notify.NewMultiNotifier(newTestLogger(), n1, nil)

	err := multi.Alert(context.Background(), unotify.NotificationEvent{Type: "job_success"})
	require.NoError(t, err)
	assert.Equal(t, 1, n1.calls)
}

func TestNoopNotifier_NeverReturnsError(t *testing.T) {
	n := notify.NewNoopNotifier()
	err := n.Alert(context.Background(), unotify.NotificationEvent{Type: "job_failure"})
	assert.NoError(t, err)
}

func TestWebhookNotifier_NilWhenEmptyURL(t *testing.T) {
	n := notify.NewWebhookNotifier("")
	assert.Nil(t, n)
}

func TestSMTPNotifier_NilWhenEmptyHost(t *testing.T) {
	n := notify.NewSMTPNotifier("", 587, "", "", "", "", 50)
	assert.Nil(t, n)
}

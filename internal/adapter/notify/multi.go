package notify

import (
	"context"
	"log/slog"

	unotify "github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

// MultiNotifier combines multiple NotificationService implementations.
// All configured notifiers are called; individual errors are logged but not propagated.
// Notifications are best-effort and must not block the main application flow.
type MultiNotifier struct {
	notifiers []unotify.NotificationService
	logger    *slog.Logger
}

// NewMultiNotifier creates a MultiNotifier from a list of notifiers.
// nil entries (disabled notifiers) are filtered out.
func NewMultiNotifier(logger *slog.Logger, notifiers ...unotify.NotificationService) *MultiNotifier {
	var active []unotify.NotificationService
	for _, n := range notifiers {
		if n != nil {
			active = append(active, n)
		}
	}
	return &MultiNotifier{notifiers: active, logger: logger}
}

// Alert calls Alert on every configured notifier.
// Always returns nil — individual failures are logged, not propagated.
func (m *MultiNotifier) Alert(ctx context.Context, event unotify.NotificationEvent) error {
	for _, n := range m.notifiers {
		if err := n.Alert(ctx, event); err != nil {
			m.logger.Warn("notifier failed",
				"type", event.Type,
				"error", err)
		}
	}
	return nil
}

// noopNotifier is a no-op implementation used when all notifiers are disabled.
type noopNotifier struct{}

func (noopNotifier) Alert(_ context.Context, _ unotify.NotificationEvent) error { return nil }

// NewNoopNotifier returns a NotificationService that does nothing.
// Used when no notification channels are configured.
func NewNoopNotifier() unotify.NotificationService { return noopNotifier{} }

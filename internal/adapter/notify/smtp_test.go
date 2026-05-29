package notify

// White-box tests for SMTPNotifier's daily rate-limiter.
// They live in package notify (not notify_test) so they can access unexported
// fields (sentToday, lastDay) and call withinDailyLimit directly — no real
// SMTP connection is needed.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSMTPNotifier(dailyLimit int) *SMTPNotifier {
	return &SMTPNotifier{
		host:       "smtp.example.com",
		port:       587,
		dailyLimit: dailyLimit,
		lastDay:    today(),
	}
}

func TestSMTPNotifier_NoDailyLimit(t *testing.T) {
	n := newTestSMTPNotifier(0)

	// Should always return true regardless of how many times called.
	for i := 0; i < 1000; i++ {
		assert.True(t, n.withinDailyLimit(), "call %d should be within limit", i+1)
	}
}

func TestSMTPNotifier_DailyLimitEnforced(t *testing.T) {
	const limit = 3
	n := newTestSMTPNotifier(limit)

	for i := 0; i < limit; i++ {
		require.True(t, n.withinDailyLimit(), "call %d should be allowed", i+1)
	}

	// The next call must be rejected.
	assert.False(t, n.withinDailyLimit(), "call after limit should be rejected")
	assert.Equal(t, limit, n.sentToday, "counter must not exceed limit")
}

func TestSMTPNotifier_CounterResetsOnNewDay(t *testing.T) {
	const limit = 2
	n := newTestSMTPNotifier(limit)

	// Exhaust today's budget.
	require.True(t, n.withinDailyLimit())
	require.True(t, n.withinDailyLimit())
	require.False(t, n.withinDailyLimit())

	// Simulate the clock advancing to tomorrow.
	n.mu.Lock()
	n.lastDay = n.lastDay.Add(-25 * time.Hour) // push lastDay into the past
	n.mu.Unlock()

	// Budget should be fresh.
	assert.True(t, n.withinDailyLimit(), "first call on new day should be allowed")
	assert.Equal(t, 1, n.sentToday, "counter should restart at 1 after reset")
}

func TestSMTPNotifier_LimitOfOne(t *testing.T) {
	n := newTestSMTPNotifier(1)

	assert.True(t, n.withinDailyLimit(), "first call must be allowed")
	assert.False(t, n.withinDailyLimit(), "second call must be rejected")
}

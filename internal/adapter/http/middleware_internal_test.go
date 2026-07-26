package http

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ─── clientIP ─────────────────────────────────────────────────────────────────

func TestClientIP_XForwardedFor_Single(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	assert.Equal(t, "203.0.113.1", clientIP(req))
}

func TestClientIP_XForwardedFor_CommaList(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1, 172.16.0.5")
	assert.Equal(t, "203.0.113.1", clientIP(req))
}

func TestClientIP_NoXFF_ValidRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.7:54321"
	assert.Equal(t, "198.51.100.7", clientIP(req))
}

func TestClientIP_NoXFF_MalformedRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "not-an-addr"
	assert.Equal(t, "not-an-addr", clientIP(req))
}

// ─── parseOrigins ─────────────────────────────────────────────────────────────

func TestParseOrigins_Empty(t *testing.T) {
	m := parseOrigins("")
	assert.Empty(t, m)
}

func TestParseOrigins_Single(t *testing.T) {
	m := parseOrigins("http://example.com")
	assert.Len(t, m, 1)
	assert.True(t, m["http://example.com"])
}

func TestParseOrigins_Multiple(t *testing.T) {
	m := parseOrigins("http://a.com, http://b.com , http://c.com")
	assert.Len(t, m, 3)
	assert.True(t, m["http://a.com"])
	assert.True(t, m["http://b.com"])
	assert.True(t, m["http://c.com"])
}

// ─── ipRateLimiter.Allow ──────────────────────────────────────────────────────

func TestIPRateLimiter_NewIP_Allowed(t *testing.T) {
	rl := NewIPRateLimiter(5, time.Minute)
	assert.True(t, rl.Allow("1.2.3.4"))
}

func TestIPRateLimiter_WithinLimit(t *testing.T) {
	rl := NewIPRateLimiter(3, time.Minute)
	assert.True(t, rl.Allow("1.2.3.4"))
	assert.True(t, rl.Allow("1.2.3.4"))
	assert.True(t, rl.Allow("1.2.3.4"))
}

func TestIPRateLimiter_ExceedsLimit(t *testing.T) {
	rl := NewIPRateLimiter(2, time.Minute)
	assert.True(t, rl.Allow("1.2.3.4"))
	assert.True(t, rl.Allow("1.2.3.4"))
	assert.False(t, rl.Allow("1.2.3.4"))
}

func TestIPRateLimiter_DifferentIPs_Independent(t *testing.T) {
	rl := NewIPRateLimiter(1, time.Minute)
	assert.True(t, rl.Allow("1.1.1.1"))
	assert.False(t, rl.Allow("1.1.1.1"))
	assert.True(t, rl.Allow("2.2.2.2")) // different IP, not affected
}

func TestIPRateLimiter_WindowReset(t *testing.T) {
	rl := NewIPRateLimiter(1, 10*time.Millisecond)
	assert.True(t, rl.Allow("5.5.5.5"))
	assert.False(t, rl.Allow("5.5.5.5"))

	// Wait for window to expire
	time.Sleep(20 * time.Millisecond)
	assert.True(t, rl.Allow("5.5.5.5"), "window should have reset")
}

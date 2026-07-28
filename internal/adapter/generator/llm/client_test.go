package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// ─── Claude streaming test doubles ────────────────────────────────────────────
//
// The client calls Messages.NewStreaming, so the fakes below speak the Messages
// API's Server-Sent Events wire format rather than returning a JSON body.

// sseEvent writes a single Server-Sent Event and flushes it, so the SDK's
// decoder sees the events arrive rather than one buffered blob at close.
func sseEvent(w http.ResponseWriter, name, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// writeClaudeStream streams a complete assistant turn carrying text as a single
// text block. An empty text emits no content block at all, which is how a turn
// with nothing usable in it looks on the wire. stopDetails may be empty.
func writeClaudeStream(w http.ResponseWriter, text, stopReason, stopDetails string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	sseEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-opus-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`)

	if text != "" {
		encoded, _ := json.Marshal(text)
		sseEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		sseEvent(w, "content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, encoded))
		sseEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
	}

	details := ""
	if stopDetails != "" {
		details = `,"stop_details":` + stopDetails
	}
	sseEvent(w, "message_delta", fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q,"stop_sequence":null%s},"usage":{"output_tokens":20}}`, stopReason, details))
	sseEvent(w, "message_stop", `{"type":"message_stop"}`)
}

// claudeTextHandler answers every request with a well-formed turn returning text.
func claudeTextHandler(text string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeClaudeStream(w, text, "end_turn", "")
	}
}

// newTestClient points a client at srv with a short timeout.
func newTestClient(baseURL string) *llm.LLMClient {
	return llm.NewLLMClient(baseURL, "test-key", "claude-opus-5", "high", 5*time.Second)
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestLLMClient_Complete_Success(t *testing.T) {
	srv := httptest.NewServer(claudeTextHandler("hello world"))
	defer srv.Close()

	got, err := newTestClient(srv.URL).Complete(context.Background(), "say hello")
	require.NoError(t, err)
	assert.Equal(t, "hello world", got)
}

func TestLLMClient_Complete_SendsModelAndEffort(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeClaudeStream(w, "ok", "end_turn", "")
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	require.NoError(t, err)

	assert.Equal(t, "claude-opus-5", body["model"])
	assert.Equal(t, true, body["stream"], "long generations must stream")
	assert.Equal(t, map[string]any{"type": "adaptive"}, body["thinking"])
	assert.Equal(t, map[string]any{"effort": "high"}, body["output_config"])
}

func TestLLMClient_Complete_OmitsOutputConfigWhenEffortEmpty(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeClaudeStream(w, "ok", "end_turn", "")
	}))
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "test-key", "claude-opus-5", "", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	require.NoError(t, err)

	assert.NotContains(t, body, "output_config", "empty effort must fall back to the API default")
}

func TestLLMClient_Complete_NoTextContent(t *testing.T) {
	srv := httptest.NewServer(claudeTextHandler(""))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "no text content")
}

func TestLLMClient_Complete_Refusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeClaudeStream(w, "", "refusal", `{"type":"refusal","category":"cyber"}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	require.Error(t, err)
	assert.ErrorContains(t, err, "declined")
	assert.ErrorContains(t, err, "cyber")
}

func TestLLMClient_Complete_TruncatedAtMaxTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeClaudeStream(w, `[{"partial":`, "max_tokens", "")
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "token limit")
}

func TestLLMClient_Complete_RateLimit429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	assert.True(t, errors.Is(err, domain.ErrRateLimit), "expected ErrRateLimit, got: %v", err)
}

func TestLLMClient_Complete_ServerError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "Claude service error")
}

func TestLLMClient_Complete_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "Claude API error (403)")
}

func TestLLMClient_Complete_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // close immediately

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "Claude request failed")
}

func TestLLMClient_Complete_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(claudeTextHandler("hello"))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call

	_, err := newTestClient(srv.URL).Complete(ctx, "prompt")
	assert.Error(t, err)
}

func TestLLMClient_Complete_NeverLogsAPIKey(t *testing.T) {
	var authHeader, apiKeyHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		apiKeyHeader = r.Header.Get("X-Api-Key")
		writeClaudeStream(w, "ok", "end_turn", "")
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Complete(context.Background(), "prompt")
	require.NoError(t, err)

	// Claude authenticates with x-api-key, not a bearer token.
	assert.Equal(t, "test-key", apiKeyHeader)
	assert.Empty(t, authHeader)
}

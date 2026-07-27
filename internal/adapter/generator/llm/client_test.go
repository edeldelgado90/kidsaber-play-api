package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kidsaber/kidsaber-play-api/internal/adapter/generator/llm"
	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

type llmResp struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func makeServer(t *testing.T, statusCode int, body any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if body != nil {
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("test server encode error: %v", err)
			}
		}
	}))
}

func validLLMBody(content string) llmResp {
	r := llmResp{}
	r.Choices = []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	}{
		{Message: struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{Role: "assistant", Content: content}},
	}
	return r
}

func TestLLMClient_Complete_Success(t *testing.T) {
	srv := makeServer(t, http.StatusOK, validLLMBody("hello world"))
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "test-key", "model", 5*time.Second)
	got, err := client.Complete(context.Background(), "say hello")
	require.NoError(t, err)
	assert.Equal(t, "hello world", got)
}

func TestLLMClient_Complete_EmptyChoices(t *testing.T) {
	srv := makeServer(t, http.StatusOK, llmResp{})
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "empty response")
}

func TestLLMClient_Complete_EmptyContent(t *testing.T) {
	srv := makeServer(t, http.StatusOK, validLLMBody(""))
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "empty response")
}

func TestLLMClient_Complete_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.Error(t, err)
}

func TestLLMClient_Complete_APIError(t *testing.T) {
	body := llmResp{}
	body.Error = &struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	}{Message: "rate limit exceeded", Type: "rate_limit"}

	srv := makeServer(t, http.StatusOK, body)
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "rate limit exceeded")
}

func TestLLMClient_Complete_RateLimit429(t *testing.T) {
	srv := makeServer(t, http.StatusTooManyRequests, nil)
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.True(t, errors.Is(err, domain.ErrRateLimit), "expected ErrRateLimit, got: %v", err)
}

func TestLLMClient_Complete_ServerError500(t *testing.T) {
	srv := makeServer(t, http.StatusInternalServerError, nil)
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "LLM provider error")
}

func TestLLMClient_Complete_UnexpectedStatus(t *testing.T) {
	srv := makeServer(t, http.StatusForbidden, nil)
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.ErrorContains(t, err, "unexpected LLM status")
}

func TestLLMClient_Complete_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(context.Background(), "prompt")
	assert.Error(t, err)
}

func TestLLMClient_Complete_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, err := client.Complete(ctx, "prompt")
	assert.Error(t, err)
}

func TestLLMClient_TrailingSlashStripped(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(validLLMBody("ok"))
	}))
	defer srv.Close()

	// Add trailing slash to base URL
	client := llm.NewLLMClient(srv.URL+"/", "key", "model", 5*time.Second)
	_, _ = client.Complete(context.Background(), "prompt")
	assert.False(t, strings.HasPrefix(receivedPath, "//"), "double-slash in path: %s", receivedPath)
	assert.Equal(t, "/chat/completions", receivedPath)
}

func TestLLMClient_NoAPIKeyOmitsHeader(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(validLLMBody("ok"))
	}))
	defer srv.Close()

	client := llm.NewLLMClient(srv.URL, "", "model", 5*time.Second)
	_, _ = client.Complete(context.Background(), "prompt")
	assert.Empty(t, authHeader)
}

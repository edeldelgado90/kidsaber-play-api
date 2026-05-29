package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// openAIRequest is the request body for the OpenAI-compatible chat completions API.
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse is the response from the OpenAI-compatible chat completions API.
type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *openAIError `json:"error,omitempty"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code,omitempty"`
}

// RateLimitError carries the HTTP 429 details returned by the LLM provider.
// It wraps domain.ErrRateLimit so callers can still use errors.Is, and
// additionally exposes the Retry-After value and the raw provider message
// so the backoff logic and logs have precise information.
type RateLimitError struct {
	RetryAfter time.Duration // 0 if the header was absent or unparseable
	Message    string        // provider error message from the response body
}

func (e *RateLimitError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", domain.ErrRateLimit.Error(), e.Message)
	}
	return domain.ErrRateLimit.Error()
}

func (e *RateLimitError) Is(target error) bool { return target == domain.ErrRateLimit }

// LLMClient wraps an OpenAI-compatible HTTP API.
// Works with Gemini Flash, Groq, and Ollama via different env vars.
type LLMClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// NewLLMClient creates a configured LLMClient.
func NewLLMClient(baseURL, apiKey, model string, timeout time.Duration) *LLMClient {
	return &LLMClient{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

// Complete sends a prompt to the LLM and returns the response text.
// It never logs the prompt or response content.
func (c *LLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	reqBody := openAIRequest{
		Model: c.model,
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshalling LLM request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating LLM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("LLM request timed out: %w", err)
		}
		return "", fmt.Errorf("LLM HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading LLM response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		rlErr := &RateLimitError{}

		// Parse Retry-After header (seconds integer, as sent by Gemini / Groq / OpenAI).
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, parseErr := time.ParseDuration(ra + "s"); parseErr == nil {
				rlErr.RetryAfter = secs
			}
		}

		// Extract provider message from the body (best-effort; never fatal).
		var rateLimitResp openAIResponse
		if jsonErr := json.Unmarshal(respBytes, &rateLimitResp); jsonErr == nil && rateLimitResp.Error != nil {
			rlErr.Message = rateLimitResp.Error.Message
		}

		return "", rlErr
	}
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("LLM provider error (%d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected LLM status %d", resp.StatusCode)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", fmt.Errorf("parsing LLM response JSON: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 || apiResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("LLM returned empty response")
	}

	return apiResp.Choices[0].Message.Content, nil
}

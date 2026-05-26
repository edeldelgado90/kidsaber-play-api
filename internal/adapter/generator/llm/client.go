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
		return "", fmt.Errorf("LLM rate limit exceeded (429)")
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

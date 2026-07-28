package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// maxOutputTokens bounds a single generation batch. A batch of ten matching
// questions with four pairs each runs long, and the ceiling covers adaptive
// thinking as well as the answer — the two share this budget.
const maxOutputTokens = 32000

// LLMClient wraps the Claude Messages API for question generation.
type LLMClient struct {
	client anthropic.Client
	model  string
	effort anthropic.OutputConfigEffort
}

// NewLLMClient creates a client for the Claude API.
//
// baseURL is optional and exists for tests; empty means the default endpoint.
// The timeout bounds the whole request — generation with adaptive thinking runs
// far longer than a plain completion, so allow minutes rather than seconds.
func NewLLMClient(baseURL, apiKey, model, effort string, timeout time.Duration) *LLMClient {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(timeout),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	return &LLMClient{
		client: anthropic.NewClient(opts...),
		model:  model,
		effort: anthropic.OutputConfigEffort(effort),
	}
}

// Complete sends a prompt to Claude and returns the response text.
// It never logs the prompt or response content.
func (c *LLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	// Adaptive thinking lets Claude decide how much to reason per question set;
	// effort caps the overall spend. Sampling parameters are rejected by the
	// current models, so output variety comes from the prompt instead.
	adaptive := anthropic.ThinkingConfigAdaptiveParam{}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxOutputTokens,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}
	if c.effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: c.effort}
	}

	// Streaming keeps the connection alive across long generations; a
	// non-streaming call at this max_tokens risks an HTTP timeout.
	stream := c.client.Messages.NewStreaming(ctx, params)

	var message anthropic.Message
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("accumulating Claude stream: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", classifyClaudeError(err)
	}

	// A safety classifier can decline the request: HTTP 200, no usable content.
	if message.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("Claude declined the request (%s)", message.StopDetails.Category)
	}

	var text string
	for _, block := range message.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text += b.Text
		}
	}
	if text == "" {
		return "", errors.New("Claude returned no text content")
	}

	// A truncated batch yields invalid JSON downstream; fail with the real cause.
	if message.StopReason == anthropic.StopReasonMaxTokens {
		return "", fmt.Errorf("Claude response hit the %d token limit", maxOutputTokens)
	}

	return text, nil
}

// classifyClaudeError maps API failures onto the domain errors the job's retry
// and backoff logic already understands.
func classifyClaudeError(err error) error {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("Claude request timed out: %w", err)
		}
		return fmt.Errorf("Claude request failed: %w", err)
	}

	switch {
	case apiErr.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w", domain.ErrRateLimit)
	case apiErr.StatusCode >= 500:
		return fmt.Errorf("Claude service error (%d)", apiErr.StatusCode)
	default:
		return fmt.Errorf("Claude API error (%d): %s", apiErr.StatusCode, apiErr.Error())
	}
}

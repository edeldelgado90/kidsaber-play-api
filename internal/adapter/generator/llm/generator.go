package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
	"github.com/kidsaber/kidsaber-play-api/pkg/validator"
)

// LLMGenerator implements questions.QuestionGenerator using an OpenAI-compatible LLM API.
// It handles prompt building, retries on schema validation failure, and topic selection.
type LLMGenerator struct {
	client      *LLMClient
	validator   *validator.QuestionValidator
	topicPicker *TopicPicker
	maxRetries  int
	logger      *slog.Logger
}

// NewLLMGenerator creates a configured LLMGenerator.
func NewLLMGenerator(
	client *LLMClient,
	v *validator.QuestionValidator,
	topicPicker *TopicPicker,
	maxRetries int,
	logger *slog.Logger,
) *LLMGenerator {
	return &LLMGenerator{
		client:      client,
		validator:   v,
		topicPicker: topicPicker,
		maxRetries:  maxRetries,
		logger:      logger,
	}
}

// Generate calls the LLM to produce questions, retrying on schema validation failures.
// If all retries fail, returns domain.ErrNoValidQuestions.
func (g *LLMGenerator) Generate(ctx context.Context, params questions.GenerateParams) ([]domain.Question, error) {
	if !domain.IsLLMGameType(params.Type) {
		return nil, fmt.Errorf("LLMGenerator does not handle type: %s", params.Type)
	}

	// Pick a topic if not specified
	topic := params.Topic
	if topic == "" {
		var err error
		topic, err = g.topicPicker.Pick(params.Subject, params.Grade, params.Type)
		if err != nil {
			g.logger.Warn("topic picker failed, using fallback topic",
				"subject", params.Subject, "grade", params.Grade, "type", params.Type, "error", err)
			topic = "general"
		}
	}

	promptData := PromptData{
		SubjectSpanish: domain.SubjectSpanish(params.Subject),
		Subject:        string(params.Subject),
		Grade:          params.Grade,
		AgeMin:         params.Grade + 5,
		AgeMax:         params.Grade + 6,
		Topic:          topic,
		Count:          params.Count,
	}

	basePrompt, err := BuildPrompt(params.Type, promptData)
	if err != nil {
		return nil, fmt.Errorf("building prompt: %w", err)
	}

	currentPrompt := basePrompt
	var lastValidationErrors string

	for attempt := 0; attempt <= g.maxRetries; attempt++ {
		if attempt > 0 {
			// Prepend correction header for retry attempts
			retryPrompt, buildErr := BuildRetryPrompt(basePrompt, lastValidationErrors, params.Count)
			if buildErr != nil {
				return nil, fmt.Errorf("building retry prompt: %w", buildErr)
			}
			currentPrompt = retryPrompt
			g.logger.Info("retrying LLM generation",
				"attempt", attempt, "subject", params.Subject,
				"grade", params.Grade, "type", params.Type)
		}

		responseText, llmErr := g.completeWithRateLimitBackoff(ctx, currentPrompt)
		if llmErr != nil {
			if errors.Is(llmErr, domain.ErrRateLimit) {
				return nil, domain.ErrRateLimit
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, domain.ErrLLMTimeout
			}
			g.logger.Warn("LLM call failed", "attempt", attempt, "error", llmErr)
			lastValidationErrors = llmErr.Error()
			continue
		}

		rawBytes, qs, parseErr := ParseQuestions(responseText)
		if parseErr != nil {
			g.logger.Warn("failed to parse LLM response", "attempt", attempt, "error", parseErr)
			lastValidationErrors = parseErr.Error()
			continue
		}

		if validErr := g.validator.ValidateRaw(params.Type, rawBytes); validErr != nil {
			g.logger.Warn("LLM response failed schema validation",
				"attempt", attempt, "error", validErr)
			lastValidationErrors = validErr.Error()
			continue
		}

		// Stamp the correct type/subject/grade on all questions (trust but verify)
		for i := range qs {
			qs[i].Type = params.Type
			qs[i].Subject = params.Subject
			qs[i].Grade = params.Grade
		}

		return qs, nil
	}

	g.logger.Error("all LLM retries exhausted",
		"subject", params.Subject, "grade", params.Grade, "type", params.Type,
		"last_error", lastValidationErrors)
	return nil, domain.ErrNoValidQuestions
}

// completeWithRateLimitBackoff calls the LLM client and retries with exponential backoff
// when the provider returns a 429. Non-rate-limit errors are returned immediately.
// Default backoff schedule: 30 s → 60 s → 120 s (3 retries, then gives up).
// If the provider sends a Retry-After header its value is used instead of the
// calculated delay, so we honour the provider's own guidance precisely.
func (g *LLMGenerator) completeWithRateLimitBackoff(ctx context.Context, prompt string) (string, error) {
	const maxRLRetries = 3
	const baseDelay = 30 * time.Second

	text, err := g.client.Complete(ctx, prompt)
	if err == nil {
		return text, nil
	}

	for i := 0; i < maxRLRetries; i++ {
		if !errors.Is(err, domain.ErrRateLimit) {
			return "", err // not a rate-limit error — surface it immediately
		}

		// Use Retry-After from the provider when available; otherwise fall back
		// to the exponential schedule (30 s → 60 s → 120 s).
		delay := baseDelay * (1 << i)
		providerMsg := ""
		if rlErr := new(RateLimitError); errors.As(err, &rlErr) {
			providerMsg = rlErr.Message
			if rlErr.RetryAfter > 0 {
				delay = rlErr.RetryAfter
			}
		}

		g.logger.Warn("LLM rate limited, backing off",
			"delay_s", int(delay.Seconds()),
			"rl_attempt", i+1,
			"max_rl_retries", maxRLRetries,
			"provider_msg", providerMsg,
		)

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}

		text, err = g.client.Complete(ctx, prompt)
		if err == nil {
			return text, nil
		}
	}

	return "", domain.ErrRateLimit
}

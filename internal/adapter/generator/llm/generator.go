package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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

		responseText, llmErr := g.client.Complete(ctx, currentPrompt)
		if llmErr != nil {
			// Distinguish rate limit errors from other failures
			if strings.Contains(llmErr.Error(), "429") {
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

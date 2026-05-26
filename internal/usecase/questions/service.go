package questions

import (
	"context"
	"log/slog"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

// GetQuestionsUseCase orchestrates question retrieval for a game session.
//
// Flow:
//  1. quick_calculation → ProceduralGenerator (always; no DB)
//  2. LLM types        → QuestionRepository.FindRandom (DB pool)
//  3. Pool empty/small → LLMGenerator fallback + async DB save + async pool_low alert
type GetQuestionsUseCase struct {
	proceduralGen QuestionGenerator
	llmGen        QuestionGenerator
	repo          QuestionRepository
	notifier      notify.NotificationService
	logger        *slog.Logger
}

// NewGetQuestionsUseCase builds the use case with all required dependencies.
func NewGetQuestionsUseCase(
	proceduralGen QuestionGenerator,
	llmGen QuestionGenerator,
	repo QuestionRepository,
	notifier notify.NotificationService,
	logger *slog.Logger,
) *GetQuestionsUseCase {
	return &GetQuestionsUseCase{
		proceduralGen: proceduralGen,
		llmGen:        llmGen,
		repo:          repo,
		notifier:      notifier,
		logger:        logger,
	}
}

// Execute retrieves a batch of questions according to the request parameters.
func (uc *GetQuestionsUseCase) Execute(ctx context.Context, params GetQuestionsParams) ([]domain.Question, error) {
	if params.Count <= 0 {
		params.Count = 10 // default session size
	}

	// quick_calculation is always generated procedurally — never reads from DB or LLM.
	if params.Type == domain.GameTypeQuickCalc {
		return uc.proceduralGen.Generate(ctx, GenerateParams{
			Subject: params.Subject,
			Grade:   params.Grade,
			Type:    params.Type,
			Count:   params.Count,
		})
	}

	// Serve from pre-generated DB pool.
	findParams := FindParams{
		Subject: params.Subject,
		Grade:   params.Grade,
		Type:    params.Type,
	}

	questions, err := uc.repo.FindRandom(ctx, findParams, params.Count)
	if err != nil {
		uc.logger.Error("failed to query question pool", "error", err,
			"subject", params.Subject, "grade", params.Grade, "type", params.Type)
		// Fall through to LLM fallback.
	}

	if len(questions) >= params.Count {
		// Increment usage counts asynchronously — does not block the HTTP response.
		ids := make([]string, len(questions))
		for i, q := range questions {
			ids[i] = q.ID
		}
		go func() {
			if err := uc.repo.IncrementUsageCount(context.Background(), ids); err != nil {
				uc.logger.Warn("failed to increment usage counts", "error", err)
			}
		}()
		return questions[:params.Count], nil
	}

	// Pool is too small — alert asynchronously and fall back to LLM.
	go func() {
		event := notify.NotificationEvent{
			Type:   "pool_low",
			Status: "warning",
			Details: []notify.FailureDetail{
				{Subject: string(params.Subject), Grade: params.Grade, Type: string(params.Type)},
			},
		}
		if alertErr := uc.notifier.Alert(context.Background(), event); alertErr != nil {
			uc.logger.Warn("failed to send pool_low notification", "error", alertErr)
		}
	}()

	uc.logger.Warn("question pool empty or too small, falling back to LLM",
		"subject", params.Subject, "grade", params.Grade, "type", params.Type,
		"found", len(questions), "needed", params.Count)

	generated, err := uc.llmGen.Generate(ctx, GenerateParams{
		Subject: params.Subject,
		Grade:   params.Grade,
		Type:    params.Type,
		Count:   params.Count,
	})
	if err != nil {
		return nil, err
	}

	// Persist for future requests asynchronously — does not block the HTTP response.
	go func() {
		if saveErr := uc.repo.Save(context.Background(), generated); saveErr != nil {
			uc.logger.Warn("failed to save LLM-generated questions", "error", saveErr)
		}
	}()

	return generated, nil
}

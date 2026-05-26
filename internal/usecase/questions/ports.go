package questions

import (
	"context"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// GenerateParams carries the parameters for question generation.
type GenerateParams struct {
	Subject domain.Subject
	Grade   int
	Type    domain.GameType
	Topic   string // specific curriculum topic; may be empty for procedural
	Count   int
}

// FindParams carries the parameters for DB question lookup.
type FindParams struct {
	Subject domain.Subject
	Grade   int
	Type    domain.GameType
}

// GetQuestionsParams carries the parameters for the GetQuestionsUseCase.
type GetQuestionsParams struct {
	Subject domain.Subject
	Grade   int
	Type    domain.GameType
	Count   int
}

// QuestionGenerator generates questions (LLM or procedural).
// Implementations: LLMGenerator (option_multiple, fill_in_the_blanks, matching)
// and ProceduralGenerator (quick_calculation).
type QuestionGenerator interface {
	Generate(ctx context.Context, params GenerateParams) ([]domain.Question, error)
}

// QuestionRepository reads and stores pre-generated questions from PostgreSQL.
type QuestionRepository interface {
	// FindRandom returns `count` random questions matching the filter.
	// Returns an empty slice (not an error) when fewer than `count` are available.
	FindRandom(ctx context.Context, params FindParams, count int) ([]domain.Question, error)

	// Save persists questions asynchronously (called after LLM fallback generation).
	Save(ctx context.Context, questions []domain.Question) error

	// InsertBatch inserts questions in bulk (used by the background job).
	InsertBatch(ctx context.Context, questions []domain.Question) error

	// Count returns the number of questions stored for a given combination.
	Count(ctx context.Context, params FindParams) (int, error)

	// DeleteMostUsed removes the `limit` most-used questions for a combination.
	// Only called after a successful InsertBatch.
	DeleteMostUsed(ctx context.Context, params FindParams, limit int) error

	// IncrementUsageCount increments usage_count for the given question IDs.
	IncrementUsageCount(ctx context.Context, ids []string) error
}

// JobRunRepository reads and writes background job execution history.
type JobRunRepository interface {
	Insert(ctx context.Context, run *domain.JobRun) error
	Update(ctx context.Context, run *domain.JobRun) error
	FindRecent(ctx context.Context, limit int) ([]domain.JobRun, error)
}

package reports

import (
	"context"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
)

// ReportRepository stores player reports about questions.
type ReportRepository interface {
	// FindQuestionSummary returns the stored question's classification and
	// statement. Returns domain.ErrNotFound when no question has that id.
	FindQuestionSummary(ctx context.Context, questionID string) (domain.QuestionSummary, error)

	// RecordReport inserts a report for the question, or increments the counter
	// of the existing one. The returned outcome says which of the two happened.
	RecordReport(ctx context.Context, summary domain.QuestionSummary) (domain.ReportOutcome, error)
}

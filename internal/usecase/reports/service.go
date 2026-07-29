package reports

import (
	"context"
	"log/slog"
	"time"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/notify"
)

// notifyTimeout caps the out-of-band Discord call. The player's request has
// already been answered by then, so this only bounds the detached goroutine.
const notifyTimeout = 10 * time.Second

// ReportQuestionUseCase records that a player flagged a question as wrong.
type ReportQuestionUseCase struct {
	repo     ReportRepository
	notifier notify.NotificationService
	logger   *slog.Logger
}

// NewReportQuestionUseCase creates a ReportQuestionUseCase.
// A nil notifier disables notifications; reports are still recorded.
func NewReportQuestionUseCase(
	repo ReportRepository,
	notifier notify.NotificationService,
	logger *slog.Logger,
) *ReportQuestionUseCase {
	return &ReportQuestionUseCase{repo: repo, notifier: notifier, logger: logger}
}

// Execute records a report for questionID.
//
// The question is looked up first, so an unknown id is rejected with
// domain.ErrNotFound instead of filling the table with reports about questions
// that do not exist. Everything stored comes from that lookup — the caller
// supplies only the id.
//
// Notification is deliberately fired only for the first report of a question:
// repeat reports raise the counter silently, so no amount of tapping can flood
// the Discord channel.
func (uc *ReportQuestionUseCase) Execute(ctx context.Context, questionID string) (domain.ReportOutcome, error) {
	summary, err := uc.repo.FindQuestionSummary(ctx, questionID)
	if err != nil {
		return domain.ReportOutcome{}, err
	}

	outcome, err := uc.repo.RecordReport(ctx, summary)
	if err != nil {
		return domain.ReportOutcome{}, err
	}

	if outcome.Created {
		uc.notifyFirstReport(summary, outcome.Count)
	}

	uc.logger.Info("question reported",
		"question_id", summary.ID,
		"subject", string(summary.Subject),
		"grade", summary.Grade,
		"type", string(summary.Type),
		"count", outcome.Count,
		"first", outcome.Created,
	)

	return outcome, nil
}

// notifyFirstReport sends the Discord alert on a detached goroutine so a slow
// or failing webhook never delays the player's response. Failures are logged
// and swallowed: losing an alert must not lose the stored report.
func (uc *ReportQuestionUseCase) notifyFirstReport(summary domain.QuestionSummary, count int) {
	if uc.notifier == nil {
		return
	}

	event := notify.NotificationEvent{
		Type:      notify.EventQuestionReport,
		Timestamp: time.Now().UTC(),
		Report: &notify.ReportDetail{
			QuestionID:  summary.ID,
			Subject:     string(summary.Subject),
			Grade:       summary.Grade,
			Type:        string(summary.Type),
			Statement:   summary.Statement,
			ReportCount: count,
		},
	}

	go func() {
		// Detached from the request context, which is cancelled as soon as the
		// handler returns.
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()

		if err := uc.notifier.Alert(ctx, event); err != nil {
			uc.logger.Warn("question report notification failed",
				"question_id", event.Report.QuestionID,
				"error", err)
		}
	}()
}

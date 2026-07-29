package notify

import (
	"context"
	"time"
)

// FailureDetail describes a single failed combination in a job run.
type FailureDetail struct {
	Subject string `json:"subject"`
	Grade   int    `json:"grade"`
	Type    string `json:"type"`
	Error   string `json:"error"`
}

// Event types carried by NotificationEvent.Type.
const (
	EventJobFailure     = "job_failure"
	EventPoolLow        = "pool_low"
	EventJobSuccess     = "job_success"
	EventQuestionReport = "question_report"
)

// ReportDetail describes a question a player flagged as wrong.
// Every field is read from the database, never from the request body.
type ReportDetail struct {
	QuestionID  string
	Subject     string
	Grade       int
	Type        string
	Statement   string
	ReportCount int
}

// NotificationEvent carries the data for a notification alert.
type NotificationEvent struct {
	// Type is one of the Event* constants above.
	Type        string
	JobRunID    string
	Status      string
	FailedCount int
	Details     []FailureDetail
	Timestamp   time.Time

	// Report is set only for EventQuestionReport.
	Report *ReportDetail
}

// NotificationService sends alerts on job events.
// Implementations must be non-blocking and best-effort.
type NotificationService interface {
	Alert(ctx context.Context, event NotificationEvent) error
}

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

// NotificationEvent carries the data for a notification alert.
type NotificationEvent struct {
	// Type is one of: "job_failure" | "pool_low" | "job_success"
	Type        string
	JobRunID    string
	Status      string
	FailedCount int
	Details     []FailureDetail
	Timestamp   time.Time
}

// NotificationService sends alerts on job events.
// Implementations must be non-blocking and best-effort.
type NotificationService interface {
	Alert(ctx context.Context, event NotificationEvent) error
}

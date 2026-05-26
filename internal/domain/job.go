package domain

import "time"

// JobStatus represents the execution status of a background job run.
type JobStatus string

const (
	JobStatusRunning JobStatus = "running"
	JobStatusSuccess JobStatus = "success"
	JobStatusPartial JobStatus = "partial"
	JobStatusFailed  JobStatus = "failed"
)

// JobErrorDetail records a single failed combination during a job run.
type JobErrorDetail struct {
	Subject string `json:"subject"`
	Grade   int    `json:"grade"`
	Type    string `json:"type"`
	Error   string `json:"error"`
}

// JobRun records the execution history and statistics of a background generation job.
type JobRun struct {
	ID                 string           `json:"id"`
	StartedAt          time.Time        `json:"started_at"`
	FinishedAt         *time.Time       `json:"finished_at,omitempty"`
	Status             JobStatus        `json:"status"`
	CombinationsTotal  int              `json:"combinations_total"`
	CombinationsDone   int              `json:"combinations_done"`
	CombinationsFailed int              `json:"combinations_failed"`
	QuestionsGenerated int              `json:"questions_generated"`
	QuestionsDeleted   int              `json:"questions_deleted"`
	ErrorDetails       []JobErrorDetail `json:"error_details,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
}

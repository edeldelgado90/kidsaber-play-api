package http

import "github.com/kidsaber/kidsaber-play-api/internal/domain"

// QuestionsResponse is the JSON response body for GET /questions.
type QuestionsResponse struct {
	Questions []domain.Question `json:"questions"`
}

// ErrorResponse is the JSON body for all HTTP error responses.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// JobRunsResponse is the JSON response body for GET /admin/jobs.
type JobRunsResponse struct {
	Runs []domain.JobRun `json:"runs"`
}

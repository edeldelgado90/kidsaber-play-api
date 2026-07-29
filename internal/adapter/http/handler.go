package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/reports"
	"github.com/kidsaber/kidsaber-play-api/internal/version"
)

// QuestionsHandler handles GET /questions.
type QuestionsHandler struct {
	useCase *questions.GetQuestionsUseCase
	logger  *slog.Logger
}

// NewQuestionsHandler creates a QuestionsHandler.
func NewQuestionsHandler(uc *questions.GetQuestionsUseCase, logger *slog.Logger) *QuestionsHandler {
	return &QuestionsHandler{useCase: uc, logger: logger}
}

// GetQuestions handles GET /questions?subject=&grade=&type=&count=
func (h *QuestionsHandler) GetQuestions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	subjectStr := q.Get("subject")
	gradeStr := q.Get("grade")
	typeStr := q.Get("type")
	countStr := q.Get("count")

	// Validate subject
	subject := domain.Subject(subjectStr)
	if !domain.IsValidSubject(subject) {
		writeError(w, http.StatusBadRequest, "invalid_params",
			"subject must be one of: mathematics, language, english, science")
		return
	}

	// Validate grade
	if gradeStr == "" {
		writeError(w, http.StatusBadRequest, "invalid_params", "grade is required")
		return
	}
	grade, err := strconv.Atoi(gradeStr)
	if err != nil || !domain.IsValidGrade(grade) {
		writeError(w, http.StatusBadRequest, "invalid_params", "grade must be an integer between 1 and 6")
		return
	}

	// Validate type
	gameType := domain.GameType(typeStr)
	if !domain.IsValidGameType(gameType) {
		writeError(w, http.StatusBadRequest, "invalid_params",
			"type must be one of: option_multiple, fill_in_the_blanks, matching, quick_calculation")
		return
	}

	// Validate count (optional, default 10)
	count := 10
	if countStr != "" {
		count, err = strconv.Atoi(countStr)
		if err != nil || !domain.IsValidCount(count) {
			writeError(w, http.StatusBadRequest, "invalid_params", "count must be an integer between 1 and 30")
			return
		}
	}

	params := questions.GetQuestionsParams{
		Subject: subject,
		Grade:   grade,
		Type:    gameType,
		Count:   count,
	}

	qs, err := h.useCase.Execute(r.Context(), params)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNoValidQuestions):
			writeError(w, http.StatusServiceUnavailable, "generation_failed",
				"Could not generate valid questions after retries")
		case errors.Is(err, domain.ErrRateLimit):
			writeError(w, http.StatusTooManyRequests, "rate_limit",
				"LLM provider rate limit exceeded")
		case errors.Is(err, domain.ErrLLMTimeout):
			writeError(w, http.StatusServiceUnavailable, "llm_timeout",
				"LLM provider timed out")
		default:
			h.logger.Error("unexpected error in GetQuestions", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error",
				"An unexpected error occurred")
		}
		return
	}

	writeJSON(w, http.StatusOK, QuestionsResponse{Questions: qs})
}

// ReportsHandler handles POST /questions/{id}/report.
type ReportsHandler struct {
	useCase *reports.ReportQuestionUseCase
	logger  *slog.Logger
}

// NewReportsHandler creates a ReportsHandler.
func NewReportsHandler(uc *reports.ReportQuestionUseCase, logger *slog.Logger) *ReportsHandler {
	return &ReportsHandler{useCase: uc, logger: logger}
}

// ReportQuestion handles POST /questions/{id}/report.
//
// The request body is ignored: a report is a bare "this one looks wrong" signal,
// and the question's subject, grade and statement are read from the database.
// That keeps player-supplied text out of both the reports table and Discord.
func (h *ReportsHandler) ReportQuestion(w http.ResponseWriter, r *http.Request) {
	questionID := chi.URLParam(r, "id")

	// Reject malformed ids before they reach the database: a non-UUID would
	// surface as a driver error and be reported as a 500, not the 400 it is.
	if !isUUID(questionID) {
		writeError(w, http.StatusBadRequest, "invalid_params", "id must be a question UUID")
		return
	}

	if _, err := h.useCase.Execute(r.Context(), questionID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "No question with that id")
			return
		}
		h.logger.Error("unexpected error in ReportQuestion", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusAccepted, ReportQuestionResponse{Status: "received"})
}

// isUUID reports whether s is a canonical 8-4-4-4-12 hex UUID.
// Written out rather than adding a UUID dependency for one format check.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

// AdminHandler handles admin endpoints.
type AdminHandler struct {
	jobRepo questions.JobRunRepository
	logger  *slog.Logger
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(jobRepo questions.JobRunRepository, logger *slog.Logger) *AdminHandler {
	return &AdminHandler{jobRepo: jobRepo, logger: logger}
}

// GetJobRuns handles GET /admin/jobs returning the last N job runs.
func (h *AdminHandler) GetJobRuns(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	runs, err := h.jobRepo.FindRecent(r.Context(), limit)
	if err != nil {
		h.logger.Error("failed to fetch job runs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"An unexpected error occurred")
		return
	}

	// Always return an array (never null) for consistent client parsing.
	if runs == nil {
		runs = []domain.JobRun{}
	}

	writeJSON(w, http.StatusOK, JobRunsResponse{Runs: runs})
}

// HealthHandler handles GET /health.
// The version field carries the commit the binary was built from, so the
// deployed revision can be identified without access to the Cloud Run console.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": version.Version,
	})
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers already sent; nothing useful we can do here.
		_ = err
	}
}

// writeError writes a structured error response.
// Raw input is never reflected back — only safe, static messages are used.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: code, Message: message})
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kidsaber/kidsaber-play-api/internal/domain"
	"github.com/kidsaber/kidsaber-play-api/internal/usecase/questions"
)

// Repository implements QuestionRepository, JobRunRepository and ReportRepository using PostgreSQL.
// All queries use parameterized inputs — never string concatenation.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a Repository and verifies the connection.
func New(ctx context.Context, databaseURL string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &Repository{pool: pool}, nil
}

// Close releases all pool connections.
func (r *Repository) Close() {
	r.pool.Close()
}

// ─── QuestionRepository ────────────────────────────────────────────────────────

// FindRandom returns up to `count` random questions for the given combination.
// Returns fewer than `count` without error when the pool is small.
func (r *Repository) FindRandom(ctx context.Context, params questions.FindParams, count int) ([]domain.Question, error) {
	const q = `
		SELECT payload FROM question_bank
		WHERE subject = $1 AND grade = $2 AND type = $3
		ORDER BY RANDOM()
		LIMIT $4`

	rows, err := r.pool.Query(ctx, q, string(params.Subject), params.Grade, string(params.Type), count)
	if err != nil {
		return nil, fmt.Errorf("querying question_bank: %w", err)
	}
	defer rows.Close()

	var result []domain.Question
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scanning payload: %w", err)
		}
		var q domain.Question
		if err := json.Unmarshal(payload, &q); err != nil {
			return nil, fmt.Errorf("unmarshalling question payload: %w", err)
		}
		result = append(result, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating question_bank rows: %w", err)
	}

	return result, nil
}

// Save persists questions to the DB. Duplicate IDs are silently ignored.
func (r *Repository) Save(ctx context.Context, qs []domain.Question) error {
	return r.InsertBatch(ctx, qs)
}

// InsertBatch inserts multiple questions in a single transaction using COPY.
// Duplicate IDs are skipped with ON CONFLICT DO NOTHING.
func (r *Repository) InsertBatch(ctx context.Context, qs []domain.Question) error {
	if len(qs) == 0 {
		return nil
	}

	const stmt = `
		INSERT INTO question_bank (id, subject, grade, type, topic, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING`

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	batch := &pgx.Batch{}
	for _, q := range qs {
		payload, err := json.Marshal(q)
		if err != nil {
			return fmt.Errorf("marshalling question %s: %w", q.ID, err)
		}
		batch.Queue(stmt, q.ID, string(q.Subject), q.Grade, string(q.Type), q.Topic, payload)
	}

	br := tx.SendBatch(ctx, batch)
	for range qs {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("inserting question batch: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("closing batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing question insert: %w", err)
	}

	return nil
}

// Count returns the number of questions for a given subject/grade/type combination.
func (r *Repository) Count(ctx context.Context, params questions.FindParams) (int, error) {
	const q = `
		SELECT COUNT(*) FROM question_bank
		WHERE subject = $1 AND grade = $2 AND type = $3`

	var count int
	err := r.pool.QueryRow(ctx, q, string(params.Subject), params.Grade, string(params.Type)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting questions: %w", err)
	}
	return count, nil
}

// DeleteMostUsed removes the `limit` most-used questions for a combination.
// Only called after a successful InsertBatch — preserves existing pool on failure.
func (r *Repository) DeleteMostUsed(ctx context.Context, params questions.FindParams, limit int) error {
	if limit <= 0 {
		return nil
	}

	const q = `
		DELETE FROM question_bank
		WHERE subject = $1 AND grade = $2 AND type = $3
		  AND id IN (
		    SELECT id FROM question_bank
		    WHERE subject = $1 AND grade = $2 AND type = $3
		    ORDER BY usage_count DESC
		    LIMIT $4
		  )`

	_, err := r.pool.Exec(ctx, q, string(params.Subject), params.Grade, string(params.Type), limit)
	if err != nil {
		return fmt.Errorf("deleting most-used questions: %w", err)
	}
	return nil
}

// IncrementUsageCount increments usage_count for the given question IDs.
func (r *Repository) IncrementUsageCount(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	const q = `UPDATE question_bank SET usage_count = usage_count + 1 WHERE id = ANY($1)`

	_, err := r.pool.Exec(ctx, q, ids)
	if err != nil {
		return fmt.Errorf("incrementing usage counts: %w", err)
	}
	return nil
}

// ─── JobRunRepository ─────────────────────────────────────────────────────────

// Insert creates a new job_run row.
func (r *Repository) Insert(ctx context.Context, run *domain.JobRun) error {
	const q = `
		INSERT INTO job_runs
		  (id, started_at, status, combinations_total, combinations_done,
		   combinations_failed, questions_generated, questions_deleted, error_details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	detailsJSON, err := marshalErrorDetails(run.ErrorDetails)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, q,
		run.ID, run.StartedAt, string(run.Status),
		run.CombinationsTotal, run.CombinationsDone, run.CombinationsFailed,
		run.QuestionsGenerated, run.QuestionsDeleted,
		detailsJSON, run.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting job_run: %w", err)
	}
	return nil
}

// Update modifies an existing job_run row.
func (r *Repository) Update(ctx context.Context, run *domain.JobRun) error {
	const q = `
		UPDATE job_runs
		SET finished_at          = $2,
		    status               = $3,
		    combinations_done    = $4,
		    combinations_failed  = $5,
		    questions_generated  = $6,
		    questions_deleted    = $7,
		    error_details        = $8
		WHERE id = $1`

	detailsJSON, err := marshalErrorDetails(run.ErrorDetails)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, q,
		run.ID, run.FinishedAt, string(run.Status),
		run.CombinationsDone, run.CombinationsFailed,
		run.QuestionsGenerated, run.QuestionsDeleted,
		detailsJSON,
	)
	if err != nil {
		return fmt.Errorf("updating job_run: %w", err)
	}
	return nil
}

// FindRecent returns the most recent job_runs ordered by started_at DESC.
func (r *Repository) FindRecent(ctx context.Context, limit int) ([]domain.JobRun, error) {
	const q = `
		SELECT id, started_at, finished_at, status,
		       combinations_total, combinations_done, combinations_failed,
		       questions_generated, questions_deleted, error_details, created_at
		FROM job_runs
		ORDER BY started_at DESC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("querying job_runs: %w", err)
	}
	defer rows.Close()

	var runs []domain.JobRun
	for rows.Next() {
		var run domain.JobRun
		var statusStr string
		var detailsJSON []byte
		var finishedAt *time.Time

		err := rows.Scan(
			&run.ID, &run.StartedAt, &finishedAt, &statusStr,
			&run.CombinationsTotal, &run.CombinationsDone, &run.CombinationsFailed,
			&run.QuestionsGenerated, &run.QuestionsDeleted,
			&detailsJSON, &run.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning job_run: %w", err)
		}

		run.FinishedAt = finishedAt
		run.Status = domain.JobStatus(statusStr)

		if len(detailsJSON) > 0 && string(detailsJSON) != "null" {
			if err := json.Unmarshal(detailsJSON, &run.ErrorDetails); err != nil {
				return nil, fmt.Errorf("unmarshalling error_details: %w", err)
			}
		}

		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating job_run rows: %w", err)
	}

	return runs, nil
}

// marshalErrorDetails serialises error details to JSON, returning nil for empty slices.
func marshalErrorDetails(details []domain.JobErrorDetail) ([]byte, error) {
	if len(details) == 0 {
		return []byte("null"), nil
	}
	b, err := json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("marshalling error_details: %w", err)
	}
	return b, nil
}

// ─── ReportRepository ──────────────────────────────────────────────────────────

// FindQuestionSummary returns the classification and statement of a stored
// question. Returns domain.ErrNotFound when the id is unknown, which is what
// keeps reports about non-existent questions out of the table.
func (r *Repository) FindQuestionSummary(ctx context.Context, questionID string) (domain.QuestionSummary, error) {
	const q = `
		SELECT subject, grade, type, COALESCE(payload->>'statement', '')
		FROM question_bank
		WHERE id = $1`

	var (
		subject   string
		grade     int
		qType     string
		statement string
	)

	err := r.pool.QueryRow(ctx, q, questionID).Scan(&subject, &grade, &qType, &statement)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.QuestionSummary{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.QuestionSummary{}, fmt.Errorf("querying question summary: %w", err)
	}

	return domain.QuestionSummary{
		ID:        questionID,
		Subject:   domain.Subject(subject),
		Grade:     grade,
		Type:      domain.GameType(qType),
		Statement: statement,
	}, nil
}

// RecordReport inserts a report for the question or, when one already exists,
// increments its counter and reopens it.
//
// The whole dedupe rests on question_id being the primary key: repeated reports
// can only ever touch one row, so a flood costs one UPDATE and never grows the
// table. `xmax = 0` is true only for a freshly inserted row, which is how the
// caller learns whether this was the first report and therefore worth notifying.
// A re-report of an already-reviewed question reopens it silently — it resurfaces
// through the review query rather than through a second Discord ping.
func (r *Repository) RecordReport(ctx context.Context, s domain.QuestionSummary) (domain.ReportOutcome, error) {
	const q = `
		INSERT INTO question_reports (question_id, subject, grade, type, statement)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (question_id) DO UPDATE
			SET report_count     = question_reports.report_count + 1,
			    last_reported_at = NOW(),
			    status           = 'open'
		RETURNING report_count, (xmax = 0) AS inserted`

	var outcome domain.ReportOutcome
	err := r.pool.QueryRow(ctx, q,
		s.ID, string(s.Subject), s.Grade, string(s.Type), s.Statement,
	).Scan(&outcome.Count, &outcome.Created)
	if err != nil {
		return domain.ReportOutcome{}, fmt.Errorf("recording question report: %w", err)
	}

	return outcome, nil
}

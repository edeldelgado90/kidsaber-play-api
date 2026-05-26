-- Migration 002: Create the job_runs table
-- Records background job execution history and error details.

CREATE TABLE IF NOT EXISTS job_runs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    started_at          TIMESTAMPTZ NOT NULL,
    finished_at         TIMESTAMPTZ,
    status              VARCHAR(20) NOT NULL
                            CHECK (status IN ('running','success','partial','failed')),
    combinations_total  INT         NOT NULL DEFAULT 72,
    combinations_done   INT         NOT NULL DEFAULT 0,
    combinations_failed INT         NOT NULL DEFAULT 0,
    questions_generated INT         NOT NULL DEFAULT 0,
    questions_deleted   INT         NOT NULL DEFAULT 0,
    error_details       JSONB,      -- array of {subject, grade, type, error} objects
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_runs_started_at
    ON job_runs (started_at DESC);

COMMENT ON TABLE job_runs IS
    'Background job execution history for the KidSaber Play question generator.';

COMMENT ON COLUMN job_runs.error_details IS
    'JSON array of {subject, grade, type, error} objects for each failed combination.';

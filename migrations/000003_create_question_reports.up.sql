-- Migration 003: Create the question_reports table
-- Records that players flagged a question as wrong, for a human to review.
--
-- question_id is the PRIMARY KEY on purpose: one row per question, never one
-- row per tap. A flood of reports about the same question increments a counter
-- instead of growing the table, which is what keeps a public, unauthenticated-ish
-- write endpoint from being usable as an amplification vector.
--
-- No FK to question_bank: the generator job deletes the most-used questions to
-- enforce the per-combination cap, and a foreign key would either block that
-- delete or silently discard the report. Keeping the row means a report stays
-- visible even if the question has since aged out of the pool.

CREATE TABLE IF NOT EXISTS question_reports (
    question_id       UUID         PRIMARY KEY,
    subject           VARCHAR(20)  NOT NULL
                          CHECK (subject IN ('mathematics','language','english','science')),
    grade             SMALLINT     NOT NULL
                          CHECK (grade BETWEEN 1 AND 6),
    type              VARCHAR(30)  NOT NULL
                          CHECK (type IN ('option_multiple','fill_in_the_blanks','matching')),
    statement         TEXT         NOT NULL,
    report_count      INTEGER      NOT NULL DEFAULT 1
                          CHECK (report_count > 0),
    status            VARCHAR(20)  NOT NULL DEFAULT 'open'
                          CHECK (status IN ('open','resolved','dismissed')),
    first_reported_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_reported_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Review queue: open reports, most-reported first.
CREATE INDEX IF NOT EXISTS idx_question_reports_review
    ON question_reports (status, report_count DESC, last_reported_at DESC);

COMMENT ON TABLE question_reports IS
    'Player-submitted reports that a question looks wrong. One row per question; '
    'repeat reports increment report_count rather than inserting a new row.';

COMMENT ON COLUMN question_reports.statement IS
    'Copy of the question statement at report time, so the report stays readable '
    'even after the question is regenerated or deleted from question_bank.';

COMMENT ON COLUMN question_reports.status IS
    'open = needs review; resolved = question fixed or replaced; dismissed = report was wrong.';

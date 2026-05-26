-- Migration 001: Create the question_bank table
-- Questions are stored as JSONB payloads (pre-validated by the API).
-- The runtime DB user needs only SELECT, INSERT, DELETE on this table.

CREATE TABLE IF NOT EXISTS question_bank (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    subject     VARCHAR(20)  NOT NULL
                    CHECK (subject IN ('mathematics','language','english','science')),
    grade       SMALLINT     NOT NULL
                    CHECK (grade BETWEEN 1 AND 6),
    type        VARCHAR(30)  NOT NULL
                    CHECK (type IN ('option_multiple','fill_in_the_blanks','matching')),
    topic       VARCHAR(100),
    payload     JSONB        NOT NULL,   -- Full Question struct, pre-validated
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    usage_count INTEGER      NOT NULL DEFAULT 0
);

-- Primary lookup index for GET /questions
CREATE INDEX IF NOT EXISTS idx_question_bank_lookup
    ON question_bank (subject, grade, type);

-- Index for the DeleteMostUsed query (cap enforcement in background job)
CREATE INDEX IF NOT EXISTS idx_question_bank_usage
    ON question_bank (subject, grade, type, usage_count DESC);

COMMENT ON TABLE question_bank IS
    'Pre-generated educational questions served by the KidSaber Play API. '
    'Populated by the QuestionGeneratorJob background task.';

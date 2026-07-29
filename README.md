# KidSaber Play API

Production-grade Golang REST API for [KidSaber Play](https://github.com/kidsaber) — a Spanish primary-school educational app. Serves pre-generated questions aligned to the LOMLOE curriculum (grades 1–6, 4 subjects).

## Architecture

Hexagonal architecture with three layers:

```
cmd/server → adapter/http → usecase → domain ← adapter/generator + adapter/repository
```

- **HTTP request path** is always fast: questions come from the PostgreSQL pool, never from real-time LLM calls.
- **Background job** pre-generates questions with the Claude API into the pool on a daily cron schedule.
- **`quick_calculation`** is always procedural — zero LLM calls, zero DB reads.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (no auth) |
| `GET` | `/questions` | Get a question batch |
| `POST` | `/questions/{id}/report` | Flag a question as wrong (players) |
| `GET` | `/admin/jobs` | Job run history (API key required) |

### Authentication

With `AUTH_ENABLED=true`, `/questions` accepts any **one** of:

| Credential | Header | Used by |
|-----------|--------|---------|
| Static API key | `X-API-Key: <key>` or `Authorization: Bearer <key>` | Server-to-server, admin tooling |
| Firebase ID token | `Authorization: Bearer <idToken>` | Web and mobile clients (anonymous sign-in) |
| Firebase App Check | `X-Firebase-AppCheck: <token>` | Attested app builds |

The last two require `FIREBASE_PROJECT_ID`. When it is unset, only the static key
is accepted.

`/admin/*` accepts the **static API key only** — anyone can mint an anonymous ID
token, so neither ID tokens nor App Check tokens reach the admin surface.

Browser clients must have their origin listed in `CORS_ALLOWED_ORIGINS`, which
supports `https://*.example.com` subdomain wildcards for preview deployments.

`POST /questions/{id}/report` takes the same credentials as `/questions`, and
`REPORTS_REQUIRE_APP_CHECK=true` narrows it to App Check or the static key. See
[Question reports](#question-reports) for why that is off by default.

### GET /questions

```
GET /questions?subject=mathematics&grade=3&type=option_multiple&count=10
```

| Parameter | Required | Values |
|-----------|----------|--------|
| `subject` | ✓ | `mathematics` `language` `english` `science` |
| `grade` | ✓ | `1`–`6` |
| `type` | ✓ | `option_multiple` `fill_in_the_blanks` `matching` `quick_calculation` |
| `count` | — | `1`–`30` (default: `10`) |

**Response 200:**
```json
{
  "questions": [
    {
      "id": "8a2f4c1e-...",
      "type": "option_multiple",
      "subject": "mathematics",
      "grade": 3,
      "topic": "multiplication_tables_complete",
      "statement": "¿Cuánto es 4 × 6?",
      "options": [
        {"id":"A","text":"20"},{"id":"B","text":"24"},
        {"id":"C","text":"26"},{"id":"D","text":"16"}
      ],
      "correctAnswers": ["B"],
      "meta": {"difficulty":"easy","timeLimitMs":15000,"tags":["multiplication"]}
    }
  ]
}
```

**Error codes:** `400` invalid params · `429` LLM rate limit · `503` generation failed · `500` internal error

### POST /questions/{id}/report

Players flag a question that looks wrong. The request body is ignored — a report
is a bare signal, and everything stored comes from looking the question up in
`question_bank`, so a caller cannot inject its own text into the reports table
or into Discord.

```
POST /questions/8a2f4c1e-0000-0000-0000-000000000000/report
```

**Response 202:**
```json
{ "status": "received" }
```

**Error codes:** `400` malformed id · `404` unknown question · `429` rate limited · `500` internal error

The count of prior reports is deliberately not returned: it would tell an
anonymous caller how close a question is to being pulled from review.

## Question reports

A report writes to `question_reports`, one row per question:

```sql
SELECT question_id, subject, grade, type, statement, report_count, last_reported_at
FROM question_reports
WHERE status = 'open'
ORDER BY report_count DESC, last_reported_at DESC;
```

Look the question up in `question_bank` by that id, fix or delete it, then close
the report:

```sql
UPDATE question_reports SET status = 'resolved' WHERE question_id = '<uuid>';
-- or 'dismissed' when the question turned out to be fine
```

**Abuse resistance.** This is the one endpoint an anonymous client can write
through, so it is fenced on four sides:

| Control | Effect |
|---------|--------|
| `question_id` is the primary key | Repeat reports `UPDATE` one row instead of inserting. A flood cannot grow the table. |
| Discord fires only on insert | Repeats bump the counter silently, so the channel cannot be spammed. |
| Route rate limit (5/hour per IP) | Much tighter than the global 60/min. In-process, so on Cloud Run the ceiling multiplies by instance count — a speed bump, not a wall. |
| `REPORTS_REQUIRE_APP_CHECK` | Drops the anonymous ID token, leaving App Check or the static key. |

The dedupe is what actually bounds the damage; the rate limit only slows a
single-source flood.

`REPORTS_REQUIRE_APP_CHECK` ships **off**. An anonymous Firebase ID token proves
nothing — anyone can mint one — so App Check is the only control that tells a
real app instance from a `curl`. But the client only obtains an App Check token
on web today; native builds would need Play Integrity via react-native-firebase,
which is not wired up. Turn this on once it is.

A re-report of a question already marked `resolved` or `dismissed` reopens it
without a second Discord message — it resurfaces through the query above.

## Quick Start (local)

### 1. Prerequisites

- Go 1.23+
- Docker + Docker Compose (for local PostgreSQL)
- Claude API key: [Anthropic Console](https://console.anthropic.com/settings/keys)

### 2. Setup

```bash
# Clone and enter
git clone https://github.com/kidsaber/kidsaber-play-api
cd kidsaber-play-api

# Install dependencies
go mod download

# Configure environment
cp .env.example .env
# Edit .env: set DATABASE_URL, LLM_API_KEY, LLM_MODEL, LLM_BASE_URL

# Start PostgreSQL
docker compose up db -d

# Apply migrations (tracked by golang-migrate; safe to re-run)
make migrate

# Run the API
make run-dev
```

### 3. Pre-populate questions (before going live)

```bash
# Run the generator job 10 times to fill the pool
# Generates up to 72 x 10 x 10 = 7,200 questions
make seed
```

This takes ~10–20 minutes depending on LLM rate limits. Monitor with:
```bash
psql "$DATABASE_URL" -c "SELECT subject, grade, type, COUNT(*) FROM question_bank GROUP BY 1,2,3 ORDER BY 1,2,3;"
```

### 4. Verify

```bash
curl http://localhost:8080/health
curl "http://localhost:8080/questions?subject=mathematics&grade=3&type=quick_calculation&count=5"
curl "http://localhost:8080/questions?subject=language&grade=4&type=option_multiple&count=10"
```

## Question generation (Claude API)

The generator job calls the Claude Messages API with adaptive thinking, streaming
each batch so long generations don't hit an HTTP timeout. Only the job and the
seed command call Claude — `GET /questions` always serves from the pool.

| Variable | Default | Notes |
|----------|---------|-------|
| `LLM_API_KEY` | — | Claude API key. Required for the job; the server runs without it. |
| `LLM_MODEL` | `claude-sonnet-5` | `claude-opus-5` if question quality needs it. |
| `LLM_EFFORT` | `medium` | `low`/`medium`/`high`/`xhigh`/`max`. Thinking bills as output, so this is the main cost lever. |
| `LLM_TIMEOUT_S` | `300` | Whole-request budget; generation runs for minutes. |
| `LLM_MAX_RETRIES` | `2` | Retries on schema-validation failure. |
| `LLM_BASE_URL` | — | Optional endpoint override (proxy/gateway). |

## Makefile targets

```bash
make build        # Compile server + seed binaries
make test         # Run tests with race detector
make lint         # go vet + govulncheck
make run-dev      # Start API locally (requires .env)
make docker-build # Build production Docker image
make docker-up    # Start full stack (API + PostgreSQL)
make migrate          # Apply pending migrations (golang-migrate)
make migrate-down     # Roll back the last migration
make migrate-version  # Show current migration version
make seed         # Pre-populate question bank
make tidy         # go mod tidy + verify
```

## Deployment (Google Cloud Run — free tier)

### 1. Database (Neon — free PostgreSQL)

1. Create a free project at [neon.tech](https://neon.tech)
2. Copy the connection string (includes SSL by default)
3. Apply migrations: `DATABASE_URL=<neon-url> make migrate`

### 2. GitHub Actions secrets

| Secret | Value |
|--------|-------|
| `GCP_PROJECT_ID` | Google Cloud project ID |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | Workload Identity Federation provider |
| `GCP_SERVICE_ACCOUNT` | Service account email for Cloud Run |
| `DATABASE_URL` | Neon PostgreSQL URL (`?sslmode=require`) |
| `LLM_API_KEY` | Claude API key |
| `LLM_MODEL` | `claude-sonnet-5` |
| `LLM_EFFORT` | `medium` |
| `NOTIFY_WEBHOOK_URL` | Slack/Discord webhook for job alerts |
| `API_KEY` | Strong random key for the mobile app |
| `CORS_ALLOWED_ORIGINS` | Comma-separated allowed origins; supports `https://*.example.com` subdomain wildcards |

### 3. Deploy

Push to `main` — the CI/CD pipeline builds the Docker image, runs migrations, and deploys to Cloud Run automatically.

## Background Job

The `QuestionGeneratorJob` runs daily at 03:00 UTC (configurable via `JOB_SCHEDULE`). Each run:
- Iterates all **72 combinations** (4 subjects x 6 grades x 3 LLM types)
- Generates `JOB_BATCH_SIZE` (default: 10) questions per combination via LLM
- Enforces a cap of `JOB_MAX_PER_COMBINATION` (default: 100) questions per combination
- Records execution stats in `job_runs` table
- Sends failure notifications via webhook and/or email

Monitor with `GET /admin/jobs` (API key required).

## Tests

```bash
make test
go test ./internal/usecase/questions/...
go test ./internal/adapter/generator/procedural/...
go test ./internal/adapter/notify/...
go test ./internal/adapter/http/...
```

## Project structure

```
cmd/
  server/main.go          Entry point: config -> DI -> HTTP + job
  seed/main.go            Seed command (runs job N times, then exits)
internal/
  domain/                 Pure Go entities
  usecase/questions/      GetQuestionsUseCase + ports
  usecase/notify/         NotificationService port
  adapter/http/           Chi router, handlers, middleware
  adapter/generator/llm/  LLM client, prompts, parser, topic picker
  adapter/generator/procedural/  Arithmetic calculator
  adapter/repository/postgres/   PostgreSQL repository
  adapter/notify/         Webhook + SMTP + MultiNotifier
  job/                    QuestionGeneratorJob (cron)
  config/                 Env var config loader
pkg/
  validator/              JSON Schema validation
  logger/                 slog structured logger
migrations/               SQL migrations (golang-migrate: *.up.sql / *.down.sql)
.github/workflows/        CI + CD (Cloud Run)
```

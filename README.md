# KidSaber Play API

Production-grade Golang REST API for [KidSaber Play](https://github.com/kidsaber) — a Spanish primary-school educational app. Serves pre-generated questions aligned to the LOMLOE curriculum (grades 1–6, 4 subjects).

## Architecture

Hexagonal architecture with three layers:

```
cmd/server → adapter/http → usecase → domain ← adapter/generator + adapter/repository
```

- **HTTP request path** is always fast: questions come from the PostgreSQL pool, never from real-time LLM calls.
- **Background job** pre-generates questions via LLM (Gemini Flash / Groq / Ollama) into the pool on a daily cron schedule.
- **`quick_calculation`** is always procedural — zero LLM calls, zero DB reads.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (no auth) |
| `GET` | `/questions` | Get a question batch |
| `GET` | `/admin/jobs` | Job run history (API key required) |

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

## Quick Start (local)

### 1. Prerequisites

- Go 1.23+
- Docker + Docker Compose (for local PostgreSQL)
- LLM API key: [Gemini](https://aistudio.google.com/apikey) (free, recommended) or [Groq](https://console.groq.com/) (free fallback)

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

## LLM Providers (all free tier)

| Provider | `LLM_PROVIDER` | `LLM_MODEL` | `LLM_BASE_URL` | Limits |
|----------|----------------|-------------|----------------|--------|
| **Gemini Flash** (recommended) | `gemini` | `gemini-2.0-flash-lite` | `https://generativelanguage.googleapis.com/v1beta/openai/` | 1,500 req/day |
| **Groq** (fallback) | `groq` | `llama-3.3-70b-versatile` | `https://api.groq.com/openai/v1` | 14,400 req/day |
| **Ollama** (local dev) | `ollama` | `llama3.2` | `http://localhost:11434/v1` | Unlimited |

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
| `LLM_PROVIDER` | `gemini` |
| `LLM_API_KEY` | Gemini API key |
| `LLM_MODEL` | `gemini-2.0-flash-lite` |
| `LLM_BASE_URL` | `https://generativelanguage.googleapis.com/v1beta/openai/` |
| `NOTIFY_WEBHOOK_URL` | Slack/Discord webhook for job alerts |
| `API_KEY` | Strong random key for the mobile app |
| `CORS_ALLOWED_ORIGINS` | Comma-separated allowed origins |

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

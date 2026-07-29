# CLAUDE.md — KidSaber Play API

Instructions for Claude Code agents. Read this before any implementation task.

## What this repo is

**KidSaber Play API** — a production-grade Golang REST API that serves pre-generated educational questions for the KidSaber Play mobile/web app. It exposes `GET /questions` and a background job that pre-fills PostgreSQL with LLM-generated questions aligned to the LOMLOE curriculum for Spanish primary education (grades 1–6).

## Agent persona

Senior Go engineer specialising in hexagonal architecture, REST APIs, PostgreSQL, and LLM integration. Write idiomatic, production-grade Go. No placeholder stubs — every function is complete and working.

## Mandatory reading order before any task

1. `CLAUDE.md` (this file)
2. `internal/domain/` — core entities and errors
3. Relevant adapter or use case for the task at hand

## Tech stack (fixed decisions)

| Component | Technology |
|-----------|-----------|
| Language | Go 1.23+ |
| HTTP Router | `github.com/go-chi/chi/v5` |
| PostgreSQL Driver | `github.com/jackc/pgx/v5` (pgxpool) |
| JSON Schema Validation | `github.com/xeipuuv/gojsonschema` |
| Job Scheduler | `github.com/robfig/cron/v3` |
| Config | `github.com/caarlos0/env/v11` |
| Logging | `log/slog` (stdlib, JSON in production) |
| Testing | `github.com/stretchr/testify` |
| Container | Docker alpine (multi-stage) |

## Architecture rules

- **Domain layer** (`internal/domain/`) imports nothing external — pure Go structs and errors only.
- **Use cases** (`internal/usecase/`) depend only on ports (interfaces defined in their own package) — no framework, no DB, no LLM imports.
- **Adapters** (`internal/adapter/`) implement the ports — HTTP, generator (LLM + procedural), repository (PostgreSQL), notify (webhook + SMTP).
- **No global state** — all dependencies injected via constructors.
- Domain knows nothing about HTTP, DB, LLM, or notifications.

## Non-negotiable rules

- **Parameterized queries only** — never string concatenation in SQL (pgx enforces this natively).
- **All LLM output validated** against JSON Schema before storing or returning to client.
- **Generate first, delete after** — never delete questions for a combination when generation/insertion fails; existing pool is preserved.
- **DB is mandatory** — no in-memory fallback; PostgreSQL is required.
- **Non-root Docker user** — final image runs as `nonroot`.
- **`quick_calculation` is always procedural** — never uses LLM, never reads from DB.
- **`correctAnswers` is the canonical answer field** — always present in all question types.
- **Question reports store only what the DB already knows** — `POST /questions/{id}/report` ignores its body; subject, grade and statement come from a `question_bank` lookup, never from the caller. No free text reaches the reports table or Discord.
- **Zero secrets in source** — all via env vars; never log API keys, DATABASE_URL, or LLM responses.

## Code conventions

- Idiomatic Go — no over-engineering, no generics unless clearly justified.
- English code and comments; Spanish (Spain) in all user-visible content.
- Structured JSON logs via `slog` in production.
- No hardcoded secrets, URLs, or schedule expressions.
- Return errors up the call stack; use `fmt.Errorf("context: %w", err)` for wrapping.

## Key paths

| Path | Purpose |
|------|---------|
| `internal/domain/` | Question, JobRun entities; domain errors |
| `internal/usecase/questions/` | GetQuestionsUseCase + ports |
| `internal/usecase/reports/` | ReportQuestionUseCase + ports |
| `internal/usecase/notify/` | NotificationService port |
| `internal/adapter/http/` | Chi router, handlers, middleware, DTOs |
| `internal/adapter/generator/llm/` | LLM client, prompts, parser, topic picker |
| `internal/adapter/generator/procedural/` | Procedural calculator for quick_calculation |
| `internal/adapter/repository/postgres/` | PostgreSQL implementation |
| `internal/adapter/notify/` | Webhook + SMTP + MultiNotifier |
| `internal/job/` | QuestionGeneratorJob (background cron) |
| `cmd/server/` | Entry point: config → DI → HTTP server + job |
| `cmd/seed/` | Seed command: runs job N times synchronously |

## What NOT to do

- No Gin, no Echo — use chi only.
- No global variables.
- No string concatenation in SQL.
- No unvalidated LLM output to client or DB.
- No user auth, sessions, or per-user progress storage (client handles that locally).
- No in-memory cache as a substitute for DB.
- Never block the HTTP server while the job runs (job runs in a separate goroutine).
- Never implement mascota, tienda, or economics (v1.5 scope).

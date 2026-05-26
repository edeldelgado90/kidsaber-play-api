Start the KidSaber Play API locally for development.

Prerequisites:
- `.env` file exists with `DATABASE_URL` set (copy from `.env.example`)
- PostgreSQL running (local or via `make docker-up`)
- Migrations applied: `make migrate`

```bash
make run-dev
```

The API starts on port 8080 (or `$PORT`). Test with:
```bash
curl "http://localhost:8080/health"
curl "http://localhost:8080/questions?subject=mathematics&grade=3&type=quick_calculation&count=5"
curl "http://localhost:8080/questions?subject=language&grade=4&type=option_multiple&count=10"
```

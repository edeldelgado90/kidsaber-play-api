Pre-populate the question bank by running the generator job N times (JOB_SEED_ITERATIONS, default 10).

This runs the LLM generator synchronously for all 72 subject×grade×type combinations,
generating up to `JOB_SEED_ITERATIONS × JOB_BATCH_SIZE` questions per combination.

Prerequisites:
- `.env` with `DATABASE_URL` and `LLM_API_KEY` configured
- Migrations applied: `make migrate`

```bash
make seed
```

Monitor progress in the logs. After seeding, verify with:
```bash
psql "$DATABASE_URL" -c "SELECT subject, grade, type, COUNT(*) FROM question_bank GROUP BY 1,2,3 ORDER BY 1,2,3;"
```

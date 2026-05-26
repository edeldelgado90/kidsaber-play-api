Inspect the last job runs via the admin endpoint.

Prerequisites: API running locally or remotely with `API_KEY` set.

```bash
# Local (no auth)
curl -s "http://localhost:8080/admin/jobs?limit=5" | jq .

# Production (with API key)
curl -s -H "X-API-Key: $API_KEY" "https://<your-cloud-run-url>/admin/jobs?limit=5" | jq .
```

Look for:
- `status`: should be "success" or "partial" (never "failed")
- `combinations_failed`: should be 0
- `questions_generated`: should be ~720 per run (72 combinations × 10 batch size)
- `error_details`: empty array means no failures

Or query the DB directly:
```sql
SELECT id, started_at, finished_at, status,
       combinations_done, combinations_failed,
       questions_generated, questions_deleted
FROM job_runs ORDER BY started_at DESC LIMIT 5;
```

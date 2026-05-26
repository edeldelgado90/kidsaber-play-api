Run all tests with the race detector and coverage report.

```bash
go test ./... -race -cover -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -5
```

If tests fail, show the full failure output and identify which package failed.

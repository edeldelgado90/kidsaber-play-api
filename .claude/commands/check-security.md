Run security checks: vulnerability scan and static analysis.

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
go vet ./...
```

If govulncheck finds vulnerabilities, update the affected dependency with `go get <module>@latest` and re-run.
If go vet reports issues, fix them before committing.

Also verify no secrets are in source:
```bash
grep -r "API_KEY\|PASSWORD\|SECRET" --include="*.go" . | grep -v "_test.go" | grep -v "env:"
```

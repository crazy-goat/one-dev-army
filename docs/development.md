# Development

## Critical Rules

1. **Every change must pass lint and tests locally before committing.** No exceptions.
2. **Every change must have tests.** No code gets merged without corresponding test coverage.

## Local Verification

Run these two commands before every commit:

```bash
golangci-lint run ./...    # Lint (29 linters, ~3s)
go test -race ./...        # Tests with race detector
```

Both must exit with zero errors. CI will reject PRs that fail either check.

## Build

```bash
go build -o oda .
```

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs on every push to `main` and on every PR:

1. **build** — `go build -v`, `go test -v -race -coverprofile`, coverage upload to Codecov
2. **lint** — `golangci-lint` v2.11+ via `golangci/golangci-lint-action@v7`

Lint configuration: `.golangci.yml` (29 linters covering security, correctness, performance, style, dead code). Formatters: `gofmt`, `goimports`.

## Testing Conventions

- Tests use the standard `testing` package
- Test files are co-located with source (`*_test.go`)
- Integration tests in `internal/integration_test.go` and `main_test.go`
- 33 test files across the codebase

## Code Conventions

- Standard Go project layout (`internal/` for private packages)
- No external web frameworks — stdlib `net/http` with `http.ServeMux`
- Embedded templates via `//go:embed`
- Error handling: always wrap with `fmt.Errorf("context: %w", err)`
- GitHub interactions exclusively through `gh` CLI (no direct API client library)
- All state transitions through `SetStageLabel()` to maintain label consistency

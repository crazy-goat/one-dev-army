# CLI Command: `oda issue log`

**Date:** 2026-03-25
**Problem:** The `stage_change_ledger` SQLite table records every stage transition for every issue, but `GetStageChanges()` is never called — the data is write-only. There is no way to inspect the history of a ticket from the CLI.

## Design

### Interface

```
oda issue log <number> [options]
oda issue log --issue <number> [options]
```

**Flags:**
- `--issue N` — issue number (alternative to positional argument)
- `--json` — output as JSON instead of a text table
- `--limit N` — limit the number of entries (default: 0 = all, ordered newest-first)

Both positional and flag forms are accepted. If both are provided, the flag wins.

### Output — text table (default)

```
Stage history for issue #42 (5 entries)

TIME                  FROM          TO            REASON                    BY
2026-03-25 15:03:02   AI Review     Code          manual_retry              orchestrator
2026-03-25 15:01:44   Code          AI Review     worker_completed_coding   orchestrator
2026-03-25 14:35:12   Plan          Code          worker_completed_analysis orchestrator
2026-03-25 14:32:01   Backlog       Plan          sync_initial              orchestrator
2026-03-25 14:30:00   —             Backlog       sync_initial              orchestrator
```

### Output — JSON (`--json`)

```json
{
  "issue_number": 42,
  "total": 5,
  "entries": [
    {
      "id": 12,
      "from_stage": "AI Review",
      "to_stage": "Code",
      "reason": "manual_retry",
      "changed_by": "orchestrator",
      "changed_at": "2026-03-25T15:03:02Z"
    }
  ]
}
```

### Changes

**File: `internal/db/db.go`**
1. Add `GetStageChangesLimit(issueNumber, limit int) ([]StageChange, error)` — queries ledger with optional `LIMIT` clause. When limit ≤ 0, returns all entries.

**File: `cmd/issue.go`**
1. Add `IssueLogFlags` struct with `Issue int`, `JSON bool`, `Limit int`
2. Add `"log"` case in `IssueCommand()` switch — requires `*db.Store` parameter
3. Add `IssueLogCommand(args []string, store *db.Store) error` — parses flags, resolves issue number (positional or `--issue`), queries ledger, formats output
4. Add `formatLogTable(issueNumber int, changes []db.StageChange)` — renders text table
5. Add `formatLogJSON(issueNumber int, changes []db.StageChange) error` — renders JSON
6. Update `PrintIssueUsage()` — add `log` subcommand docs

**File: `main.go`**
1. Update `runIssue()` — open `db.Store` and pass it to `cmd.IssueCommand()`
2. Update `IssueCommand` signature to accept `*db.Store`

**File: `cmd/issue_test.go`**
1. Tests for issue number parsing (positional, flag, both, missing)
2. Tests for `--limit` validation
3. Tests for table and JSON output formatting
4. Tests for empty ledger (0 entries)

### Error handling

- No issue number provided → error with usage hint
- Invalid issue number (not a positive integer) → error
- 0 ledger entries → `"No stage changes found for issue #N"`
- Database error → wrapped error with context

### Out of scope

- `--since` date filtering — YAGNI
- Colored terminal output — YAGNI
- Filtering by reason or stage — YAGNI

## Implementation Plan

### Step 1: Add `GetStageChangesLimit` to `internal/db/db.go`
Add a new method that wraps the existing query with an optional LIMIT clause. Keep `GetStageChanges` unchanged for backward compatibility.

### Step 2: Update `IssueCommand` signature in `cmd/issue.go`
Change `IssueCommand(args []string, client *github.Client, dashboardPort int)` to also accept `*db.Store`. Update the `"create"` case to pass through unchanged. Add `"log"` case.

### Step 3: Update `main.go` `runIssue()`
Open `db.Store` (same path as `runServe`: `<dir>/.oda/metrics.db`) and pass it to `IssueCommand`. Close store with defer.

### Step 4: Implement `IssueLogCommand` in `cmd/issue.go`
Parse flags, resolve issue number, query store, format output (table or JSON).

### Step 5: Update `PrintIssueUsage()`
Add `log` subcommand documentation and examples.

### Step 6: Write tests in `cmd/issue_test.go`
Cover flag parsing, output formatting, edge cases.

### Step 7: Run lint and tests
`golangci-lint run ./...` and `go test -race ./...`

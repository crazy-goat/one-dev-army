# Implementation Plan for Issue #177

**Created:** 2026-03-23T13:01:51+01:00
**Updated:** 2026-03-23T13:01:51+01:00

## Analysis

### 1. Core Requirements
- Create SQLite table `issue_cache` with fields: issue_number (PK), title, body, state, labels (JSON), assignee, milestone, updated_at, cached_at
- Add 3 indexes: state, cached_at, milestone
- Implement 5 CRUD methods in `db.go`: SaveIssueCache, GetIssueCache, GetIssuesCacheByMilestone, ClearIssueCache, GetAllCachedIssues
- Handle JSON serialization for labels array
- Write comprehensive unit tests

### 2. Files That Need Changes
- `internal/db/migrations.go` - Add 4 new migrations (table + 3 indexes)
- `internal/db/db.go` - Add IssueCache struct and 5 methods
- `internal/db/db_test.go` - Add tests for all cache operations

### 3. Implementation Approach
Follow existing patterns in the codebase:
- Use `encoding/json` for labels serialization (stored as TEXT in SQLite)
- Use `sql.NullString` for nullable fields (body, assignee, milestone, updated_at)
- Follow error wrapping pattern: `fmt.Errorf("context: %w", err)`
- Use `rows.Scan()` with pointer fields for NULL handling
- Use `time.Now()` for cached_at timestamp

### 4. Testing Strategy
- Test migrations run successfully (idempotent)
- Test SaveIssueCache with full Issue data
- Test GetIssueCache retrieval and JSON deserialization
- Test GetIssuesCacheByMilestone filtering
- Test ClearIssueCache deletes all records
- Test GetAllCachedIssues returns all issues
- Test edge cases: empty cache, NULL fields, JSON parsing

## Implementation Steps

### Step 1: **Add migrations** (`internal/db/migrations.go`)

- Append 4 migration strings to `migrations` slice:
- CREATE TABLE issue_cache
- CREATE INDEX idx_issue_cache_state
- CREATE INDEX idx_issue_cache_cached_at
- CREATE INDEX idx_issue_cache_milestone

### Step 2: **Add IssueCache struct and methods** (`internal/db/db.go`)

- Add `IssueCache` struct matching github.Issue fields
- Add `SaveIssueCache(issue github.Issue, milestone string) error`
- Add `GetIssueCache(issueNumber int) (github.Issue, error)`
- Add `GetIssuesCacheByMilestone(milestone string) ([]github.Issue, error)`
- Add `GetAllCachedIssues() ([]github.Issue, error)`
- Add `ClearIssueCache() error`

### Step 3: **Add unit tests** (`internal/db/db_test.go`)

- `TestSaveAndGetIssueCache` - full roundtrip test
- `TestGetIssueCache_NotFound` - empty result handling
- `TestGetIssuesCacheByMilestone` - filtering by milestone
- `TestGetAllCachedIssues` - retrieve all
- `TestClearIssueCache` - deletion verification
- `TestIssueCache_JSONLabels` - labels serialization/deserialization
### Detailed Code Changes
**File: `internal/db/migrations.go`**
- Append to `migrations` slice (after line 37):
```go
`CREATE TABLE IF NOT EXISTS issue_cache (
issue_number INTEGER PRIMARY KEY,
title TEXT NOT NULL,
body TEXT,
state TEXT NOT NULL,
labels TEXT,
assignee TEXT,
milestone TEXT,
updated_at DATETIME,
cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE INDEX IF NOT EXISTS idx_issue_cache_state ON issue_cache(state)`,
`CREATE INDEX IF NOT EXISTS idx_issue_cache_cached_at ON issue_cache(cached_at)`,
`CREATE INDEX IF NOT EXISTS idx_issue_cache_milestone ON issue_cache(milestone)`,
```
**File: `internal/db/db.go`**
- Add import: `"encoding/json"`
- Add struct after line 22:
```go
type IssueCache struct {
IssueNumber int
Title       string
Body        string
State       string
Labels      string // JSON array
Assignee    string
Milestone   string
UpdatedAt   *time.Time
CachedAt    time.Time
}
```
- Add 5 methods after line 253 (end of file)
**File: `internal/db/db_test.go`**
- Add 6 test functions following existing patterns (lines 257+)


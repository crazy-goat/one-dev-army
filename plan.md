# Implementation Plan for Issue #176

**Created:** 2026-03-23T13:48:44+01:00
**Updated:** 2026-03-23T13:48:44+01:00

## Analysis

**Current State Verification:**

1. **Core requirements** — The main changes have already been implemented:
   - ✅ `main.go` (lines 205-215): No `EnsureProject()` or `EnsureProjectColumns()` calls exist
   - ✅ `NewOrchestrator()` signature: Does NOT include `projectNumber` parameter
   - ✅ `NewServer()` signature: Does NOT include `projectNumber` parameter
   - ✅ `orchestrator.go`: No `projectNumber` field exists; `moveToColumn()` method exists but is already a no-op (lines 374-378)
   - ✅ `dashboard/handlers.go`: Server struct has no `projectNumber` field

2. **Files that need changes:**
   - `internal/github/project.go` - Only file needing updates (add deprecation comments)

3. **Implementation approach:**
   - Add deprecation comments to `EnsureProject()`, `EnsureProjectColumns()`, and `setupProject()` methods
   - The methods can remain for backward compatibility but should be marked deprecated

4. **Testing strategy:**
   - Run existing tests to ensure no regressions
   - Verify ODA starts without GitHub Projects API calls

---

## Implementation Steps

### Step 1: **File to modify:** `internal/github/project.go`

- Add deprecation comment to `EnsureProject()` method (line 27)
- Add deprecation comment to `EnsureProjectColumns()` method (line 93)
- Add deprecation comment to `setupProject()` method (line 69)

### Step 2: **Example deprecation format:**

```go
// Deprecated: GitHub Projects dependency is being removed. This method is no longer called on startup.
```

### Step 3: **Testing:**

- Run `go test ./...` to verify all tests pass
- Start ODA to confirm no GitHub Projects API calls are made
The implementation is essentially complete. Only documentation/deprecation markers remain as optional cleanup.


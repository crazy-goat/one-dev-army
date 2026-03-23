# Implementation Plan for Issue #176

**Created:** 2026-03-23T13:58:37+01:00
**Updated:** 2026-03-23T13:58:37+01:00

## Analysis

### 1. Core Requirements
The issue requests removing GitHub Projects API dependency from ODA startup by:
- Removing `EnsureProject()` and `EnsureProjectColumns()` calls from `main.go`
- Removing `projectNumber` parameter from `NewOrchestrator()` and `NewServer()`
- Marking project-related methods as deprecated in `project.go`
- Removing `projectNumber` field and `moveToColumn()` method from `orchestrator.go`
- Removing `projectNumber` from dashboard Server struct

### 2. Current State Verification
After examining all relevant files:

| Requirement | Status | Evidence |
|-------------|--------|----------|
| `main.go` - Remove `EnsureProject()` calls | ✅ **Already removed** | Lines 204-213: No calls exist |
| `main.go` - Remove `EnsureProjectColumns()` calls | ✅ **Already removed** | Not present in startup sequence |
| `NewOrchestrator()` - Remove `projectNumber` param | ✅ **Already removed** | `internal/mvp/orchestrator.go:54` - signature has 5 params, no `projectNumber` |
| `NewServer()` - Remove `projectNumber` param | ✅ **Already removed** | `internal/dashboard/server.go:36` - signature has 7 params, no `projectNumber` |
| `project.go` - Mark methods deprecated | ✅ **Already done** | Lines 30, 72, 97 have deprecation comments |
| `orchestrator.go` - Remove `projectNumber` field | ✅ **Already removed** | Struct has no `projectNumber` field |
| `orchestrator.go` - Remove `moveToColumn()` method | ⚠️ **Partially done** | Method exists at line 374 but is already a no-op with TODO comment |
| `dashboard/handlers.go` - Remove `projectNumber` | ✅ **Already removed** | Server struct has no `projectNumber` field |

### 3. Implementation Approach
The GitHub Projects dependency has already been removed from the startup sequence. The remaining `moveToColumn()` method:
- Is already a no-op (just logs a message)
- Has a TODO comment indicating it will be replaced with label-based implementation
- Is called in 4 places but causes no actual GitHub API calls

### 4. Testing Strategy
- Run existing tests to verify no regressions
- The `moveToColumn()` calls are harmless no-ops

---

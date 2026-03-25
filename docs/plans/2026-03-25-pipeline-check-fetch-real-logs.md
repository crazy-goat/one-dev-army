# Pipeline Check: Fetch Real CI Failure Logs

**Date:** 2026-03-25
**Problem:** `GetPRChecks()` returns only check names and status (e.g. `"- lint: FAILURE"`) but no actual error output. The LLM has no information to fix failures, leading to 5 blind retry attempts that all fail.

## Design

### Approach: `gh run view --log-failed`

When `gh pr checks` reports a FAILURE, extract the run ID from the check's `link` field and fetch actual failure logs via `gh run view <run-id> --log-failed`.

### Changes

**File: `internal/github/issues.go`**

1. Add `Link` field to `PRCheck` struct
2. Add `link` to `--json` fields in `GetPRChecks()`
3. On failure: extract unique run IDs from link URLs, call `gh run view <id> --log-failed` for each, concatenate logs
4. New helper: `extractRunID(link string) string` — regex `/actions/runs/(\d+)/`
5. New helper: `getFailedRunLogs(runID string) string` — calls `gh run view` and returns output

### Flow

```
gh pr checks <branch> --json name,state,link
    |
    v
Any FAILURE? --no--> return pass/pending
    |
   yes
    |
    v
For each failed check:
  1. extractRunID(link) --> run ID
  2. Deduplicate run IDs (multiple jobs share one run)
  3. gh run view <run-id> --log-failed
  4. Append to logs
    |
    v
return PRChecksResult{Status: "fail", Logs: <real logs>}
```

### What stays the same

- `worker.go` — no changes, `checkPipeline()` and `handlePipelineFailure()` work as before
- `implementation.md` prompt — reads `pipeline-fail.log` which now contains real logs
- Orchestrator — purely reactive, no changes

### Key details

- **Dedup run IDs:** `build` and `lint` are separate jobs but may share a workflow run ID. Collect unique IDs to avoid fetching the same log twice.
- **`--log-failed`** returns only output from failed steps — exactly what the LLM needs.
- **Graceful degradation:** If `gh run view` fails (e.g. logs expired), fall back to the current behavior (check name + status).

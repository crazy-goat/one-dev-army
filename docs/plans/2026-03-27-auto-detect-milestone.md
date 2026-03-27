# Auto-Detect New Sprint/Milestone Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make ODA automatically detect when a new sprint (milestone) is created and switch to it without requiring restart

**Architecture:** SyncService will periodically check for the oldest open milestone (same logic as orchestrator) and automatically update its `activeMilestone` when a new one is detected. This ensures both sync and orchestrator work with the same sprint.

**Tech Stack:** Go, SQLite, GitHub CLI (gh), existing ODA sync service

---

## Problem Analysis

Currently:
1. **Orchestrator** correctly fetches the latest milestone in each loop iteration (`GetOldestOpenMilestone()`)
2. **SyncService** has `activeMilestone` set once at startup and never updates it
3. When a new sprint is created, orchestrator sees it but SyncService keeps syncing old sprint's issues
4. New issues in the new sprint never get cached, so orchestrator finds 0 candidates

## Solution

Add milestone detection to SyncService's sync cycle. When a different milestone is detected, update `activeMilestone` and log the change.

### Edge Cases to Handle

1. **No active milestone** - When all milestones are closed, SyncService should detect `nil` and stop syncing (log warning)
2. **Milestone closed externally** - When someone closes milestone via GitHub UI (not dashboard), SyncService should detect the next open milestone
3. **Close sprint via dashboard** - Already works (handlers.go:835-840), but verify it still works after our changes
4. **Same milestone title, different number** - Should handle case where milestone is recreated with same name

---

### Task 1: Add Milestone Detection to SyncService

**Files:**
- Modify: `internal/dashboard/sync.go:12-35` (add GitHubClient interface method)
- Modify: `internal/dashboard/sync.go:150-201` (add milestone check in doSync)
- Test: `internal/dashboard/sync_test.go` (add test for milestone switching)

**Step 1: Extend GitHubClient interface**

Add `GetOldestOpenMilestone() (*github.Milestone, error)` to the interface in sync.go:

```go
// GitHubClient defines the interface for GitHub operations needed by SyncService
type GitHubClient interface {
	ListIssuesWithPRStatus(milestone string) ([]github.Issue, error)
	AddLabel(issueNum int, label string) error
	GetOldestOpenMilestone() (*github.Milestone, error)  // ADD THIS
}
```

**Step 2: Add milestone detection logic**

In `doSync()` method, before syncing issues, check if milestone changed:

```go
func (s *SyncService) doSync() {
	// Check for milestone change first
	currentMilestone := s.GetActiveMilestone()
	latestMilestone, err := s.gh.GetOldestOpenMilestone()
	if err != nil {
		log.Printf("[SyncService] Error checking for new milestone: %v", err)
	} else if latestMilestone == nil {
		// No open milestones at all
		if currentMilestone != "" {
			log.Printf("[SyncService] No open milestones found (was: %s), stopping sync", currentMilestone)
			s.SetActiveMilestone("")
		}
		return
	} else if latestMilestone.Title != currentMilestone {
		log.Printf("[SyncService] Milestone change detected: %s (was: %s)", 
			latestMilestone.Title, currentMilestone)
		s.SetActiveMilestone(latestMilestone.Title)
		// Also update GitHub client's active milestone
		if s.gh != nil {
			if client, ok := s.gh.(*github.Client); ok {
				client.SetActiveMilestone(latestMilestone)
			}
		}
	}
	
	milestone := s.GetActiveMilestone()
	if milestone == "" {
		log.Println("[SyncService] No active milestone, skipping sync")
		return
	}
	// ... rest of existing doSync logic
}
```

**Step 3: Run existing tests**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard/... -v -run TestSyncService
```

Expected: All existing tests pass

**Step 4: Commit**

```bash
git add internal/dashboard/sync.go
git commit -m "feat: add milestone auto-detection to sync service"
```

---

### Task 2: Add Test for Milestone Auto-Detection

**Files:**
- Create: `internal/dashboard/sync_milestone_test.go`

**Step 1: Write test for milestone switching**

```go
package dashboard

import (
	"testing"
	"time"

	"github.com/crazy-goat/one-dev-army/internal/github"
)

// mockGitHubClientWithMilestone is a mock that can return different milestones
type mockGitHubClientWithMilestone struct {
	mockGitHubClient
	milestones []*github.Milestone
	callCount  int
}

func (m *mockGitHubClientWithMilestone) GetOldestOpenMilestone() (*github.Milestone, error) {
	if m.callCount < len(m.milestones) {
		ms := m.milestones[m.callCount]
		m.callCount++
		return ms, nil
	}
	return m.milestones[len(m.milestones)-1], nil
}

func TestSyncService_AutoDetectsNewMilestone(t *testing.T) {
	// First milestone
	oldMilestone := &github.Milestone{
		Number:    1,
		Title:     "Sprint 2026-03-26",
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}
	
	// New milestone (created later)
	newMilestone := &github.Milestone{
		Number:    2,
		Title:     "Sprint 2026-03-27",
		CreatedAt: time.Now(),
	}

	mockGH := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{oldMilestone, newMilestone},
	}
	mockStore := &mockStore{}
	mockHub := &mockHub{}

	service := NewSyncService(mockGH, mockStore, mockHub, nil)
	service.SetActiveMilestone(oldMilestone.Title)

	// First sync - should keep old milestone
	service.doSync()
	
	if service.GetActiveMilestone() != oldMilestone.Title {
		t.Errorf("Expected milestone to be %s, got %s", 
			oldMilestone.Title, service.GetActiveMilestone())
	}

	// Second sync - should detect and switch to new milestone
	service.doSync()
	
	if service.GetActiveMilestone() != newMilestone.Title {
		t.Errorf("Expected milestone to switch to %s, got %s", 
			newMilestone.Title, service.GetActiveMilestone())
	}
}
```

**Step 2: Run the new test**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard/... -v -run TestSyncService_AutoDetectsNewMilestone
```

Expected: Test passes

**Step 3: Commit**

```bash
git add internal/dashboard/sync_milestone_test.go
git commit -m "test: add test for milestone auto-detection"
```

---

### Task 3: Update Mock in Existing Tests

**Files:**
- Modify: `internal/dashboard/sync_test.go:30-50` (update mockGitHubClient)

**Step 1: Add GetOldestOpenMilestone to mock**

Find the `mockGitHubClient` struct in sync_test.go and add:

```go
func (m *mockGitHubClient) GetOldestOpenMilestone() (*github.Milestone, error) {
	return nil, nil
}
```

**Step 2: Run all sync tests**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard/... -v
```

Expected: All tests pass

**Step 3: Commit**

```bash
git add internal/dashboard/sync_test.go
git commit -m "test: update mock to support GetOldestOpenMilestone"
```

---

### Task 4: Integration Test - Verify End-to-End

**Files:**
- Manual verification (no code changes)

**Step 1: Build ODA**

```bash
cd /home/decodo/work/one-dev-army
go build -o /tmp/oda-test .
```

**Step 2: Start ODA and check logs**

```bash
# Terminal 1: Start ODA
/tmp/oda-test 2>&1 | tee /tmp/oda_milestone_test.log

# Wait for it to start and show "Found X candidates"
```

**Step 3: Create a new milestone via GitHub**

```bash
# Create new sprint milestone
gh api repos/crazy-goat/one-dev-army/milestones \
  -f title="Sprint $(date '+%Y-%m-%d %H:%M')" \
  -f state=open \
  -f due_on="$(date -d '+14 days' '+%Y-%m-%dT00:00:00Z')"
```

**Step 4: Create a test issue in new sprint**

```bash
# Get the new milestone title
NEW_SPRINT=$(gh api repos/crazy-goat/one-dev-army/milestones --jq '.[] | select(.state=="open") | .title' | sort | tail -1)

# Create issue in new sprint
gh issue create --title "[Test] Auto-detect milestone" \
  --body "Test issue for milestone auto-detection" \
  --milestone "$NEW_SPRINT" \
  --label "test"
```

**Step 5: Verify ODA detects new sprint**

Watch logs for:
```
[SyncService] New milestone detected: Sprint 2026-03-27 XX:XX (was: Sprint 2026-03-26 XX:XX)
[Orchestrator] Found 1 candidates, resume=false
```

**Step 6: Cleanup test issue**

```bash
gh issue close <issue-number> --comment "Test complete"
```

**Step 7: Commit**

```bash
git commit --allow-empty -m "test: verify milestone auto-detection works end-to-end"
```

---

### Task 5: Documentation Update

**Files:**
- Modify: `docs/workflow.md:127-145` (add note about milestone detection)

**Step 1: Add documentation**

Add section after "Resume Capability":

```markdown
### Milestone Auto-Detection

The sync service automatically detects when a new sprint (milestone) is created:
- Every 30 seconds, checks if the oldest open milestone has changed
- When a new sprint is detected, immediately switches to sync issues from the new sprint
- No restart required - seamless transition between sprints
- Orchestrator independently fetches the latest milestone each iteration
```

**Step 2: Commit**

```bash
git add docs/workflow.md
git commit -m "docs: document milestone auto-detection feature"
```

---

### Task 6: Handle Edge Cases - No Active Milestone

**Files:**
- Modify: `internal/dashboard/sync.go:150-201` (already updated in Task 1)
- Test: `internal/dashboard/sync_milestone_test.go`

**Step 1: Add test for no milestone scenario**

```go
func TestSyncService_HandlesNoMilestone(t *testing.T) {
	mockGH := &mockGitHubClientWithMilestone{
		milestones: []*github.Milestone{nil}, // No milestones
	}
	mockStore := &mockStore{}
	mockHub := &mockHub{}

	service := NewSyncService(mockGH, mockStore, mockHub, nil)
	service.SetActiveMilestone("Old Sprint")

	// Sync with no milestones
	service.doSync()
	
	if service.GetActiveMilestone() != "" {
		t.Errorf("Expected milestone to be empty, got %s", service.GetActiveMilestone())
	}
}
```

**Step 2: Run test**

```bash
cd /home/decodo/work/one-dev-army
go test ./internal/dashboard/... -v -run TestSyncService_HandlesNoMilestone
```

Expected: Test passes

**Step 3: Commit**

```bash
git add internal/dashboard/sync_milestone_test.go
git commit -m "test: add test for no milestone scenario"
```

---

### Task 7: Verify Close Sprint Still Works

**Files:**
- Manual verification (no code changes)

**Step 1: Start ODA with current sprint**

```bash
cd /home/decodo/work/one-dev-army
/tmp/oda-test 2>&1 | tee /tmp/oda_close_test.log
```

**Step 2: Create test issue and complete it**

```bash
# Create issue in current sprint
CURRENT_SPRINT=$(gh api repos/crazy-goat/one-dev-army/milestones --jq '.[] | select(.state=="open") | .title' | head -1)
gh issue create --title "[Test] Close sprint test" \
  --body "Test issue" \
  --milestone "$CURRENT_SPRINT" \
  --label "test"

# Move to Done column (via project board or labels)
```

**Step 3: Close sprint via dashboard**

1. Open http://localhost:7000
2. Click "Close Sprint" button
3. Select version bump type
4. Confirm

**Step 4: Verify in logs**

Watch for:
```
[Dashboard] Closed milestone: Sprint ...
[Dashboard] Created new sprint: Sprint ...
[Dashboard] Set new active milestone: Sprint ...
[Dashboard] Updated sync service with new milestone: Sprint ...
[SyncService] Milestone change detected: Sprint ... (was: Sprint ...)
```

**Step 5: Verify orchestrator picks up new sprint**

Check that orchestrator shows:
```
[Orchestrator] Found 0 candidates, resume=false
```
(for the new empty sprint)

**Step 6: Cleanup**

```bash
# Close test issue
gh issue close <issue-number> --comment "Test complete"
```

**Step 7: Commit**

```bash
git commit --allow-empty -m "test: verify close sprint integration works"
```

---

## Verification Checklist

Before claiming complete:
- [ ] All tests pass: `go test ./internal/dashboard/... -v`
- [ ] Lint passes: `golangci-lint run ./...`
- [ ] Manual test shows milestone auto-detection in logs (new milestone via GitHub UI)
- [ ] Manual test shows milestone cleared when all milestones closed
- [ ] Close sprint via dashboard still works correctly
- [ ] New issues in new sprint are picked up by orchestrator
- [ ] Documentation updated

## Summary

This change ensures ODA automatically adapts to new sprints without manual intervention. The sync service checks for milestone changes during its regular 30-second sync cycle, making the transition seamless.

### Key Behaviors:

1. **New milestone created via GitHub UI** → SyncService detects within 30s, switches automatically
2. **Milestone closed via dashboard** → Already works, now also detected by SyncService
3. **All milestones closed** → SyncService stops syncing, logs warning
4. **No restart required** → ODA adapts to milestone changes on-the-fly

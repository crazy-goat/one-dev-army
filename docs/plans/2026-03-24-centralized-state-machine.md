# Centralized State Machine Architecture Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Przenieść kontrolę nad state machine z Workera do Orchestratora - Worker wykonuje, Orchestrator decyduje o stage.

**Architecture:** 
- Worker zgłasza gotowość etapu do Orchestratora
- Orchestrator jako jedyny źródło prawdy o stage
- SyncService informuje Orchestratora o zmianach zewnętrznych
- Wszystkie zmiany stage przez Orchestrator

**Tech Stack:** Go, SQLite, GitHub API, WebSocket

---

## Current State Analysis

### Obecny flow (BROKEN):
```
Worker.setStageLabel("Code") 
  → BroadcastStageUpdate() 
  → GitHub API (async)
  → Ledger save (async)
  → Orchestrator nie wie o zmianie!
```

### Problem:
- Worker sam zmienia stage
- Orchestrator zakłada że ticket jest w "stage:coding"
- Brak synchronizacji między Worker a Orchestrator
- Ledger ma błędne dane (stage:coding → awaiting-approval, pominięto etapy)

### Target flow (FIXED):
```
Worker wykonuje pracę
  → Worker.reportStageComplete("coding") 
  → Orchestrator.decideNextStage()
  → Orchestrator.SetStageLabel("AI Review")
  → Orchestrator.updateCache()
  → Orchestrator.saveToLedger()
  → Worker dostaje instrukcję: "przejdź do AI Review"
```

---

## Task 1: Remove stage changes from Worker

**Files:**
- Modify: `internal/mvp/worker.go:135-143` (remove setStageLabel)
- Modify: `internal/mvp/worker.go:212,240,256,310` (remove calls)

**Step 1: Remove setStageLabel method**

Delete method `setStageLabel()` entirely from worker.go

**Step 2: Remove all setStageLabel calls**

Remove calls at lines:
- Line 212: `w.setStageLabel("Plan")`
- Line 240: `w.setStageLabel("Code")`  
- Line 256: `w.setStageLabel("AI Review")`
- Line 310: `w.setStageLabel("Create PR")`

**Step 3: Commit**

```bash
git add internal/mvp/worker.go
git commit -m "refactor(worker): Remove stage changes from worker

- Worker no longer changes stages
- Worker will report completion to orchestrator
- Orchestrator will control state machine"
```

---

## Task 2: Add Worker → Orchestrator communication

**Files:**
- Create: `internal/mvp/worker_events.go`
- Modify: `internal/mvp/orchestrator.go`

**Step 1: Create worker events types**

```go
package mvp

// WorkerEvent represents a message from worker to orchestrator
type WorkerEvent struct {
	IssueNumber int
	Stage       string      // Stage that was completed
	Status      EventStatus // success, failed, blocked
	Output      string      // Result/output from stage
}

type EventStatus string

const (
	EventSuccess WorkerStatus = "success"
	EventFailed  WorkerStatus = "failed" 
	EventBlocked WorkerStatus = "blocked"
)
```

**Step 2: Add event channel to Orchestrator**

In `internal/mvp/orchestrator.go`:
- Add `workerEventCh chan WorkerEvent` to Orchestrator struct
- Initialize in NewOrchestrator

**Step 3: Add event handler in Orchestrator**

```go
func (o *Orchestrator) handleWorkerEvent(event WorkerEvent) {
	log.Printf("[Orchestrator] Received event from worker: issue #%d completed stage %s with status %s", 
		event.IssueNumber, event.Stage, event.Status)
	
	// Decide next stage based on state machine
	nextStage := o.decideNextStage(event)
	
	// Update stage
	if nextStage != "" {
		o.changeStage(event.IssueNumber, nextStage, "worker_completed_"+string(event.Stage))
	}
}

func (o *Orchestrator) decideNextStage(event WorkerEvent) string {
	// State machine logic
	switch event.Stage {
	case "analysis":
		if event.Status == EventSuccess {
			return "Code"
		}
	case "coding":
		if event.Status == EventSuccess {
			return "AI Review"
		}
	case "code-review":
		if event.Status == EventSuccess {
			return "Create PR"
		}
	case "create-pr":
		if event.Status == EventSuccess {
			return "Approve"
		}
	}
	
	if event.Status == EventFailed {
		return "Failed"
	}
	
	if event.Status == EventBlocked {
		return "NeedsUser"
	}
	
	return ""
}
```

**Step 4: Commit**

```bash
git add internal/mvp/worker_events.go internal/mvp/orchestrator.go
git commit -m "feat(orchestrator): Add worker event handling

- Worker reports stage completion via events
- Orchestrator decides next stage
- Centralized state machine logic"
```

---

## Task 3: Modify Worker to report instead of changing

**Files:**
- Modify: `internal/mvp/worker.go`

**Step 1: Add event reporting to Worker**

```go
func (w *Worker) reportStageComplete(stage string, status EventStatus, output string) {
	if w.orchestrator == nil {
		return
	}
	
	event := WorkerEvent{
		IssueNumber: w.orchestrator.currentTask.Issue.Number,
		Stage:       stage,
		Status:      status,
		Output:      output,
	}
	
	// Send to orchestrator (non-blocking)
	select {
	case w.orchestrator.workerEventCh <- event:
		log.Printf("[Worker] Reported completion of stage %s for issue #%d", stage, event.IssueNumber)
	default:
		log.Printf("[Worker] Failed to report stage completion - channel full")
	}
}
```

**Step 2: Replace setStageLabel with reportStageComplete**

At each stage completion:
```go
// Instead of: w.setStageLabel("Code")
// Use: w.reportStageComplete("coding", EventSuccess, "implementation done")
```

**Step 3: Commit**

```bash
git add internal/mvp/worker.go
git commit -m "feat(worker): Report stage completion to orchestrator

- Worker reports instead of changing stages
- Uses event channel to notify orchestrator
- Waits for orchestrator decisions"
```

---

## Task 4: Add centralized stage change method to Orchestrator

**Files:**
- Modify: `internal/mvp/orchestrator.go`

**Step 1: Create unified stage change method**

```go
// changeStage is the ONLY way to change stages
// It updates: GitHub, cache, ledger, WebSocket
func (o *Orchestrator) changeStage(issueNumber int, toStage, reason string) error {
	log.Printf("[Orchestrator] Changing stage of #%d to %s (reason: %s)", issueNumber, toStage, reason)
	
	// Get current stage from cache
	fromStage := "Unknown"
	if o.store != nil {
		if existing, err := o.store.GetIssueCache(issueNumber); err == nil {
			fromStage = o.getStageFromIssue(existing)
		}
	}
	
	// Update GitHub
	updatedIssue, err := o.gh.SetStageLabel(issueNumber, toStage)
	if err != nil {
		return fmt.Errorf("setting stage %s on #%d: %w", toStage, issueNumber, err)
	}
	
	// Update cache
	if o.store != nil {
		milestone := o.activeMilestone()
		now := time.Now().UTC()
		updatedIssue.UpdatedAt = &now
		
		if err := o.store.SaveIssueCache(updatedIssue, milestone, true); err != nil {
			log.Printf("[Orchestrator] Error saving issue cache for #%d: %v", issueNumber, err)
		}
		
		// Save to ledger
		toLabel := toStage
		if labels, ok := github.StageToLabels[toStage]; ok && len(labels) > 0 {
			toLabel = labels[0]
		}
		if err := o.store.SaveStageChange(issueNumber, fromStage, toLabel, reason, "orchestrator"); err != nil {
			log.Printf("[Orchestrator] Error saving stage change to ledger for #%d: %v", issueNumber, err)
		}
	}
	
	// Broadcast
	if o.hub != nil {
		o.hub.BroadcastIssueUpdate(updatedIssue)
	}
	
	log.Printf("[Orchestrator] Successfully changed stage of #%d from %s to %s", issueNumber, fromStage, toStage)
	return nil
}
```

**Step 2: Update all stage changes to use changeStage**

Replace all `SetStageLabel` calls in orchestrator with `changeStage`.

**Step 3: Commit**

```bash
git add internal/mvp/orchestrator.go
git commit -m "feat(orchestrator): Add centralized stage change method

- changeStage is the ONLY way to change stages
- Updates GitHub, cache, ledger, WebSocket
- All stage changes go through this method"
```

---

## Task 5: Update SyncService to notify Orchestrator

**Files:**
- Modify: `internal/dashboard/sync.go`
- Modify: `internal/mvp/orchestrator.go`

**Step 1: Add sync event handling to Orchestrator**

```go
// HandleSyncEvent processes external changes from GitHub sync
func (o *Orchestrator) HandleSyncEvent(issue github.Issue) {
	log.Printf("[Orchestrator] Received sync event for issue #%d", issue.Number)
	
	// Check if issue is being processed
	if o.currentTask != nil && o.currentTask.Issue.Number == issue.Number {
		log.Printf("[Orchestrator] Issue #%d is currently being processed, ignoring sync", issue.Number)
		return
	}
	
	// Update internal state if needed
	// (e.g., if someone manually changed stage on GitHub)
}
```

**Step 2: Modify SyncService to notify orchestrator**

In `sync.go`, after caching issues:
```go
// Notify orchestrator of external changes
if s.orchestrator != nil {
	for _, issue := range issues {
		s.orchestrator.HandleSyncEvent(issue)
	}
}
```

**Step 3: Commit**

```bash
git add internal/dashboard/sync.go internal/mvp/orchestrator.go
git commit -m "feat(sync): Notify orchestrator of external changes

- SyncService informs orchestrator of GitHub changes
- Orchestrator can react to manual changes
- Prevents conflicts between sync and processing"
```

---

## Task 6: Testing

**Files:**
- Test: `internal/mvp/orchestrator_test.go`
- Test: `internal/mvp/worker_test.go`

**Step 1: Test state machine logic**

```go
func TestDecideNextStage(t *testing.T) {
	tests := []struct {
		name     string
		event    WorkerEvent
		expected string
	}{
		{"coding success → AI Review", WorkerEvent{Stage: "coding", Status: EventSuccess}, "AI Review"},
		{"coding failed → Failed", WorkerEvent{Stage: "coding", Status: EventFailed}, "Failed"},
		{"create-pr success → Approve", WorkerEvent{Stage: "create-pr", Status: EventSuccess}, "Approve"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewOrchestrator(...)
			result := o.decideNextStage(tt.event)
			if result != tt.expected {
				t.Errorf("decideNextStage() = %v, want %v", result, tt.expected)
			}
		})
	}
}
```

**Step 2: Test centralized stage change**

```go
func TestChangeStage(t *testing.T) {
	// Test that changeStage updates all systems
	// - GitHub
	// - Cache  
	// - Ledger
	// - WebSocket
}
```

**Step 3: Commit**

```bash
git add internal/mvp/*_test.go
git commit -m "test: Add tests for centralized state machine

- Test state machine transitions
- Test centralized stage change method
- Verify all systems are updated"
```

---

## Summary

After this refactoring:

1. **Worker** - tylko wykonuje pracę i zgłasza gotowość
2. **Orchestrator** - jedyny zmienia stage, kontroluje state machine
3. **SyncService** - informuje orchestratora o zmianach zewnętrznych
4. **Ledger** - zawsze poprawne dane (jedno źródło prawdy)

**Key principle:** Orchestrator jako **centralny kontroler** wszystkich zmian stage.
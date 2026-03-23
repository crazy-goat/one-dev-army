# State Machine - Ticket Flow

## Overview

This document describes the complete state machine for ticket processing in the One Dev Army system. Each state corresponds to a specific label on the GitHub issue, and transitions are triggered by either the orchestrator/worker or manual user actions.

## States

The system has 10 distinct states, each represented by a GitHub label with the `stage:` prefix:

| State | Label | Description |
|-------|-------|-------------|
| **Backlog** | no `stage:*` label | Initial state, ticket waiting to be picked up |
| **Plan** | `stage:analysis` | Technical planning phase |
| **Code** | `stage:coding` | Implementation phase |
| **AI Review** | `stage:code-review` | AI code review phase |
| **Create PR** | `stage:create-pr` | Creating pull request |
| **Approve** | `stage:awaiting-approval` | PR created, waiting for manual approval |
| **Merge** | `stage:merging` | Merging PR to main branch |
| **Done** | *no label* | Ticket completed, issue closed |
| **Failed** | `stage:failed` | Error occurred during processing |
| **Blocked** | `stage:blocked` | Manually blocked by user |

## State Transitions

### Normal Flow (Happy Path)

```
Backlog → Plan → Code → AI Review → Create PR → Approve → Merge → Done
```

### Error Flow

All errors lead to **Failed** state, and retry always goes back to **Code**:

```
Any State → Failed → [Retry] → Code
```

### Manual Actions

```
Backlog → [User: Close] → Done
Any State → [User: Block] → Blocked → [User: Unblock] → Backlog
Failed → [User: Cancel] → Backlog
```

## Detailed Transitions

### Backlog

**Entry Conditions:**
- Ticket is in milestone
- No `stage:*` label

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Plan** | Orchestrator picks ticket | `SetStageLabel("Plan")` → adds `stage:analysis` |
| **Done** | User closes ticket manually | Close issue (e.g., duplicate, wontfix) |

### Plan

**Entry Conditions:**
- Label: `stage:analysis`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Code** | Technical planning completed successfully | `SetStageLabel("Code")` → adds `stage:coding` |
| **Done** | AI detects "Already Done" in another ticket | `SetStageLabel("Done")` → closes issue with comment |

**Note:** Technical planning always produces a result. If the AI determines the ticket is already implemented elsewhere, it transitions to Done instead of Code.

### Code

**Entry Conditions:**
- Label: `stage:coding`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **AI Review** | Implementation completed successfully | `SetStageLabel("AI Review")` → adds `stage:code-review` |

**Note:** Implementation always succeeds. The worker will retry internally if needed, but never fails at this stage.

### AI Review

**Entry Conditions:**
- Label: `stage:code-review`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Create PR** | AI review approved | `SetStageLabel("Create PR")` → adds `stage:create-pr` |
| **Code** | AI review has comments/issues | `SetStageLabel("Code")` → adds `stage:coding` (retry implementation) |

### Create PR

**Entry Conditions:**
- Label: `stage:create-pr`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Approve** | PR created successfully | `SetStageLabel("Approve")` → adds `stage:awaiting-approval` |
| **Failed** | Error creating PR (permissions, conflicts, etc.) | `SetStageLabel("Failed")` → adds `stage:failed` |

### Approve

**Entry Conditions:**
- Label: `stage:awaiting-approval`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Merge** | User clicks "Approve & Merge" | `SetStageLabel("Merge")` → adds `stage:merging` |
| **Code** | User has comments/rejects PR | `SetStageLabel("Code")` → adds `stage:coding` (retry) |

### Merge

**Entry Conditions:**
- Label: `stage:merging`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Done** | Merge successful | `SetStageLabel("Done")` → removes all labels, closes issue |
| **Failed** | Merge conflict | `SetStageLabel("Failed")` → adds `stage:failed` |

### Failed

**Entry Conditions:**
- Label: `stage:failed`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Code** | User clicks "Retry" | `SetStageLabel("Code")` → adds `stage:coding` |
| **Backlog** | User clicks "Cancel" | `SetStageLabel("Backlog")` → removes all stage labels |

**Note:** Retry always goes back to **Code** state, regardless of which stage originally failed.

### Blocked

**Entry Conditions:**
- Label: `stage:blocked`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Backlog** | User clicks "Unblock" | `SetStageLabel("Backlog")` → removes all stage labels |

**Note:** When unblocking, ticket always returns to Backlog, not to the previous state.

### Done

**Entry Conditions:**
- No `stage:*` label
- Issue is closed

**Exit Transitions:**
- None. This is a terminal state.

## Column Mapping

Dashboard columns are determined by the current label with the following priority:

1. `stage:blocked` → **Blocked**
2. `stage:failed` → **Failed**
3. `stage:merging` → **Approve** (part of Approve column)
4. `stage:awaiting-approval` → **Approve**
5. `stage:create-pr` → **AI Review** (part of AI Review column)
6. `stage:code-review` → **AI Review**
7. `stage:coding` → **Code**
8. `stage:analysis` → **Plan**
9. No `stage:*` label → **Backlog**
10. Issue closed → **Done**

## Dashboard Actions

| Column | Action | Transition |
|--------|--------|------------|
| **Backlog** | Block | → Blocked |
| **Blocked** | Unblock | → Backlog |
| **Approve** | Approve & Merge | → Merge → Done (or → Failed on conflict) |
| **Approve** | Decline | → Code |
| **Failed** | Retry | → Code |
| **Failed** | Cancel | → Backlog |

## Special Cases

### Already Done Detection

During the **Plan** phase, the AI may detect that the ticket is already implemented in another issue or PR. In this case:
- Transition: Plan → Done
- Actions: `SetStageLabel("Done")`, add comment explaining why

### Retry Behavior

When retrying from **Failed** state:
- Always transition to **Code** state
- `SetStageLabel("Code")` removes `stage:failed` and adds `stage:coding`
- Worker restarts implementation from scratch

### Block/Unblock

When a ticket is blocked:
- `SetStageLabel("Blocked")` removes all previous stage labels and adds `stage:blocked`
- Ticket appears in Blocked column

When unblocked:
- `SetStageLabel("Backlog")` removes `stage:blocked`
- Ticket returns to Backlog
- User must manually restart processing

## Implementation Notes

### Universal Stage Transition: `SetStageLabel`

All stage transitions go through `github.Client.SetStageLabel(issueNumber, stageName)`:

1. Validates the stage name against `StageToLabels` map
2. Gets current issue labels
3. **Removes ALL `stage:*` labels** (and legacy bare labels for backward compatibility)
4. Adds the new stage's labels
5. Special case: "Done" also closes the issue
6. Returns the updated issue

### GitHub Labels Required

The system requires these labels to be created in the GitHub repository:

```
stage:backlog
stage:analysis
stage:coding
stage:code-review
stage:create-pr
stage:awaiting-approval
stage:merging
stage:failed
stage:blocked
```

### State Consistency

- A ticket should never have more than one `stage:*` label
- `SetStageLabel` enforces this by removing ALL stage labels before adding the new one
- When transitioning, the old label is always removed before the new one is added
- Done state has no `stage:*` labels at all
- Backlog state has no `stage:*` labels (or optionally `stage:backlog`)

### Error Handling

All errors during processing result in Failed state:
- Technical planning errors: Not applicable (always succeeds or detects Already Done)
- Implementation errors: Not applicable (always succeeds)
- AI review errors: Transition to Code for fixes
- PR creation errors: Failed state
- Merge errors: Failed state

## Version History

- **2026-03-23**: Initial state machine definition
- **2026-03-23**: Added Merge state between Approve and Done
- **2026-03-23**: Simplified error handling - all errors go to Failed, all retries go to Code
- **2026-03-23**: Aligned all labels to use `stage:` prefix, added Create PR stage, added Block/Unblock actions, documented `SetStageLabel` as universal transition method

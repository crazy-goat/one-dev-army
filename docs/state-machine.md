# State Machine - Ticket Flow

## Overview

This document describes the complete state machine for ticket processing in the One Dev Army system. Each state corresponds to a specific label on the GitHub issue, and transitions are triggered by either the orchestrator/worker or manual user actions.

## States

The system has 11 distinct states, each represented by a GitHub label:

| State | Label | Description |
|-------|-------|-------------|
| **Backlog** | `stage:backlog` or no label | Initial state, ticket waiting to be picked up |
| **Plan** | `stage:analysis` | Technical planning phase |
| **Code** | `stage:coding` | Implementation phase |
| **AI Review** | `stage:code-review` | AI code review phase |
| **Create PR** | `stage:create-pr` | Creating pull request |
| **Approve** | `stage:awaiting-approval` | PR created, waiting for manual approval |
| **Merge** | `stage:merging` | Merging PR to main branch |
| **Done** | *no label* | Ticket completed, issue closed |
| **Failed** | `stage:failed` + previous stage label | Error occurred during processing |
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
- No `stage:*` label OR has `stage:backlog` label

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Plan** | Orchestrator picks ticket | Remove `stage:backlog`, add `stage:analysis` |
| **Done** | User closes ticket manually | Close issue (e.g., duplicate, wontfix) |

### Plan

**Entry Conditions:**
- Label: `stage:analysis`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Code** | Technical planning completed successfully | Remove `stage:analysis`, add `stage:coding` |
| **Done** | AI detects "Already Done" in another ticket | Remove `stage:analysis`, close issue with comment |

**Note:** Technical planning always produces a result. If the AI determines the ticket is already implemented elsewhere, it transitions to Done instead of Code.

### Code

**Entry Conditions:**
- Label: `stage:coding`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **AI Review** | Implementation completed successfully | Remove `stage:coding`, add `stage:code-review` |

**Note:** Implementation always succeeds. The worker will retry internally if needed, but never fails at this stage.

### AI Review

**Entry Conditions:**
- Label: `stage:code-review`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Create PR** | AI review approved | Remove `stage:code-review`, add `stage:create-pr` |
| **Code** | AI review has comments/issues | Remove `stage:code-review`, add `stage:coding` (retry implementation) |

### Create PR

**Entry Conditions:**
- Label: `stage:create-pr`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Approve** | PR created successfully | Remove `stage:create-pr`, add `stage:awaiting-approval` |
| **Failed** | Error creating PR (permissions, conflicts, etc.) | Add `stage:failed` (keep `stage:create-pr`) |

### Approve

**Entry Conditions:**
- Label: `stage:awaiting-approval`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Merge** | User clicks "Approve & Merge" | Remove `stage:awaiting-approval`, add `stage:merging` |
| **Code** | User has comments/rejects PR | Remove `stage:awaiting-approval`, add `stage:coding` (retry) |

### Merge

**Entry Conditions:**
- Label: `stage:merging`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Done** | Merge successful | Remove all `stage:*` labels, close issue |
| **Failed** | Merge conflict | Add `stage:failed` (keep `stage:merging`) |

### Failed

**Entry Conditions:**
- Label: `stage:failed` + previous stage label

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Code** | User clicks "Retry" | Remove `stage:failed`, add `stage:coding` (always restart from Code) |
| **Backlog** | User clicks "Cancel" | Remove `stage:failed`, add `stage:backlog` |

**Note:** Retry always goes back to **Code** state, regardless of which stage originally failed.

### Blocked

**Entry Conditions:**
- Label: `stage:blocked`

**Exit Transitions:**

| To | Trigger | Actions |
|----|---------|---------|
| **Backlog** | User unblocks ticket | Remove `stage:blocked`, add `stage:backlog` |

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
3. `stage:merging` → **Merge** (or part of Approve column)
4. `stage:awaiting-approval` → **Approve**
5. `stage:create-pr` → **Create PR** (or part of AI Review column)
6. `stage:code-review` → **AI Review**
7. `stage:coding` → **Code**
8. `stage:analysis` → **Plan**
9. `stage:backlog` or no label → **Backlog**

## Special Cases

### Already Done Detection

During the **Plan** phase, the AI may detect that the ticket is already implemented in another issue or PR. In this case:
- Transition: Plan → Done
- Actions: Remove `stage:analysis`, add comment explaining why, close issue

### Retry Behavior

When retrying from **Failed** state:
- Always transition to **Code** state
- Remove `stage:failed` label
- Add `stage:coding` label
- Worker restarts implementation from scratch

### Block/Unblock

When a ticket is blocked:
- Add `stage:blocked` label (keep existing stage label)
- Ticket appears in Blocked column

When unblocked:
- Remove `stage:blocked`
- Add `stage:backlog` (return to Backlog)
- User must manually restart processing

## Implementation Notes

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

- A ticket should never have more than one `stage:*` label (except when in Failed state)
- Failed state is the only state where two labels coexist: `stage:failed` + the stage where error occurred
- When transitioning, always remove the old label before adding the new one
- Done state has no `stage:*` labels at all

### Error Handling

All errors during processing (except "Already Done") result in Failed state:
- Technical planning errors: Not applicable (always succeeds or detects Already Done)
- Implementation errors: Not applicable (always succeeds)
- AI review errors: Transition to Code for fixes
- PR creation errors: Failed state
- Merge errors: Failed state

## Version History

- **2026-03-23**: Initial state machine definition
- **2026-03-23**: Added Merge state between Approve and Done
- **2026-03-23**: Simplified error handling - all errors go to Failed, all retries go to Code

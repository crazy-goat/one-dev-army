# ODA Ticket Workflow

This document describes the complete lifecycle of a ticket (GitHub Issue) as it flows through the ODA (One Dev Army) system.

## Overview

ODA acts as your automated development team, processing GitHub Issues through a structured pipeline from analysis to PR creation.

## Ticket Lifecycle Flowchart

```mermaid
flowchart TD
    Start([GitHub Issue Created]) --> Milestone{In Active<br/>Milestone?}
    Milestone -->|No| Wait[Wait for Sprint<br/>Planning]
    Milestone -->|Yes| Pick[Orchestrator Picks<br/>Next Ticket]
    Wait --> Milestone
    
    Pick --> Label1[Add Label:<br/>in-progress]
    Label1 --> Column1[Move to Column:<br/>In Progress]
    Column1 --> Analyze[Step 1: Analyze<br/>LLM analyzes requirements]
    
    Analyze --> Plan[Step 2: Plan<br/>Create implementation plan]
    Plan --> AlreadyDone{Already<br/>Done?}
    AlreadyDone -->|Yes| Close[Close Issue<br/>Add Comment]
    AlreadyDone -->|No| Implement[Step 3: Implement<br/>Write code & tests]
    
    Implement --> Review[Step 4: Code Review<br/>AI reviews changes]
    Review --> Approved{Approved?}
    Approved -->|No| Fix[Fix Issues<br/>Push Changes]
    Fix --> Review
    Approved -->|Yes| CreatePR[Step 5: Create PR<br/>Push & Open PR]
    
    CreatePR --> CheckPipeline[Step 6: Check Pipeline<br/>Run lint, tests, typecheck]
    CheckPipeline --> ChecksPass{Checks Pass?}
    ChecksPass -->|Yes| Label2[Add Label:<br/>awaiting-approval]
    Label2 --> Column2[Move to Column:<br/>Approve]
    Column2 --> Manual[Await Manual<br/>Approval]
    
    ChecksPass -->|No| Label3[Add Label:<br/>failed]
    Label3 --> Column3[Move to Column:<br/>Blocked]
    Column3 --> Error[Log Error<br/>Wait for User]
    
    Close --> Column4[Move to Column:<br/>Done]
    Column4 --> End1([End])
    
    Manual --> End2([End])
    Error --> End3([End])
```

## State Machine Diagram

```mermaid
stateDiagram-v2
    [*] --> Pending: Issue created
    
    Pending --> Analyzing: Orchestrator picks ticket
    Analyzing --> Planning: Analysis complete
    Planning --> Coding: Plan created
    Planning --> Done: Already implemented
    
    Coding --> Reviewing: Implementation done
    Reviewing --> CreatingPR: Code approved
    Reviewing --> Coding: Issues found (retry)
    
    CreatingPR --> CheckingPipeline: PR created
    CheckingPipeline --> AwaitingApproval: All checks pass
    CheckingPipeline --> Failed: Checks failed
    
    AwaitingApproval --> Merging: User approves
    Merging --> Done: PR merged
    
    Analyzing --> Failed: Error
    Planning --> Failed: Error
    Coding --> Failed: Error
    
    Failed --> [*]: User intervention required
    Done --> [*]: Complete
```

## Pipeline Steps Detail

| Step | Name | Description | LLM Role | Status |
|------|------|-------------|----------|--------|
| 1 | **Analyze** | Reads issue, analyzes codebase, identifies files to change | Planning LLM | `StatusAnalyzing` |
| 2 | **Plan** | Creates step-by-step implementation plan | Planning LLM | `StatusPlanning` |
| 3 | **Implement** | Writes code, creates tests, runs test command | Epic Analysis LLM | `StatusCoding` |
| 4 | **Code Review** | AI reviews code for correctness, quality, security | Planning LLM | `StatusReviewing` |
| 5 | **Create PR** | Pushes branch to GitHub, opens pull request | - | `StatusCreatingPR` |

## Error Handling

### Retry Logic
- **Code Review**: If review finds issues, fixes are applied and review re-runs
- **Max Retries**: Unlimited retries for code review fixes (until approved or manually stopped)

### Failure States
When any step fails:
1. Label `in-progress` is removed
2. Label `failed` is added
3. Issue moved to **Blocked** column
4. Error logged to console and database
5. Orchestrator waits for user intervention

### Already Done Detection
Both planning and code review stages check if the issue is already implemented:
- If detected, issue is closed automatically with explanatory comment
- Moved to **Done** column

## GitHub Integration

### Labels Managed by ODA
| Label | Purpose |
|-------|---------|
| `in-progress` | Ticket is being processed |
| `awaiting-approval` | PR created, waiting for manual merge |
| `failed` | Processing failed, needs attention |
| `merge-failed` | Previous merge attempt failed (cleared on retry) |

### Project Board Columns
| Column | Meaning |
|--------|---------|
| **In Progress** | Active development |
| **Approve** | PR ready, awaiting manual approval |
| **Blocked** | Failed or manually blocked issues |
| **Done** | Completed or closed as already done |

## Orchestrator Behavior

### Ticket Selection
1. Polls oldest open milestone every 30 seconds
2. Prioritizes tickets that unblock most dependencies
3. Considers priority labels (high > medium > low)
4. Skips epics (tracking issues)
5. Resumes in-progress tickets on restart

### Blocking Logic
Issues with these labels block new work:
- `awaiting-approval` - Unmerged PR exists
- `failed` - Needs user intervention
- Manually moved to **Blocked** column

### Resume Capability
- Progress saved to SQLite database after each step
- On restart, resumes from last completed step
- Branch and worktree preserved across restarts

### Milestone Auto-Detection

The sync service automatically detects when a new sprint (milestone) is created:
- Every 30 seconds, checks if the oldest open milestone has changed
- When a new sprint is detected, immediately switches to sync issues from the new sprint
- No restart required - seamless transition between sprints
- Orchestrator independently fetches the latest milestone each iteration

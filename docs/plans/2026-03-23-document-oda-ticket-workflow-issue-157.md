# Document ODA Ticket Workflow with Mermaid Diagrams - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create comprehensive visual documentation of the ODA ticket lifecycle using Mermaid diagrams, showing the flow from issue creation through to PR completion.

**Architecture:** Create a new `docs/workflow.md` file with Mermaid flowcharts and state diagrams that accurately reflect the actual implementation in `internal/mvp/orchestrator.go` and `internal/mvp/worker.go`. Update `README.md` to link to the new documentation.

**Tech Stack:** Markdown with Mermaid syntax (GitHub-native rendering)

---

## Research Summary

### Actual Implementation States (from `internal/mvp/task.go`):
- `StatusPending` → `StatusAnalyzing` → `StatusPlanning` → `StatusCoding` → `StatusReviewing` → `StatusCreatingPR` → `StatusDone`
- Error path: → `StatusFailed`

### Worker Pipeline Steps (from `internal/mvp/worker.go:132`):
1. **analyze** - LLM analyzes issue requirements
2. **plan** - Creates implementation plan
3. **implement** - Writes code and tests
4. **code-review** - AI reviews the code (with retry loop for fixes)
5. **create-pr** - Pushes branch and creates PR

### Orchestrator Labels (from `internal/mvp/orchestrator.go`):
- `in-progress` - Added when work starts
- `awaiting-approval` - Added when PR is ready
- `failed` - Added when processing fails
- `merge-failed` - Removed when retrying

### Project Board Columns:
- **In Progress** - Active work
- **Approve** - Awaiting manual approval
- **Blocked** - Failed or manually blocked
- **Done** - Completed

---

### Task 1: Create docs/workflow.md with Mermaid Diagrams

**Files:**
- Create: `docs/workflow.md`

**Step 1: Create the workflow documentation file**

Create `docs/workflow.md` with the following content:

```markdown
# ODA Ticket Workflow

This document describes the complete lifecycle of a ticket (GitHub Issue) as it flows through the ODA (One Dev Army) system.

## Overview

ODA acts as your automated development team, processing GitHub Issues through a structured pipeline from analysis to PR creation.

## Ticket Lifecycle Flowchart

\`\`\`mermaid
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
    
    CreatePR --> Success{Success?}
    Success -->|Yes| Label2[Add Label:<br/>awaiting-approval]
    Label2 --> Column2[Move to Column:<br/>Approve]
    Column2 --> Manual[Await Manual<br/>Approval]
    
    Success -->|No| Label3[Add Label:<br/>failed]
    Label3 --> Column3[Move to Column:<br/>Blocked]
    Column3 --> Error[Log Error<br/>Wait for User]
    
    Close --> Column4[Move to Column:<br/>Done]
    Column4 --> End1([End])
    
    Manual --> End2([End])
    Error --> End3([End])
\`\`\`

## State Machine Diagram

\`\`\`mermaid
stateDiagram-v2
    [*] --> Pending: Issue created
    
    Pending --> Analyzing: Orchestrator picks ticket
    Analyzing --> Planning: Analysis complete
    Planning --> Coding: Plan created
    Planning --> Done: Already implemented
    
    Coding --> Reviewing: Implementation done
    Reviewing --> CreatingPR: Code approved
    Reviewing --> Coding: Issues found (retry)
    
    CreatingPR --> Done: PR created
    CreatingPR --> Failed: Error creating PR
    
    Analyzing --> Failed: Error
    Planning --> Failed: Error
    Coding --> Failed: Error
    
    Failed --> [*]: User intervention required
    Done --> [*]: Complete
\`\`\`

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
```

**Step 2: Verify file was created**

Run: `ls -la docs/workflow.md`
Expected: File exists with size > 0

**Step 3: Commit**

```bash
git add docs/workflow.md
git commit -m "docs: add ODA ticket workflow documentation with Mermaid diagrams

- Add comprehensive workflow.md with flowchart and state diagram
- Document all 5 pipeline steps: analyze, plan, implement, code-review, create-pr
- Include error handling and retry logic documentation
- Document GitHub labels and project board columns"
```

---

### Task 2: Update README.md to Link to Workflow Documentation

**Files:**
- Modify: `README.md:98-100`

**Step 1: Update the Status section in README**

Replace lines 98-100 in `README.md`:

```markdown
## Status

Early development. See [docs/plans/](docs/plans/) for design documents and current status.
For ticket workflow documentation, see [docs/workflow.md](docs/workflow.md).
```

With:

```markdown
## Documentation

- **Workflow**: [docs/workflow.md](docs/workflow.md) - Visual guide to the ODA ticket lifecycle
- **Design**: [docs/plans/](docs/plans/) - Architecture and implementation plans
- **Status**: Early development
```

**Step 2: Verify the change**

Run: `grep -A 5 "## Documentation" README.md`
Expected: Shows the new Documentation section with workflow.md link

**Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add workflow documentation link to README

- Replace Status section with Documentation section
- Add link to new workflow.md with Mermaid diagrams
- Keep existing links to design plans"
```

---

### Task 3: Verify Mermaid Rendering (Optional Visual Check)

**Files:**
- None (documentation verification only)

**Step 1: Preview Mermaid syntax**

The Mermaid diagrams use standard GitHub-flavored Mermaid syntax:
- `flowchart TD` for top-down flowcharts
- `stateDiagram-v2` for state machines
- Standard node shapes and arrows

**Step 2: Verify on GitHub**

After pushing, verify diagrams render correctly at:
`https://github.com/crazy-goat/one-dev-army/blob/main/docs/workflow.md`

Expected: Both flowchart and state diagram render as visual diagrams

---

## Summary

This implementation creates:
1. **docs/workflow.md** - Complete visual documentation with:
   - Flowchart showing full ticket lifecycle
   - State diagram showing status transitions
   - Pipeline steps table
   - Error handling documentation
   - GitHub integration details

2. **README.md update** - Links to new workflow documentation

**No code changes required** - this is documentation-only.

**Testing**: Visual verification that Mermaid renders correctly on GitHub.

---
name: creating-oda-ticket
description: >
  Use when the user asks to create a ticket, issue, or task for ODA or One Dev Army —
  e.g. "add a ticket to oda", "create an issue for one dev army", "new oda ticket",
  "dodaj ticket do oda", "stwórz issue w one dev army".
  Guides the conversation to gather required information (title, body) and optional
  metadata (priority, size, type, sprint), then runs the `oda issue create` CLI command.
---

# Creating an ODA Ticket

## Overview

ODA (One Dev Army) has a CLI command `oda issue create` that creates GitHub Issues with optional labels and sprint assignment. This skill ensures you gather all necessary information from the user and run the command correctly.

## When to Use

- User asks to create/add a ticket, issue, or task for ODA / One Dev Army
- User describes a bug or feature they want tracked in the ODA pipeline
- User says something like "add this to the backlog" in an ODA project context

## Command Reference

```bash
oda issue create \
  --title "Issue title" \
  --text "Issue body text" \
  [--priority high|medium|low] \
  [--size S|M|L|XL] \
  [--type bug|feature] \
  [--current-sprint]
```

### Required Flags

| Flag | Description |
|------|-------------|
| `--title` | Short, descriptive issue title |
| `--text` | Full issue body — requirements, acceptance criteria, context |

### Optional Flags

| Flag | Values | Effect |
|------|--------|--------|
| `--priority` | `high`, `medium`, `low` | Adds `priority:<value>` label |
| `--size` | `S`, `M`, `L`, `XL` | Adds `size:<value>` label |
| `--type` | `bug`, `feature` | Adds `bug` or `feature` label |
| `--current-sprint` | (boolean) | Assigns to the oldest open milestone |

### Labels Created

- `--priority high` → label `priority:high`
- `--size M` → label `size:M`
- `--type bug` → label `bug`
- `--type feature` → label `feature`

## Workflow

### Step 1 — Gather Information

Before running the command, ensure you have at minimum:

1. **Title** — a concise summary (required)
2. **Body text** — detailed description with context, requirements, and acceptance criteria (required)

If the user provides a vague request (e.g. "add a ticket for fixing the login"), ask clarifying questions to produce a well-written issue body. A good issue body includes:
- **What** needs to be done
- **Why** it matters (context, user impact)
- **Acceptance criteria** — concrete conditions for "done"

Also ask about or infer from context:
- **Priority** — how urgent is this? (high/medium/low)
- **Size** — estimated effort (S/M/L/XL)
- **Type** — is this a bug fix or a new feature?
- **Sprint** — should it go into the current sprint?

If the user provides enough detail to infer these values confidently, use them without asking. If unclear, ask.

### Step 2 — Confirm Before Creating

Present the user with a summary of what will be created:

```
Title: <title>
Body: <body text>
Priority: <value or "not set">
Size: <value or "not set">
Type: <value or "not set">
Sprint: <current sprint or "not assigned">
```

Wait for user confirmation before proceeding. If the user explicitly says "just do it" or provides all details upfront with clear intent, skip confirmation.

### Step 3 — Run the Command

Execute the command from the ODA project directory (where `.oda/config.yaml` exists):

```bash
oda issue create \
  --title "Title here" \
  --text "Body text here" \
  --priority medium \
  --size M \
  --type feature \
  --current-sprint
```

Only include optional flags that have values. Do not pass empty strings.

### Step 4 — Report the Result

The command outputs:
```
Created issue #<number>
Assigned to sprint: <milestone>    (if --current-sprint was used)
Labels: label1, label2, label3     (if any labels were applied)
```

Report the issue number and any labels/sprint assignment to the user.

If the command fails, check:
- Is `.oda/config.yaml` present in the working directory?
- Is `gh` CLI authenticated? (`gh auth status`)
- Does the repository exist and is accessible?
- If `--current-sprint` was used, does the repo have open milestones?

## Important Notes

- The command requires `.oda/config.yaml` to exist in the working directory (it reads `github.repo` from it)
- It shells out to `gh` CLI — the user must be authenticated with GitHub
- Issues created via CLI do **not** automatically get a `stage:backlog` label — they enter the backlog by having no `stage:*` label
- The `--text` flag value should be properly quoted — escape any quotes inside the body text
- For multi-line body text, use `\n` for line breaks or pass the text in a single quoted string

## Example

User: "Add a high-priority bug ticket to ODA for the dashboard WebSocket disconnecting after 5 minutes of inactivity"

```bash
oda issue create \
  --title "Dashboard WebSocket disconnects after 5 minutes of inactivity" \
  --text "The dashboard WebSocket connection drops after approximately 5 minutes of user inactivity. This causes the real-time ticket status updates to stop, and the user sees stale data until they manually refresh the page.

## Steps to Reproduce
1. Open the ODA dashboard
2. Wait 5 minutes without interacting with the page
3. Observe that ticket status changes are no longer reflected in real-time

## Expected Behavior
WebSocket connection should remain alive indefinitely, using ping/pong keepalive frames.

## Acceptance Criteria
- [ ] WebSocket connection stays alive during idle periods
- [ ] Implement ping/pong keepalive mechanism
- [ ] Add automatic reconnection with exponential backoff if connection drops
- [ ] Add tests for keepalive and reconnection logic" \
  --priority high \
  --size M \
  --type bug \
  --current-sprint
```

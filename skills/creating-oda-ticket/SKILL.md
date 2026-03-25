---
name: creating-oda-ticket
description: >
  Use when the user asks to create a ticket, issue, or task for ODA or One Dev Army —
  e.g. "add a ticket to oda", "create an issue for one dev army", "new oda ticket".
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
cat > /tmp/issue_body.md << 'EOF'
Issue body text (markdown supported)
EOF

oda issue create \
  --title "Issue title" \
  --text "$(cat /tmp/issue_body.md)" \
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

## The Golden Rule — Never Guess, Always Ask

**NEVER invent, assume, or fabricate ticket content.** Every piece of information in the ticket must come from the user — either stated explicitly or clearly inferable from the current conversation context.

If the user says "create an oda ticket" or "add a ticket" without describing WHAT the ticket is about — **you MUST have a conversation with the user first.** Do not guess. Do not make up a title. Do not infer from unrelated conversation context.

### How to gather information — conversation, not interrogation

**Talk to the user naturally.** Do NOT dump a list of questions like a form. Instead, have a normal conversation — ask about the topic, understand the problem, and organically gather what you need through dialogue.

```
❌ BAD — interrogation / form-style:
"Sure! I need a few details:
1. What's the title?
2. Describe the problem
3. Priority: high/medium/low?
4. Size: S/M/L/XL?
5. Bug or feature?"

✅ GOOD — natural conversation:
"Sure, what should this ticket be about?"

(user describes the problem)

"Got it. Is this urgent or can it wait?"
```

**Key principles:**
- Ask ONE thing at a time, starting with the most important: **what is this about?**
- Let the conversation flow — follow up naturally based on what the user says
- Infer metadata (priority, size, type) from the conversation when obvious — don't ask about things you already know
- If the user gives you everything in one message, don't ask more — just proceed

### When you have NO context — start the conversation:

- "create an oda ticket" → ask what it's about
- "add this to the backlog" → "this" is undefined — ask what they mean
- "add a ticket for that bug" → which bug? Ask.

### When you have ENOUGH context — proceed:

- "add a ticket for the dashboard WebSocket disconnecting after 5 minutes" — clear subject, go
- "create a ticket: login page returns 500 on wrong password" — clear bug, go
- User just discussed a specific problem and says "make a ticket for that" — context is in the conversation, go

## Workflow

### Step 1 — Gather Information

You need at minimum:

1. **Title** — a concise summary (required)
2. **Body text** — detailed description with context, requirements, and acceptance criteria (required)

If the user's request is vague or lacks a subject, **have a conversation** to understand what they need. A good issue body includes:
- **What** needs to be done
- **Why** it matters (context, user impact)
- **Acceptance criteria** — concrete conditions for "done"

For optional metadata (priority, size, type, sprint) — infer from conversation if obvious, otherwise weave the question into the dialogue naturally.

### Step 2 — Show the Full Issue Before Creating

**Before running the command, ALWAYS show the user the exact issue content that will be sent.** This is not optional. The user must see the full title and body text before you create anything.

Display it like this:

```
Title: <title>

Body:
<full body text exactly as it will be sent — including markdown, acceptance criteria, everything>

Priority: <value or "not set">
Size: <value or "not set">
Type: <value or "not set">
Sprint: <current sprint or "not assigned">
```

Wait for user confirmation before proceeding. Do NOT create the issue until the user approves.

### Step 3 — Send It

Once the user confirms, run `oda issue create` immediately. Do not ask again, do not hesitate — the user said go, so go.

**CRITICAL: NEVER pass body text inline with `--text`.** Passing text directly will break on quotes, backticks, dollar signs, and markdown formatting. You MUST write the body to a local file first, read it back with `cat`, and delete the file after.

Execute the command from the ODA project directory (where `.oda/config.yaml` exists):

```bash
# 1. Write body to a local file (single-quoted 'EOF' prevents bash interpretation)
cat > .issue_body.md << 'EOF'
Body text here with full markdown support.

## Acceptance Criteria
- [ ] Criterion one
- [ ] Criterion two
EOF

# 2. Create the issue, reading body from the file
oda issue create \
  --title "Title here" \
  --text "$(cat .issue_body.md)" \
  --priority medium \
  --size M \
  --type feature \
  --current-sprint

# 3. Clean up
rm .issue_body.md
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

## Example

User: "Add a high-priority bug ticket to ODA for the dashboard WebSocket disconnecting after 5 minutes of inactivity"

```bash
cat > /tmp/issue_body.md << 'EOF'
The dashboard WebSocket connection drops after approximately 5 minutes of user inactivity. This causes the real-time ticket status updates to stop, and the user sees stale data until they manually refresh the page.

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
- [ ] Add tests for keepalive and reconnection logic
EOF

oda issue create \
  --title "Dashboard WebSocket disconnects after 5 minutes of inactivity" \
  --text "$(cat /tmp/issue_body.md)" \
  --priority high \
  --size M \
  --type bug \
  --current-sprint
```

# ODA (One Dev Army)

A single Go binary that turns a solo developer into a full scrum team. ODA orchestrates AI coding agents via [opencode](https://opencode.ai) to analyze, plan, implement, test, and review code — all driven by a lean scrum process.

## How It Works

ODA acts as your Scrum Master + Product Owner:

1. You describe an idea or epic
2. ODA breaks it into tasks (GitHub Issues)
3. ODA plans a sprint and assigns tasks
4. Workers (goroutines) process tasks through a pipeline:
   **analysis → planning → plan review → coding → testing → code review → merge**
5. Each pipeline stage runs in a separate opencode session with a configurable LLM
6. Results appear on a real-time HTMX dashboard

## Prerequisites

- [Go 1.22+](https://go.dev)
- [opencode](https://opencode.ai) (`opencode serve` must be running)
- [gh CLI](https://cli.github.com) (authenticated)
- Git

### GitHub CLI permissions

ODA manages GitHub Projects (board, columns) on your behalf. The default `gh` token does not include project scopes. Add them once:

```bash
gh auth refresh -s project
```

Required scopes:
| Scope | Purpose |
|-------|---------|
| `repo` | Read/write issues, PRs, labels, milestones |
| `project` | Read/write GitHub Projects v2 (board, columns) |

## Quick Start

```bash
# Install
go install github.com/crazy-goat/one-dev-army@latest

# Initialize in your project
cd /path/to/your/project
oda init

# Start the agent
oda
```

## Configuration

ODA stores its config in `.oda/config.yaml`:

```yaml
github:
  repo: "owner/repo"
  use_projects: false  # Set to true to enable GitHub Projects integration
dashboard:
  port: 5000
workers:
  count: 3
opencode:
  url: "http://localhost:5002"
tools:
  lint_cmd: "make lint"
  test_cmd: "make test"
  e2e_cmd: "make e2e"
pipeline:
  stages:
    - name: analysis
      llm: claude-sonnet-4
    - name: planning
      llm: claude-opus-4
    - name: plan-review
      llm: claude-opus-4
    - name: coding
      llm: claude-sonnet-4
    - name: testing
      llm: claude-sonnet-4
    - name: code-review
      llm: claude-opus-4
    - name: merge
      manual_approval: true
  max_retries: 5
sprint:
  tasks_per_sprint: 10
```

### GitHub Projects Integration

By default, ODA uses a label-based workflow and does not require GitHub Projects. To enable GitHub Projects integration:

1. Set `use_projects: true` in your config
2. Run `gh auth refresh -s project` to add project scope
3. Restart ODA

The label-based system (default) is recommended for new projects.

## Architecture

- **Workers** — goroutines with dedicated git worktrees for parallel task execution
- **Pipeline** — configurable stage machine with retry logic (max 5 retries, then escalate to user)
- **GitHub** — source of truth (issues, PRs, project board, milestones)
- **SQLite** — local metrics storage (tokens, costs, duration)
- **Dashboard** — HTMX + Go templates, real-time worker status

## Documentation

- **Workflow**: [docs/workflow.md](docs/workflow.md) - Visual guide to the ODA ticket lifecycle
- **Design**: [docs/plans/](docs/plans/) - Architecture and implementation plans
- **Status**: Early development

## License

MIT

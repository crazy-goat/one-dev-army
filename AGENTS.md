# ODA (One Dev Army)

## Project Overview

ODA is a Go CLI tool (`oda`) that orchestrates AI coding agents to automate the full software development lifecycle. It turns a solo developer into a virtual scrum team by processing GitHub Issues through a multi-stage pipeline: **analysis, planning, coding, AI code review, PR creation, approval, and merge**. Each stage runs in a dedicated [opencode](https://opencode.ai) session with a configurable LLM model.

## Tech Stack

- **Language**: Go 1.25 (single binary, zero external runtime dependencies)
- **AI Backend**: [opencode](https://opencode.ai) — LLM session management via HTTP/WebSocket API
- **Source of Truth**: GitHub (issues, PRs, labels, milestones, optionally Projects v2)
- **Local Storage**: SQLite (metrics, issue cache, task progress)
- **Dashboard**: HTMX + Go `html/template` with embedded assets, real-time updates via WebSocket
- **CLI**: GitHub CLI (`gh`) for all GitHub API interactions
- **Config**: YAML (`.oda/config.yaml`)

## Critical Rules

1. **Every change must have tests.** No code gets merged without corresponding test coverage.
2. **Every change must pass lint and tests locally before committing.** Run `golangci-lint run ./...` and `go test -race ./...` — both must exit with zero errors.
3. **Always commit and push to a feature branch.** Never push directly to `master`. After finishing work, commit all changes and push the branch.

## Documentation

- **[Architecture](docs/architecture.md)** — Orchestrator loop, state machine, worker pool, LLM routing, dashboard. Covers how tickets flow through stages, how workers are managed, and how LLM models are selected per task category.
- **[Configuration](docs/configuration.md)** — CLI commands and `.oda/config.yaml` reference. Describes all config sections: GitHub repo, worker count, opencode URL, LLM model routing, pipeline retries.
- **[Development](docs/development.md)** — Build, pre-commit checklist, CI/linting, testing & code conventions. Contains the exact commands to run before every commit and the full list of 29 enabled linters.
- **[Repository Structure](docs/structure.md)** — Full directory layout with detailed package descriptions. Explains every package in `internal/`, what files it contains, and how they interact.
- **[Workflow](docs/workflow.md)** — Ticket lifecycle flowchart (Mermaid). Shows the full path from issue creation through analysis, coding, review, PR, approval, to merge.
- **[State Machine](docs/state-machine.md)** — Full state machine specification with all transitions. Defines every valid stage, the labels used, retry targets, and terminal states.

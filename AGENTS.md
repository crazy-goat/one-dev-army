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

## Documentation

- **[Architecture](docs/architecture.md)** — Orchestrator loop, state machine, worker pool, LLM routing, dashboard
- **[Configuration](docs/configuration.md)** — CLI commands and `.oda/config.yaml` reference
- **[Development](docs/development.md)** — Build, pre-commit checklist, CI/linting, testing & code conventions
- **[Repository Structure](docs/structure.md)** — Full directory layout with detailed package descriptions
- **[Workflow](docs/workflow.md)** — Ticket lifecycle flowchart (Mermaid)
- **[State Machine](docs/state-machine.md)** — Full state machine specification with all transitions

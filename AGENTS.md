# Developer Workstation - AGENTS.md

## Workspace Overview

This is a multi-project developer workstation. Projects live under `/home/decodo/work/`.

### Primary Projects

| Project | Language | Purpose |
|---------|----------|---------|
| `subscriptions-api` | PHP 8.4 / Symfony 6 | Subscription management microservice (payments, orders, wallets, proxy integrations) |
| `rtk` | Rust | CLI proxy that minimizes LLM token consumption (60-90% savings) |
| `decodo-coding-tools` | Go 1.24 | CLI tool for Nexos AI services, Jira, GitLab operations |
| `opencode` | TypeScript | AI-powered development tool (monorepo: desktop, web, console, SDK) |

---

## subscriptions-api (PHP/Symfony)

### Structure


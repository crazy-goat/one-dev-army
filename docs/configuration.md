# Configuration

## Key Commands

```bash
oda init                    # Initialize .oda/ directory with default config
oda                         # Start orchestrator + dashboard (main mode)
oda --working-dir /path     # Start with custom working directory
oda --debug-websocket       # Enable WebSocket debug logging
```

## Sprint Management Commands

```bash
oda sprint cleanup                    # Clean up artifacts from most recent sprint
oda sprint cleanup --sprint "Name"    # Clean up specific sprint
oda sprint cleanup --dry-run          # Preview what would be deleted
oda sprint cleanup --force            # Skip confirmation prompt
```

The `sprint cleanup` command removes artifacts from completed sprints:
- **Branches**: Deletes merged feature branches (local and remote)
- **Worktrees**: Removes leftover git worktrees
- **Temp files**: Deletes `.oda/tmp/` files older than sprint end

Safety features:
- Shows summary before execution (unless `--force`)
- `--dry-run` mode to preview without deleting
- Skips branches with unmerged changes
- Skips branches with open PRs

## Config File

Config lives in `.oda/config.yaml`. Key sections:

- `github.repo` — target repository (owner/repo)
- `workers.count` — number of parallel workers
- `opencode.url` — opencode API endpoint (default: `http://localhost:5002`)
- `tools.test_cmd` — test command workers run after implementation
- `llm.*` — per-mode model configuration with strong/weak variants
- `pipeline.max_retries` — max retry attempts before escalating to user
- `sprint.auto_start` — automatically start sprint on ODA startup (default: `false`)

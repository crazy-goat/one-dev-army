# Configuration

## Key Commands

```bash
oda init                    # Initialize .oda/ directory with default config
oda                         # Start orchestrator + dashboard (main mode)
oda --working-dir /path     # Start with custom working directory
oda --debug-websocket       # Enable WebSocket debug logging
```

## Config File

Config lives in `.oda/config.yaml`. Key sections:

- `github.repo` — target repository (owner/repo)
- `workers.count` — number of parallel workers
- `opencode.url` — opencode API endpoint (default: `http://localhost:5002`)
- `tools.test_cmd` — test command workers run after implementation
- `llm.*` — per-mode model configuration with strong/weak variants
- `pipeline.max_retries` — max retry attempts before escalating to user
- `sprint.auto_start` — automatically start sprint on ODA startup (default: `false`)

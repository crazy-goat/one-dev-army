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
- `dashboard.keep_screen_on` — prevent screen sleep when dashboard is open (default: `false`)

## Dashboard Settings

### keep_screen_on

When set to `true`, the dashboard will request a screen wake lock when the page loads, preventing the screen from sleeping/turning off. This is useful for:
- Monitoring active processing tasks
- Displaying the board on a shared screen/monitor
- Keeping an eye on sprint progress during work

```yaml
dashboard:
  port: 7000
  keep_screen_on: true  # Prevent screen sleep
```

Users can still toggle this on/off from the dashboard UI. The wake lock is automatically released when:
- The user switches to another tab
- The user closes the browser tab
- The user clicks the "Keep Screen On" button to disable it

**Browser Support:**
- Chrome/Edge: Full support
- Firefox: 128+
- Safari: iOS 16.4+ / macOS 14.4+

For unsupported browsers, the toggle button will be disabled with a tooltip explaining the limitation.

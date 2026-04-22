# TokenTally

<p align="center">
  <img src="banner.png" alt="TokenTally — See it. Spend it. Shrink it." width="680">
</p>

A Windows desktop app for tracking Claude Code token usage, costs, and session history. Reads the JSONL transcripts that Claude Code writes to `~/.claude/projects/` and turns them into a live dashboard — no cloud, no account, no telemetry.

---

## Features

- **7-tab dashboard** — Overview, Prompts, Sessions, Projects, Skills, Tips, Settings
- **Cost estimates** — per-prompt and aggregate costs using live `pricing.json` rates
- **Cache analytics** — cache hit rate, 5-minute vs 1-hour cache breakdown
- **Incremental scanner** — only reads new bytes on each scan; safe with mid-flush partial writes
- **Background service** — optional Windows SCM service keeps the DB up to date even when the dashboard is closed
- **Tips engine** — rule-based suggestions (low cache hit rate, high output ratio, short sessions)
- **Privacy blur** — `Ctrl+B` / `Cmd+B` blurs prompt text and sensitive content for screenshots

## Installation

### Quick start (no service)

Download `tokentally.exe` and run it. The dashboard opens immediately and scans every 30 seconds while it's open.

### With background service (recommended)

Run once as administrator to install the Windows service and add the UI to your login startup:

```
tokentally.exe --install
```

The service (`TokenTally`) starts at boot and keeps `tokentally.db` current. The UI auto-starts at login via the Run registry key. Uninstall with:

```
tokentally.exe --uninstall
```

## Data

| Item | Default path |
|------|-------------|
| Database | `%USERPROFILE%\.claude\tokentally.db` |
| Transcripts scanned | `%USERPROFILE%\.claude\projects\` |

Override with environment variables `TOKENTALLY_DB` and `TOKENTALLY_PROJECTS_DIR`.

## Customising pricing

Edit `pricing.json` in the same directory as `tokentally.exe` and reload the dashboard, or point `TOKENTALLY_PRICING_JSON` at any JSON file with the same structure. Rates are per 1 M tokens (USD):

```json
{
  "models": {
    "claude-sonnet-4-6": {
      "tier": "sonnet",
      "input": 3.00, "output": 15.00,
      "cache_read": 0.30, "cache_create_5m": 3.75, "cache_create_1h": 6.00
    }
  },
  "plans": {
    "api":  { "monthly": 0,   "label": "API (pay-per-token)" },
    "pro":  { "monthly": 20,  "label": "Pro" },
    "max":  { "monthly": 100, "label": "Max" }
  }
}
```

## Building from source

Prerequisites: [Go 1.22+](https://go.dev/dl/), [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation), Windows.

```bash
git clone <repo-url>
cd tokentally

# Run tests
go test ./...

# Production build
wails build -platform windows/amd64

# Faster build (skips binding generation — functionally identical at runtime)
wails build -platform windows/amd64 -skipbindings
```

The binary lands at `build/bin/tokentally.exe`.

## Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `TOKENTALLY_DB` | `~/.claude/tokentally.db` | SQLite database path |
| `TOKENTALLY_PROJECTS_DIR` | `~/.claude/projects` | Directory to scan for JSONL files |
| `TOKENTALLY_PRICING_JSON` | *(embedded)* | Path to a custom `pricing.json` |

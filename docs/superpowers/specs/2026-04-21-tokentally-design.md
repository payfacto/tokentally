# TokenTally — Design Spec

**Date:** 2026-04-21  
**Status:** Approved  
**Project directory:** `C:\claudecode\tokentally`

---

## Overview

TokenTally is a standalone Windows desktop application for tracking Claude Code token usage, costs, and session history. It is a Go rewrite of the Python-based `token-dashboard` project, packaged as a single `.exe` that runs as both a Windows service (background scanner) and a system tray UI application.

Full feature parity with `token-dashboard`: seven tabs (Overview, Prompts, Sessions, Projects, Skills, Tips, Settings), ECharts-based charts, incremental JSONL scanner, pricing engine, tips engine, and cache analytics.

---

## Architecture

### Dual-Mode Single Binary

The same `tokentally.exe` binary operates in two modes, detected at startup:

**Service mode** (`tokentally.exe --service`)
- Registered with Windows Service Control Manager (SCM) via `golang.org/x/sys/windows/svc`
- Starts at system boot, runs in Session 0 (no GUI)
- Scans `~/.claude/projects/*.jsonl` every 30 seconds
- Writes results to `~/.claude/tokentally.db` (SQLite, WAL mode)
- Handles SCM signals: start, stop, pause, continue

**UI mode** (`tokentally.exe`, no flags)
- Auto-started at user login via `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
- Runs in the user's desktop session
- Shows a system tray icon (`icon.png` / `mascot.png`)
- Left-click: opens/focuses the Wails window
- Right-click: context menu — Open Dashboard, Scan Now, Quit
- Wails WebView2 window (1100×700 default, resizable) hosts the seven-tab dashboard
- Reads from the same `tokentally.db` written by the service; can trigger an on-demand scan directly (WAL mode allows concurrent reads)

**Additional CLI verbs** (run as administrator):
- `tokentally.exe --install` — registers the service with SCM and adds the UI to the Run key
- `tokentally.exe --uninstall` — removes both

### Data Flow

```
~/.claude/projects/**/*.jsonl
        ↓
  internal/scanner  (incremental, mtime + byte offset)
        ↓
  ~/.claude/tokentally.db  (SQLite, WAL)
        ↓
  internal/db  (typed query functions)
        ↓
  app/app.go  (Wails App struct — Go bindings)
        ↓
  Wails IPC  (window.go.App.*)
        ↓
  frontend/  (vanilla JS + ECharts)
```

Live refresh: the UI mode process runs its own 30-second scan loop independent of the service. After each scan completes, it calls `runtime.EventsEmit(ctx, "scan", result)` — the JS frontend subscribes with `window.runtime.EventsOn("scan", handler)`. The service (Session 0) has no Wails context and does not emit events; its role is to keep the DB populated between UI sessions. This replaces the SSE stream from the Python version.

---

## Package Layout

```
tokentally/
├── main.go                        # Mode detection, --install/--uninstall/--service dispatch
├── go.mod
├── pricing.json                   # Embedded via go:embed; overridable via TOKENTALLY_PRICING_JSON
│
├── internal/
│   ├── scanner/
│   │   └── scanner.go             # Scan(projectsDir, dbPath) ScanResult; stateless
│   ├── db/
│   │   └── db.go                  # Schema, migrations, all query functions
│   ├── pricing/
│   │   └── pricing.go             # LoadPricing, CostFor, GetPlan, SetPlan
│   └── tips/
│       └── tips.go                # AllTips, DismissTip — rule-based engine
│
├── app/
│   ├── app.go                     # Wails App struct — all methods bound to JS frontend
│   └── tray.go                    # System tray icon, left/right-click handlers
│
├── svc/
│   └── service.go                 # golang.org/x/sys/windows/svc handler
│
└── frontend/                      # Adapted from token-dashboard/web/
    ├── index.html                 # TokenTally branding
    ├── app.js                     # Router; fetch() → window.go.App.*() 
    ├── charts.js                  # ECharts helpers (unchanged)
    ├── echarts.min.js             # Vendored (unchanged)
    ├── style.css                  # Restyled: #0d0d1a bg, #ff6b35 accent, #ffd166 secondary
    └── routes/
        ├── overview.js
        ├── prompts.js
        ├── sessions.js
        ├── projects.js
        ├── skills.js
        ├── tips.js
        └── settings.js            # Gains "Service" section: install/uninstall/status/scan interval
```

---

## Scanner & Database

### Scanner (`internal/scanner`)

Direct Go port of `token_dashboard/scanner.py`:

- Reads `~/.claude/projects/<project-slug>/<session-id>.jsonl`
- Tracks each file's `mtime` and byte offset in a `files` table — only reads new bytes per scan pass (incremental)
- Dedup key: `(session_id, message_id)` — same as Python `_evict_prior_snapshots`; handles streaming snapshot overwrites correctly
- Entry point: `scanner.Scan(projectsDir, dbPath string) (ScanResult, error)` — stateless, no globals, safe to call from both service and UI mode

### Database (`internal/db`)

SQLite via `modernc.org/sqlite` (pure Go, no CGO):

- **WAL mode**: `PRAGMA journal_mode=WAL` — service writes and UI reads do not block each other
- **Schema**: mirrors Python DB — `messages`, `files`, `sessions`, `projects`, `tips_dismissed`, `settings` tables
- **Migrations**: one function per schema version, applied on `Open()` — never destructive
- **Exposed query functions** (direct Go port of Python `db.py`):
  - `OverviewTotals`, `ExpensivePrompts`, `ProjectSummary`, `ToolTokenBreakdown`
  - `RecentSessions`, `SessionTurns`, `DailyTokenBreakdown`, `ModelBreakdown`, `SkillBreakdown`

### Pricing (`internal/pricing`)

- `pricing.json` embedded via `//go:embed pricing.json`
- Runtime override: if `TOKENTALLY_PRICING_JSON` env var is set, load from that path instead
- `CostFor(model string, usage Usage, pricing Pricing) CostResult` — pure function, same logic as Python `cost_for`

---

## Wails App Bindings (`app/app.go`)

The `App` struct exposes one method per data query, callable from JS as `window.go.App.<Method>()`:

| Method | Replaces |
|---|---|
| `GetOverview(since, until string)` | `GET /api/overview` |
| `GetPrompts(limit int, sort string)` | `GET /api/prompts` |
| `GetProjects(since, until string)` | `GET /api/projects` |
| `GetSessions(limit int, since, until string)` | `GET /api/sessions` |
| `GetSessionTurns(sessionID string)` | `GET /api/sessions/:id` |
| `GetTools(since, until string)` | `GET /api/tools` |
| `GetDaily(since, until string)` | `GET /api/daily` |
| `GetByModel(since, until string)` | `GET /api/by-model` |
| `GetSkills()` | `GET /api/skills` |
| `GetTips()` | `GET /api/tips` |
| `DismissTip(key string)` | `POST /api/tips/dismiss` |
| `GetPlan()` | `GET /api/plan` |
| `SetPlan(plan string)` | `POST /api/plan` |
| `ScanNow()` | `GET /api/scan` |
| `GetServiceStatus()` | *(new)* SCM query |
| `InstallService()` | *(new)* re-launches self elevated via `runas` ShellExecute verb |
| `UninstallService()` | *(new)* re-launches self elevated via `runas` ShellExecute verb |

Live refresh replaces SSE: Go calls `runtime.EventsEmit(ctx, "scan", result)` after each scan; JS subscribes with `window.runtime.EventsOn("scan", handler)`.

---

## Frontend Restyle

The `frontend/` directory is a direct copy of `token-dashboard/web/` with these changes:

1. **`style.css`**: colour tokens updated throughout
   - Background: `#0d0d1a` (deep navy, from banner)
   - Primary accent: `#ff6b35` (orange — "Tally" colour)
   - Secondary accent: `#ffd166` (gold — mascot highlight)
   - Surface: `#1a1a2e`
   - Muted text: `#8b98b8`
   - Font: `Inter, system-ui, sans-serif`
2. **`index.html`**: title → "TokenTally"; brand div → "Token**Tally**" (orange span)
3. **`app.js`**: `fetch('/api/...')` calls replaced with `window.go.App.*()` equivalents; SSE listener replaced with `window.runtime.EventsOn`
4. **`routes/settings.js`**: adds a "Service" card — shows SCM status, install/uninstall buttons, scan interval input
5. All other route files: only the fetch-to-binding substitution; chart logic and layout unchanged

---

## System Tray (`app/tray.go`)

Uses `github.com/getlantern/systray`:

- Icon: `icon.png` (16×16 and 32×32 embedded via `go:embed`)
- Tooltip: `"TokenTally — $36.72 this month"` (updated after each scan)
- Left-click: `app.Show()` / `app.Focus()` on the Wails window
- Right-click menu:
  - **Open Dashboard**
  - **Scan Now**
  - *(separator)*
  - **Quit TokenTally**

---

## Error Handling

| Scenario | Behaviour |
|---|---|
| Malformed JSONL line | Skip line, log warning; scan continues |
| File permission denied | Log error, skip file; scan continues |
| SQLite locked | Retry with backoff: 1s, 2s, 4s … max 30s |
| SQLite corrupt | Log to Windows Event Log; service stops gracefully (SCM restart policy applies) |
| DB missing on UI startup | Show empty-state screen ("No data yet — scanner starting up") |
| Install/uninstall without elevation | `InstallService` / `UninstallService` re-spawns `tokentally.exe --install/--uninstall` via `runas` ShellExecute verb, which triggers a UAC prompt automatically |
| WebView2 not installed | Show error dialog with download link on Wails window open |

---

## Testing

- **`internal/scanner`**: unit tests using fixture JSONL files copied from `token-dashboard/tests/fixtures/`; covers incremental scan, dedup, streaming snapshot handling
- **`internal/db`**: integration tests against in-memory SQLite (`:memory:`); one test per query function
- **`internal/pricing`** and **`internal/tips`**: pure-function unit tests
- **`app/app.go`**: thin wrappers — tested implicitly via `internal/db` tests; no separate test suite for v1
- **No UI automation** for v1

Run all tests: `go test ./...`

---

## Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `TOKENTALLY_DB` | `~/.claude/tokentally.db` | SQLite DB path |
| `TOKENTALLY_PROJECTS_DIR` | `~/.claude/projects/` | JSONL source directory |
| `TOKENTALLY_PRICING_JSON` | *(embedded)* | Override pricing.json path |
| `TOKENTALLY_SCAN_INTERVAL` | `30` | Scan interval in seconds |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/wailsapp/wails/v2` | Desktop app framework (WebView2) |
| `golang.org/x/sys/windows/svc` | Windows service SCM integration |
| `modernc.org/sqlite` | Pure-Go SQLite driver (no CGO) |
| `github.com/getlantern/systray` | System tray icon and menu |

All others are Go stdlib.

---

## Out of Scope (v1)

- macOS / Linux support (Windows only for now)
- Auto-update mechanism
- Multiple user profiles
- Remote/networked DB
- Skills `tokens_per_call` for project-local or subagent-dispatched skills (same known limitation as Python version)

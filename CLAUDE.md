# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**TokenTally** — a cross-platform desktop application (Wails v2 + Go) that reads Claude Code JSONL transcripts from `~/.claude/projects/` and presents a 7-tab token usage dashboard. On Windows the same binary also runs as a system tray icon and a background Windows SCM service.

## Commands

```bash
# Install Vue inspector dependencies (first time, or after npm changes)
npm install --prefix frontend/inspector

# Run all tests (any platform)
go test ./...

# Run tests for a single package
go test ./internal/db/...
go test ./internal/scanner/... -v -run TestScanDir

# macOS — dev mode (live reload)
wails dev

# macOS — production build → build/bin/TokenTally.app
wails build -platform darwin/arm64
wails build -platform darwin/amd64

# Windows — production build → build/bin/tokentally.exe
wails build -platform windows/amd64

# Windows — faster build (skips binding generation)
wails build -platform windows/amd64 -skipbindings

# Install / uninstall the background Windows service (requires admin — UAC prompt appears)
./tokentally.exe --install
./tokentally.exe --uninstall
```

## Architecture

### Entry points

Platform entry points use Go's filename-based build constraints (`_windows.go`, `_darwin.go`). Shared helpers live in `main_shared.go` (no constraint).

**Windows** — `main_windows.go` dispatches to one of four modes:

| Flag | Mode |
| --- | --- |
| *(none)* | Wails GUI + systray on main thread |
| `--service` | Windows SCM service (scanner loop only) |
| `--install` | Register SCM service + startup registry key (admin) |
| `--uninstall` | Remove SCM service + startup registry key (admin) |

Wails owns the main goroutine (WebView2Loader requirement); `go a.StartTray()` runs systray in a goroutine (`getlantern/systray` calls `runtime.LockOSThread()` internally — safe in any goroutine). `os.Exit(0)` after `wails.Run()` returns to kill the systray goroutine; never call `systray.Quit()` — deadlocks the Win32 message loop. `HideWindowOnClose: true` keeps the runtime alive so the tray can re-show the window. "Open Dashboard" uses a brief `WindowSetAlwaysOnTop(true/false)` pulse to force foreground focus.

**macOS** — `main_darwin.go` runs the Wails GUI only (no systray, no service). WebKit owns the main thread; closing the window quits the app.

### Data flow

```text
~/.claude/projects/<slug>/<session>.jsonl
         ↓ internal/scanner
     tokentally.db (SQLite, WAL mode)
         ↓ internal/db  (query helpers)
         ↓ app/app.go   (Wails-bound methods)
         ↓ window.go.App.*()   (JS ↔ Go bridge)
     frontend/web/  (vanilla JS, hash router, ECharts)
```

### Key packages

- **`internal/db`** — schema, all SQL query helpers (`ExpensivePrompts`, `OverviewTotals`, `ProjectSummary`, etc.), plan/tips persistence. `db.Pool` holds two `*sql.DB` handles to the same file: Read (`MaxOpenConns=4`) and Write (`MaxOpenConns=1`); `:memory:` paths share one handle. `scanMaps` converts `sql.Rows` → `[]map[string]any` and returns `make([]map[string]any, 0)` — never nil (Wails serializes nil as JSON `null`, crashing Vue `v-if` guards). No ORM.
- **`internal/scanner`** — incremental JSONL walker. Tracks `(path, mtime, bytes_read)` per file; stops at partial lines for mid-flush safety. `evictPriorSnapshots` removes older streaming snapshots sharing `(session_id, message_id)` before upserting. `attachment`-type records (hook results) are parsed via `attachmentPromptText`: the hook name + stdout are stored in `prompt_text` so they appear as clickable rows in the Sessions turn-by-turn view. **Never delete `files` table rows** — they are the "already scanned" markers; removing them causes the scanner to re-import sessions from disk on the next tick.
- **`internal/pricing`** — loads `pricing.json` (rates per 1 M tokens, not per token). `CostFor` looks up by model name; tier fallback is present in the JSON but not yet wired in `CostFor`. The `plan` parameter accepted by `CostFor` is currently unused — cost is always token-based. The `monthly` field on plan entries is used by the Overview frontend only: subscription plans (monthly > 0) show the flat monthly fee as the headline cost with the token-equivalent below.
- **`internal/tips`** — three rule-based tips (`cache-hit-low`, `high-output-ratio`, `many-sessions`). `AllTips` calls `OverviewTotals` and filters against dismissed tip keys.
- **`app/app.go`** — `App` struct with all exported methods Wails binds to `window.go.App.*()`. `Startup` launches `scanLoop` (30 s ticker, emits `"scan"` Wails event after changes).
- **`app/tray_windows.go`** — `StartTray` → `systray.Run`. Menu: Open Dashboard, Scan Now, Quit.
- **`app/tray_darwin.go`** — `StartTray` is a no-op; systray conflicts with WebKit's main-thread ownership on macOS.
- **`app/service_windows.go`** — `GetServiceStatus`, `InstallService`, `UninstallService` for the Settings page; elevation via PowerShell `Start-Process -Verb RunAs`.
- **`app/service_darwin.go`** — stubs for the above methods returning "not supported on macOS".
- **`svc/`** — `//go:build windows`; SCM service handler (`Execute` loop with pause/continue/stop), `Install`/`Uninstall` via `golang.org/x/sys/windows/svc/mgr`.

### Frontend

`frontend/` is served by Wails as embedded assets (no build step). `frontend/web/app.js` is the SPA entry point:

- `_apiMap` maps URL paths to `window.go.app.App.*()` calls — this replaces all `fetch()` calls.
- `api(path)` parses path + query string and routes to the right binding.
- Hash router: `#/overview`, `#/prompts`, `#/sessions`, `#/sessions/<id>`, `#/projects`, `#/skills`, `#/tips`, `#/settings`.
- `window.runtime.EventsOn('scan', () => render())` replaces SSE for live refresh.
- `fmt.htmlSafe()` must be used for any user-derived string placed in innerHTML.

Route modules live in `frontend/web/routes/*.js`. Each exports a default `async function(root)` that sets `root.innerHTML`.

**JS binding namespace:** the correct path is `window.go.app.App.*` (lowercase `app` = Go package name). `window.go.App.*` silently hangs — every call blocks forever. Use `const App = window.go.app.App` alias so there is one place to fix if the package name changes.

**Vue inspector SPA** lives at `frontend/inspector/` (Vite + Vue 3, bundled to `frontend/web/app.bundle.js`). The `wails.json` `frontend:build` command uses `--prefix inspector` — Wails runs lifecycle commands from the `frontend/` directory, so `--prefix frontend/inspector` would double the path. `vite.config.ts` must `define: { 'process.env.NODE_ENV': '"production"' }` for IIFE builds — Vue's dev-mode checks reference it and the bundle silently fails without this define. Generated Wails bindings land in `frontend/web/wailsjs/` (gitignored); regenerate with `wails build` (without `-skipbindings`). Runtime calls via `window.go.app.App.*` work without stubs.

**WebView2 context menu** is disabled in production Wails builds. Any right-click UX must be implemented as a custom JS context menu (see `CalculatorView.vue` for the pattern).

### SQL conventions

- **Parameter binding always.** Any value reachable from user input goes through `?`; column names and `ORDER BY` direction may be interpolated only when they come from internal, caller-controlled values.
- **`(session_id, message_id)`** is the streaming-snapshot dedup key (not `uuid`). See `evictPriorSnapshots`.
- All token columns use `COALESCE(..., 0)` in aggregates.

### Schema migrations

Schema evolution is tracked via a `schema_version` row in the `plan` table (key `schema_version`). To add a migration:

1. Append a `func(tx *sql.Tx) error` to the `migrations` slice in `internal/db/db.go`.
2. Increment `targetSchemaVersion`.
3. Add an internal-package test in `internal/db/migrations_test.go`.

`readSchemaVersion` infers the starting version from legacy gate flags so existing DBs skip already-applied work. Fresh DBs start at `targetSchemaVersion` (migrations never run for them). For "drop a column from an existing table" migrations, follow the `migrateDropToolCallsAutoincrement` pattern: inspect `sqlite_master` first and no-op if the feature is already gone.

### Env vars

| Var | Default |
| --- | --- |
| `TOKENTALLY_DB` | `~/.claude/tokentally.db` |
| `TOKENTALLY_PROJECTS_DIR` | `~/.claude/projects` |
| `TOKENTALLY_PRICING_JSON` | *(uses embedded pricing.json)* |

### Pricing data

`pricing.json` (embedded at build time, overridable via env var) has two top-level sections: `models` (exact name → rates per 1 M tokens) and `plans` (plan key → `{monthly, label}`). Rates use field names `input`, `output`, `cache_read`, `cache_create_5m`, `cache_create_1h` — **not** `_mtok` suffixes.

## Build constraints

Platform-specific files use filename suffixes (`_windows.go`, `_darwin.go`) — no explicit `//go:build` tags needed. Files without a suffix compile on all platforms. Tests run on any platform since they use `:memory:` SQLite.

## CI secrets

| Secret | Where | Purpose |
| --- | --- | --- |
| `HOMEBREW_TAP_TOKEN` | GitHub repo settings → Secrets | Fine-grained PAT with `contents:write` on `payfacto/homebrew-tap`; used by the `brew-tap` CI job to push updated `Casks/tokentally.rb` after each `v*` tag release |

**One-time tap setup:** `tokentally.rb` must exist at the root of `payfacto/homebrew-tap` before the first brew release (any placeholder content — the CI job overwrites it). The `brew-tap` job runs after all platform matrix builds complete and only fires on `v*` tag pushes.

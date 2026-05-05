# TokenTally — Architecture Reference

## Tech Stack

### Languages
| Language | Version | Where Used |
|---|---|---|
| Go | 1.25 | Backend, Wails bindings, SQLite, scanner, all platform logic |
| TypeScript | 5.4 | Vue inspector SPA (`frontend/inspector/`) |
| JavaScript | ES2020+ (vanilla) | Main web SPA (`frontend/web/`) |

### Desktop Framework
**Wails v2.12.0** — embeds a WebView (WebKit on macOS/Linux, WebView2 on Windows) inside a native Go binary. The Go backend is exposed to the frontend via code-generated JS bindings (`window.go.app.App.*`). No Electron; no Node.js at runtime.

### Database
**SQLite** via `modernc.org/sqlite v1.49.1` — pure-Go CGO-free driver. WAL mode enabled. Database path defaults to `~/.claude/tokentally.db`, overridable via `TOKENTALLY_DB`.

Connection pool: `db.Pool` holds two `*sql.DB` handles to the same file — Read (`MaxOpenConns=4`) and Write (`MaxOpenConns=1`). In-memory paths (`:memory:`, used in tests) share one handle.

### Frontend — Inspector SPA
| Library | Version | Purpose |
|---|---|---|
| Vue | 3.5 | Component framework |
| Vue Router | 4.3 | Hash-based client routing |
| Pinia | 2.2 | State management |
| Vite | 5.3 | Build tool (IIFE bundle → `app.bundle.js`) |
| marked | 15.0 | Markdown rendering |
| DOMPurify | 3.4 | XSS sanitisation for rendered markdown |
| gpt-tokenizer | 3.4 | Client-side token counting |
| @vitejs/plugin-vue | 5.0 | Vue SFC transform |

### Frontend — Main SPA
Vanilla JS (no framework, no build step). Served by Wails as embedded assets.
- ECharts (bundled as `echarts.min.js`) for all data visualisations
- Hash router (`#/overview`, `#/prompts`, `#/sessions`, etc.)
- `_apiMap` replaces `fetch()` — maps URL paths to `window.go.app.App.*()` calls
- `window.runtime.EventsOn('scan', ...)` drives live refresh (Wails event bus, replaces SSE)

### Key Go Dependencies
| Module | Version | Purpose |
|---|---|---|
| `github.com/wailsapp/wails/v2` | 2.12.0 | Desktop framework |
| `modernc.org/sqlite` | 1.49.1 | SQLite driver (pure Go) |
| `github.com/getlantern/systray` | 1.2.2 | Windows system tray icon |
| `golang.org/x/sys` | 0.42.0 | Windows SCM service management |
| `github.com/google/uuid` | 1.6.0 | UUID generation |
| `github.com/labstack/echo/v4` | 4.13.3 | HTTP (Wails internal dev server) |

### Build & CI
| Tool | Version | Purpose |
|---|---|---|
| Wails CLI | latest | Cross-platform build orchestration |
| Node.js | 20 | Frontend build (inspector only) |
| Go | 1.25 | All Go compilation |
| GitHub Actions | — | CI matrix: macOS arm64, Windows amd64, Linux amd64 |
| Bitbucket Pipelines | — | Primary CI trigger; mirrors to GitHub for Actions |

Linux build requires `webkit2gtk-4.1` with the `-tags webkit2_41` build tag and `xvfb-run` for the binding-generation step (GTK needs a display).

---

## Architectural Patterns & Decisions

### 1. Platform Dispatch via Filename Build Constraints
No `//go:build` tags. Platform-specific files use Go's filename suffix convention: `_windows.go`, `_darwin.go`, `_linux.go`. Files without a suffix compile on all platforms.

`main_windows.go` is the most complex entry point — it dispatches based on CLI flags to one of four modes: Wails GUI + systray, Windows SCM service, service install, or service uninstall. `main_darwin.go` runs Wails only. `main_linux.go` runs Wails with an AppIndicator tray.

### 2. Wails JS Binding Namespace
The correct binding path is `window.go.app.App.*` (lowercase `app` = Go package name). `window.go.App.*` silently hangs — calls block forever. All frontend JS uses `const App = window.go.app.App` as a single aliased reference.

### 3. Incremental JSONL Scanner
`internal/scanner` tracks `(path, mtime, bytes_read)` per file. On each tick it re-reads only appended bytes, stopping at partial lines for mid-flush safety. `evictPriorSnapshots` removes older streaming snapshots matching `(session_id, message_id)` before upsert. **`files` table rows are never deleted** — they are the "already scanned" markers; deletion causes re-import of all sessions on next tick.

`attachment`-type records (hook results) are stored via `attachmentPromptText`: hook name + stdout land in `prompt_text` so they appear as clickable rows in the session turn-by-turn view.

### 4. Schema Migrations
Versioned migrations run at startup via a `migrations` slice in `internal/db/db.go`. `schema_version` is stored as a row in the `plan` table. `readSchemaVersion` infers starting version from legacy gate flags so existing databases skip already-applied work. Fresh databases start at `targetSchemaVersion` (migrations never run for them). Multi-step column-drop migrations follow the `migrateDropToolCallsAutoincrement` pattern: inspect `sqlite_master` first and no-op if already applied.

### 5. Null-Safe Query Helpers
`scanMaps` always returns `make([]map[string]any, 0)` — never nil. Wails serializes Go `nil` as JSON `null`, which crashes Vue `v-if` guards. All token columns use `COALESCE(..., 0)` in aggregates.

### 6. Windows Systray Threading
`go a.StartTray()` runs systray in a goroutine (safe because `getlantern/systray` calls `runtime.LockOSThread()` internally). Wails owns the main goroutine (WebView2Loader requirement). `os.Exit(0)` after `wails.Run()` returns to kill the systray goroutine — `systray.Quit()` is never called (deadlocks the Win32 message loop). `HideWindowOnClose: true` keeps the runtime alive so the tray can re-show the window.

### 7. Vue Inspector Build Pipeline
Vite bundles `frontend/inspector/src/` to `frontend/web/app.bundle.js` (IIFE format). `vite.config.ts` must `define: { 'process.env.NODE_ENV': '"production"' }` — Vue's dev-mode checks reference it and the bundle silently fails without this define. Wails lifecycle commands run from `frontend/`, so `wails.json` uses `--prefix inspector` (not `--prefix frontend/inspector`). Generated Wails bindings land in `frontend/web/wailsjs/` (gitignored); regenerate with `wails build` (without `-skipbindings`).

### 8. Pricing Model
`pricing.json` (embedded at build time, overridable via `TOKENTALLY_PRICING_JSON`) has two sections: `models` (exact model name → rates per 1 M tokens) and `plans` (plan key → `{monthly, label}`). Field names are `input`, `output`, `cache_read`, `cache_create_5m`, `cache_create_1h` — not `_mtok` suffixes. Subscription plans (`monthly > 0`) show the flat monthly fee as the headline cost in the Overview frontend with token-equivalent below.

### 9. Context Health (One-Shot Load)
`GetContextHealth()` in `app/app.go` reads `~/.claude/settings.json` (MCP server count, hook count, file size) and `~/.claude/CLAUDE.md` (line count, bullet-rule count, file size). It is intentionally excluded from the range-reactive `fetchAll()` in `OverviewView.vue` — it loads once on mount and only refreshes on explicit user action.

### 10. SQL Safety
Parameter binding always. Any value reachable from user input goes through `?`. Column names and `ORDER BY` direction are interpolated only when they come from internal, caller-controlled values (never from user input).

---

## Directory Layout

```
tokentally/
├── main_*.go              # Platform entry points
├── main_shared.go         # Shared startup helpers
├── app/
│   ├── app.go             # All Wails-bound methods (App struct)
│   ├── tray_*.go          # Systray (Windows/Linux) or no-op (macOS)
│   ├── service_*.go       # Windows SCM helpers or stubs
│   └── platform.go        # Platform capability flags
├── internal/
│   ├── db/                # Schema, migrations, all SQL query helpers
│   ├── scanner/           # Incremental JSONL walker
│   ├── pricing/           # pricing.json loader, CostFor()
│   ├── tips/              # Rule-based tips engine
│   ├── skills/            # Skills data helpers
│   └── version/           # Version string (injected at build via ldflags)
├── svc/                   # Windows SCM service handler (//go:build windows)
├── frontend/
│   ├── inspector/         # Vite + Vue 3 SPA source
│   └── web/               # Embedded runtime assets (vanilla JS SPA + bundle)
├── pricing.json           # Embedded pricing data
└── wails.json             # Wails project config
```

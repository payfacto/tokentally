# TokenTally

<p align="center">
  <img src="assets/banner.png" alt="TokenTally — See it. Spend it. Shrink it." width="680">
</p>

A desktop app for tracking Claude Code token usage, costs, and session history. Reads the JSONL transcripts that Claude Code writes to `~/.claude/projects/` and turns them into a live dashboard — no cloud, no account, no telemetry. Runs on Windows, macOS, and Linux.

---

## Features

### Dashboard tabs

| Tab | What it shows |
| --- | --- |
| **Overview** | Aggregate token usage, cost totals, cache performance, and daily trend chart |
| **Prompts** | Most expensive prompts across all sessions — searchable, sortable, with per-prompt cost breakdown |
| **Sessions** | Session list with turn-by-turn drilldown; hook/attachment rows shown inline |
| **Projects** | Per-project token and cost summaries |
| **Skills** | Breakdown of Claude Code skills invoked across sessions |
| **Tools** | Overage & auth status checker; RTK Token Savings dashboard (see below) |
| **Tips** | Rule-based suggestions: low cache hit rate, high output ratio, many short sessions |
| **Calculator** | Interactive token cost estimator — enter token counts and model to see cost instantly |
| **Settings** | Plan, pricing models, currency, exchange rates, data retention, and Windows service management |

### Cost & pricing

- Per-prompt and aggregate costs using configurable `pricing.json` rates (per 1 M tokens)
- Subscription plan support — Pro / Max show the flat monthly fee as the headline cost with token-equivalent below
- Multi-currency display with live exchange rate refresh
- Fully overridable: edit `pricing.json` in place or point `TOKENTALLY_PRICING_JSON` at your own file

### Cache analytics

- Cache hit rate, 5-minute vs 1-hour cache breakdown, cache creation cost tracking

### RTK Token Savings (Tools tab)

- Runs `rtk gain` and displays a full graphical dashboard: summary stats (commands, input/output/saved tokens, exec time), circular efficiency meter, and a ranked "By Command" table with impact bars
- Detects whether RTK is installed; links to [rtk-ai.app](https://www.rtk-ai.app/) if not

### Data & scanning

- **Incremental JSONL scanner** — tracks `(path, mtime, bytes_read)` per file; only reads new bytes on each tick; safe with mid-flush partial writes
- **30-second background scan loop** — emits a live-refresh event to the UI after each change
- **Data retention** — configurable purge policy; manually trigger via Settings
- **HTML export** — one-click export of the current session to a self-contained HTML report

### Platform integration

| Feature | Windows | macOS | Linux |
| --- | --- | --- | --- |
| Desktop GUI (WebView2 / WebKit) | ✓ | ✓ | ✓ |
| System tray icon (Open, Scan Now, Quit) | ✓ | — | ✓ |
| Background service (SCM / systemd) | ✓ | — | ✓ |
| Startup at login | ✓ | — | ✓ |

---

## Installation

### macOS

Download `TokenTally.app` and open it, or build from source (see below).

### Linux

Download the `tokentally` binary and run it. Install the systemd user service and autostart entry with:

```
./tokentally --install
```

Uninstall with:

```
./tokentally --uninstall
```

### Windows — quick start (no service)

Download `tokentally.exe` and run it. The dashboard opens immediately and scans every 30 seconds while it's open.

### Windows — with background service (recommended)

Run once as administrator to install the Windows service and add the UI to your login startup:

```
tokentally.exe --install
```

The service (`TokenTally`) starts at boot and keeps `tokentally.db` current. The UI auto-starts at login via the Run registry key. Uninstall with:

```
tokentally.exe --uninstall
```

---

## Data

| Item | Windows default | macOS / Linux default |
| --- | --- | --- |
| Database | `%USERPROFILE%\.claude\tokentally.db` | `~/.claude/tokentally.db` |
| Transcripts scanned | `%USERPROFILE%\.claude\projects\` | `~/.claude/projects/` |

Override with environment variables `TOKENTALLY_DB` and `TOKENTALLY_PROJECTS_DIR`.

---

## Customising pricing

Edit `pricing.json` in the same directory as the binary and reload the dashboard, or point `TOKENTALLY_PRICING_JSON` at any JSON file with the same structure. Rates are per 1 M tokens (USD):

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

---

## Building from source

Prerequisites: [Go 1.22+](https://go.dev/dl/), [Node.js 18+](https://nodejs.org/), [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation).

```bash
git clone <repo-url>
cd tokentally

# Install Vue inspector dependencies (first time, or after npm changes)
npm install --prefix frontend/inspector

# Run tests (any platform)
go test ./...

# macOS
wails build -platform darwin/arm64   # Apple Silicon
wails build -platform darwin/amd64   # Intel
open build/bin/TokenTally.app

# macOS — dev mode (live reload)
wails dev

# Windows
wails build -platform windows/amd64

# Windows — faster build (skips binding generation)
wails build -platform windows/amd64 -skipbindings

# Linux
wails build -platform linux/amd64
```

Output: `build/bin/TokenTally.app` (macOS), `build/bin/tokentally.exe` (Windows), or `build/bin/tokentally` (Linux).

> **macOS note:** The system tray and background service are not available on macOS. Closing the window quits the app.

---

## Version info

TokenTally embeds a version string via Go's `-ldflags -X`. The variable lives at `tokentally/internal/version.Version` and defaults to `dev` for plain `go build` / `wails build` invocations.

Check the compiled version:

```bash
tokentally --version
# TokenTally version v1.2.3
```

The version is also shown in the topbar header next to the **TokenTally** brand.

### Building with a version stamp

The `Makefile` derives the version from the nearest git tag:

```bash
make build           # -> build/bin/<binary>, version from `git describe --tags --always --dirty`
make build-windows   # cross-compile windows/amd64
make build-darwin    # cross-compile darwin/arm64
make build-linux     # cross-compile linux/amd64
make test            # go test ./...
make clean           # removes build/bin
make version         # prints the resolved VERSION
```

Override the version explicitly if needed:

```bash
make build VERSION=v1.2.3
```

Equivalent raw `wails build` command (what `make build` runs under the hood):

```bash
wails build -ldflags "-X 'tokentally/internal/version.Version=v1.2.3'"
```

### Cutting a release

Releases are automated end-to-end: Bitbucket is the source of truth; [`bitbucket-pipelines.yml`](bitbucket-pipelines.yml) mirrors `main` and all `v*` tags to a private GitHub mirror; [`.github/workflows/release.yml`](.github/workflows/release.yml) builds Windows, macOS (arm64), and Linux on push, and publishes archives to a GitHub Release on tag push.

Checklist:

1. Make sure `main` is clean and green.

   ```bash
   git checkout main
   git pull
   go test ./...
   ```

2. Pick the next version. We use Semantic Versioning (`vMAJOR.MINOR.PATCH`); the `v` prefix is required.

   ```bash
   git tag --list --sort=-v:refname | head
   ```

3. Create an annotated tag.

   ```bash
   git tag -a v1.2.3 -m "Release v1.2.3"
   ```

4. Push the tag to Bitbucket — the mirror pipeline forwards it to GitHub:

   ```bash
   git push origin v1.2.3
   ```

5. Watch the build in the GitHub mirror's Actions tab. When it goes green a new entry appears under Releases with `tokentally-windows-amd64.zip`, `tokentally-darwin-arm64.zip`, and `tokentally-linux-amd64.tar.gz` attached.

#### Fixing a broken tag

If a release needs redoing for the same version (rare — prefer bumping PATCH):

```bash
git tag -d v1.2.3                   # delete locally
git push --delete origin v1.2.3     # delete on Bitbucket (mirror will not auto-delete on GitHub)
# Manually delete the tag and Release on the GitHub mirror via the UI, then re-tag and re-push.
```

### Version-stamp wiring (how it plumbs through the code)

- [`internal/version/version.go`](internal/version/version.go) declares `var Version = "dev"`.
- Each `main_*.go` parses a `--version` flag and prints `TokenTally version <Version>` before exit.
- [`app/app.go`](app/app.go) exposes `GetVersion()` over the Wails bridge for the frontend.
- [`frontend/inspector/src/App.vue`](frontend/inspector/src/App.vue) renders it in the topbar header.
- The `Makefile` injects the tag value via `-X 'tokentally/internal/version.Version=<tag>'` at build time.

If you rename the variable or move it to another package, update the `Makefile` to match.

---

## Environment variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `TOKENTALLY_DB` | `~/.claude/tokentally.db` | SQLite database path |
| `TOKENTALLY_PROJECTS_DIR` | `~/.claude/projects` | Directory to scan for JSONL files |
| `TOKENTALLY_PRICING_JSON` | *(embedded)* | Path to a custom `pricing.json` |

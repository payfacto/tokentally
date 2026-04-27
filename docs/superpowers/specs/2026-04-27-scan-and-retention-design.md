# Scan Now + Data Retention Design

**Date:** 2026-04-27  
**Status:** Approved

## Problem

The scanner runs every 30 seconds on a fixed tick. If a session was created since the last tick, the turn-by-turn panel shows blank until the next scan fires. There is no way for the user to trigger a scan from the UI (only from the system tray on Windows). There is also no mechanism to prune the TokenTally database as it grows indefinitely.

## Goals

1. Expose "Scan Now" as a clickable button in the Settings UI.
2. Add a configurable data retention policy that auto-prunes on each scan tick and supports an immediate manual purge.

## Non-Goals

- Deleting the JSONL source files from `~/.claude/projects/`.
- A separate "Maintenance" nav tab or richer DB stats view (future scope).
- Retention enforcement at the scanner level (the `files` table handles re-import prevention).

## Approach

Option A: new "Data Management" card in the existing Settings page, above the Windows Service card. The scan loop auto-purges at the end of each tick when retention is configured.

## Backend

### `internal/db` — new helpers

```go
// GetRetentionDays reads plan k='retention_days'; returns 0 if not set (= off).
func GetRetentionDays(conn *sql.DB) (int, error)

// SetRetentionDays upserts plan k='retention_days'.
func SetRetentionDays(conn *sql.DB, days int) error

// PurgeMessages deletes tool_calls then messages where timestamp <
// datetime('now', '-N days'). Returns the number of message rows deleted.
// The files table is left intact so the scanner treats those paths as
// already-processed and does not re-import the pruned data.
func PurgeMessages(conn *sql.DB, days int) (int64, error)
```

Storage: `plan` table (existing k/v store), key `retention_days`, value is the integer as a string. Missing key = 0 = off.

Delete order: `tool_calls` first (references `messages.uuid`), then `messages`. Both filtered by `timestamp < datetime('now', '-N days')`. SQLite's `datetime()` handles the ISO8601 `timestamp` column correctly.

### `app/app.go` — new exported methods

```go
func (a *App) GetRetentionDays() (int, error)
func (a *App) SetRetentionDays(days int) error
// PurgeOlderThan deletes data older than days from the DB and emits no event.
// Returns the number of message rows deleted.
func (a *App) PurgeOlderThan(days int) (int64, error)
```

### `app/app.go` — `scanLoop` change

After each successful scan, if `GetRetentionDays` returns > 0, call `db.PurgeMessages` silently. No Wails event is emitted for auto-purge (background cleanup, no UI feedback needed).

```go
func (a *App) scanLoop() {
    interval := 30 * time.Second
    for {
        result, err := scanner.ScanDir(a.conn, a.projectsDir)
        if err == nil && (result.Messages > 0 || result.Files > 0) {
            runtime.EventsEmit(a.ctx, "scan", result)
        }
        if days, _ := db.GetRetentionDays(a.conn); days > 0 {
            db.PurgeMessages(a.conn, days) //nolint:errcheck
        }
        time.Sleep(interval)
    }
}
```

## Frontend

### Settings page — new "Data Management" card

Inserted between the Exchange Rate API section and the Windows Service card in `frontend/web/routes/settings.js`.

**Scan sub-section:**
- "Scan Now" button → `App.ScanNow()` → inline feedback for 2.5 s:
  - Success with data: `"Scanned 12 messages in 3 files"`
  - Success, nothing new: `"Nothing new"`
  - Error: `"Error: <message>"` in red

**Retention sub-section:**
- Number input, label: "Delete data older than ___ days"
- Placeholder: `e.g. 90`
- Blank or 0 = keep forever (auto-purge disabled)
- "Save" button → `App.SetRetentionDays(days)`
- "Purge Now" button → `App.PurgeOlderThan(days)` → inline feedback:
  - Deleted rows: `"Deleted 1,204 messages"`
  - Nothing to prune: `"Nothing to purge"`
  - Error: `"Error: <message>"` in red
- "Purge Now" is disabled when the input is blank or 0
- Explanatory note: *"Removes messages from TokenTally's database only. Your `~/.claude/projects/` files are not affected and won't be re-imported."*

### Settings page — `renderAll` change

`GetRetentionDays()` added to the existing `Promise.all` at the top of `renderAll`, alongside `GetPlan`, `GetPricingModels`, etc.

## Data flow summary

```
User clicks "Purge Now" (days=90)
  → App.PurgeOlderThan(90)
  → db.PurgeMessages(conn, 90)
  → DELETE FROM tool_calls WHERE timestamp < datetime('now', '-90 days')
  → DELETE FROM messages  WHERE timestamp < datetime('now', '-90 days')
  → files table unchanged — scanner skips those paths on next tick
  → returns deleted message count → shown in UI

scanLoop tick (every 30 s)
  → scanner.ScanDir(...)
  → db.GetRetentionDays → e.g. 90
  → db.PurgeMessages(conn, 90) — silent
  → time.Sleep(30s)
```

## Edge cases

| Scenario | Behaviour |
|---|---|
| Retention = 0 or blank | Auto-purge skipped; "Purge Now" button disabled |
| Purge returns 0 deleted | Show "Nothing to purge" |
| Scanner error on ScanNow | Show error text in red, clear after 2.5 s |
| days=1 (aggressive) | Valid; user accepts risk |
| First run after setting retention | Next scan tick auto-purges; no restart needed |

## Files changed

| File | Change |
|---|---|
| `internal/db/db.go` | Add `GetRetentionDays`, `SetRetentionDays`, `PurgeMessages` |
| `app/app.go` | Add `GetRetentionDays`, `SetRetentionDays`, `PurgeOlderThan`; update `scanLoop` |
| `frontend/web/routes/settings.js` | Add Data Management card; wire Scan Now + Retention controls |

# Findings Subsystem — Design

**Date:** 2026-05-27
**Status:** Approved (brainstorming) — pending implementation plan
**Scope:** v1 of a per-session waste-detection + quality-score reporting layer for TokenTally.

## Goal

Turn TokenTally from "here's what you used" into "here's what was wasteful and how to fix it" — entirely from JSONL data already ingested. No hooks, no interception, no output rewriting. Inspired by the analysis layer of the `token-optimizer` project, reimplemented natively in Go over TokenTally's existing schema.

## Non-goals (explicitly out of scope)

- Anything that intercepts, compresses, or rewrites tool output or context.
- Lifecycle hooks, checkpoint/restore, read-cache, delta diffs.
- Compaction-depth tracking and context-fill *bands as standalone features* (deferred; context-fill is still used as one input to the quality score).
- The `underpowered-model` (Haiku-on-heavy-work) detector — dropped from v1 to keep the rule that every finding carries a token + $ estimate.

## v1 deliverables

1. New package `internal/findings` (pure functions, unit-testable, no DB/Wails imports).
2. Six per-session **detectors**.
3. A per-session **quality score** (0–100 + letter grade), penalty-from-100 model.
4. Two new tables (`findings`, `session_scores`) + a schema migration.
5. Scanner integration: recompute findings/score for each affected session per scan tick.
6. `internal/db` query helpers for aggregation, per-session detail, and Sessions-list badges.
7. `app/app.go` Wails-bound methods.
8. New **Findings tab** (leaderboard layout) + per-session badges on the Sessions tab.

---

## Architecture & data model

### Package `internal/findings`

```go
type Finding struct {
    Kind      string         // "retry-churn", "tool-cascade", ...
    Severity  string         // "high" | "med" | "low"
    SessionID string
    EstTokens int64          // estimated wasted tokens (heuristic)
    Detail    string         // human sentence with the numbers
    Meta      map[string]any // structured evidence (turn ids, counts)
}

type SessionInput struct {
    SessionID    string
    ProjectSlug  string
    LastActivity string        // ISO timestamp; used for range filtering
    Messages     []MessageRow  // ordered by timestamp
    ToolCalls    []ToolCallRow // ordered by timestamp
}

func Detect(in SessionInput) []Finding
func Score(in SessionInput, findings []Finding) (score int, grade string)
```

`MessageRow` / `ToolCallRow` are plain structs mirroring the columns the scanner already holds in memory (no SQL types). The package depends on nothing but the standard library, so detectors and scoring are fully testable with hand-built inputs.

### Tables (new migration)

```sql
CREATE TABLE findings (
  session_id   TEXT NOT NULL,
  project_slug TEXT NOT NULL,
  kind         TEXT NOT NULL,
  severity     TEXT NOT NULL,
  est_tokens   INTEGER NOT NULL DEFAULT 0,
  detail       TEXT,
  meta_json    TEXT,
  timestamp    TEXT NOT NULL,          -- session's last activity, for range filter
  PRIMARY KEY (session_id, kind)
);
CREATE INDEX idx_findings_kind ON findings(kind);
CREATE INDEX idx_findings_ts   ON findings(timestamp);

CREATE TABLE session_scores (
  session_id   TEXT PRIMARY KEY,
  project_slug TEXT NOT NULL,
  score        INTEGER NOT NULL,
  grade        TEXT NOT NULL,
  timestamp    TEXT NOT NULL
);
CREATE INDEX idx_scores_ts ON session_scores(timestamp);
```

Migration: append a `func(tx *sql.Tx) error` to the `migrations` slice in `internal/db/db.go`, bump `targetSchemaVersion`, add a test in `internal/db/migrations_test.go`. Fresh DBs start at the new target (migration body not run).

### Scanner integration

In the scan tick, after messages/tool_calls for the tick are upserted:

1. Collect the set of `session_id`s touched this tick.
2. For each, load its full message + tool_call set from the DB (the session may span many ticks, so derive from stored rows, not just this tick's lines).
3. Run `findings.Detect` + `findings.Score`.
4. Replace cleanly: `DELETE FROM findings WHERE session_id=?` then insert the new rows; upsert `session_scores`.

Clean-replace is safe because findings are *derived* data — unlike the `files` table, which must never be deleted (it is the "already scanned" marker).

### Cost ($) computation

`est_tokens` is stored; the dollar figure is **not**. The query layer multiplies `est_tokens` by the user's current model rate at read time, so the $ always reflects the active plan/pricing. Every estimate is labelled "est." in the UI, with a one-line "how this is estimated" tooltip.

---

## The six detectors

"Simple tools" = {Read, Glob, Grep, Edit, Write}. Thresholds are named constants in `internal/findings`, tunable later. Multipliers (3000/2500/5000) are inherited from token-optimizer heuristics and are intentionally rough — the "est." label covers this.

| Kind | Fires when | Est. wasted tokens | Severity |
|---|---|---|---|
| **retry-churn** | same `(tool_name, target)` invoked ≥3× where a prior call on it had `is_error=1` | `retries × 3000` | high |
| **tool-cascade** | ≥4 consecutive `is_error=1` tool_calls (ordered by timestamp) | `streak × 2500` | high if streak ≥6, else med |
| **looping** | ≥4 consecutive user turns with Jaccard word-overlap > 0.75 | `streak × 5000` | high |
| **output-waste** | across ≥3 simple-tool turns, `Σoutput / Σinput > 3.0` | `Σoutput − Σinput×1.5` (only if > 5000) | med |
| **overpowered-model** | Opus-tier ≥ 50% of session tokens **and** avg output/turn ≤ 5000 **and** simple-tool share ≥ 70% | real $ delta: `opus_tokens` re-priced at Sonnet rate; also expressed in tokens | med |
| **wasteful-thinking** | ≥4 turns where `len(thinking_text)/4 > 4 × output_tokens` | `Σ(thinking_est − output_tokens)` | low |

Detector specifics:

- **retry-churn / tool-cascade** read the `tool_calls` table (`tool_name`, `target`, `is_error`, `timestamp`). `target` is used as the input proxy (TokenTally stores `target`, not full tool input).
- **looping** uses `prompt_text` of `type='user'` messages; word set = `set(lower(text).split())`; Jaccard = `|A∩B| / |A∪B|`; track the longest streak of consecutive comparisons > 0.75.
- **output-waste** defines a "simple-tool turn" as an assistant turn whose tool_calls are all simple tools and ≤2 in count; aggregates output/input across such turns.
- **overpowered-model** maps model → tier via the pricing data; savings = re-pricing the Opus token volume at the Sonnet rate (a real cost delta, also shown as a token figure for the ranked sort).
- **wasteful-thinking** uses the stored `thinking_text` length / 4 as the thinking-token proxy (TokenTally stores the thinking text, not a usage count).

Each `Finding.Detail` is a human sentence, e.g. *"Read on `internal/db/db.go` retried 4× after errors — ~12K tokens."*

---

## Quality score (penalty-from-100)

Built only from signals TokenTally actually has. Start at 100, subtract:

- **Findings penalty** — `min(50, round(100 × totalEstWaste / sessionBillableTokens))`. Ties the score to the findings already on screen.
- **Context-fill penalty** — peak `tokens_before` vs the model's context window: > 80% → −15; 70–80% → −8; 50–70% → −3; else 0. Requires a per-model window size: add an optional `context_window` field to `pricing.json` model entries, default 200000 when absent.
- **Error-rate penalty** — tool `is_error` ratio: > 30% → −10; > 15% → −5.
- **Duplicate-read penalty** — any single file Read ≥3× in the session → −5.

Clamp to [0, 100]. Grade bands: **A ≥90 · B ≥80 · C ≥70 · D ≥60 · F <60**.

Rejected alternative — a weighted composite of normalized signals — was set aside because its weights are arbitrary and the resulting number floats free of the findings list. The penalty model keeps every lost point traceable to something visible.

`token-optimizer`'s "decision density" signal is **not** implemented: TokenTally does not store assistant prose, only `output_tokens`, `thinking_text`, and `tool_calls`.

---

## Query layer (`internal/db`)

All accept `since`/`until` ISO strings, follow existing conventions (parameter binding; `scanMaps` returns `make([]map[string]any, 0)`, never nil).

- `FindingsSummary(p, since, until)` — rolled up by `kind`: total `est_tokens`, session count, max severity. Feeds the leaderboard cards + the top "recoverable" banner (sum across kinds).
- `LowestScoringSessions(p, since, until, limit)` — sessions ordered by ascending score, with project name + grade. Feeds the leaderboard's bottom table.
- `SessionFindings(p, sessionID)` — that session's findings + its score/grade. Feeds the card-expand and the session detail view.
- `FindingsBadges(p, sessionIDs)` — per session: finding count + max severity + grade. Feeds the Sessions-tab row badges.

The $ value is derived in the app layer (or a small helper) from `est_tokens` × current rate; queries return tokens only.

---

## App layer (`app/app.go`)

New Wails-bound methods (range-reactive, included in `fetchAll`):

- `GetFindingsSummary(since, until)` → leaderboard cards + banner total (tokens + computed $).
- `GetLowestScoringSessions(since, until)` → bottom table.
- `GetSessionFindings(sessionID)` → per-session detail (used by Sessions tab + card expand).
- `GetSessionBadges(sessionIDs)` → badges for the Sessions list.

---

## Frontend (Findings tab — leaderboard layout)

New route/view following the existing pattern (`frontend/inspector/src/views/FindingsView.vue` + store wiring; vanilla route module mirrored if needed). Hash route `#/findings`. Tab added to the nav (TokenTally becomes an 8-tab dashboard).

Layout, top to bottom:

1. **Recoverable banner** — big number: total est. recoverable tokens + computed $ + session count, for the selected range.
2. **Ranked finding-kind cards** — one per kind that fired, sorted by total `est_tokens` desc. Each card: severity color bar, kind name, occurrence meta ("7 sessions · same tool retried after errors"), and right-aligned est. tokens + $. Click → expand to the sessions involved (via `GetSessionFindings` per session, or a kind-scoped query).
3. **Lowest-scoring sessions table** — project · session label · grade chip (`F · 52`). Rows link to the session detail.

**Sessions tab:** each row gains a small badge — e.g. `C · 2 findings` — colored by grade, linking to `#/findings` (or scrolling to that session's findings). Fed by `GetSessionBadges`.

Consistency: ECharts not required for v1 (cards + tables suffice); reuse existing range selector and currency formatting; `fmt.htmlSafe()` for any user-derived strings in vanilla innerHTML paths.

---

## Testing

- `internal/findings`: table-driven unit tests per detector (positive + negative + boundary at each threshold) and for `Score` (each penalty in isolation + combined + clamp). Pure functions, hand-built `SessionInput`, no DB.
- `internal/db`: `:memory:` tests for each query helper + the migration test (verify table creation, and that fresh DBs skip the migration body).
- Detector multipliers and thresholds referenced from named constants so tests assert against the constants, not magic numbers.

## Open implementation details (resolve during planning)

- Exact model → tier mapping source for overpowered-model and context-window lookup (reuse `internal/pricing` tier logic).
- Whether card-expand uses a dedicated `FindingsByKind(kind, since, until)` query rather than N× `SessionFindings`.
- Performance of per-tick full-session reload for very large sessions (likely fine; confirm with the largest real session).

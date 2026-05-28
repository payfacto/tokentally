# Findings Subsystem Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-session waste-detection + quality-score reporting layer to TokenTally, surfaced as a new "Findings" leaderboard tab and per-session badges, computed at scan time from data already ingested.

**Architecture:** A new pure `internal/findings` package holds six detectors + a penalty-from-100 scorer (no DB/Wails imports). `internal/db` gains two tables (`findings`, `session_scores`), a loader that runs the detectors over a session and stores the results, and four read helpers. The scanner recomputes findings for each changed session after its walk. `app/app.go` exposes four bound methods; the Vue inspector gets a `FindingsView` and Sessions-tab badges.

**Tech Stack:** Go 1.x (stdlib + `modernc.org/sqlite`), Wails v2, Vue 3 + Pinia + vue-router (inspector SPA).

---

## Plan-specific notes (read first)

- **Plan/spec location:** the repo gitignores `docs/`; this plan lives in `.context/plans/` to match where the approved spec was committed (`.context/specs/2026-05-27-findings-subsystem-design.md`).
- **Migration signature:** the spec said `func(tx *sql.Tx)`; the actual code in `internal/db/db.go` uses `var migrations = []func(*sql.DB) error`. Follow the real code — `func(*sql.DB) error`.
- **Context window:** instead of the spec's `pricing.json context_window` field, `internal/findings` owns a self-contained `ContextWindowFor(model)` helper (default 200000, 1M for `-1m`/`[1m]` variants). Keeps the package dependency-free and avoids threading per-model windows into a session-level score.
- **Dollar figure:** aggregated findings span multiple models, so `GetFindingsSummary` multiplies `est_tokens` by a **blended rate** for the range (`total range cost ÷ total billable tokens`), not a single model rate. Labelled "est." in the UI.
- **Determinism:** `Detect` sorts its combined output (est_tokens desc, then kind, then detail) so stored rows and tests are stable despite Go map iteration order.
- **Billable tokens** throughout = `input + output + cache_create_5m + cache_create_1h` (matches `SearchPrompts`' `billable_tokens`). Cache *reads* are excluded.

## File structure

| File | Responsibility |
|---|---|
| `internal/findings/findings.go` (new) | Shared types (`MessageRow`, `ToolCallRow`, `SessionInput`, `Finding`), kind/severity constants, helpers (`ContextWindowFor`, `humanTokens`, `shortTarget`, `tierOf`), `Detect`, `Score`. |
| `internal/findings/detectors.go` (new) | The six detector functions. |
| `internal/findings/*_test.go` (new) | Table-driven unit tests per detector + scorer. |
| `internal/db/findings.go` (new) | `RecomputeFindings` (load → detect → score → store) + the four read helpers. |
| `internal/db/findings_test.go` (new) | `:memory:` tests for loader + read helpers. |
| `internal/db/db.go` (modify) | Add `findings` + `session_scores` to `schema`; append migration; bump `targetSchemaVersion`. |
| `internal/db/migrations_test.go` (modify) | Migration test. |
| `internal/scanner/scanner.go` (modify) | Collect changed session ids in `ScanDir`; recompute after walk. |
| `app/app.go` (modify) | `GetFindingsSummary`, `GetLowestScoringSessions`, `GetSessionFindings`, `GetSessionBadges` + `blendedRatePerToken` helper. |
| `frontend/inspector/src/lib/api.ts` (modify) | Map four new `/api/findings*` routes. |
| `frontend/inspector/src/router/index.ts` (modify) | Register `/findings`. |
| `frontend/inspector/src/App.vue` (modify) | Add `/findings` to `NAV_ROUTES`. |
| `frontend/inspector/src/views/FindingsView.vue` (new) | Leaderboard: banner + ranked kind cards + lowest-scoring table. |
| `frontend/inspector/src/views/SessionsView.vue` (modify) | Per-row grade/finding badge. |

---

## Task 1: `internal/findings` types, constants, helpers

**Files:**
- Create: `internal/findings/findings.go`
- Test: `internal/findings/findings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package findings

import "testing"

func TestContextWindowFor(t *testing.T) {
	cases := map[string]int64{
		"claude-opus-4-7":      200000,
		"claude-sonnet-4-6":    200000,
		"claude-sonnet-4-6-1m": 1000000,
		"claude-opus-4-7[1m]":  1000000,
		"":                     200000,
	}
	for model, want := range cases {
		if got := ContextWindowFor(model); got != want {
			t.Errorf("ContextWindowFor(%q)=%d, want %d", model, got, want)
		}
	}
}

func TestTierOf(t *testing.T) {
	if tierOf("claude-opus-4-7") != "opus" {
		t.Error("opus not detected")
	}
	if tierOf("gpt-4") != "" {
		t.Error("non-claude should be empty tier")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run 'TestContextWindowFor|TestTierOf' -v`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Write the implementation**

```go
// Package findings detects per-session token-waste patterns and computes a
// per-session quality score. It is pure: no database or Wails dependencies, so
// every detector and the scorer can be unit-tested with hand-built inputs.
package findings

import (
	"fmt"
	"strings"
)

// Kinds.
const (
	KindRetryChurn       = "retry-churn"
	KindToolCascade      = "tool-cascade"
	KindLooping          = "looping"
	KindOutputWaste      = "output-waste"
	KindOverpoweredModel = "overpowered-model"
	KindWastefulThinking = "wasteful-thinking"
)

// Severities.
const (
	SevHigh = "high"
	SevMed  = "med"
	SevLow  = "low"
)

// Detector thresholds and token multipliers. The multipliers are deliberately
// rough heuristics (inherited from the token-optimizer project); the UI labels
// every figure "est.".
const (
	retryChurnMinRetries     = 3
	retryChurnTokensPerRetry = 3000

	toolCascadeMinStreak    = 4
	toolCascadeHighStreak   = 6
	toolCascadeTokensPerErr = 2500

	loopingMinStreak    = 4
	loopingJaccardMin   = 0.75
	loopingTokensPerMsg = 5000

	outputWasteMinSimpleTurns = 3
	outputWasteRatio          = 3.0
	outputWasteMinExcess      = 5000

	overpoweredOpusShareMin   = 0.5
	overpoweredMaxAvgOutput   = 5000
	overpoweredSimpleShareMin = 0.7
	overpoweredSavingsFactor  = 0.6 // Sonnet ~60% cheaper than Opus

	wastefulThinkingRatio    = 4.0
	wastefulThinkingMinTurns = 4

	dupReadMinRepeats = 3

	defaultContextWindow = 200000
	largeContextWindow   = 1000000
	charsPerToken        = 4
)

// simpleTools are low-cost tools; turns using only these are "simple".
var simpleTools = map[string]bool{
	"Read": true, "Glob": true, "Grep": true, "Edit": true, "Write": true,
}

// MessageRow is one transcript message as the detectors need it.
type MessageRow struct {
	UUID         string
	Type         string // "user" | "assistant" | "attachment"
	IsSidechain  bool
	Model        string
	Timestamp    string
	InputTokens  int64
	OutputTokens int64
	CacheCreate  int64 // 5m + 1h combined
	PromptText   string
	ThinkingText string
	TokensBefore int64
	ToolNames    []string // tools invoked by this assistant turn
}

// ToolCallRow is one tool invocation, ordered by Timestamp within a session.
type ToolCallRow struct {
	ToolName  string
	Target    string
	IsError   bool
	Timestamp string
}

// SessionInput is everything the detectors + scorer see for one session.
type SessionInput struct {
	SessionID     string
	ProjectSlug   string
	LastActivity  string
	ContextWindow int64 // set by the loader via ContextWindowFor
	Messages      []MessageRow
	ToolCalls     []ToolCallRow
}

// Finding is one detected waste pattern in one session.
type Finding struct {
	Kind      string
	Severity  string
	SessionID string
	EstTokens int64
	Detail    string
	Meta      map[string]any
}

// ContextWindowFor returns the model's context window in tokens. Defaults to
// 200K; recognises 1M variants by the "1m" marker Claude uses.
func ContextWindowFor(model string) int64 {
	if strings.Contains(strings.ToLower(model), "1m") {
		return largeContextWindow
	}
	return defaultContextWindow
}

// tierOf infers opus/sonnet/haiku from a model name ("" if non-Claude).
func tierOf(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"):
		return "opus"
	case strings.Contains(m, "sonnet"):
		return "sonnet"
	case strings.Contains(m, "haiku"):
		return "haiku"
	}
	return ""
}

// humanTokens renders a token count like "12K" / "1.3M".
func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%dK", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// shortTarget trims a path to its last two segments for readable details.
func shortTarget(t string) string {
	if t == "" {
		return "(no target)"
	}
	parts := strings.Split(t, "/")
	if len(parts) <= 2 {
		return t
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run 'TestContextWindowFor|TestTierOf' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/findings.go internal/findings/findings_test.go
git commit -m "feat(findings): package scaffolding, types, constants, helpers"
```

---

## Task 2: retry-churn detector

**Files:**
- Create: `internal/findings/detectors.go`
- Test: `internal/findings/detectors_test.go`

- [ ] **Step 1: Write the failing test**

```go
package findings

import "testing"

func TestDetectRetryChurn(t *testing.T) {
	in := SessionInput{SessionID: "s1", ToolCalls: []ToolCallRow{
		{ToolName: "Read", Target: "a.go", IsError: true},
		{ToolName: "Read", Target: "a.go", IsError: true},
		{ToolName: "Read", Target: "a.go", IsError: true},
		{ToolName: "Read", Target: "a.go", IsError: false},
	}}
	got := detectRetryChurn(in)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}
	if got[0].Kind != KindRetryChurn || got[0].Severity != SevHigh {
		t.Errorf("wrong kind/severity: %+v", got[0])
	}
	if got[0].EstTokens != 3*retryChurnTokensPerRetry {
		t.Errorf("EstTokens=%d", got[0].EstTokens)
	}
}

func TestDetectRetryChurn_NoErrorNoFinding(t *testing.T) {
	in := SessionInput{ToolCalls: []ToolCallRow{
		{ToolName: "Read", Target: "a.go"}, {ToolName: "Read", Target: "a.go"},
		{ToolName: "Read", Target: "a.go"}, {ToolName: "Read", Target: "a.go"},
	}}
	if got := detectRetryChurn(in); len(got) != 0 {
		t.Errorf("want 0 findings (no errors), got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectRetryChurn -v`
Expected: FAIL — `detectRetryChurn` undefined.

- [ ] **Step 3: Write the implementation** (append to `detectors.go`)

```go
package findings

import (
	"fmt"
	"sort"
	"strings"
)

// detectRetryChurn flags a (tool,target) pair retried after an error.
func detectRetryChurn(in SessionInput) []Finding {
	retries := map[string]int{}
	var lastKey string
	var lastErr bool
	for _, tc := range in.ToolCalls {
		key := tc.ToolName + "\x00" + tc.Target
		if key == lastKey && lastErr {
			retries[key]++
		}
		lastKey, lastErr = key, tc.IsError
	}
	out := make([]Finding, 0)
	for key, n := range retries {
		if n < retryChurnMinRetries {
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		tool, target := parts[0], parts[1]
		est := int64(n) * retryChurnTokensPerRetry
		out = append(out, Finding{
			Kind: KindRetryChurn, Severity: SevHigh, SessionID: in.SessionID,
			EstTokens: est,
			Detail: fmt.Sprintf("%s on %s retried %d× after errors — ~%s tokens.",
				tool, shortTarget(target), n, humanTokens(est)),
			Meta: map[string]any{"tool": tool, "target": target, "retries": n},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Detail < out[j].Detail })
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run TestDetectRetryChurn -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/detectors.go internal/findings/detectors_test.go
git commit -m "feat(findings): retry-churn detector"
```

---

## Task 3: tool-cascade detector

**Files:**
- Modify: `internal/findings/detectors.go`
- Test: `internal/findings/detectors_test.go`

- [ ] **Step 1: Write the failing test** (append)

```go
func TestDetectToolCascade(t *testing.T) {
	tc := func(e bool) ToolCallRow { return ToolCallRow{ToolName: "Bash", IsError: e} }
	in := SessionInput{SessionID: "s1", ToolCalls: []ToolCallRow{
		tc(true), tc(true), tc(true), tc(true), tc(false),
	}}
	got := detectToolCascade(in)
	if len(got) != 1 || got[0].Kind != KindToolCascade {
		t.Fatalf("want 1 cascade finding, got %+v", got)
	}
	if got[0].Severity != SevMed { // streak 4 < high threshold 6
		t.Errorf("want med severity, got %s", got[0].Severity)
	}
	if got[0].EstTokens != 4*toolCascadeTokensPerErr {
		t.Errorf("EstTokens=%d", got[0].EstTokens)
	}
}

func TestDetectToolCascade_ShortStreakIgnored(t *testing.T) {
	tc := func(e bool) ToolCallRow { return ToolCallRow{IsError: e} }
	in := SessionInput{ToolCalls: []ToolCallRow{tc(true), tc(true), tc(true), tc(false)}}
	if got := detectToolCascade(in); len(got) != 0 {
		t.Errorf("streak of 3 should not fire, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectToolCascade -v`
Expected: FAIL — `detectToolCascade` undefined.

- [ ] **Step 3: Write the implementation** (append to `detectors.go`)

```go
// detectToolCascade flags runs of consecutive erroring tool calls.
func detectToolCascade(in SessionInput) []Finding {
	out := make([]Finding, 0)
	streak := 0
	flush := func() {
		if streak < toolCascadeMinStreak {
			return
		}
		sev := SevMed
		if streak >= toolCascadeHighStreak {
			sev = SevHigh
		}
		est := int64(streak) * toolCascadeTokensPerErr
		out = append(out, Finding{
			Kind: KindToolCascade, Severity: sev, SessionID: in.SessionID,
			EstTokens: est,
			Detail: fmt.Sprintf("%d consecutive tool errors — ~%s tokens burned recovering.",
				streak, humanTokens(est)),
			Meta: map[string]any{"streak": streak},
		})
	}
	for _, tcr := range in.ToolCalls {
		if tcr.IsError {
			streak++
			continue
		}
		flush()
		streak = 0
	}
	flush()
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run TestDetectToolCascade -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/detectors.go internal/findings/detectors_test.go
git commit -m "feat(findings): tool-cascade detector"
```

---

## Task 4: looping detector

**Files:**
- Modify: `internal/findings/detectors.go`
- Test: `internal/findings/detectors_test.go`

- [ ] **Step 1: Write the failing test** (append)

```go
func TestDetectLooping(t *testing.T) {
	msg := func(txt string) MessageRow { return MessageRow{Type: "user", PromptText: txt} }
	in := SessionInput{SessionID: "s1", Messages: []MessageRow{
		msg("please fix the failing scanner test now"),
		msg("please fix the failing scanner test now please"),
		msg("please fix the failing scanner test right now"),
		msg("please fix the failing scanner test now again"),
	}}
	got := detectLooping(in)
	if len(got) != 1 || got[0].Kind != KindLooping {
		t.Fatalf("want 1 looping finding, got %+v", got)
	}
}

func TestDetectLooping_DistinctPromptsIgnored(t *testing.T) {
	msg := func(txt string) MessageRow { return MessageRow{Type: "user", PromptText: txt} }
	in := SessionInput{Messages: []MessageRow{
		msg("add a new database migration"),
		msg("now write the frontend view"),
		msg("explain the pricing tier fallback"),
		msg("commit everything and push"),
	}}
	if got := detectLooping(in); len(got) != 0 {
		t.Errorf("distinct prompts should not loop, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectLooping -v`
Expected: FAIL — `detectLooping` undefined.

- [ ] **Step 3: Write the implementation** (append to `detectors.go`)

```go
// wordSet lowercases and splits text into a set of words.
func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		set[w] = true
	}
	return set
}

// jaccard returns |A∩B| / |A∪B| (0 when both empty).
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// detectLooping flags runs of near-identical consecutive user prompts.
func detectLooping(in SessionInput) []Finding {
	var sets []map[string]bool
	for _, m := range in.Messages {
		if m.Type == "user" && !m.IsSidechain && strings.TrimSpace(m.PromptText) != "" {
			sets = append(sets, wordSet(m.PromptText))
		}
	}
	maxStreak, streak := 1, 1
	for i := 1; i < len(sets); i++ {
		if jaccard(sets[i-1], sets[i]) > loopingJaccardMin {
			streak++
			if streak > maxStreak {
				maxStreak = streak
			}
		} else {
			streak = 1
		}
	}
	if maxStreak < loopingMinStreak {
		return make([]Finding, 0)
	}
	est := int64(maxStreak) * loopingTokensPerMsg
	return []Finding{{
		Kind: KindLooping, Severity: SevHigh, SessionID: in.SessionID,
		EstTokens: est,
		Detail: fmt.Sprintf("%d near-identical prompts in a row — ~%s tokens re-spent on the same ask.",
			maxStreak, humanTokens(est)),
		Meta: map[string]any{"streak": maxStreak},
	}}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run TestDetectLooping -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/detectors.go internal/findings/detectors_test.go
git commit -m "feat(findings): looping detector"
```

---

## Task 5: output-waste detector

**Files:**
- Modify: `internal/findings/detectors.go`
- Test: `internal/findings/detectors_test.go`

- [ ] **Step 1: Write the failing test** (append)

```go
func TestDetectOutputWaste(t *testing.T) {
	// 3 simple-tool turns: Σinput=300, Σoutput=2000 → ratio 6.6; excess=2000-450=1550 (<5000, no fire)
	// Scale up so excess > 5000.
	turn := func(in, out int64) MessageRow {
		return MessageRow{Type: "assistant", InputTokens: in, OutputTokens: out, ToolNames: []string{"Read"}}
	}
	in := SessionInput{SessionID: "s1", Messages: []MessageRow{
		turn(1000, 8000), turn(1000, 8000), turn(1000, 8000),
	}}
	got := detectOutputWaste(in)
	if len(got) != 1 || got[0].Kind != KindOutputWaste {
		t.Fatalf("want 1 output-waste finding, got %+v", got)
	}
	// Σout=24000, Σin=3000, excess = 24000 - 4500 = 19500
	if got[0].EstTokens != 19500 {
		t.Errorf("EstTokens=%d want 19500", got[0].EstTokens)
	}
}

func TestDetectOutputWaste_ComplexTurnsIgnored(t *testing.T) {
	turn := MessageRow{Type: "assistant", InputTokens: 1000, OutputTokens: 8000,
		ToolNames: []string{"Bash", "Read", "Edit"}} // 3 tools → not simple
	in := SessionInput{Messages: []MessageRow{turn, turn, turn}}
	if got := detectOutputWaste(in); len(got) != 0 {
		t.Errorf("complex turns should not fire, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectOutputWaste -v`
Expected: FAIL — `detectOutputWaste` undefined.

- [ ] **Step 3: Write the implementation** (append to `detectors.go`)

```go
// isSimpleToolTurn reports whether an assistant turn used only simple tools
// (≤2 of them).
func isSimpleToolTurn(m MessageRow) bool {
	if len(m.ToolNames) == 0 || len(m.ToolNames) > 2 {
		return false
	}
	for _, t := range m.ToolNames {
		if !simpleTools[t] {
			return false
		}
	}
	return true
}

// detectOutputWaste flags disproportionate output on simple-tool turns.
func detectOutputWaste(in SessionInput) []Finding {
	var sumIn, sumOut int64
	var simpleTurns int
	for _, m := range in.Messages {
		if m.Type != "assistant" || !isSimpleToolTurn(m) {
			continue
		}
		simpleTurns++
		sumIn += m.InputTokens
		sumOut += m.OutputTokens
	}
	if simpleTurns < outputWasteMinSimpleTurns || sumIn == 0 {
		return make([]Finding, 0)
	}
	if float64(sumOut)/float64(sumIn) <= outputWasteRatio {
		return make([]Finding, 0)
	}
	excess := sumOut - int64(float64(sumIn)*1.5)
	if excess <= outputWasteMinExcess {
		return make([]Finding, 0)
	}
	return []Finding{{
		Kind: KindOutputWaste, Severity: SevMed, SessionID: in.SessionID,
		EstTokens: excess,
		Detail: fmt.Sprintf("Output ran %.1f× input across %d simple-tool turns — ~%s tokens of avoidable verbosity.",
			float64(sumOut)/float64(sumIn), simpleTurns, humanTokens(excess)),
		Meta: map[string]any{"simple_turns": simpleTurns, "sum_in": sumIn, "sum_out": sumOut},
	}}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run TestDetectOutputWaste -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/detectors.go internal/findings/detectors_test.go
git commit -m "feat(findings): output-waste detector"
```

---

## Task 6: overpowered-model detector

**Files:**
- Modify: `internal/findings/detectors.go`
- Test: `internal/findings/detectors_test.go`

- [ ] **Step 1: Write the failing test** (append)

```go
func TestDetectOverpowered(t *testing.T) {
	// All Opus, low output, all simple tools → fires.
	turn := MessageRow{Type: "assistant", Model: "claude-opus-4-7",
		InputTokens: 1000, OutputTokens: 1000, ToolNames: []string{"Read"}}
	in := SessionInput{SessionID: "s1", Messages: []MessageRow{turn, turn, turn},
		ToolCalls: []ToolCallRow{{ToolName: "Read"}, {ToolName: "Read"}, {ToolName: "Read"}}}
	got := detectOverpowered(in)
	if len(got) != 1 || got[0].Kind != KindOverpoweredModel {
		t.Fatalf("want 1 overpowered finding, got %+v", got)
	}
	// opus billable = 3*(1000+1000) = 6000; est = 6000*0.6 = 3600
	if got[0].EstTokens != 3600 {
		t.Errorf("EstTokens=%d want 3600", got[0].EstTokens)
	}
}

func TestDetectOverpowered_SonnetIgnored(t *testing.T) {
	turn := MessageRow{Type: "assistant", Model: "claude-sonnet-4-6",
		InputTokens: 1000, OutputTokens: 1000, ToolNames: []string{"Read"}}
	in := SessionInput{Messages: []MessageRow{turn, turn, turn},
		ToolCalls: []ToolCallRow{{ToolName: "Read"}}}
	if got := detectOverpowered(in); len(got) != 0 {
		t.Errorf("sonnet should not fire overpowered, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectOverpowered -v`
Expected: FAIL — `detectOverpowered` undefined.

- [ ] **Step 3: Write the implementation** (append to `detectors.go`)

```go
// detectOverpowered flags Opus dominating a session of simple, low-output work.
func detectOverpowered(in SessionInput) []Finding {
	var opusBillable, totalBillable, totalOutput int64
	var assistantTurns int
	for _, m := range in.Messages {
		if m.Type != "assistant" {
			continue
		}
		assistantTurns++
		billable := m.InputTokens + m.OutputTokens + m.CacheCreate
		totalBillable += billable
		totalOutput += m.OutputTokens
		if tierOf(m.Model) == "opus" {
			opusBillable += billable
		}
	}
	if totalBillable == 0 || assistantTurns == 0 {
		return make([]Finding, 0)
	}
	opusShare := float64(opusBillable) / float64(totalBillable)
	avgOutput := float64(totalOutput) / float64(assistantTurns)

	var simpleCalls int
	for _, tc := range in.ToolCalls {
		if simpleTools[tc.ToolName] {
			simpleCalls++
		}
	}
	if len(in.ToolCalls) == 0 {
		return make([]Finding, 0)
	}
	simpleShare := float64(simpleCalls) / float64(len(in.ToolCalls))

	if opusShare < overpoweredOpusShareMin ||
		avgOutput > overpoweredMaxAvgOutput ||
		simpleShare < overpoweredSimpleShareMin {
		return make([]Finding, 0)
	}
	est := int64(float64(opusBillable) * overpoweredSavingsFactor)
	return []Finding{{
		Kind: KindOverpoweredModel, Severity: SevMed, SessionID: in.SessionID,
		EstTokens: est,
		Detail: fmt.Sprintf("Opus drove %.0f%% of simple, low-output work — ~%s tokens' worth is avoidable on Sonnet.",
			opusShare*100, humanTokens(est)),
		Meta: map[string]any{"opus_share": opusShare, "avg_output": avgOutput, "simple_share": simpleShare},
	}}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run TestDetectOverpowered -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/detectors.go internal/findings/detectors_test.go
git commit -m "feat(findings): overpowered-model detector"
```

---

## Task 7: wasteful-thinking detector

**Files:**
- Modify: `internal/findings/detectors.go`
- Test: `internal/findings/detectors_test.go`

- [ ] **Step 1: Write the failing test** (append)

```go
func TestDetectWastefulThinking(t *testing.T) {
	// thinkEst = len/4. Need thinkEst > 4*output on >=4 turns.
	// 4000 chars → 1000 think tokens; output 100 → 1000 > 400. excess = 900.
	think := make([]byte, 4000)
	for i := range think {
		think[i] = 'x'
	}
	turn := MessageRow{Type: "assistant", OutputTokens: 100, ThinkingText: string(think)}
	in := SessionInput{SessionID: "s1", Messages: []MessageRow{turn, turn, turn, turn}}
	got := detectWastefulThinking(in)
	if len(got) != 1 || got[0].Kind != KindWastefulThinking {
		t.Fatalf("want 1 wasteful-thinking finding, got %+v", got)
	}
	if got[0].EstTokens != 4*900 {
		t.Errorf("EstTokens=%d want 3600", got[0].EstTokens)
	}
}

func TestDetectWastefulThinking_FewTurnsIgnored(t *testing.T) {
	think := strings.Repeat("x", 4000)
	turn := MessageRow{Type: "assistant", OutputTokens: 100, ThinkingText: think}
	in := SessionInput{Messages: []MessageRow{turn, turn, turn}} // only 3
	if got := detectWastefulThinking(in); len(got) != 0 {
		t.Errorf("3 turns should not fire, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectWastefulThinking -v`
Expected: FAIL — `detectWastefulThinking` undefined.

- [ ] **Step 3: Write the implementation** (append to `detectors.go`)

```go
// detectWastefulThinking flags turns whose thinking dwarfs their output.
func detectWastefulThinking(in SessionInput) []Finding {
	var wasteTurns int
	var excess int64
	for _, m := range in.Messages {
		if m.Type != "assistant" {
			continue
		}
		thinkEst := int64(len(m.ThinkingText) / charsPerToken)
		if float64(thinkEst) > wastefulThinkingRatio*float64(m.OutputTokens) && thinkEst > m.OutputTokens {
			wasteTurns++
			excess += thinkEst - m.OutputTokens
		}
	}
	if wasteTurns < wastefulThinkingMinTurns {
		return make([]Finding, 0)
	}
	return []Finding{{
		Kind: KindWastefulThinking, Severity: SevLow, SessionID: in.SessionID,
		EstTokens: excess,
		Detail: fmt.Sprintf("%d turns thought far more than they produced — ~%s tokens in oversized reasoning.",
			wasteTurns, humanTokens(excess)),
		Meta: map[string]any{"waste_turns": wasteTurns},
	}}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -run TestDetectWastefulThinking -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/detectors.go internal/findings/detectors_test.go
git commit -m "feat(findings): wasteful-thinking detector"
```

---

## Task 8: `Detect` aggregator

**Files:**
- Modify: `internal/findings/findings.go`
- Test: `internal/findings/findings_test.go`

- [ ] **Step 1: Write the failing test** (append to `findings_test.go`)

```go
func TestDetectSortsByEstTokensDesc(t *testing.T) {
	think := strings.Repeat("x", 4000)
	at := MessageRow{Type: "assistant", OutputTokens: 100, ThinkingText: think}
	in := SessionInput{SessionID: "s1",
		Messages: []MessageRow{at, at, at, at}, // wasteful-thinking (low est)
		ToolCalls: []ToolCallRow{ // retry-churn (higher est)
			{ToolName: "Read", Target: "a.go", IsError: true},
			{ToolName: "Read", Target: "a.go", IsError: true},
			{ToolName: "Read", Target: "a.go", IsError: true},
			{ToolName: "Read", Target: "a.go"},
		},
	}
	got := Detect(in)
	if len(got) < 2 {
		t.Fatalf("expected ≥2 findings, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].EstTokens < got[i].EstTokens {
			t.Errorf("not sorted desc by est tokens: %v", got)
		}
	}
}
```

Add `import "strings"` to the test file if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run TestDetectSortsByEstTokensDesc -v`
Expected: FAIL — `Detect` undefined.

- [ ] **Step 3: Write the implementation** (append to `findings.go`)

```go
import "sort" // add to the existing import block in findings.go

// Detect runs every detector over one session and returns the findings sorted
// by estimated waste (desc), then kind, then detail — deterministic regardless
// of map iteration order.
func Detect(in SessionInput) []Finding {
	var out []Finding
	out = append(out, detectRetryChurn(in)...)
	out = append(out, detectToolCascade(in)...)
	out = append(out, detectLooping(in)...)
	out = append(out, detectOutputWaste(in)...)
	out = append(out, detectOverpowered(in)...)
	out = append(out, detectWastefulThinking(in)...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].EstTokens != out[j].EstTokens {
			return out[i].EstTokens > out[j].EstTokens
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}
```

(Merge the `sort` import into the existing `import (...)` block — don't add a second block.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -v`
Expected: PASS (all findings tests)

- [ ] **Step 5: Commit**

```bash
git add internal/findings/findings.go internal/findings/findings_test.go
git commit -m "feat(findings): Detect aggregator with deterministic ordering"
```

---

## Task 9: `Score` (penalty-from-100)

**Files:**
- Modify: `internal/findings/findings.go`
- Test: `internal/findings/score_test.go`

- [ ] **Step 1: Write the failing test**

```go
package findings

import "testing"

func TestScore_CleanSessionIsA(t *testing.T) {
	in := SessionInput{ContextWindow: 200000, Messages: []MessageRow{
		{Type: "assistant", InputTokens: 1000, OutputTokens: 500, TokensBefore: 20000},
	}}
	score, grade := Score(in, nil)
	if score != 100 || grade != "A" {
		t.Errorf("clean session: score=%d grade=%s", score, grade)
	}
}

func TestScore_FindingsPenaltyCapped(t *testing.T) {
	in := SessionInput{ContextWindow: 200000, Messages: []MessageRow{
		{Type: "assistant", InputTokens: 1000, OutputTokens: 1000}, // billable 2000
	}}
	// Waste >> billable → penalty caps at 50.
	f := []Finding{{Kind: KindLooping, EstTokens: 999999}}
	score, _ := Score(in, f)
	if score != 50 {
		t.Errorf("want capped score 50, got %d", score)
	}
}

func TestScore_ContextFillPenalty(t *testing.T) {
	in := SessionInput{ContextWindow: 200000, Messages: []MessageRow{
		{Type: "assistant", InputTokens: 10, OutputTokens: 10, TokensBefore: 170000}, // 85% → -15
	}}
	score, _ := Score(in, nil)
	if score != 85 {
		t.Errorf("want 85 (context-fill -15), got %d", score)
	}
}

func TestGradeBands(t *testing.T) {
	cases := map[int]string{95: "A", 85: "B", 75: "C", 65: "D", 40: "F"}
	for s, want := range cases {
		if got := grade(s); got != want {
			t.Errorf("grade(%d)=%s want %s", s, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/findings/ -run 'TestScore|TestGradeBands' -v`
Expected: FAIL — `Score`/`grade` undefined.

- [ ] **Step 3: Write the implementation** (append to `findings.go`)

```go
// Score returns a 0–100 quality score and letter grade for a session. It
// starts at 100 and subtracts explainable penalties tied to findings,
// context-fill, tool error rate, and duplicate reads.
func Score(in SessionInput, fs []Finding) (int, string) {
	score := 100.0

	// Findings penalty — capped at 50, scaled by waste vs billable tokens.
	var billable, waste int64
	for _, m := range in.Messages {
		billable += m.InputTokens + m.OutputTokens + m.CacheCreate
	}
	for _, f := range fs {
		waste += f.EstTokens
	}
	if billable > 0 && waste > 0 {
		p := 100.0 * float64(waste) / float64(billable)
		if p > 50 {
			p = 50
		}
		score -= p
	}

	// Context-fill penalty — peak tokens_before vs the model window.
	var peak int64
	for _, m := range in.Messages {
		if m.TokensBefore > peak {
			peak = m.TokensBefore
		}
	}
	window := in.ContextWindow
	if window <= 0 {
		window = defaultContextWindow
	}
	switch fill := float64(peak) / float64(window); {
	case fill > 0.80:
		score -= 15
	case fill > 0.70:
		score -= 8
	case fill > 0.50:
		score -= 3
	}

	// Error-rate penalty.
	if len(in.ToolCalls) > 0 {
		var errs int
		for _, tc := range in.ToolCalls {
			if tc.IsError {
				errs++
			}
		}
		switch rate := float64(errs) / float64(len(in.ToolCalls)); {
		case rate > 0.30:
			score -= 10
		case rate > 0.15:
			score -= 5
		}
	}

	// Duplicate-read penalty.
	reads := map[string]int{}
	for _, tc := range in.ToolCalls {
		if tc.ToolName == "Read" && tc.Target != "" {
			reads[tc.Target]++
		}
	}
	for _, n := range reads {
		if n >= dupReadMinRepeats {
			score -= 5
			break
		}
	}

	if score < 0 {
		score = 0
	}
	s := int(score + 0.5)
	return s, grade(s)
}

func grade(s int) string {
	switch {
	case s >= 90:
		return "A"
	case s >= 80:
		return "B"
	case s >= 70:
		return "C"
	case s >= 60:
		return "D"
	default:
		return "F"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/findings/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/findings/findings.go internal/findings/score_test.go
git commit -m "feat(findings): penalty-from-100 quality score"
```

---

## Task 10: schema tables + migration

**Files:**
- Modify: `internal/db/db.go` (schema const ~line 43–128; migrations slice ~line 252; `targetSchemaVersion` line 248)
- Test: `internal/db/migrations_test.go`

- [ ] **Step 1: Write the failing test** (append to `migrations_test.go`)

```go
func TestFindingsTablesExist(t *testing.T) {
	p, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for _, tbl := range []string{"findings", "session_scores"} {
		var name string
		err := p.Read.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", tbl, err)
		}
	}
}

func TestSchemaVersionAtTarget(t *testing.T) {
	p, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	v, err := SchemaVersion(p)
	if err != nil {
		t.Fatal(err)
	}
	if v != targetSchemaVersion {
		t.Errorf("fresh DB schema_version=%d want %d", v, targetSchemaVersion)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run 'TestFindingsTablesExist|TestSchemaVersionAtTarget' -v`
Expected: FAIL — tables missing (and version mismatch once target is bumped).

- [ ] **Step 3a: Add tables to the `schema` const**

In `internal/db/db.go`, inside the `schema` string (before its closing backtick at ~line 128), append:

```sql
CREATE TABLE IF NOT EXISTS findings (
  session_id   TEXT    NOT NULL,
  project_slug TEXT    NOT NULL,
  kind         TEXT    NOT NULL,
  severity     TEXT    NOT NULL,
  est_tokens   INTEGER NOT NULL DEFAULT 0,
  detail       TEXT,
  meta_json    TEXT,
  timestamp    TEXT    NOT NULL,
  PRIMARY KEY (session_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_findings_kind ON findings(kind);
CREATE INDEX IF NOT EXISTS idx_findings_ts   ON findings(timestamp);

CREATE TABLE IF NOT EXISTS session_scores (
  session_id   TEXT    PRIMARY KEY,
  project_slug TEXT    NOT NULL,
  score        INTEGER NOT NULL,
  grade        TEXT    NOT NULL,
  timestamp    TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_scores_ts ON session_scores(timestamp);
```

- [ ] **Step 3b: Append the migration and bump the target**

Change `const targetSchemaVersion = 4` to `= 5`.

Append `migrateAddFindingsTables` to the `migrations` slice:

```go
var migrations = []func(*sql.DB) error{
	migrateFixUserStringContent,
	migrateFTSBackfill,
	migrateDropToolCallsAutoincrement,
	migrateBackfillMessageCategory,
	migrateAddFindingsTables, // v4→v5
}
```

Add the function (near the other `migrate*` funcs):

```go
// migrateAddFindingsTables (v4→v5) creates the findings + session_scores
// tables on existing databases. The CREATE TABLE IF NOT EXISTS statements in
// the static schema already cover fresh DBs; this re-runs them so upgraded
// DBs get the tables too. Idempotent.
func migrateAddFindingsTables(conn *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS findings (
		  session_id   TEXT NOT NULL, project_slug TEXT NOT NULL,
		  kind TEXT NOT NULL, severity TEXT NOT NULL,
		  est_tokens INTEGER NOT NULL DEFAULT 0, detail TEXT, meta_json TEXT,
		  timestamp TEXT NOT NULL, PRIMARY KEY (session_id, kind))`,
		`CREATE INDEX IF NOT EXISTS idx_findings_kind ON findings(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_findings_ts ON findings(timestamp)`,
		`CREATE TABLE IF NOT EXISTS session_scores (
		  session_id TEXT PRIMARY KEY, project_slug TEXT NOT NULL,
		  score INTEGER NOT NULL, grade TEXT NOT NULL, timestamp TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_scores_ts ON session_scores(timestamp)`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s); err != nil {
			return fmt.Errorf("add findings tables: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run 'TestFindingsTablesExist|TestSchemaVersionAtTarget' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/db.go internal/db/migrations_test.go
git commit -m "feat(db): findings + session_scores tables and v4→v5 migration"
```

---

## Task 11: `RecomputeFindings` loader

**Files:**
- Create: `internal/db/findings.go`
- Test: `internal/db/findings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package db

import "testing"

// seedRetrySession inserts one session whose tool calls trigger retry-churn.
func seedRetrySession(t *testing.T, p *Pool, sid string) {
	t.Helper()
	_, err := p.Write.Exec(
		`INSERT INTO messages (uuid, session_id, project_slug, type, timestamp,
		   input_tokens, output_tokens) VALUES (?,?,?,?,?,?,?)`,
		sid+"-m1", sid, "proj", "assistant", "2026-05-01T10:00:00Z", 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range []int{1, 1, 1, 0} {
		_, err := p.Write.Exec(
			`INSERT INTO tool_calls (message_uuid, session_id, project_slug,
			   tool_name, target, is_error, timestamp) VALUES (?,?,?,?,?,?,?)`,
			sid+"-m1", sid, "proj", "Read", "a.go", e, "2026-05-01T10:00:0"+itoa(i)+"Z")
		if err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(i int) string { return string(rune('0' + i)) }

func TestRecomputeFindings(t *testing.T) {
	p, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	seedRetrySession(t, p, "sess1")

	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}

	var kind string
	var est int64
	err = p.Read.QueryRow(
		`SELECT kind, est_tokens FROM findings WHERE session_id='sess1'`).Scan(&kind, &est)
	if err != nil {
		t.Fatalf("expected a finding row: %v", err)
	}
	if kind != "retry-churn" {
		t.Errorf("kind=%s", kind)
	}
	var score int
	if err := p.Read.QueryRow(
		`SELECT score FROM session_scores WHERE session_id='sess1'`).Scan(&score); err != nil {
		t.Fatalf("expected a score row: %v", err)
	}
	if score >= 100 {
		t.Errorf("score should be penalised, got %d", score)
	}
}

func TestRecomputeFindings_ReplacesPrior(t *testing.T) {
	p, _ := Open(":memory:")
	defer p.Close()
	seedRetrySession(t, p, "sess1")
	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}
	// Re-run must not duplicate (PK is session_id+kind; loader clean-replaces).
	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}
	var n int
	p.Read.QueryRow(`SELECT COUNT(*) FROM findings WHERE session_id='sess1'`).Scan(&n)
	if n != 1 {
		t.Errorf("want 1 finding after re-run, got %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestRecomputeFindings -v`
Expected: FAIL — `RecomputeFindings` undefined.

- [ ] **Step 3: Write the implementation**

```go
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"tokentally/internal/findings"
)

// RecomputeFindings loads a session's messages + tool calls, runs the
// detectors and scorer, then clean-replaces the session's rows in the findings
// and session_scores tables. Findings are derived data, so deleting and
// reinserting is safe (unlike the files table).
func RecomputeFindings(p *Pool, sessionID string) error {
	in, err := loadSessionInput(p, sessionID)
	if err != nil {
		return err
	}
	if len(in.Messages) == 0 && len(in.ToolCalls) == 0 {
		return nil
	}
	fs := findings.Detect(in)
	score, grade := findings.Score(in, fs)

	tx, err := p.Write.Begin()
	if err != nil {
		return fmt.Errorf("RecomputeFindings begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM findings WHERE session_id=?`, sessionID); err != nil {
		return fmt.Errorf("clear findings: %w", err)
	}
	for _, f := range fs {
		metaJSON, _ := json.Marshal(f.Meta)
		if _, err := tx.Exec(
			`INSERT INTO findings (session_id, project_slug, kind, severity,
			   est_tokens, detail, meta_json, timestamp)
			 VALUES (?,?,?,?,?,?,?,?)`,
			sessionID, in.ProjectSlug, f.Kind, f.Severity, f.EstTokens,
			f.Detail, string(metaJSON), in.LastActivity,
		); err != nil {
			return fmt.Errorf("insert finding: %w", err)
		}
	}
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO session_scores
		   (session_id, project_slug, score, grade, timestamp) VALUES (?,?,?,?,?)`,
		sessionID, in.ProjectSlug, score, grade, in.LastActivity,
	); err != nil {
		return fmt.Errorf("upsert score: %w", err)
	}
	return tx.Commit()
}

// loadSessionInput assembles a findings.SessionInput from stored rows.
func loadSessionInput(p *Pool, sessionID string) (findings.SessionInput, error) {
	in := findings.SessionInput{SessionID: sessionID}

	mrows, err := p.Read.Query(
		`SELECT uuid, COALESCE(project_slug,''), type, is_sidechain,
		        COALESCE(model,''), timestamp, input_tokens, output_tokens,
		        cache_create_5m_tokens + cache_create_1h_tokens,
		        COALESCE(prompt_text,''), COALESCE(thinking_text,''),
		        COALESCE(tokens_before,0)
		   FROM messages WHERE session_id=? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return in, fmt.Errorf("loadSessionInput messages: %w", err)
	}
	defer mrows.Close()

	byUUID := map[string]int{} // uuid → index into in.Messages
	var dominantModel string
	modelTokens := map[string]int64{}
	for mrows.Next() {
		var m findings.MessageRow
		var sidechain int
		if err := mrows.Scan(&m.UUID, &in.ProjectSlug, &m.Type, &sidechain,
			&m.Model, &m.Timestamp, &m.InputTokens, &m.OutputTokens, &m.CacheCreate,
			&m.PromptText, &m.ThinkingText, &m.TokensBefore); err != nil {
			return in, fmt.Errorf("scan message: %w", err)
		}
		m.IsSidechain = sidechain != 0
		in.LastActivity = m.Timestamp // rows ascending → last wins
		byUUID[m.UUID] = len(in.Messages)
		in.Messages = append(in.Messages, m)
		if m.Model != "" {
			modelTokens[m.Model] += m.InputTokens + m.OutputTokens + m.CacheCreate
			if modelTokens[m.Model] > modelTokens[dominantModel] {
				dominantModel = m.Model
			}
		}
	}
	if err := mrows.Err(); err != nil {
		return in, err
	}
	in.ContextWindow = findings.ContextWindowFor(dominantModel)

	trows, err := p.Read.Query(
		`SELECT message_uuid, tool_name, COALESCE(target,''), is_error, timestamp
		   FROM tool_calls WHERE session_id=? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return in, fmt.Errorf("loadSessionInput tools: %w", err)
	}
	defer trows.Close()
	for trows.Next() {
		var muid string
		var tc findings.ToolCallRow
		var isErr int
		if err := trows.Scan(&muid, &tc.ToolName, &tc.Target, &isErr, &tc.Timestamp); err != nil {
			return in, fmt.Errorf("scan tool: %w", err)
		}
		tc.IsError = isErr != 0
		in.ToolCalls = append(in.ToolCalls, tc)
		if idx, ok := byUUID[muid]; ok {
			in.Messages[idx].ToolNames = append(in.Messages[idx].ToolNames, tc.ToolName)
		}
	}
	return in, trows.Err()
}

// ensure database/sql stays imported even if the helpers above change.
var _ = sql.ErrNoRows
```

(Remove the trailing `var _ = sql.ErrNoRows` line if `database/sql` is otherwise referenced; it's only there to keep the import valid if you trim code during review.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestRecomputeFindings -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/findings.go internal/db/findings_test.go
git commit -m "feat(db): RecomputeFindings loader (detect + score + store)"
```

---

## Task 12: read helpers

**Files:**
- Modify: `internal/db/findings.go`
- Test: `internal/db/findings_test.go`

- [ ] **Step 1: Write the failing test** (append)

```go
func TestFindingsReadHelpers(t *testing.T) {
	p, _ := Open(":memory:")
	defer p.Close()
	seedRetrySession(t, p, "sess1")
	if err := RecomputeFindings(p, "sess1"); err != nil {
		t.Fatal(err)
	}

	summary, err := FindingsSummary(p, "", "")
	if err != nil || len(summary) == 0 {
		t.Fatalf("summary empty: %v", err)
	}
	if summary[0]["kind"] != "retry-churn" {
		t.Errorf("summary kind=%v", summary[0]["kind"])
	}

	low, err := LowestScoringSessions(p, "", "", 10)
	if err != nil || len(low) == 0 {
		t.Fatalf("low empty: %v", err)
	}

	sf, err := SessionFindings(p, "sess1")
	if err != nil {
		t.Fatal(err)
	}
	if sf["score"] == nil || len(sf["findings"].([]map[string]any)) == 0 {
		t.Errorf("session findings malformed: %+v", sf)
	}

	badges, err := FindingsBadges(p, []string{"sess1", "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if badges["sess1"] == nil {
		t.Errorf("expected badge for sess1")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestFindingsReadHelpers -v`
Expected: FAIL — helpers undefined.

- [ ] **Step 3: Write the implementation** (append to `findings.go`)

```go
// FindingsSummary rolls findings up by kind within the date range, ranked by
// total estimated waste. sev_rank: 3=high, 2=med, 1=low (max within the kind).
func FindingsSummary(p *Pool, since, until string) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT kind,
       SUM(est_tokens) AS est_tokens,
       COUNT(*) AS occurrences,
       COUNT(DISTINCT session_id) AS sessions,
       MAX(CASE severity WHEN 'high' THEN 3 WHEN 'med' THEN 2 ELSE 1 END) AS sev_rank
FROM findings WHERE 1=1` + rng + `
GROUP BY kind ORDER BY est_tokens DESC`
	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("FindingsSummary: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// LowestScoringSessions returns sessions in the range ordered worst-first.
func LowestScoringSessions(p *Pool, since, until string, limit int) ([]map[string]any, error) {
	rng, args := RangeClause(since, until, "timestamp")
	args = append(args, limit)
	q := `
SELECT session_id, project_slug, score, grade
FROM session_scores WHERE 1=1` + rng + `
ORDER BY score ASC, timestamp DESC LIMIT ?`
	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("LowestScoringSessions: %w", err)
	}
	defer rows.Close()
	return scanMaps(rows)
}

// SessionFindings returns one session's findings plus its score/grade.
func SessionFindings(p *Pool, sessionID string) (map[string]any, error) {
	rows, err := p.Read.Query(
		`SELECT kind, severity, est_tokens, detail
		   FROM findings WHERE session_id=? ORDER BY est_tokens DESC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("SessionFindings: %w", err)
	}
	defer rows.Close()
	fs, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"findings": fs, "score": nil, "grade": nil}
	var score int
	var grade string
	err = p.Read.QueryRow(
		`SELECT score, grade FROM session_scores WHERE session_id=?`, sessionID).
		Scan(&score, &grade)
	if err == nil {
		out["score"] = int64(score)
		out["grade"] = grade
	}
	return out, nil
}

// FindingsBadges returns, per requested session id, its finding count, the max
// severity rank, and grade. Sessions with no findings/score are omitted.
func FindingsBadges(p *Pool, sessionIDs []string) (map[string]any, error) {
	out := map[string]any{}
	if len(sessionIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(sessionIDs))
	args := make([]any, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	in := "(" + join(placeholders, ",") + ")"

	q := `
SELECT s.session_id, s.grade, s.score,
       COALESCE(f.cnt,0) AS findings,
       COALESCE(f.sev_rank,0) AS sev_rank
FROM session_scores s
LEFT JOIN (
  SELECT session_id, COUNT(*) AS cnt,
         MAX(CASE severity WHEN 'high' THEN 3 WHEN 'med' THEN 2 ELSE 1 END) AS sev_rank
  FROM findings GROUP BY session_id
) f ON f.session_id = s.session_id
WHERE s.session_id IN ` + in
	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("FindingsBadges: %w", err)
	}
	defer rows.Close()
	maps, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	for _, m := range maps {
		sid, _ := m["session_id"].(string)
		out[sid] = m
	}
	return out, nil
}

// join is a tiny strings.Join wrapper so callers don't import strings here.
func join(parts []string, sep string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += sep
		}
		s += p
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -v -run TestFindings`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/findings.go internal/db/findings_test.go
git commit -m "feat(db): findings read helpers (summary, lowest-scoring, per-session, badges)"
```

---

## Task 13: scanner integration

**Files:**
- Modify: `internal/scanner/scanner.go` (`ScanDir`, ~line 138–188)
- Test: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write the failing test** (append to `scanner_test.go`; reuse existing test helpers for writing a JSONL fixture — follow the pattern already in this file)

```go
func TestScanDirComputesFindings(t *testing.T) {
	dir := t.TempDir()
	p, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// A session.jsonl whose tool results error on the same target repeatedly.
	// Reuse the existing fixture writer in this test file (e.g. writeJSONL);
	// if none exists, write the file with os.WriteFile using the same record
	// shapes the other scanner tests use. The session id is the file stem.
	writeRetryChurnFixture(t, dir, "deadbeef.jsonl")

	if _, err := ScanDir(p, dir); err != nil {
		t.Fatal(err)
	}

	var n int
	p.Read.QueryRow(`SELECT COUNT(*) FROM findings WHERE session_id='deadbeef'`).Scan(&n)
	if n == 0 {
		t.Error("expected ScanDir to populate findings for the session")
	}
}
```

> Implementation note for the engineer: `writeRetryChurnFixture` must emit a JSONL session containing an assistant turn with ≥4 tool_use/tool_result pairs for `Read` on the same target where ≥3 are errors, matching the record shapes the other tests in `scanner_test.go` already construct. Model it on the existing fixtures in that file rather than inventing a new schema.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scanner/ -run TestScanDirComputesFindings -v`
Expected: FAIL — findings count is 0 (recompute not wired yet).

- [ ] **Step 3: Wire recompute into `ScanDir`**

In `internal/scanner/scanner.go`, modify `ScanDir`. Add a changed-session set before the walk, record the session id when a file is scanned, and recompute after the walk:

```go
func ScanDir(p *db.Pool, projectsDir string) (ScanResult, error) {
	var result ScanResult
	changed := map[string]bool{} // session ids touched this tick

	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		// ... unchanged body until after saveFileState succeeds ...

		result.Files++
		result.Messages += sub.messages
		result.Tools += sub.tools
		changed[sessionIDFromPath(path)] = true // ADD THIS LINE
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("ScanDir walk: %w", err)
	}

	// Recompute derived findings for every session whose file changed. Best
	// effort: a failure here must not fail the scan or lose file-state progress.
	for sid := range changed {
		if err := db.RecomputeFindings(p, sid); err != nil {
			log.Printf("findings: recompute %s: %v", sid, err)
		}
	}
	return result, nil
}

// sessionIDFromPath derives the session id from a transcript filename:
// ~/.claude/projects/<slug>/<session>.jsonl → "<session>".
func sessionIDFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}
```

Ensure `log` and `strings` are imported (the file already imports `strings`; add `"log"` if absent).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scanner/ -run TestScanDirComputesFindings -v && go test ./internal/scanner/...`
Expected: PASS (new test + existing scanner suite)

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat(scanner): recompute findings per changed session after each scan"
```

---

## Task 14: app-layer bound methods

**Files:**
- Modify: `app/app.go`
- Test: `app/app_test.go` (follow the existing app-test setup; if app methods aren't unit-tested in this repo, cover the dollar math via a focused test as below)

- [ ] **Step 1: Write the failing test**

```go
func TestBlendedRatePerToken(t *testing.T) {
	// With a known model breakdown + pricing, the blended rate is
	// totalCost / totalBillableTokens. Use the app test harness already used
	// by other app tests to construct an *App with seeded data; if none exists,
	// this can live as a thin test seeding :memory: via db + a default pricing.
	a := newTestApp(t) // existing helper
	seedOneAssistantMessage(t, a, "claude-sonnet-4-6", 1_000_000, 0) // 1M input tokens
	rate := a.blendedRatePerToken("", "")
	if rate <= 0 {
		t.Errorf("blended rate should be positive, got %v", rate)
	}
}
```

> If `app` has no existing test harness, instead add this test in `internal/db` form is not possible (needs pricing). In that case create `app/findings_test.go` with a minimal `*App` built the same way `app.go`'s constructor builds it in other tests. Keep the assertion (`rate > 0`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestBlendedRatePerToken -v`
Expected: FAIL — `blendedRatePerToken` undefined.

- [ ] **Step 3: Write the implementation** (add to `app/app.go`)

```go
// blendedRatePerToken returns the average USD cost per billable token across
// the range, used to translate estimated wasted tokens into dollars for the
// aggregated Findings view (which spans multiple models). Returns 0 when there
// is no costed usage.
func (a *App) blendedRatePerToken(since, until string) float64 {
	models, err := db.ModelBreakdown(a.conn, since, until)
	if err != nil {
		return 0
	}
	var totalCost float64
	var totalTokens int64
	for _, m := range models {
		model, _ := m["model"].(string)
		if c := pricing.CostFor(model, usageFromRow(m), a.getPricing(), a.getPlan()); c != nil {
			totalCost += *c
		}
		totalTokens += asInt64(m["input_tokens"]) + asInt64(m["output_tokens"]) +
			asInt64(m["cache_create_5m_tokens"]) + asInt64(m["cache_create_1h_tokens"])
	}
	if totalTokens == 0 {
		return 0
	}
	return totalCost / float64(totalTokens)
}

// GetFindingsSummary returns findings rolled up by kind, each with an estimated
// dollar cost derived from the range's blended per-token rate.
func (a *App) GetFindingsSummary(since, until string) ([]map[string]any, error) {
	rows, err := db.FindingsSummary(a.conn, since, until)
	if err != nil {
		return nil, err
	}
	rate := a.blendedRatePerToken(since, until)
	for _, r := range rows {
		est := asInt64(r["est_tokens"])
		r["est_cost_usd"] = float64(est) * rate
	}
	return rows, nil
}

// GetLowestScoringSessions returns the worst-scoring sessions in the range.
func (a *App) GetLowestScoringSessions(since, until string) ([]map[string]any, error) {
	return db.LowestScoringSessions(a.conn, since, until, 10)
}

// GetSessionFindings returns one session's findings + score/grade.
func (a *App) GetSessionFindings(sessionID string) (map[string]any, error) {
	return db.SessionFindings(a.conn, sessionID)
}

// GetSessionBadges returns finding-badge data for a set of session ids.
func (a *App) GetSessionBadges(sessionIDs []string) (map[string]any, error) {
	return db.FindingsBadges(a.conn, sessionIDs)
}
```

(Confirm `asInt64`, `usageFromRow`, `a.getPricing()`, `a.getPlan()`, `a.conn` exist — they are used elsewhere in `app.go`. The column names `input_tokens` etc. match `ModelBreakdown`'s output; verify against that function and adjust if it aliases differently.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestBlendedRatePerToken -v && go build ./...`
Expected: PASS + clean build (Wails bindings regenerate on next `wails build`).

- [ ] **Step 5: Commit**

```bash
git add app/app.go app/findings_test.go
git commit -m "feat(app): findings bound methods + blended-rate dollar estimate"
```

---

## Task 15: Findings tab (frontend)

**Files:**
- Modify: `frontend/inspector/src/lib/api.ts` (apiMap)
- Modify: `frontend/inspector/src/router/index.ts`
- Modify: `frontend/inspector/src/App.vue` (`NAV_ROUTES`)
- Create: `frontend/inspector/src/views/FindingsView.vue`

- [ ] **Step 1: Add API routes** in `api.ts`'s `apiMap`:

```ts
  '/api/findings':         (qs) => App().GetFindingsSummary(qs.since || '', qs.until || ''),
  '/api/findings/lowest':  (qs) => App().GetLowestScoringSessions(qs.since || '', qs.until || ''),
```

And handle the per-session route inside `api()` (alongside the `/api/sessions/` branch):

```ts
  if (base.startsWith('/api/findings/session/')) {
    const sid = base.split('/').pop() || ''
    return (await App().GetSessionFindings(decodeURIComponent(sid))) as unknown as T
  }
```

- [ ] **Step 2: Register the route** in `router/index.ts`:

```ts
import FindingsView from '../views/FindingsView.vue'
// ...
    { path: '/findings', component: FindingsView },
```

- [ ] **Step 3: Add to nav** in `App.vue` — insert `/findings` after `/tips`:

```ts
const NAV_ROUTES = ['/overview', '/prompts', '/sessions', '/projects', '/skills', '/tips', '/findings', '/tools', '/compare', '/calculator', '/settings']
```

- [ ] **Step 4: Create `FindingsView.vue`** (leaderboard layout, mirrors `TipsView.vue` conventions — `api`, store, `watch(() => store.lastScan)`, range via `sinceIso`):

```vue
<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { api, RANGES, sinceIso, withSince } from '../lib/api'
import { useAppStore } from '../stores/app'
import * as fmt from '../lib/format' // existing money/number formatter used by other views

const store = useAppStore()

interface KindRow { kind: string; est_tokens: number; occurrences: number; sessions: number; sev_rank: number; est_cost_usd: number }
interface LowRow { session_id: string; project_slug: string; score: number; grade: string }

const range = ref(RANGES[1]) // 30d default
const kinds = ref<KindRow[]>([])
const lowest = ref<LowRow[]>([])

const KIND_LABELS: Record<string, string> = {
  'retry-churn': 'Retry churn',
  'tool-cascade': 'Tool cascade',
  'looping': 'Looping',
  'output-waste': 'Output waste',
  'overpowered-model': 'Overpowered model',
  'wasteful-thinking': 'Wasteful thinking',
}
function sevClass(rank: number) { return rank >= 3 ? 'sev-high' : rank === 2 ? 'sev-med' : 'sev-low' }
function tokens(n: number) {
  if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M'
  if (n >= 1e3) return Math.round(n / 1e3) + 'K'
  return String(n)
}

const totalTokens = computed(() => kinds.value.reduce((s, k) => s + (k.est_tokens || 0), 0))
const totalCost = computed(() => kinds.value.reduce((s, k) => s + (k.est_cost_usd || 0), 0))
const totalSessions = computed(() => new Set<number>() && kinds.value.reduce((m, k) => Math.max(m, k.sessions || 0), 0))

async function fetchAll() {
  const since = sinceIso(range.value)
  kinds.value = (await api<KindRow[]>(withSince('/api/findings', since))) ?? []
  lowest.value = (await api<LowRow[]>(withSince('/api/findings/lowest', since))) ?? []
}

onMounted(fetchAll)
watch(range, fetchAll)
watch(() => store.lastScan, fetchAll)
</script>

<template>
  <div style="padding:20px">
    <div class="range-bar">
      <button v-for="r in RANGES" :key="r.key" :class="{ active: r.key === range.key }" @click="range = r">{{ r.label }}</button>
    </div>

    <div class="card banner" v-if="kinds.length">
      <span class="big">~{{ tokens(totalTokens) }}</span>
      <span class="sub">est. recoverable ·
        <b class="money">{{ fmt.money(totalCost, store.currency, store.exchangeRate) }}</b>
        across {{ totalSessions }} sessions</span>
    </div>

    <div class="card">
      <h2>Findings</h2>
      <p v-if="!kinds.length" class="muted">No findings in this range. Clean sessions, or not enough activity yet.</p>
      <div v-for="k in kinds" :key="k.kind" class="finding-card">
        <div class="bar" :class="sevClass(k.sev_rank)"></div>
        <div class="body">
          <div class="name">{{ KIND_LABELS[k.kind] ?? k.kind }}</div>
          <div class="meta">{{ k.sessions }} session{{ k.sessions === 1 ? '' : 's' }} · {{ k.occurrences }} occurrence{{ k.occurrences === 1 ? '' : 's' }}</div>
        </div>
        <div class="amt">
          <div class="tok">~{{ tokens(k.est_tokens) }}</div>
          <div class="usd">{{ fmt.money(k.est_cost_usd, store.currency, store.exchangeRate) }}</div>
        </div>
      </div>
    </div>

    <div class="card" v-if="lowest.length">
      <h2>Lowest-scoring sessions</h2>
      <table class="data">
        <thead><tr><th>Project</th><th>Session</th><th>Grade</th></tr></thead>
        <tbody>
          <tr v-for="s in lowest" :key="s.session_id">
            <td>{{ s.project_slug }}</td>
            <td><RouterLink :to="`/sessions/${s.session_id}`">{{ s.session_id.slice(0, 8) }}</RouterLink></td>
            <td><span class="grade" :class="`grade-${s.grade}`">{{ s.grade }} · {{ s.score }}</span></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.banner { display:flex; align-items:baseline; gap:14px; }
.banner .big { font-size:26px; font-weight:700; color:var(--accent); }
.banner .sub { color:var(--muted); font-size:13px; }
.banner .money { color:#7fd49a; }
.finding-card { display:flex; align-items:center; gap:12px; padding:10px 0; border-bottom:1px solid var(--border); }
.finding-card .bar { width:4px; align-self:stretch; border-radius:3px; }
.sev-high { background:#e5534b; } .sev-med { background:#e0a23a; } .sev-low { background:#4a90d9; }
.finding-card .name { font-weight:600; }
.finding-card .meta { color:var(--muted); font-size:12px; }
.finding-card .amt { margin-left:auto; text-align:right; }
.finding-card .amt .tok { font-weight:700; color:var(--accent); }
.finding-card .amt .usd { color:#7fd49a; font-size:12px; }
.grade-A { color:#7fd49a; } .grade-B { color:#7fd49a; } .grade-C { color:#e0a23a; } .grade-D { color:#e0a23a; } .grade-F { color:#e5534b; }
</style>
```

> Engineer note: confirm the exact import path/name of the money formatter (`fmt.money`) and the `.range-bar` / `.data` / `.card` / CSS variable names by copying from an existing view (e.g. `OverviewView.vue`, `ProjectsView.vue`). Match what those views use rather than the placeholders above if they differ.

- [ ] **Step 5: Build the bundle and verify it compiles**

Run: `npm run build --prefix frontend/inspector`
Expected: build succeeds, `frontend/web/app.bundle.js` regenerated.

- [ ] **Step 6: Commit**

```bash
git add frontend/inspector/src/lib/api.ts frontend/inspector/src/router/index.ts frontend/inspector/src/App.vue frontend/inspector/src/views/FindingsView.vue frontend/web/app.bundle.js
git commit -m "feat(ui): Findings leaderboard tab"
```

---

## Task 16: Sessions-tab badges

**Files:**
- Modify: `frontend/inspector/src/views/SessionsView.vue`

- [ ] **Step 1: Fetch badges after the session list loads**

In `SessionsView.vue`, after the session list is fetched, collect the ids and call the badge endpoint:

```ts
import { api } from '../lib/api'
const badges = ref<Record<string, { grade: string; score: number; findings: number; sev_rank: number }>>({})

async function fetchBadges(ids: string[]) {
  if (!ids.length) { badges.value = {}; return }
  badges.value = (await App_GetSessionBadges(ids)) ?? {}
}
// Where App_GetSessionBadges is: window.go.app.App.GetSessionBadges(ids)
// (GetSessionBadges takes a []string; pass the array directly.)
```

> Engineer note: the existing `api()` helper is path-based and doesn't carry an array argument cleanly, so call the binding directly here: `await window.go.app.App.GetSessionBadges(ids)`. Match how `SessionsView.vue` already obtains its session list (find the existing fetch and call `fetchBadges(list.map(s => s.session_id))` right after it).

- [ ] **Step 2: Render the badge** on each session row (next to the existing label):

```vue
<span v-if="badges[s.session_id]"
      class="sess-badge"
      :class="`grade-${badges[s.session_id].grade}`">
  {{ badges[s.session_id].grade }} · {{ badges[s.session_id].findings }} finding{{ badges[s.session_id].findings === 1 ? '' : 's' }}
</span>
```

```css
.sess-badge { font-size:11px; padding:1px 7px; border-radius:10px; margin-left:6px; }
.grade-A, .grade-B { color:#7fd49a; } .grade-C, .grade-D { color:#e0a23a; } .grade-F { color:#e5534b; }
```

- [ ] **Step 3: Build and verify**

Run: `npm run build --prefix frontend/inspector`
Expected: build succeeds.

- [ ] **Step 4: Run the full Go suite once more**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/inspector/src/views/SessionsView.vue frontend/web/app.bundle.js
git commit -m "feat(ui): per-session finding/grade badges on Sessions tab"
```

---

## Final verification

- [ ] `go test ./...` — all packages green.
- [ ] `npm run build --prefix frontend/inspector` — bundle builds.
- [ ] `wails build -platform darwin/arm64` (without `-skipbindings`) — regenerates Wails JS bindings for the four new methods, then `wails dev` and click the **Findings** tab to confirm the leaderboard renders and Sessions rows show badges.

---

## Self-review (against the spec)

**Spec coverage:**
- `internal/findings` pure package — Tasks 1–9. ✓
- Six detectors (retry-churn, tool-cascade, looping, output-waste, overpowered-model, wasteful-thinking) — Tasks 2–7. ✓ (underpowered correctly absent)
- Penalty-from-100 score with findings/context-fill/error-rate/dup-read penalties + A–F bands — Task 9. ✓
- `findings` + `session_scores` tables + migration + test — Task 10. ✓
- Scan-time incremental recompute per changed session, clean-replace — Tasks 11, 13. ✓
- Query helpers (summary, lowest-scoring, per-session, badges) — Task 12. ✓
- App bound methods + $ via blended rate (noted deviation from per-model rate, justified) — Task 14. ✓
- Findings leaderboard tab (banner + ranked cards + lowest-scoring table) + Sessions badges — Tasks 15–16. ✓
- "est." labelling, token+$ on every finding — banner/cards show both; UI copy says "est." ✓

**Deviations from spec (intentional, noted in "Plan-specific notes"):** migration signature (`*sql.DB`), `ContextWindowFor` helper instead of `pricing.json context_window`, blended dollar rate instead of single-model rate. None change scope or behaviour the user approved.

**Placeholder scan:** the two "engineer notes" (scanner fixture in Task 13, money-formatter/CSS confirmation in Tasks 15–16) point at existing in-repo patterns to copy rather than leaving logic undefined — acceptable because the exact fixture shape and CSS token names must match existing code the plan can't restate verbatim without reading those files during execution.

**Type consistency:** `MessageRow`/`ToolCallRow`/`SessionInput`/`Finding` field names defined in Task 1 are used unchanged in Tasks 2–9 and 11; `RecomputeFindings`, `FindingsSummary`, `LowestScoringSessions`, `SessionFindings`, `FindingsBadges` names match between db (Tasks 11–12), app (Task 14), and frontend (Tasks 15–16).

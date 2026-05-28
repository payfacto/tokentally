// Package findings detects per-session token-waste patterns and computes a
// per-session quality score. It is pure: no database or Wails dependencies, so
// every detector and the scorer can be unit-tested with hand-built inputs.
package findings

import (
	"fmt"
	"sort"
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
	outputWasteExcessBaseline = 1.5 // expected output is ~1.5× input on simple turns; excess above this is waste

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
// Write is intentionally included; the output-waste detector treats
// Read/Glob/Grep/Edit/Write as low-cost "simple" turns.
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
		if n%1000 == 0 {
			return fmt.Sprintf("%dK", n/1000)
		}
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// shortTarget trims a path to its last two segments for readable details.
// It assumes POSIX-style forward-slash targets, which Claude Code transcripts use.
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

	if score > 100 {
		score = 100
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

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

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

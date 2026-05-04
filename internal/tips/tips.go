package tips

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tokentally/internal/db"
)

const (
	lowCacheHitThreshold     = 0.20
	highOutputRatioThreshold = 0.5
	shortSessionTurnRatio    = 3.0
	manySessionsMin          = 10
	lowReadEditMinEdits      = 20  // minimum edit count before the ratio is meaningful
	lowReadEditRatioCeiling  = 0.3 // reads/edits ratio below which the tip fires
	unusedMCPMinSessions     = 5
	mcpConfiguredCacheTTL    = 30 * time.Second
)

// ConfiguredMCPLoader returns the number of MCP servers in settings.json.
// Override in tests to avoid filesystem access.
var ConfiguredMCPLoader func() int = loadConfiguredMCP

var mcpConfiguredCache struct {
	mu      sync.Mutex
	value   int
	expires time.Time
}

type tip struct {
	Key     string
	Title   string
	Body    string
	Link    string
	Applies func(stats map[string]any) bool
}

var allTipDefs = []tip{
	{
		Key:   "cache-hit-low",
		Title: "Low cache hit rate",
		Body:  "Your cache hit rate is below 20%. Structuring prompts to reuse system prompts can save significant cost.",
		Link:  "https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching",
		Applies: func(s map[string]any) bool {
			read := intVal(s["cache_read_tokens"])
			total := intVal(s["input_tokens"])
			if total == 0 {
				return false
			}
			return float64(read)/float64(total) < lowCacheHitThreshold
		},
	},
	{
		Key:   "high-output-ratio",
		Title: "High output token ratio",
		Body:  "Output tokens are more expensive than input. Consider asking Claude to be more concise.",
		Applies: func(s map[string]any) bool {
			out := intVal(s["output_tokens"])
			inp := intVal(s["input_tokens"])
			if inp == 0 {
				return false
			}
			return float64(out)/float64(inp) > highOutputRatioThreshold
		},
	},
	{
		Key:   "many-sessions",
		Title: "Many short sessions",
		Body:  "You have many sessions with few turns. Longer sessions reuse cached context more efficiently.",
		Applies: func(s map[string]any) bool {
			sessions := intVal(s["sessions"])
			turns := intVal(s["turns"])
			if sessions == 0 {
				return false
			}
			return sessions > manySessionsMin && float64(turns)/float64(sessions) < shortSessionTurnRatio
		},
	},
	{
		Key:   "low-read-edit-ratio",
		Title: "Low read-to-edit ratio",
		Body:  "Claude is editing files much more than it reads them. Reading before editing reduces retries and wasted tokens.",
		Applies: func(s map[string]any) bool {
			edits := intVal(s["edit_calls"])
			reads := intVal(s["read_calls"])
			if edits < lowReadEditMinEdits {
				return false
			}
			return float64(reads)/float64(edits) < lowReadEditRatioCeiling
		},
	},
	{
		Key:   "unused-mcp-servers",
		Title: "MCP servers configured but never called",
		Body:  "You have MCP servers configured in settings.json but none were invoked in recent sessions.",
		Applies: func(s map[string]any) bool {
			configuredMCP := intVal(s["mcp_configured"])
			mcpCalls := intVal(s["mcp_calls"])
			sessions := intVal(s["sessions"])
			return configuredMCP > 0 && mcpCalls == 0 && sessions >= unusedMCPMinSessions
		},
	},
}

func intVal(v any) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

// AllTips returns applicable, non-dismissed tips as maps ready for JSON serialisation.
func AllTips(p *db.Pool) ([]map[string]any, error) {
	dismissed, err := db.DismissedTips(p)
	if err != nil {
		return nil, err
	}
	stats, err := db.OverviewTotals(p, "", "")
	if err != nil {
		return nil, err
	}
	toolCounts, err := db.ToolCallCounts(p, "", "")
	if err != nil {
		return nil, err
	}
	var editCalls, readCalls, mcpCalls int64
	for name, cnt := range toolCounts {
		switch name {
		case "Edit", "Write", "MultiEdit":
			editCalls += cnt
		case "Read":
			readCalls += cnt
		}
		if strings.HasPrefix(name, "mcp__") {
			mcpCalls += cnt
		}
	}
	stats["edit_calls"] = editCalls
	stats["read_calls"] = readCalls
	stats["mcp_calls"] = mcpCalls
	stats["mcp_configured"] = int64(countConfiguredMCP())

	var result []map[string]any
	for _, t := range allTipDefs {
		if dismissed[t.Key] {
			continue
		}
		if !t.Applies(stats) {
			continue
		}
		m := map[string]any{
			"key": t.Key, "title": t.Title, "body": t.Body,
		}
		if t.Link != "" {
			m["link"] = t.Link
		}
		result = append(result, m)
	}
	return result, nil
}

func countConfiguredMCP() int {
	now := time.Now()
	mcpConfiguredCache.mu.Lock()
	if now.Before(mcpConfiguredCache.expires) {
		v := mcpConfiguredCache.value
		mcpConfiguredCache.mu.Unlock()
		return v
	}
	mcpConfiguredCache.mu.Unlock()

	v := ConfiguredMCPLoader()

	mcpConfiguredCache.mu.Lock()
	mcpConfiguredCache.value = v
	mcpConfiguredCache.expires = now.Add(mcpConfiguredCacheTTL)
	mcpConfiguredCache.mu.Unlock()

	return v
}

func loadConfiguredMCP() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return 0
	}
	var raw struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0
	}
	return len(raw.MCPServers)
}

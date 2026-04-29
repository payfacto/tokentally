package tips

import (
	"database/sql"

	"tokentally/internal/db"
)

const (
	lowCacheHitThreshold     = 0.20
	highOutputRatioThreshold = 0.5
	shortSessionTurnRatio    = 3.0
	manySessionsMin          = 10
)

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
func AllTips(conn *sql.DB) ([]map[string]any, error) {
	dismissed, err := db.DismissedTips(conn)
	if err != nil {
		return nil, err
	}
	stats, err := db.OverviewTotals(conn, "", "")
	if err != nil {
		return nil, err
	}
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

package db

import "fmt"

// LmsgoCall is one Bash tool_calls row whose target starts with `lmsgo`.
// Used by the savings estimator to combine the parsed command (input files)
// with the response size already recorded in result_tokens.
type LmsgoCall struct {
	Timestamp    string
	SessionID    string
	ProjectSlug  string
	Target       string // truncated at maxTargetLen chars by the scanner
	ResultTokens int64  // approximate tokens in the lmsgo response
	OutputChars  int64  // length of output_text, source of truth for response size
	IsError      bool
}

// LmsgoCalls returns every Bash invocation in the range whose target starts
// with `lmsgo `. Ordered by timestamp ascending so the caller can show a
// chronological view if it wants.
func LmsgoCalls(p *Pool, since, until string) ([]LmsgoCall, error) {
	rng, args := RangeClause(since, until, "timestamp")
	q := `
SELECT timestamp,
       session_id,
       project_slug,
       target,
       COALESCE(result_tokens, 0) AS result_tokens,
       COALESCE(LENGTH(output_text), 0) AS output_chars,
       is_error
FROM tool_calls
WHERE tool_name = 'Bash'
  AND target LIKE 'lmsgo %'` + rng + `
ORDER BY timestamp ASC`

	rows, err := p.Read.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("LmsgoCalls: %w", err)
	}
	defer rows.Close()

	var out []LmsgoCall
	for rows.Next() {
		var c LmsgoCall
		var isErr int
		if err := rows.Scan(
			&c.Timestamp, &c.SessionID, &c.ProjectSlug,
			&c.Target, &c.ResultTokens, &c.OutputChars, &isErr,
		); err != nil {
			return nil, fmt.Errorf("LmsgoCalls scan: %w", err)
		}
		c.IsError = isErr != 0
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("LmsgoCalls rows: %w", err)
	}
	return out, nil
}

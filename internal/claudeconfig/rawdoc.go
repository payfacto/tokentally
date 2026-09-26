package claudeconfig

import (
	"bytes"
	"encoding/json"
)

// RawDoc is a JSON object decoded key by key. Unknown keys are kept as raw
// bytes rather than dropped, so a write that only touches (say) mcpServers
// round-trips every other top-level field this package doesn't otherwise
// understand - permissions, env, or any future settings.json key.
type RawDoc map[string]json.RawMessage

// ParseRawDoc decodes data into a RawDoc. Empty or nil input yields an empty
// (not nil) doc, matching this package's "absent file is not an error"
// convention for a config file that may not exist yet.
func ParseRawDoc(data []byte) (RawDoc, error) {
	if len(data) == 0 {
		return RawDoc{}, nil
	}
	var d RawDoc
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	if d == nil {
		d = RawDoc{}
	}
	return d, nil
}

// Marshal re-encodes the doc, pretty-printed to match Claude Code's own
// on-disk formatting convention.
func (d RawDoc) Marshal() ([]byte, error) {
	return PrettyJSON(d)
}

// PrettyJSON indent-encodes v without HTML-escaping '<', '>' and '&'.
// json.Marshal/MarshalIndent escape those by default (a html/template-era
// default that doesn't apply here); left on, a hook command containing "&&"
// or a redirect would be silently rewritten to && on every save -
// still valid JSON and lossless to any compliant parser, but a needless,
// confusing diff for a human reading the file or this app's Raw JSON tab.
func PrettyJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encoder.Encode appends a trailing newline that MarshalIndent wouldn't;
	// trim it so callers get the same shape either way.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// getJSON decodes d[key] into out. It reports ok=false (not an error) when
// the key is absent, so callers can treat "missing" the same as "empty".
func getJSON(d RawDoc, key string, out any) (ok bool, err error) {
	raw, present := d[key]
	if !present {
		return false, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false, err
	}
	return true, nil
}

// setJSON encodes value into d[key].
func setJSON(d RawDoc, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	d[key] = raw
	return nil
}

// MCPServer is the shape this package writes for a server being added: the
// fields TokenTally's Add-server form collects. It is NOT used to decode
// existing entries for write-back - see MCPServersRaw for why.
type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Type    string            `json:"type,omitempty"`
}

// MCPServerSummary is a best-effort, display-only decode of one entry.
// Fields this package doesn't model (a remote server's "url"/"headers", or
// "oauth"/"timeout") are simply absent here - MCPServersRaw is what's
// actually written back, so they're never lost.
type MCPServerSummary struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// MCPServersRaw returns the doc's "mcpServers" entries as raw JSON, keyed by
// name. Deliberately not decoded into a fixed struct: a caller that adds or
// deletes one entry only ever touches that one map key and writes every
// other entry back byte-for-byte via SetMCPServersRaw, so a remote server's
// "url"/"headers", an "oauth" block, or any field this package has never
// heard of survives untouched. A doc with no such key returns an empty
// (non-nil) map.
func MCPServersRaw(d RawDoc) (map[string]json.RawMessage, error) {
	servers := map[string]json.RawMessage{}
	if _, err := getJSON(d, "mcpServers", &servers); err != nil {
		return nil, err
	}
	if servers == nil {
		servers = map[string]json.RawMessage{}
	}
	return servers, nil
}

// SetMCPServersRaw replaces the doc's "mcpServers" key wholesale - omitting a
// name that was previously present deletes that server. Pair with
// MCPServersRaw so entries you don't touch keep their exact original bytes.
func SetMCPServersRaw(d RawDoc, servers map[string]json.RawMessage) error {
	return setJSON(d, "mcpServers", servers)
}

// MCPServerSummaries decodes every entry from MCPServersRaw into a
// best-effort MCPServerSummary, for listing in the UI. An entry that isn't a
// JSON object at all is skipped rather than failing the whole list.
func MCPServerSummaries(d RawDoc) (map[string]MCPServerSummary, error) {
	raw, err := MCPServersRaw(d)
	if err != nil {
		return nil, err
	}
	out := make(map[string]MCPServerSummary, len(raw))
	for name, entry := range raw {
		var s MCPServerSummary
		if json.Unmarshal(entry, &s) == nil {
			out[name] = s
		}
	}
	return out, nil
}

// CountHooks counts configured hook *matcher groups* across every event
// (PreToolUse, PostToolUse, etc.) - matching the counting semantics
// GetContextHealth used before this package existed: each event's value only
// needs to be a JSON array to be counted by length, so one event with an
// unexpected shape doesn't zero out the whole count.
func CountHooks(d RawDoc) (int, error) {
	events := map[string]json.RawMessage{}
	if _, err := getJSON(d, "hooks", &events); err != nil {
		// A "hooks" value that isn't even an object: report 0 rather than
		// erroring the whole doc, matching the tolerant read this replaces.
		return 0, nil
	}
	count := 0
	for _, raw := range events {
		var groups []json.RawMessage
		if json.Unmarshal(raw, &groups) == nil {
			count += len(groups)
		}
	}
	return count, nil
}

// ProjectStats summarizes one entry under ~/.claude.json's "projects" map.
type ProjectStats struct {
	Path         string `json:"path"`
	SizeBytes    int    `json:"size_bytes"`
	MessageCount int    `json:"message_count"`
}

type projectEntry struct {
	History []json.RawMessage `json:"history"`
}

// ProjectStatsList returns one ProjectStats per entry under the doc's
// "projects" key. Size is the byte length of that project's own raw JSON
// (matching claude-config-editor's JSON.stringify(data).length), not the
// whole file. A project entry whose "history" field doesn't decode (an
// unexpected shape) still gets a size-only stats row with MessageCount 0,
// rather than failing the whole list.
func ProjectStatsList(d RawDoc) ([]ProjectStats, error) {
	projects := map[string]json.RawMessage{}
	if _, err := getJSON(d, "projects", &projects); err != nil {
		return nil, err
	}
	stats := make([]ProjectStats, 0, len(projects))
	for path, raw := range projects {
		var entry projectEntry
		_ = json.Unmarshal(raw, &entry) // best-effort; entry stays zero-valued on failure
		stats = append(stats, ProjectStats{
			Path:         path,
			SizeBytes:    len(raw),
			MessageCount: len(entry.History),
		})
	}
	return stats, nil
}

// DeleteProjects removes the given paths from the doc's "projects" map.
// Paths not present are silently ignored.
func DeleteProjects(d RawDoc, paths []string) error {
	projects := map[string]json.RawMessage{}
	if _, err := getJSON(d, "projects", &projects); err != nil {
		return err
	}
	for _, p := range paths {
		delete(projects, p)
	}
	return setJSON(d, "projects", projects)
}

// ExportProject returns one project's raw JSON entry, for the "export as
// JSON" action. ok is false if path isn't a configured project.
func ExportProject(d RawDoc, path string) (raw json.RawMessage, ok bool, err error) {
	projects := map[string]json.RawMessage{}
	if _, err := getJSON(d, "projects", &projects); err != nil {
		return nil, false, err
	}
	entry, present := projects[path]
	return entry, present, nil
}

// NumStartups returns the doc's "numStartups" counter, or 0 if absent.
func NumStartups(d RawDoc) (int, error) {
	var n int
	if _, err := getJSON(d, "numStartups", &n); err != nil {
		return 0, err
	}
	return n, nil
}

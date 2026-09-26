package claudeconfig_test

import (
	"encoding/json"
	"strings"
	"testing"

	"tokentally/internal/claudeconfig"
)

func TestParseRawDoc_Empty(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc(nil)
	if err != nil {
		t.Fatalf("ParseRawDoc(nil): %v", err)
	}
	if len(d) != 0 {
		t.Fatalf("expected an empty doc, got %+v", d)
	}

	d2, err := claudeconfig.ParseRawDoc([]byte{})
	if err != nil {
		t.Fatalf("ParseRawDoc([]byte{}): %v", err)
	}
	if len(d2) != 0 {
		t.Fatalf("expected an empty doc, got %+v", d2)
	}
}

func TestRawDoc_MarshalRoundTrip_PreservesUnknownKeys(t *testing.T) {
	input := []byte(`{"mcpServers":{"a":{"command":"foo"}},"someUnknownField":{"x":1},"numStartups":7}`)
	d, err := claudeconfig.ParseRawDoc(input)
	if err != nil {
		t.Fatal(err)
	}

	servers, err := claudeconfig.MCPServersRaw(d)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(claudeconfig.MCPServer{Command: "bar"})
	if err != nil {
		t.Fatal(err)
	}
	servers["b"] = raw
	if err := claudeconfig.SetMCPServersRaw(d, servers); err != nil {
		t.Fatal(err)
	}

	out, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	var roundTripped map[string]json.RawMessage
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if _, ok := roundTripped["someUnknownField"]; !ok {
		t.Fatalf("unknown field was dropped by the round trip: %s", out)
	}
	if _, ok := roundTripped["numStartups"]; !ok {
		t.Fatalf("numStartups was dropped by the round trip: %s", out)
	}

	d2, err := claudeconfig.ParseRawDoc(out)
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := claudeconfig.MCPServerSummaries(d2)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries["a"].Command != "foo" || summaries["b"].Command != "bar" {
		t.Fatalf("mcpServers after round trip = %+v", summaries)
	}
}

// backslashU is the two literal characters backslash + 'u' - spelled via
// concatenation rather than a `&`-style literal so the test file itself
// never contains a real unicode escape sequence for this assertion to
// accidentally match against.
const backslashU = "\\u"

func TestRawDoc_Marshal_DoesNotHTMLEscape(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{"cmd":"npm test 2>&1 && echo ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), backslashU+"0026") || strings.Contains(string(out), backslashU+"003e") {
		t.Fatalf("expected literal &/> in output, got HTML-escaped JSON: %s", out)
	}
	if !strings.Contains(string(out), "2>&1 && echo ok") {
		t.Fatalf("command text was altered: %s", out)
	}
}

func TestMCPServersRaw_AbsentKey_ReturnsEmptyMapNotError(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	servers, err := claudeconfig.MCPServersRaw(d)
	if err != nil {
		t.Fatalf("MCPServersRaw on a doc with no mcpServers key: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected an empty map, got %+v", servers)
	}
}

func TestSetMCPServersRaw_DeleteByOmission(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{"mcpServers":{"a":{"command":"foo"},"b":{"command":"bar"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	servers, err := claudeconfig.MCPServersRaw(d)
	if err != nil {
		t.Fatal(err)
	}
	delete(servers, "b")
	if err := claudeconfig.SetMCPServersRaw(d, servers); err != nil {
		t.Fatal(err)
	}

	got, err := claudeconfig.MCPServerSummaries(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["a"].Command != "foo" {
		t.Fatalf("mcpServers after delete = %+v", got)
	}
}

// TestMCPServersRaw_PreservesUnknownFieldsOnSiblingEntries is the regression
// test for the bug where deleting or adding one MCP server silently dropped
// every other server's fields this package doesn't model (a remote server's
// "url"/"headers", "oauth", "timeout", ...). MCPServersRaw/SetMCPServersRaw
// must round-trip an untouched sibling byte-for-byte.
func TestMCPServersRaw_PreservesUnknownFieldsOnSiblingEntries(t *testing.T) {
	remoteServerJSON := `{"type":"http","url":"https://example.com/mcp","headers":{"Authorization":"Bearer secret-token"},"oauth":{"clientId":"abc"}}`
	input := `{"mcpServers":{"remote":` + remoteServerJSON + `,"local":{"command":"npx","args":["-y","thing"]}}}`
	d, err := claudeconfig.ParseRawDoc([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	// Delete an unrelated entry ("local") - "remote" must survive untouched.
	servers, err := claudeconfig.MCPServersRaw(d)
	if err != nil {
		t.Fatal(err)
	}
	delete(servers, "local")
	if err := claudeconfig.SetMCPServersRaw(d, servers); err != nil {
		t.Fatal(err)
	}

	after, err := claudeconfig.MCPServersRaw(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, stillThere := after["local"]; stillThere {
		t.Fatal("expected /local/ to be deleted")
	}
	remoteAfter, ok := after["remote"]
	if !ok {
		t.Fatal("expected /remote/ to survive an unrelated delete")
	}

	var beforeParsed, afterParsed map[string]any
	if err := json.Unmarshal([]byte(remoteServerJSON), &beforeParsed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(remoteAfter, &afterParsed); err != nil {
		t.Fatal(err)
	}
	if afterParsed["url"] != beforeParsed["url"] {
		t.Fatalf("remote server's url field was lost: %+v", afterParsed)
	}
	headersAfter, _ := afterParsed["headers"].(map[string]any)
	if headersAfter["Authorization"] != "Bearer secret-token" {
		t.Fatalf("remote server's headers field was lost: %+v", afterParsed)
	}
	if _, ok := afterParsed["oauth"]; !ok {
		t.Fatalf("remote server's oauth field was lost: %+v", afterParsed)
	}
}

func TestMCPServerSummaries_SkipsAnEntryThatIsNotAnObject(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{"mcpServers":{"good":{"command":"foo"},"bad":"not-an-object"}}`))
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := claudeconfig.MCPServerSummaries(d)
	if err != nil {
		t.Fatalf("MCPServerSummaries should tolerate one malformed entry: %v", err)
	}
	if len(summaries) != 1 || summaries["good"].Command != "foo" {
		t.Fatalf("summaries = %+v, want only /good/", summaries)
	}
}

func TestCountHooks_MatchesArrayLengthShape(t *testing.T) {
	data := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command"}]},{"matcher":"Edit","hooks":[{"type":"command"},{"type":"command"}]}],"PostToolUse":[{"matcher":"*","hooks":[{"type":"command"}]}]}}`)
	d, err := claudeconfig.ParseRawDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	n, err := claudeconfig.CountHooks(d)
	if err != nil {
		t.Fatal(err)
	}
	// Counts matcher-group entries (2 under PreToolUse + 1 under
	// PostToolUse = 3), not individual inner hook commands.
	if n != 3 {
		t.Fatalf("CountHooks = %d, want 3", n)
	}
}

func TestCountHooks_AbsentKey_ReturnsZero(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	n, err := claudeconfig.CountHooks(d)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("CountHooks on empty doc = %d, want 0", n)
	}
}

// TestCountHooks_TolerantOfOneMalformedEvent guards the regression the
// code review flagged: one event whose value isn't an array must not zero
// out the count for every other, well-formed event.
func TestCountHooks_TolerantOfOneMalformedEvent(t *testing.T) {
	data := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[]}],"Weird":{"not":"an array"}}}`)
	d, err := claudeconfig.ParseRawDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	n, err := claudeconfig.CountHooks(d)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("CountHooks = %d, want 1 (the malformed event should be skipped, not fail the count)", n)
	}
}

func TestCountHooks_TolerantOfNonObjectHooksValue(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{"hooks":null}`))
	if err != nil {
		t.Fatal(err)
	}
	n, err := claudeconfig.CountHooks(d)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("CountHooks = %d, want 0 for a non-object hooks value", n)
	}
}

func TestProjectStatsList(t *testing.T) {
	data := []byte(`{"projects":{"/a":{"history":[{"m":1},{"m":2}]},"/b":{"history":[]}}}`)
	d, err := claudeconfig.ParseRawDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := claudeconfig.ProjectStatsList(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 projects, got %d: %+v", len(stats), stats)
	}
	byPath := map[string]claudeconfig.ProjectStats{}
	for _, s := range stats {
		byPath[s.Path] = s
	}
	if byPath["/a"].MessageCount != 2 {
		t.Fatalf("/a MessageCount = %d, want 2", byPath["/a"].MessageCount)
	}
	if byPath["/a"].SizeBytes <= 0 {
		t.Fatalf("/a SizeBytes = %d, want > 0", byPath["/a"].SizeBytes)
	}
	if byPath["/b"].MessageCount != 0 {
		t.Fatalf("/b MessageCount = %d, want 0", byPath["/b"].MessageCount)
	}
}

// TestProjectStatsList_TolerantOfOneMalformedEntry guards the same kind of
// regression as CountHooks: one project whose "history" field is an
// unexpected shape must not fail the whole list.
func TestProjectStatsList_TolerantOfOneMalformedEntry(t *testing.T) {
	data := []byte(`{"projects":{"/a":{"history":[{"m":1}]},"/weird":{"history":"not-an-array"}}}`)
	d, err := claudeconfig.ParseRawDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := claudeconfig.ProjectStatsList(d)
	if err != nil {
		t.Fatalf("ProjectStatsList should tolerate one malformed entry: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected both projects listed even though /weird is malformed, got %+v", stats)
	}
}

func TestDeleteProjects_RemovesOnlyGivenPaths(t *testing.T) {
	data := []byte(`{"projects":{"/a":{"history":[]},"/b":{"history":[]},"/c":{"history":[]}}}`)
	d, err := claudeconfig.ParseRawDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := claudeconfig.DeleteProjects(d, []string{"/a", "/c"}); err != nil {
		t.Fatal(err)
	}
	stats, err := claudeconfig.ProjectStatsList(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Path != "/b" {
		t.Fatalf("after delete = %+v, want only /b remaining", stats)
	}
}

func TestExportProject(t *testing.T) {
	data := []byte(`{"projects":{"/a":{"history":[{"m":1}]}}}`)
	d, err := claudeconfig.ParseRawDoc(data)
	if err != nil {
		t.Fatal(err)
	}
	raw, ok, err := claudeconfig.ExportProject(d, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected ok=true for an existing project")
	}
	var parsed struct {
		History []json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.History) != 1 {
		t.Fatalf("exported history len = %d, want 1", len(parsed.History))
	}

	_, ok, err = claudeconfig.ExportProject(d, "/missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected ok=false for a missing project")
	}
}

func TestNumStartups(t *testing.T) {
	d, err := claudeconfig.ParseRawDoc([]byte(`{"numStartups":42}`))
	if err != nil {
		t.Fatal(err)
	}
	n, err := claudeconfig.NumStartups(d)
	if err != nil {
		t.Fatal(err)
	}
	if n != 42 {
		t.Fatalf("NumStartups = %d, want 42", n)
	}

	d2, err := claudeconfig.ParseRawDoc([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	n2, err := claudeconfig.NumStartups(d2)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("NumStartups on empty doc = %d, want 0", n2)
	}
}

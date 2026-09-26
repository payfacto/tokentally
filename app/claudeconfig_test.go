package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tokentally/internal/claudeconfig"
)

// newTestAppWithHome returns a newTestApp whose claudeconfigStore() is rooted
// at a fresh, empty temp directory - no real ~/.claude.json/settings.json/
// CLAUDE.md is ever touched by these tests.
func newTestAppWithHome(t *testing.T) *App {
	t.Helper()
	a := newTestApp(t)
	a.testHomeDir = t.TempDir()
	return a
}

func writeHomeFile(t *testing.T, a *App, rel, content string) {
	t.Helper()
	path := filepath.Join(a.testHomeDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- ~/.claude.json: overview + project history ---

func TestGetClaudeJSONOverview(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{
		"projects": {"/a": {"history":[{}]}, "/b": {"history":[]}},
		"mcpServers": {"x": {"command":"foo"}},
		"numStartups": 5
	}`)

	overview, err := a.GetClaudeJSONOverview()
	if err != nil {
		t.Fatalf("GetClaudeJSONOverview: %v", err)
	}
	if overview["project_count"] != 2 {
		t.Errorf("project_count = %v, want 2", overview["project_count"])
	}
	if overview["mcp_count"] != 1 {
		t.Errorf("mcp_count = %v, want 1", overview["mcp_count"])
	}
	if overview["num_startups"] != 5 {
		t.Errorf("num_startups = %v, want 5", overview["num_startups"])
	}
	if overview["version"] == "" || overview["version"] == nil {
		t.Error("expected a non-empty version token")
	}
	if overview["health"] != "healthy" {
		t.Errorf("health = %v, want healthy for a small fixture", overview["health"])
	}
}

func TestGetClaudeJSONOverview_MissingFile(t *testing.T) {
	a := newTestAppWithHome(t)
	overview, err := a.GetClaudeJSONOverview()
	if err != nil {
		t.Fatalf("GetClaudeJSONOverview: %v", err)
	}
	if overview["project_count"] != 0 {
		t.Errorf("project_count = %v, want 0 for a missing file", overview["project_count"])
	}
}

func TestListClaudeJSONProjects(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{"projects":{"/a":{"history":[{},{}]},"/b":{"history":[]}}}`)

	result, err := a.ListClaudeJSONProjects()
	if err != nil {
		t.Fatalf("ListClaudeJSONProjects: %v", err)
	}
	projects, ok := result["projects"].([]map[string]any)
	if !ok || len(projects) != 2 {
		t.Fatalf("projects = %#v", result["projects"])
	}
}

func TestExportClaudeJSONProject(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{"projects":{"/a":{"history":[{"m":1}]}}}`)

	out, err := a.ExportClaudeJSONProject("/a")
	if err != nil {
		t.Fatalf("ExportClaudeJSONProject: %v", err)
	}
	content, _ := out["content"].(string)
	var parsed struct {
		History []json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("exported content is not valid JSON: %v (%s)", err, content)
	}
	if len(parsed.History) != 1 {
		t.Fatalf("exported history len = %d, want 1", len(parsed.History))
	}

	if _, err := a.ExportClaudeJSONProject("/missing"); err == nil {
		t.Fatal("expected an error for a project that does not exist")
	}
}

func TestDeleteClaudeJSONProjects(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{"projects":{"/a":{"history":[]},"/b":{"history":[]}}}`)

	overview, err := a.GetClaudeJSONOverview()
	if err != nil {
		t.Fatal(err)
	}
	version, _ := overview["version"].(string)

	if err := a.DeleteClaudeJSONProjects([]string{"/a"}, version); err != nil {
		t.Fatalf("DeleteClaudeJSONProjects: %v", err)
	}

	result, err := a.ListClaudeJSONProjects()
	if err != nil {
		t.Fatal(err)
	}
	projects, _ := result["projects"].([]map[string]any)
	if len(projects) != 1 || projects[0]["path"] != "/b" {
		t.Fatalf("projects after delete = %+v", projects)
	}

	// A backup of the pre-delete file must exist.
	entries, err := os.ReadDir(filepath.Join(a.testHomeDir, ".claude", "backups"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly 1 backup, err=%v entries=%v", err, entries)
	}
}

func TestDeleteClaudeJSONProjects_RejectsStaleVersion(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{"projects":{"/a":{"history":[]}}}`)

	err := a.DeleteClaudeJSONProjects([]string{"/a"}, "stale-version")
	if err != claudeconfig.ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	result, _ := a.ListClaudeJSONProjects()
	projects, _ := result["projects"].([]map[string]any)
	if len(projects) != 1 {
		t.Fatalf("project was deleted despite a version conflict: %+v", projects)
	}
}

func TestGetClaudeJSONRaw(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{"a":1}`)

	out, err := a.GetClaudeJSONRaw()
	if err != nil {
		t.Fatalf("GetClaudeJSONRaw: %v", err)
	}
	content, _ := out["content"].(string)
	var m map[string]any
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		t.Fatalf("raw content not valid JSON: %v", err)
	}
	if m["a"] != float64(1) {
		t.Fatalf("raw content = %v", m)
	}
}

// --- ~/.claude.json MCP servers ---

func TestClaudeJSONMCPServers_AddListDelete(t *testing.T) {
	a := newTestAppWithHome(t)

	// Add against a file that doesn't exist yet.
	overview, err := a.GetClaudeJSONOverview()
	if err != nil {
		t.Fatal(err)
	}
	v0, _ := overview["version"].(string)

	if err := a.AddClaudeJSONMCPServer("my-server", "npx", []string{"-y", "thing"}, nil, v0); err != nil {
		t.Fatalf("AddClaudeJSONMCPServer: %v", err)
	}

	listed, err := a.ListClaudeJSONMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := listed["servers"].([]map[string]any)
	if len(servers) != 1 || servers[0]["name"] != "my-server" || servers[0]["command"] != "npx" {
		t.Fatalf("servers after add = %+v", servers)
	}
	v1, _ := listed["version"].(string)
	if v1 == v0 {
		t.Fatal("version should change after a write")
	}

	if err := a.DeleteClaudeJSONMCPServer("my-server", v1); err != nil {
		t.Fatalf("DeleteClaudeJSONMCPServer: %v", err)
	}
	listed2, err := a.ListClaudeJSONMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	servers2, _ := listed2["servers"].([]map[string]any)
	if len(servers2) != 0 {
		t.Fatalf("expected no servers after delete, got %+v", servers2)
	}
}

func TestAddClaudeJSONMCPServer_RejectsEmptyNameOrCommand(t *testing.T) {
	a := newTestAppWithHome(t)
	if err := a.AddClaudeJSONMCPServer("", "npx", nil, nil, ""); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := a.AddClaudeJSONMCPServer("name", "", nil, nil, ""); err == nil {
		t.Fatal("expected error for empty command")
	}
}

// --- ~/.claude/settings.json MCP servers - independent of ~/.claude.json ---

func TestSettingsMCPServers_IndependentFromClaudeJSON(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude.json", `{"mcpServers":{"claude-json-server":{"command":"a"}}}`)
	writeHomeFile(t, a, ".claude/settings.json", `{"mcpServers":{"settings-server":{"command":"b"}}}`)

	claudeJSONServers, err := a.ListClaudeJSONMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	settingsServers, err := a.ListSettingsMCPServers()
	if err != nil {
		t.Fatal(err)
	}

	cj, _ := claudeJSONServers["servers"].([]map[string]any)
	st, _ := settingsServers["servers"].([]map[string]any)
	if len(cj) != 1 || cj[0]["name"] != "claude-json-server" {
		t.Fatalf("claude.json servers = %+v", cj)
	}
	if len(st) != 1 || st[0]["name"] != "settings-server" {
		t.Fatalf("settings.json servers = %+v", st)
	}
}

func TestSettingsMCPServers_AddDelete(t *testing.T) {
	a := newTestAppWithHome(t)
	listed, err := a.ListSettingsMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	v0, _ := listed["version"].(string)

	if err := a.AddSettingsMCPServer("s1", "node", []string{"server.js"}, map[string]string{"KEY": "val"}, v0); err != nil {
		t.Fatalf("AddSettingsMCPServer: %v", err)
	}
	listed2, err := a.ListSettingsMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := listed2["servers"].([]map[string]any)
	if len(servers) != 1 || servers[0]["name"] != "s1" {
		t.Fatalf("servers = %+v", servers)
	}
	v1, _ := listed2["version"].(string)

	if err := a.DeleteSettingsMCPServer("s1", v1); err != nil {
		t.Fatalf("DeleteSettingsMCPServer: %v", err)
	}
	listed3, _ := a.ListSettingsMCPServers()
	servers3, _ := listed3["servers"].([]map[string]any)
	if len(servers3) != 0 {
		t.Fatalf("expected no servers after delete, got %+v", servers3)
	}
}

// --- ~/.claude/settings.json hooks (raw JSON surface) ---

func TestSettingsHooksRaw_GetSave(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude/settings.json", `{"mcpServers":{"keep":{"command":"x"}},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command"}]}]}}`)

	got, err := a.GetSettingsHooksRaw()
	if err != nil {
		t.Fatalf("GetSettingsHooksRaw: %v", err)
	}
	content, _ := got["content"].(string)
	var hooks map[string]any
	if err := json.Unmarshal([]byte(content), &hooks); err != nil {
		t.Fatalf("hooks content not valid JSON: %v", err)
	}
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Fatalf("expected PreToolUse in hooks content: %s", content)
	}
	version, _ := got["version"].(string)

	newHooks := `{"PostToolUse":[{"matcher":"*","hooks":[{"type":"command"}]}]}`
	if err := a.SaveSettingsHooksRaw(newHooks, version); err != nil {
		t.Fatalf("SaveSettingsHooksRaw: %v", err)
	}

	// mcpServers (an unrelated key) must survive the hooks-only save untouched.
	servers, err := a.ListSettingsMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	list, _ := servers["servers"].([]map[string]any)
	if len(list) != 1 || list[0]["name"] != "keep" {
		t.Fatalf("mcpServers was clobbered by a hooks-only save: %+v", list)
	}

	got2, err := a.GetSettingsHooksRaw()
	if err != nil {
		t.Fatal(err)
	}
	content2, _ := got2["content"].(string)
	var hooks2 map[string]any
	if err := json.Unmarshal([]byte(content2), &hooks2); err != nil {
		t.Fatal(err)
	}
	if _, ok := hooks2["PostToolUse"]; !ok {
		t.Fatalf("hooks content after save = %s, want PostToolUse", content2)
	}
	if _, ok := hooks2["PreToolUse"]; ok {
		t.Fatalf("hooks content after save still has the old PreToolUse: %s", content2)
	}
}

func TestSaveSettingsHooksRaw_RejectsInvalidJSON(t *testing.T) {
	a := newTestAppWithHome(t)
	got, err := a.GetSettingsHooksRaw()
	if err != nil {
		t.Fatal(err)
	}
	version, _ := got["version"].(string)

	if err := a.SaveSettingsHooksRaw("{not valid json", version); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

// --- ~/.claude/CLAUDE.md ---

func TestClaudeMdContent_GetSave(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude/CLAUDE.md", "# Hello\n")

	got, err := a.GetClaudeMdContent()
	if err != nil {
		t.Fatalf("GetClaudeMdContent: %v", err)
	}
	if got["content"] != "# Hello\n" {
		t.Fatalf("content = %v", got["content"])
	}
	version, _ := got["version"].(string)

	if err := a.SaveClaudeMdContent("# Updated\n", version); err != nil {
		t.Fatalf("SaveClaudeMdContent: %v", err)
	}
	got2, err := a.GetClaudeMdContent()
	if err != nil {
		t.Fatal(err)
	}
	if got2["content"] != "# Updated\n" {
		t.Fatalf("content after save = %v", got2["content"])
	}
}

// --- claudeconfigStore's production fallback (os.UserHomeDir(), not testHomeDir) ---

// TestClaudeconfigStore_FallsBackToRealHomeDir exercises the code path every
// other test in this file bypasses by setting testHomeDir directly: with
// testHomeDir left empty, claudeconfigStore() must resolve os.UserHomeDir().
// t.Setenv("HOME", ...) redirects that call to a scratch directory, so this
// still never touches the real user's files.
func TestClaudeconfigStore_FallsBackToRealHomeDir(t *testing.T) {
	scratchHome := t.TempDir()
	t.Setenv("HOME", scratchHome)

	a := newTestApp(t) // testHomeDir left at its zero value ("")
	writeHomeFile(t, &App{testHomeDir: scratchHome}, ".claude.json", `{"projects":{"/a":{"history":[]}}}`)

	overview, err := a.GetClaudeJSONOverview()
	if err != nil {
		t.Fatalf("GetClaudeJSONOverview via real os.UserHomeDir(): %v", err)
	}
	if overview["project_count"] != 1 {
		t.Fatalf("project_count = %v, want 1 (fixture written under $HOME=%s)", overview["project_count"], scratchHome)
	}
}

func TestSaveClaudeMdContent_RejectsStaleVersion(t *testing.T) {
	a := newTestAppWithHome(t)
	writeHomeFile(t, a, ".claude/CLAUDE.md", "original")

	err := a.SaveClaudeMdContent("clobber", "stale")
	if err != claudeconfig.ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	got, _ := a.GetClaudeMdContent()
	if got["content"] != "original" {
		t.Fatalf("content = %v, want unchanged", got["content"])
	}
}

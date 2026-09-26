package app

import (
	"encoding/json"
	"fmt"

	"tokentally/internal/claudeconfig"
)

// Overview health thresholds, ported from claude-config-editor's "Quick
// Analysis" heuristic: healthy below the low bounds, a warning at or above
// the high bounds, moderate in between.
const (
	healthyMaxBytes    = 1 * 1024 * 1024
	healthyMaxProjects = 10
	warningMinBytes    = 5 * 1024 * 1024
	warningMinProjects = 20
)

// mcpServerList converts a name->summary map into the []map[string]any shape
// the frontend renders, with name folded in as a field.
func mcpServerList(servers map[string]claudeconfig.MCPServerSummary) []map[string]any {
	out := make([]map[string]any, 0, len(servers))
	for name, s := range servers {
		out = append(out, map[string]any{
			"name": name, "command": s.Command, "args": s.Args,
			"env": s.Env, "cwd": s.Cwd, "type": s.Type, "url": s.URL,
		})
	}
	return out
}

// loadRawDoc loads path and parses it as a claudeconfig.RawDoc.
func loadRawDoc(path string) (claudeconfig.RawDoc, claudeconfig.Loaded, error) {
	loaded, err := claudeconfig.Load(path)
	if err != nil {
		return nil, claudeconfig.Loaded{}, err
	}
	doc, err := claudeconfig.ParseRawDoc(loaded.Data)
	if err != nil {
		return nil, claudeconfig.Loaded{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc, loaded, nil
}

// addMCPServer loads path, adds/replaces one mcpServers entry, and writes it
// back through SafeWrite. Shared by the ~/.claude.json and settings.json
// variants; they differ only in which file they target.
//
// It touches only the one entry named name - every sibling entry keeps its
// exact original bytes via MCPServersRaw/SetMCPServersRaw, so a remote MCP
// server's "url"/"headers" (fields this app's Add-server form doesn't
// collect) are never dropped by adding or replacing an unrelated server.
func (a *App) addMCPServer(path, name string, server claudeconfig.MCPServer, expectedVersion string) error {
	if name == "" {
		return fmt.Errorf("MCP server name is required")
	}
	if server.Command == "" {
		return fmt.Errorf("MCP server command is required")
	}
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	doc, _, err := loadRawDoc(path)
	if err != nil {
		return err
	}
	servers, err := claudeconfig.MCPServersRaw(doc)
	if err != nil {
		return err
	}
	entry, err := json.Marshal(server)
	if err != nil {
		return err
	}
	servers[name] = entry
	if err := claudeconfig.SetMCPServersRaw(doc, servers); err != nil {
		return err
	}
	out, err := doc.Marshal()
	if err != nil {
		return err
	}
	return claudeconfig.SafeWrite(path, store.BackupDir(), out, expectedVersion)
}

// deleteMCPServer is addMCPServer's counterpart for removing an entry - see
// its comment for why siblings are untouched.
func (a *App) deleteMCPServer(path, name, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	doc, _, err := loadRawDoc(path)
	if err != nil {
		return err
	}
	servers, err := claudeconfig.MCPServersRaw(doc)
	if err != nil {
		return err
	}
	delete(servers, name)
	if err := claudeconfig.SetMCPServersRaw(doc, servers); err != nil {
		return err
	}
	out, err := doc.Marshal()
	if err != nil {
		return err
	}
	return claudeconfig.SafeWrite(path, store.BackupDir(), out, expectedVersion)
}

// listMCPServers loads path and returns {"version": ..., "servers": [...]}.
func (a *App) listMCPServers(path string) (map[string]any, error) {
	doc, loaded, err := loadRawDoc(path)
	if err != nil {
		return nil, err
	}
	servers, err := claudeconfig.MCPServerSummaries(doc)
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": loaded.Version, "servers": mcpServerList(servers)}, nil
}

// --- ~/.claude.json: overview, project history, raw JSON ---

// GetClaudeJSONOverview returns size/project/MCP/startup stats for
// ~/.claude.json, plus a "health" heuristic ("healthy"/"moderate"/"warning")
// ported from claude-config-editor's Quick Analysis card.
func (a *App) GetClaudeJSONOverview() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	doc, loaded, err := loadRawDoc(store.ClaudeJSONPath())
	if err != nil {
		return nil, err
	}
	stats, err := claudeconfig.ProjectStatsList(doc)
	if err != nil {
		return nil, err
	}
	servers, err := claudeconfig.MCPServersRaw(doc)
	if err != nil {
		return nil, err
	}
	numStartups, err := claudeconfig.NumStartups(doc)
	if err != nil {
		return nil, err
	}

	totalBytes := len(loaded.Data)
	projectCount := len(stats)
	health := "moderate"
	if totalBytes >= warningMinBytes || projectCount >= warningMinProjects {
		health = "warning"
	} else if totalBytes < healthyMaxBytes && projectCount < healthyMaxProjects {
		health = "healthy"
	}

	return map[string]any{
		"version":          loaded.Version,
		"total_size_bytes": totalBytes,
		"project_count":    projectCount,
		"mcp_count":        len(servers),
		"num_startups":     numStartups,
		"health":           health,
	}, nil
}

// ListClaudeJSONProjects returns per-project size/message-count stats.
func (a *App) ListClaudeJSONProjects() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	doc, loaded, err := loadRawDoc(store.ClaudeJSONPath())
	if err != nil {
		return nil, err
	}
	stats, err := claudeconfig.ProjectStatsList(doc)
	if err != nil {
		return nil, err
	}
	projects := make([]map[string]any, 0, len(stats))
	for _, s := range stats {
		projects = append(projects, map[string]any{
			"path": s.Path, "size_bytes": s.SizeBytes, "message_count": s.MessageCount,
		})
	}
	return map[string]any{"version": loaded.Version, "projects": projects}, nil
}

// ExportClaudeJSONProject returns one project's raw history entry as
// pretty-printed JSON, for the "export as JSON" action.
func (a *App) ExportClaudeJSONProject(path string) (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	doc, _, err := loadRawDoc(store.ClaudeJSONPath())
	if err != nil {
		return nil, err
	}
	raw, ok, err := claudeconfig.ExportProject(doc, path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no such project: %s", path)
	}
	pretty, err := claudeconfig.PrettyJSON(json.RawMessage(raw))
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "content": string(pretty)}, nil
}

// DeleteClaudeJSONProjects removes the given project paths' history from
// ~/.claude.json. expectedVersion must match the file's current version
// (from GetClaudeJSONOverview or ListClaudeJSONProjects) or the write is
// rejected with claudeconfig.ErrConflict.
func (a *App) DeleteClaudeJSONProjects(paths []string, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	path := store.ClaudeJSONPath()
	doc, _, err := loadRawDoc(path)
	if err != nil {
		return err
	}
	if err := claudeconfig.DeleteProjects(doc, paths); err != nil {
		return err
	}
	out, err := doc.Marshal()
	if err != nil {
		return err
	}
	return claudeconfig.SafeWrite(path, store.BackupDir(), out, expectedVersion)
}

// GetClaudeJSONRaw returns the whole file, pretty-printed, for the Raw JSON tab.
func (a *App) GetClaudeJSONRaw() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	return rawFileContent(store.ClaudeJSONPath())
}

// rawFileContent loads path and returns its content pretty-printed as JSON,
// alongside its version. An absent file reads as "{}".
func rawFileContent(path string) (map[string]any, error) {
	doc, loaded, err := loadRawDoc(path)
	if err != nil {
		return nil, err
	}
	pretty, err := doc.Marshal()
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": loaded.Version, "content": string(pretty)}, nil
}

// --- ~/.claude.json MCP servers ---

func (a *App) ListClaudeJSONMCPServers() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	return a.listMCPServers(store.ClaudeJSONPath())
}

// AddClaudeJSONMCPServer takes command/args/env as separate scalar
// parameters (rather than a single struct) because that's what a Wails
// binding needs to map cleanly onto individual JS call arguments; it bundles
// them into a claudeconfig.MCPServer immediately before delegating.
func (a *App) AddClaudeJSONMCPServer(name, command string, args []string, env map[string]string, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	server := claudeconfig.MCPServer{Command: command, Args: args, Env: env}
	return a.addMCPServer(store.ClaudeJSONPath(), name, server, expectedVersion)
}

func (a *App) DeleteClaudeJSONMCPServer(name, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	return a.deleteMCPServer(store.ClaudeJSONPath(), name, expectedVersion)
}

// --- ~/.claude/settings.json MCP servers (separate from ~/.claude.json's) ---

func (a *App) ListSettingsMCPServers() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	return a.listMCPServers(store.SettingsPath())
}

// AddSettingsMCPServer - see AddClaudeJSONMCPServer's comment for why this
// takes scalar params instead of a claudeconfig.MCPServer directly.
func (a *App) AddSettingsMCPServer(name, command string, args []string, env map[string]string, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	server := claudeconfig.MCPServer{Command: command, Args: args, Env: env}
	return a.addMCPServer(store.SettingsPath(), name, server, expectedVersion)
}

func (a *App) DeleteSettingsMCPServer(name, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	return a.deleteMCPServer(store.SettingsPath(), name, expectedVersion)
}

// --- ~/.claude/settings.json hooks (raw JSON surface - no structured CRUD yet) ---

// GetSettingsHooksRaw returns settings.json's "hooks" key, pretty-printed
// ("{}" if absent), plus the *whole file's* version. SaveSettingsHooksRaw
// checks that version, so any concurrent change anywhere in settings.json
// (not just to hooks) blocks the save.
func (a *App) GetSettingsHooksRaw() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	doc, loaded, err := loadRawDoc(store.SettingsPath())
	if err != nil {
		return nil, err
	}
	raw, ok := doc["hooks"]
	if !ok {
		raw = json.RawMessage("{}")
	}
	pretty, err := claudeconfig.PrettyJSON(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": loaded.Version, "content": string(pretty)}, nil
}

// SaveSettingsHooksRaw replaces settings.json's "hooks" key with content,
// leaving every other key untouched. content must be a JSON *object* -
// Claude Code's own settings.json schema expects hooks to be an object
// keyed by event name, and something else (null, an array, a bare string)
// would make the file invalid, which could in turn make Claude Code ignore
// the whole file, including unrelated permission rules.
func (a *App) SaveSettingsHooksRaw(content, expectedVersion string) error {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return fmt.Errorf("hooks content must be a JSON object: %w", err)
	}
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	path := store.SettingsPath()
	doc, _, err := loadRawDoc(path)
	if err != nil {
		return err
	}
	doc["hooks"] = json.RawMessage(content)
	out, err := doc.Marshal()
	if err != nil {
		return err
	}
	return claudeconfig.SafeWrite(path, store.BackupDir(), out, expectedVersion)
}

// --- ~/.claude/CLAUDE.md ---

// GetClaudeMdContent returns the raw markdown content plus its version.
func (a *App) GetClaudeMdContent() (map[string]any, error) {
	store, err := a.claudeconfigStore()
	if err != nil {
		return nil, err
	}
	loaded, err := claudeconfig.Load(store.ClaudeMDPath())
	if err != nil {
		return nil, err
	}
	return map[string]any{"version": loaded.Version, "content": string(loaded.Data)}, nil
}

// SaveClaudeMdContent overwrites CLAUDE.md with content.
func (a *App) SaveClaudeMdContent(content, expectedVersion string) error {
	store, err := a.claudeconfigStore()
	if err != nil {
		return err
	}
	path := store.ClaudeMDPath()
	return claudeconfig.SafeWrite(path, store.BackupDir(), []byte(content), expectedVersion)
}

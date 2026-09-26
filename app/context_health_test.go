package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetContextHealth exercises the refactored settings.json/CLAUDE.md
// parsing path (now backed by internal/claudeconfig) against a fixture home
// directory, guarding the internal/claudeconfig refactor against regressions.
func TestGetContextHealth(t *testing.T) {
	a := newTestApp(t)
	home := t.TempDir()
	a.testHomeDir = home

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	settings := `{
		"mcpServers": {"a": {"command":"foo"}, "b": {"command":"bar"}},
		"hooks": {
			"PreToolUse": [{"matcher":"Bash","hooks":[{"type":"command"}]}],
			"PostToolUse": [{"matcher":"*","hooks":[{"type":"command"}]},{"matcher":"Edit","hooks":[{"type":"command"}]}]
		}
	}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	claudeMD := "# Rules\n- rule one\n* rule two\nplain line\n"
	if err := os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeMD), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := a.GetContextHealth()
	if err != nil {
		t.Fatalf("GetContextHealth: %v", err)
	}
	if result.MCPCount != 2 {
		t.Errorf("MCPCount = %d, want 2", result.MCPCount)
	}
	if result.HookCount != 3 {
		t.Errorf("HookCount = %d, want 3 (1 PreToolUse + 2 PostToolUse matcher groups)", result.HookCount)
	}
	if result.LineCount != 4 {
		t.Errorf("LineCount = %d, want 4", result.LineCount)
	}
	if result.RuleCount != 2 {
		t.Errorf("RuleCount = %d, want 2", result.RuleCount)
	}
	if result.SettingsKB <= 0 {
		t.Error("SettingsKB should be > 0")
	}
	if result.ClaudeKB <= 0 {
		t.Error("ClaudeKB should be > 0")
	}
}

func TestGetContextHealth_MissingFiles_ReturnsZeroValues(t *testing.T) {
	a := newTestApp(t)
	a.testHomeDir = t.TempDir() // no ~/.claude at all

	result, err := a.GetContextHealth()
	if err != nil {
		t.Fatalf("GetContextHealth: %v", err)
	}
	if result.MCPCount != 0 || result.HookCount != 0 || result.LineCount != 0 || result.RuleCount != 0 {
		t.Errorf("expected all-zero result for a missing ~/.claude, got %+v", result)
	}
}

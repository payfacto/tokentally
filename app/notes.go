package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tokentally/internal/db"
)

// GetMarkdownFolders returns the configured folders the Notes tab lists *.md files from.
func (a *App) GetMarkdownFolders() ([]map[string]any, error) {
	folders, err := db.GetMarkdownFolders(a.conn)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(folders))
	for _, f := range folders {
		out = append(out, map[string]any{"path": f.Path, "label": f.Label})
	}
	return out, nil
}

// AddMarkdownFolder configures a folder for the Notes tab to list, or
// replaces its label if the path is already configured. path may start with
// "~" for the user's home directory.
func (a *App) AddMarkdownFolder(path, label string) error {
	resolved, err := resolveMarkdownFolderPath(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("folder does not exist: %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", resolved)
	}
	return db.AddMarkdownFolder(a.conn, resolved, label)
}

// DeleteMarkdownFolder removes a configured folder. The files on disk are untouched.
func (a *App) DeleteMarkdownFolder(path string) error {
	return db.DeleteMarkdownFolder(a.conn, path)
}

// ListMarkdownFiles lists the *.md files (non-recursive, case-insensitive
// extension match) directly inside each configured folder. A configured
// folder that no longer exists on disk is skipped rather than failing the
// whole call.
func (a *App) ListMarkdownFiles() ([]map[string]any, error) {
	folders, err := db.GetMarkdownFolders(a.conn)
	if err != nil {
		return nil, err
	}

	files := make([]map[string]any, 0)
	for _, f := range folders {
		entries, err := os.ReadDir(f.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, map[string]any{
				"folder_path":  f.Path,
				"folder_label": f.Label,
				"filename":     e.Name(),
				"full_path":    filepath.Join(f.Path, e.Name()),
				"size":         info.Size(),
				"mod_time":     info.ModTime().UTC().Format(time.RFC3339),
			})
		}
	}
	return files, nil
}

// ReadMarkdownFile returns the raw content of a markdown file. path must
// resolve to a direct child of one of the configured folders — anything
// else (traversal, a sibling folder whose name merely shares a path prefix)
// is rejected.
func (a *App) ReadMarkdownFile(path string) (string, error) {
	folders, err := db.GetMarkdownFolders(a.conn)
	if err != nil {
		return "", err
	}

	cleaned := filepath.Clean(path)
	if !isDirectChildOfAny(cleaned, folders) {
		return "", fmt.Errorf("path is not inside a configured markdown folder: %s", path)
	}

	data, err := os.ReadFile(cleaned)
	if err != nil {
		return "", fmt.Errorf("ReadMarkdownFile: %w", err)
	}
	return string(data), nil
}

// isDirectChildOfAny reports whether path's parent directory is exactly one
// of folders' paths — a prefix match would let a sibling directory whose
// name merely starts with a configured folder's path (e.g. "handoffs-evil")
// be treated as inside it.
func isDirectChildOfAny(path string, folders []db.MarkdownFolder) bool {
	parent := filepath.Dir(path)
	for _, f := range folders {
		if parent == filepath.Clean(f.Path) {
			return true
		}
	}
	return false
}

// resolveMarkdownFolderPath expands a leading "~" to the user's home
// directory and returns a cleaned absolute path.
func resolveMarkdownFolderPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve ~: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	return abs, nil
}

// seedMarkdownFolderDefaults configures the two known out-of-repo note
// folders (handoff and afk skill outputs) once. Gated by its own marker,
// independent of the pricing-seed gate, so re-seeding pricing (e.g. "Reset
// to defaults" in Settings) never resurrects a folder the user deleted.
func (a *App) seedMarkdownFolderDefaults() {
	seeded, err := db.IsMarkdownFoldersSeeded(a.conn)
	if err != nil {
		log.Printf("IsMarkdownFoldersSeeded: %v", err)
		return
	}
	if seeded {
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("seedMarkdownFolderDefaults: %v", err)
		return
	}
	db.SeedMarkdownFolder(a.conn, filepath.Join(home, ".claude", "handoffs"), "Handoffs") //nolint:errcheck
	db.SeedMarkdownFolder(a.conn, filepath.Join(home, ".claude", "afk"), "AFK Notes")     //nolint:errcheck
	db.MarkMarkdownFoldersSeeded(a.conn)                                                  //nolint:errcheck
}

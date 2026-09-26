// Package claudeconfig provides safe, versioned read/write access to Claude
// Code's own on-disk configuration files (~/.claude.json, ~/.claude/settings.json,
// ~/.claude/CLAUDE.md). Every write goes through SafeWrite: it backs up the
// prior content, writes atomically (temp file + rename), and rejects the
// write with ErrConflict if the file changed on disk since it was last
// loaded - Claude Code itself rewrites these files often, so callers must not
// blindly overwrite a newer version they never saw.
package claudeconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// backupTimeFormat produces a filesystem-safe, sortable timestamp for backup
// filenames (no colons, which are invalid in Windows paths).
const backupTimeFormat = "20060102T150405.000000000"

// maxBackupsPerFile caps how many timestamped backups SafeWrite keeps per
// target file; older ones are pruned after each successful write so the
// backup directory doesn't grow without bound.
const maxBackupsPerFile = 20

// defaultNewFileMode is the permission a brand-new config file is created
// with. These files can hold MCP server env secrets, so default to owner-only
// rather than the more permissive 0644 an empty os.CreateTemp would leave
// behind if never chmod'd.
const defaultNewFileMode = 0o600

// Store resolves the well-known Claude Code config paths under a home
// directory. Production code roots it at os.UserHomeDir(); tests root it at
// t.TempDir() so no test ever touches a real user's files.
type Store struct {
	home string
}

// NewStore returns a Store rooted at home.
func NewStore(home string) *Store {
	return &Store{home: home}
}

// ClaudeJSONPath is Claude Code's per-user project-history and MCP config file.
func (s *Store) ClaudeJSONPath() string {
	return filepath.Join(s.home, ".claude.json")
}

// SettingsPath is Claude Code's global settings file (mcpServers, hooks, etc.).
func (s *Store) SettingsPath() string {
	return filepath.Join(s.home, ".claude", "settings.json")
}

// ClaudeMDPath is the user's global CLAUDE.md instructions file.
func (s *Store) ClaudeMDPath() string {
	return filepath.Join(s.home, ".claude", "CLAUDE.md")
}

// BackupDir is where SafeWrite stores a timestamped copy of a file's prior
// content before overwriting it.
func (s *Store) BackupDir() string {
	return filepath.Join(s.home, ".claude", "backups")
}

// Loaded is a file's content plus a version token for optimistic concurrency.
type Loaded struct {
	Data    []byte
	Version string // sha256 hex digest of Data; "" when Exists is false
	Exists  bool
}

// Load reads path. A missing file is not an error: it returns a zero Loaded
// with Exists=false, matching this codebase's existing "missing config file
// is not fatal" convention (see GetContextHealth, resolveMarkdownFolderPath).
func Load(path string) (Loaded, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Loaded{}, nil
	}
	if err != nil {
		return Loaded{}, fmt.Errorf("claudeconfig: read %s: %w", path, err)
	}
	return Loaded{Data: data, Version: hashOf(data), Exists: true}, nil
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ErrConflict is returned by SafeWrite when the file's on-disk content no
// longer matches the version the caller last loaded.
var ErrConflict = errors.New("claudeconfig: file changed on disk since it was loaded")

// writeLocks serializes concurrent SafeWrite calls to the same path within
// this process. Wails dispatches frontend calls concurrently; without this,
// two rapid calls both loading the same expectedVersion could each pass the
// version check and race to write, silently discarding one of them. This
// can't protect against a write from *outside* the process (Claude Code
// itself) - that's what the version check above is for - but it closes the
// in-process half of the race.
var writeLocks sync.Map // path (string) -> *sync.Mutex

func lockFor(path string) *sync.Mutex {
	v, _ := writeLocks.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// SafeWrite atomically replaces path's content with data.
//
// expectedVersion must equal the Version from the most recent Load of path
// (or "" if the caller expects path not to exist yet). If the file's current
// on-disk content doesn't match, SafeWrite makes no changes and returns
// ErrConflict - the caller should reload and ask the user to retry rather
// than silently clobbering a newer version (Claude Code itself may have
// rewritten the file in the meantime).
//
// On success, any prior content is first copied into a timestamped, owner-
// only-readable file under backupDir (created only if a prior file existed;
// older backups beyond maxBackupsPerFile are pruned), then the new content
// is written to a temp file in path's directory and renamed into place - a
// rename is atomic on the same filesystem on both POSIX and Windows, so a
// reader never observes a partially-written file. If path is a symlink (for
// example into a dotfiles repo), the write follows it and replaces the real
// target rather than the link itself.
func SafeWrite(path, backupDir string, data []byte, expectedVersion string) error {
	mu := lockFor(path)
	mu.Lock()
	defer mu.Unlock()

	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}

	current, err := Load(target)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}

	mode := os.FileMode(defaultNewFileMode)
	if current.Exists {
		if info, err := os.Stat(target); err == nil {
			mode = info.Mode().Perm()
		}
		if err := backupFile(path, backupDir, current.Data); err != nil {
			return fmt.Errorf("claudeconfig: backup: %w", err)
		}
	}

	return writeAtomic(target, data, mode)
}

// writeAtomic writes data to a temp file beside target (same directory, so
// the rename below stays on one filesystem), chmods it to mode, then renames
// it over target. A rename is atomic on the same filesystem on both POSIX
// and Windows, so a reader never observes a partially-written file.
func writeAtomic(target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("claudeconfig: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".claudeconfig-tmp-*")
	if err != nil {
		return fmt.Errorf("claudeconfig: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("claudeconfig: write temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close() //nolint:errcheck
		return fmt.Errorf("claudeconfig: chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("claudeconfig: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("claudeconfig: rename into place: %w", err)
	}
	return nil
}

// backupFile writes data (the file's pre-write content) to a new timestamped
// file under backupDir, creating backupDir if needed, then prunes anything
// beyond maxBackupsPerFile for path's base name. Backups are always
// owner-only (0600/0700) regardless of the source file's own permissions -
// these files can hold MCP server env secrets, and a backup copy should
// never be more exposed than the original.
func backupFile(path, backupDir string, data []byte) error {
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	base := filepath.Base(path)
	name := base + "." + time.Now().UTC().Format(backupTimeFormat) + ".bak"
	if err := os.WriteFile(filepath.Join(backupDir, name), data, 0o600); err != nil {
		return err
	}
	return pruneOldBackups(backupDir, base)
}

// pruneOldBackups keeps only the maxBackupsPerFile most recent backups whose
// name starts with base+"." (the timestamp format sorts lexicographically in
// chronological order, so a plain string sort is enough). Best-effort:
// failing to remove an old backup doesn't fail the write that triggered it.
func pruneOldBackups(backupDir, base string) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}
	prefix := base + "."
	var matches []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			matches = append(matches, e.Name())
		}
	}
	if len(matches) <= maxBackupsPerFile {
		return nil
	}
	sort.Strings(matches)
	for _, name := range matches[:len(matches)-maxBackupsPerFile] {
		_ = os.Remove(filepath.Join(backupDir, name)) //nolint:errcheck
	}
	return nil
}

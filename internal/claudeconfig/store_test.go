package claudeconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"tokentally/internal/claudeconfig"
)

func TestStore_PathHelpers(t *testing.T) {
	s := claudeconfig.NewStore("/home/user")

	if got, want := s.ClaudeJSONPath(), filepath.Join("/home/user", ".claude.json"); got != want {
		t.Errorf("ClaudeJSONPath = %q, want %q", got, want)
	}
	if got, want := s.SettingsPath(), filepath.Join("/home/user", ".claude", "settings.json"); got != want {
		t.Errorf("SettingsPath = %q, want %q", got, want)
	}
	if got, want := s.ClaudeMDPath(), filepath.Join("/home/user", ".claude", "CLAUDE.md"); got != want {
		t.Errorf("ClaudeMDPath = %q, want %q", got, want)
	}
	if got, want := s.BackupDir(), filepath.Join("/home/user", ".claude", "backups"); got != want {
		t.Errorf("BackupDir = %q, want %q", got, want)
	}
}

func TestLoad_MissingFile_ReturnsEmptyNotError(t *testing.T) {
	loaded, err := claudeconfig.Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Exists {
		t.Fatal("expected Exists=false for a missing file")
	}
	if loaded.Version != "" {
		t.Fatalf("expected empty Version for a missing file, got %q", loaded.Version)
	}
}

func TestLoad_ExistingFile_ReturnsDataAndVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.Exists {
		t.Fatal("expected Exists=true")
	}
	if string(loaded.Data) != `{"a":1}` {
		t.Fatalf("Data = %q", loaded.Data)
	}
	if loaded.Version == "" {
		t.Fatal("expected a non-empty Version for an existing file")
	}

	// Same content loaded again must produce the same version token.
	loaded2, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatalf("Load (2nd): %v", err)
	}
	if loaded2.Version != loaded.Version {
		t.Fatalf("Version changed across loads of identical content: %q vs %q", loaded.Version, loaded2.Version)
	}
}

func TestSafeWrite_CreatesFile_WhenAbsentAndExpectedVersionEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	backupDir := filepath.Join(dir, "backups")

	if err := claudeconfig.SafeWrite(path, backupDir, []byte(`{"x":1}`), ""); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after SafeWrite: %v", err)
	}
	if string(got) != `{"x":1}` {
		t.Fatalf("file content = %q", got)
	}
}

func TestSafeWrite_RejectsCreate_WhenFileAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"already":"here"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{"new":1}`), "")
	if err != claudeconfig.ErrConflict {
		t.Fatalf("expected ErrConflict when creating over an existing file, got %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != `{"already":"here"}` {
		t.Fatalf("existing file was modified despite the conflict: %q", got)
	}
}

func TestSafeWrite_UpdatesFile_WhenVersionMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{"v":2}`), loaded.Version); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != `{"v":2}` {
		t.Fatalf("file content = %q, want updated content", got)
	}
}

func TestSafeWrite_RejectsUpdate_WhenVersionStale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate Claude Code writing the file concurrently, after our load.
	if err := os.WriteFile(path, []byte(`{"v":"changed-elsewhere"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err = claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{"v":2}`), loaded.Version)
	if err != claudeconfig.ErrConflict {
		t.Fatalf("expected ErrConflict for a stale version, got %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != `{"v":"changed-elsewhere"}` {
		t.Fatalf("the concurrently-written content was overwritten: %q", got)
	}
}

func TestSafeWrite_CreatesTimestampedBackupOfPriorContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	backupDir := filepath.Join(dir, "backups")
	if err := os.WriteFile(path, []byte(`{"v":"original"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := claudeconfig.SafeWrite(path, backupDir, []byte(`{"v":"updated"}`), loaded.Version); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("ReadDir(backupDir): %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 backup file, got %d: %+v", len(entries), entries)
	}
	if !strings.HasPrefix(entries[0].Name(), "settings.json.") {
		t.Fatalf("backup filename = %q, want prefix %q", entries[0].Name(), "settings.json.")
	}
	backupContent, err := os.ReadFile(filepath.Join(backupDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(backupContent) != `{"v":"original"}` {
		t.Fatalf("backup content = %q, want the pre-write content", backupContent)
	}
}

func TestSafeWrite_LeavesNoTempFileBehindAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{}`), ""); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "claudeconfig-tmp") {
			t.Fatalf("leftover temp file after a successful SafeWrite: %s", e.Name())
		}
	}
}

func TestSafeWrite_BackupFileAndDir_AreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	backupDir := filepath.Join(dir, "backups")
	// Original file is world-readable - the backup must still be locked down.
	if err := os.WriteFile(path, []byte(`{"v":"original"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := claudeconfig.SafeWrite(path, backupDir, []byte(`{"v":"updated"}`), loaded.Version); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}

	dirInfo, err := os.Stat(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("backup dir mode = %o, want 0700", perm)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 backup file, err=%v entries=%v", err, entries)
	}
	fileInfo, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("backup file mode = %o, want 0600 even though the source was 0644", perm)
	}
}

func TestSafeWrite_PrunesOldBackupsBeyondLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	backupDir := filepath.Join(dir, "backups")

	version := ""
	const total = 25 // > maxBackupsPerFile (20)
	for i := range total {
		content := fmt.Appendf(nil, `{"v":%d}`, i)
		if err := claudeconfig.SafeWrite(path, backupDir, content, version); err != nil {
			t.Fatalf("SafeWrite #%d: %v", i, err)
		}
		loaded, err := claudeconfig.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		version = loaded.Version
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 20 {
		t.Fatalf("expected pruning to cap backups at 20, got %d", len(entries))
	}
}

func TestSafeWrite_PreservesLiveFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{"v":2}`), loaded.Version); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("live file mode after update = %o, want the original 0644 preserved", perm)
	}
}

func TestSafeWrite_NewFileDefaultsToOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")
	if err := claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{}`), ""); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("new file mode = %o, want 0600", perm)
	}
}

func TestSafeWrite_FollowsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows by default")
	}
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(realDir, "settings.json")
	if err := os.WriteFile(realPath, []byte(`{"v":"original"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "settings.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}

	loaded, err := claudeconfig.Load(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := claudeconfig.SafeWrite(linkPath, filepath.Join(dir, "backups"), []byte(`{"v":"updated"}`), loaded.Version); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}

	// The symlink itself must still be a symlink, pointing at the same target.
	fi, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("SafeWrite replaced the symlink with a regular file")
	}
	got, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"v":"updated"}` {
		t.Fatalf("real file content = %q, want the update to have landed there", got)
	}
}

func TestSafeWrite_SerializesConcurrentWritesToSamePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"v":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := claudeconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0] = claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{"v":1}`), loaded.Version)
	}()
	go func() {
		defer wg.Done()
		results[1] = claudeconfig.SafeWrite(path, filepath.Join(dir, "backups"), []byte(`{"v":2}`), loaded.Version)
	}()
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	// Both goroutines load the SAME expectedVersion (simulating a race). The
	// mutex must serialize them so the second one's re-check sees the
	// first's write and correctly reports ErrConflict - never two "successes"
	// silently clobbering each other.
	if successes != 1 {
		t.Fatalf("expected exactly 1 of 2 concurrent same-version writes to succeed, got %d successes: %v", successes, results)
	}
}

func TestSafeWrite_DoesNotCreateBackupDir_WhenFileDidNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	backupDir := filepath.Join(dir, "backups")

	if err := claudeconfig.SafeWrite(path, backupDir, []byte(`{}`), ""); err != nil {
		t.Fatalf("SafeWrite: %v", err)
	}
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("expected no backup dir to be created for a brand-new file, stat err = %v", err)
	}
}

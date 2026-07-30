package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddMarkdownFolder_RejectsMissingDir(t *testing.T) {
	a := newTestApp(t)

	err := a.AddMarkdownFolder(filepath.Join(t.TempDir(), "does-not-exist"), "Nope")
	if err == nil {
		t.Fatal("expected error for a folder that does not exist")
	}
}

func TestAddMarkdownFolder_RejectsFile(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir.md")
	if err := os.WriteFile(filePath, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.AddMarkdownFolder(filePath, "Nope"); err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
}

func TestAddGetDeleteMarkdownFolder(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()

	if err := a.AddMarkdownFolder(dir, "My Notes"); err != nil {
		t.Fatalf("AddMarkdownFolder failed: %v", err)
	}

	folders, err := a.GetMarkdownFolders()
	if err != nil {
		t.Fatalf("GetMarkdownFolders failed: %v", err)
	}
	if len(folders) != 1 || folders[0]["path"] != dir || folders[0]["label"] != "My Notes" {
		t.Fatalf("unexpected folders: %+v", folders)
	}

	if err := a.DeleteMarkdownFolder(dir); err != nil {
		t.Fatalf("DeleteMarkdownFolder failed: %v", err)
	}
	folders, err = a.GetMarkdownFolders()
	if err != nil {
		t.Fatalf("GetMarkdownFolders after delete failed: %v", err)
	}
	if len(folders) != 0 {
		t.Fatalf("expected no folders after delete, got %+v", folders)
	}
}

func TestListMarkdownFiles(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.md"), "# One")
	writeFile(t, filepath.Join(dir, "two.MD"), "# Two")
	writeFile(t, filepath.Join(dir, "ignore.txt"), "nope")
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := a.AddMarkdownFolder(dir, "Notes"); err != nil {
		t.Fatalf("AddMarkdownFolder failed: %v", err)
	}

	files, err := a.ListMarkdownFiles()
	if err != nil {
		t.Fatalf("ListMarkdownFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 .md files (case-insensitive, non-recursive), got %d: %+v", len(files), files)
	}
	for _, f := range files {
		if f["folder_label"] != "Notes" {
			t.Errorf("folder_label = %v, want Notes", f["folder_label"])
		}
	}
}

func TestListMarkdownFiles_SkipsMissingFolder(t *testing.T) {
	a := newTestApp(t)
	missing := filepath.Join(t.TempDir(), "gone")
	// Insert directly since AddMarkdownFolder validates existence.
	if _, err := a.conn.Write.Exec(`INSERT INTO markdown_folders (path, label) VALUES (?,?)`, missing, "Gone"); err != nil {
		t.Fatalf("seed missing folder failed: %v", err)
	}

	files, err := a.ListMarkdownFiles()
	if err != nil {
		t.Fatalf("ListMarkdownFiles should not error on a missing folder: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no files from a missing folder, got %+v", files)
	}
}

func TestReadMarkdownFile(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.md"), "# Hello")
	if err := a.AddMarkdownFolder(dir, "Notes"); err != nil {
		t.Fatalf("AddMarkdownFolder failed: %v", err)
	}

	content, err := a.ReadMarkdownFile(filepath.Join(dir, "one.md"))
	if err != nil {
		t.Fatalf("ReadMarkdownFile failed: %v", err)
	}
	if content != "# Hello" {
		t.Errorf("content = %q, want %q", content, "# Hello")
	}
}

func TestReadMarkdownFile_RejectsPathOutsideConfiguredFolders(t *testing.T) {
	a := newTestApp(t)
	dir := t.TempDir()
	if err := a.AddMarkdownFolder(dir, "Notes"); err != nil {
		t.Fatalf("AddMarkdownFolder failed: %v", err)
	}

	// Classic traversal.
	if _, err := a.ReadMarkdownFile(filepath.Join(dir, "..", "secret.md")); err == nil {
		t.Fatal("expected traversal outside configured folder to be rejected")
	}

	// Sibling folder whose name merely has the configured folder's path as a
	// prefix must not be treated as "inside" it.
	siblingWithPrefixedName := dir + "-evil"
	if err := os.MkdirAll(siblingWithPrefixedName, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(siblingWithPrefixedName, "evil.md"), "evil")
	if _, err := a.ReadMarkdownFile(filepath.Join(siblingWithPrefixedName, "evil.md")); err == nil {
		t.Fatal("expected sibling-prefix path to be rejected")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

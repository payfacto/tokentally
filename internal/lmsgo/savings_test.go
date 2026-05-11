package lmsgo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEstimateInput(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), strings.Repeat("x", 400))
	mustWrite(t, filepath.Join(dir, "b.go"), strings.Repeat("y", 800))
	mustWrite(t, filepath.Join(dir, "c.txt"), strings.Repeat("z", 100))
	subDir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(subDir, "d.go"), strings.Repeat("w", 200))

	tests := []struct {
		name      string
		cmd       Command
		wantBytes int64
		wantFound int
	}{
		{
			name:      "single file",
			cmd:       Command{Subcommand: "ask", Paths: []string{filepath.Join(dir, "a.go")}},
			wantBytes: 400,
			wantFound: 1,
		},
		{
			name:      "directory recurses",
			cmd:       Command{Subcommand: "ask", Paths: []string{dir}},
			wantBytes: 400 + 800 + 100 + 200,
			wantFound: 4,
		},
		{
			name:      "directory with glob filters by extension",
			cmd:       Command{Subcommand: "ask", Glob: "*.go", Paths: []string{dir}},
			wantBytes: 400 + 800 + 200,
			wantFound: 3,
		},
		{
			name:      "non-lmsgo zero",
			cmd:       Command{},
			wantBytes: 0,
			wantFound: 0,
		},
		{
			name:      "missing file counted",
			cmd:       Command{Subcommand: "ask", Paths: []string{filepath.Join(dir, "nope.go")}},
			wantBytes: 0,
			wantFound: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateInput(tt.cmd)
			if got.InputBytes != tt.wantBytes {
				t.Errorf("InputBytes = %d, want %d", got.InputBytes, tt.wantBytes)
			}
			if got.FilesFound != tt.wantFound {
				t.Errorf("FilesFound = %d, want %d", got.FilesFound, tt.wantFound)
			}
		})
	}
}

func TestEstimateInputMissingPath(t *testing.T) {
	cmd := Command{Subcommand: "ask", Paths: []string{"/definitely/does/not/exist.go"}}
	got := EstimateInput(cmd)
	if got.FilesMissing != 1 {
		t.Errorf("FilesMissing = %d, want 1", got.FilesMissing)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

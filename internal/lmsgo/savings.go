package lmsgo

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CharsPerToken is the rough heuristic (1 token ≈ 4 chars of source code or
// English prose) used to convert byte counts into approximate Claude tokens.
const CharsPerToken = 4

// Estimate is the result of stat-ing the input files of one parsed lmsgo
// invocation. All sizes are in bytes; tokens are approximate.
type Estimate struct {
	InputBytes      int64 // sum of file sizes that lmsgo actually read
	InputTokensApx  int64 // InputBytes / CharsPerToken
	FilesFound      int   // how many files contributed to InputBytes
	FilesMissing    int   // paths from the command that no longer exist
}

// EstimateInput resolves the input file sizes referenced by a parsed Command.
// Directories are walked recursively. The --glob filter, if present, restricts
// matches to its pattern (matched against the file basename).
//
// Missing files are counted but not treated as errors — transcripts can be
// older than the on-disk filesystem, so we report best-effort numbers.
func EstimateInput(cmd Command) Estimate {
	var e Estimate
	if !cmd.IsLmsgo() {
		return e
	}
	for _, p := range cmd.Paths {
		info, err := os.Stat(p)
		if err != nil {
			e.FilesMissing++
			continue
		}
		if !info.IsDir() {
			if matchGlob(cmd.Glob, info.Name()) {
				e.InputBytes += info.Size()
				e.FilesFound++
			}
			continue
		}
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil // skip unreadable subtrees
			}
			if d.IsDir() {
				return nil
			}
			if !matchGlob(cmd.Glob, d.Name()) {
				return nil
			}
			fi, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			e.InputBytes += fi.Size()
			e.FilesFound++
			return nil
		})
	}
	e.InputTokensApx = e.InputBytes / CharsPerToken
	return e
}

// matchGlob returns true when name matches the glob pattern. An empty pattern
// matches everything.
func matchGlob(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	ok, err := filepath.Match(pattern, name)
	if err != nil || !ok {
		// `**/*.go` and similar — fall back to suffix match on the trailing
		// `*.ext` portion so common globs still work.
		if i := strings.LastIndex(pattern, "*"); i >= 0 && i < len(pattern)-1 {
			return strings.HasSuffix(name, pattern[i+1:])
		}
		return false
	}
	return true
}

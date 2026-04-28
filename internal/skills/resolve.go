// Package skills resolves Claude Code skill file paths and reports their sizes.
package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// Bytes returns the byte count of the SKILL.md file for the given skill name.
// Names may be plain ("update-config") or plugin-namespaced ("superpowers:writing-plans").
// Reports false if the file cannot be located or stat'd.
func Bytes(name string) (int64, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, false
	}
	base := filepath.Join(home, ".claude")

	if idx := strings.IndexByte(name, ':'); idx > 0 {
		// plugin:skill-name — search plugin cache
		// Layout: ~/.claude/plugins/cache/<package>/<plugin>/<version>/skills/<skill>/SKILL.md
		plugin := name[:idx]
		skill := name[idx+1:]
		pattern := filepath.Join(base, "plugins", "cache", "*", plugin, "*", "skills", skill, "SKILL.md")
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			// Multiple version dirs may match; take the last (lexicographically highest).
			if info, err := os.Stat(matches[len(matches)-1]); err == nil {
				return info.Size(), true
			}
		}
		return 0, false
	}

	// Unnamespaced: user skill at ~/.claude/skills/<name>/SKILL.md or ~/.claude/skills/<name>.md
	if info, err := os.Stat(filepath.Join(base, "skills", name, "SKILL.md")); err == nil {
		return info.Size(), true
	}
	if info, err := os.Stat(filepath.Join(base, "skills", name+".md")); err == nil {
		return info.Size(), true
	}
	return 0, false
}

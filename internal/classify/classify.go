// Package classify provides deterministic activity categorization for
// assistant turns based on their tool usage patterns.
package classify

import "strings"

// Message classifies an assistant turn given tool names and bash targets.
// Returns one of: Delegation, Planning, Testing, GitOps, BuildDeploy,
// Coding, Exploration, Conversation, General.
func Message(toolNames []string, bashTargets []string) string {
	tools := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		tools[n] = true
	}

	if tools["Task"] || tools["Agent"] {
		return "Delegation"
	}
	if tools["EnterPlanMode"] || tools["TodoWrite"] || tools["ExitPlanMode"] {
		return "Planning"
	}
	if tools["Bash"] {
		if hasTestCmd(bashTargets) {
			return "Testing"
		}
		if hasGitCmd(bashTargets) {
			return "GitOps"
		}
		if hasBuildCmd(bashTargets) {
			return "BuildDeploy"
		}
	}
	if tools["Edit"] || tools["Write"] || tools["MultiEdit"] {
		return "Coding"
	}
	if tools["Skill"] {
		return "General"
	}
	if tools["Read"] || tools["Grep"] || tools["Glob"] || tools["WebSearch"] || tools["WebFetch"] {
		return "Exploration"
	}
	if len(toolNames) == 0 {
		return "Conversation"
	}
	return "General"
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		return s[:idx]
	}
	return s
}

func hasTestCmd(targets []string) bool {
	for _, target := range targets {
		cmd, full := effectiveCommand(target)
		if isTestCmd(cmd, full) {
			return true
		}
	}
	return false
}

func hasGitCmd(targets []string) bool {
	for _, target := range targets {
		cmd, _ := effectiveCommand(target)
		if cmd == "git" {
			return true
		}
	}
	return false
}

func hasBuildCmd(targets []string) bool {
	for _, target := range targets {
		cmd, _ := effectiveCommand(target)
		if isBuildCmd(cmd) {
			return true
		}
	}
	return false
}

func effectiveCommand(target string) (cmd string, full string) {
	target = strings.TrimSpace(target)
	cmd = firstWord(target)
	if cmd != "rtk" {
		return leadingCommand(target), target
	}

	wrapped := unwrapRTKTarget(target)
	if wrapped == "" {
		return cmd, target
	}
	return leadingCommand(wrapped), wrapped
}

func unwrapRTKTarget(target string) string {
	fields := strings.Fields(target)
	if len(fields) < 2 || fields[0] != "rtk" {
		return ""
	}

	// Prefer explicit delimiter: rtk ... -- <command>
	for i, f := range fields {
		if f == "--" && i+1 < len(fields) {
			return strings.Join(fields[i+1:], " ")
		}
	}

	// Common wrappers: rtk exec|run <command>
	for i, f := range fields {
		if f != "exec" && f != "run" {
			continue
		}
		for j := i + 1; j < len(fields); j++ {
			if strings.HasPrefix(fields[j], "-") {
				continue
			}
			return strings.Join(fields[j:], " ")
		}
	}

	return ""
}

func leadingCommand(full string) string {
	fields := strings.Fields(strings.TrimSpace(full))
	if len(fields) == 0 {
		return ""
	}

	cmd := trimShellToken(fields[0])

	if isPOSIXShell(cmd) && len(fields) >= 3 && (fields[1] == "-c" || fields[1] == "-lc") {
		return trimShellToken(fields[2])
	}
	if (cmd == "cmd" || cmd == "cmd.exe") && len(fields) >= 3 && strings.EqualFold(fields[1], "/c") {
		return trimShellToken(fields[2])
	}
	if isPowerShell(cmd) {
		for i := 1; i < len(fields)-1; i++ {
			if strings.EqualFold(fields[i], "-command") || strings.EqualFold(fields[i], "-c") {
				return trimShellToken(fields[i+1])
			}
		}
	}

	return cmd
}

func trimShellToken(s string) string {
	return strings.Trim(strings.ToLower(s), "\"'`")
}

func isPOSIXShell(cmd string) bool {
	switch cmd {
	case "sh", "bash", "zsh", "fish":
		return true
	}
	return false
}

func isPowerShell(cmd string) bool {
	switch cmd {
	case "powershell", "powershell.exe", "pwsh":
		return true
	}
	return false
}

func isTestCmd(cmd, full string) bool {
	if cmd == "go" && strings.Contains(full, "test") {
		return true
	}
	switch cmd {
	case "pytest", "jest", "vitest", "mocha", "rspec", "cargo":
		return true
	case "npm", "npx", "yarn", "bun", "pnpm":
		return strings.Contains(full, "test")
	}
	return false
}

func isBuildCmd(cmd string) bool {
	switch cmd {
	case "npm", "npx", "yarn", "bun", "pnpm",
		"docker", "make", "wails", "cargo",
		"gradle", "mvn", "ant":
		return true
	}
	return false
}

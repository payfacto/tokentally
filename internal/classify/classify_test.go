package classify_test

import (
	"testing"

	"tokentally/internal/classify"
)

func TestMessage(t *testing.T) {
	cases := []struct {
		name  string
		tools []string
		bash  []string
		want  string
	}{
		// Delegation
		{name: "task_tool", tools: []string{"Task", "Read"}, want: "Delegation"},
		{name: "agent_tool", tools: []string{"Agent"}, want: "Delegation"},

		// Planning
		{name: "enter_and_todo", tools: []string{"EnterPlanMode", "TodoWrite"}, want: "Planning"},
		{name: "todo_write_alone", tools: []string{"TodoWrite"}, want: "Planning"},
		{name: "exit_plan_mode", tools: []string{"ExitPlanMode"}, want: "Planning"},

		// Testing
		{name: "go_test", tools: []string{"Bash"}, bash: []string{"go test ./..."}, want: "Testing"},
		{name: "pytest", tools: []string{"Bash"}, bash: []string{"pytest"}, want: "Testing"},
		{name: "npm_test", tools: []string{"Bash"}, bash: []string{"npm test"}, want: "Testing"},
		{name: "multi_bash_testing", tools: []string{"Bash"}, bash: []string{"ls -la", "go test ./..."}, want: "Testing"},

		// GitOps
		{name: "git_commit", tools: []string{"Bash"}, bash: []string{"git commit -m \"fix\""}, want: "GitOps"},

		// BuildDeploy
		{name: "npm_build", tools: []string{"Bash"}, bash: []string{"npm run build"}, want: "BuildDeploy"},

		// RTK wrappers
		{name: "rtk_delimiter_test", tools: []string{"Bash"}, bash: []string{"rtk exec -- go test ./..."}, want: "Testing"},
		{name: "rtk_run_git", tools: []string{"Bash"}, bash: []string{"rtk run git status"}, want: "GitOps"},
		{name: "rtk_exec_npm_build", tools: []string{"Bash"}, bash: []string{"rtk exec -- npm run build"}, want: "BuildDeploy"},
		{name: "rtk_quoted_cmd", tools: []string{"Bash"}, bash: []string{"rtk exec \"go test ./...\""}, want: "Testing"},
		{name: "rtk_shell_wrapped", tools: []string{"Bash"}, bash: []string{"rtk exec -- bash -lc \"git status\""}, want: "GitOps"},
		{name: "rtk_no_wrap_fallback", tools: []string{"Bash"}, bash: []string{"rtk gain"}, want: "General"},

		// POSIX shell wrapping
		{name: "bash_c_git", tools: []string{"Bash"}, bash: []string{"bash -c git status"}, want: "GitOps"},

		// Windows cmd
		{name: "cmd_c_npm", tools: []string{"Bash"}, bash: []string{"cmd /c npm run build"}, want: "BuildDeploy"},

		// Coding
		{name: "edit_and_read", tools: []string{"Edit", "Read"}, want: "Coding"},
		{name: "write_alone", tools: []string{"Write"}, want: "Coding"},
		{name: "multi_edit", tools: []string{"MultiEdit"}, want: "Coding"},

		// Exploration
		{name: "read_grep_glob", tools: []string{"Read", "Grep", "Glob"}, want: "Exploration"},
		{name: "web_search", tools: []string{"WebSearch"}, want: "Exploration"},
		{name: "web_fetch", tools: []string{"WebFetch"}, want: "Exploration"},

		// Conversation
		{name: "no_tools", tools: []string{}, want: "Conversation"},
		{name: "nil_tools", tools: nil, want: "Conversation"},

		// General
		{name: "skill_tool", tools: []string{"Skill"}, want: "General"},
		{name: "unknown_tool", tools: []string{"UnknownToolXYZ"}, want: "General"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify.Message(tc.tools, tc.bash)
			if got != tc.want {
				t.Errorf("Message(%v, %v) = %q, want %q", tc.tools, tc.bash, got, tc.want)
			}
		})
	}
}

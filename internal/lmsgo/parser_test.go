package lmsgo

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Command
	}{
		{
			name: "non-lmsgo command",
			in:   "git status",
			want: Command{},
		},
		{
			name: "ask single file",
			in:   `lmsgo ask --question "where is X?" main.go`,
			want: Command{Subcommand: "ask", Question: "where is X?", Paths: []string{"main.go"}},
		},
		{
			name: "ask multiple files",
			in:   `lmsgo ask --question "summarize" a.go b.go c.go`,
			want: Command{Subcommand: "ask", Question: "summarize", Paths: []string{"a.go", "b.go", "c.go"}},
		},
		{
			name: "ask with glob",
			in:   `lmsgo ask --glob "*.go" --question "find handler" src/`,
			want: Command{Subcommand: "ask", Glob: "*.go", Question: "find handler", Paths: []string{"src/"}},
		},
		{
			name: "write skips target as input",
			in:   `lmsgo write --spec "tests for X" --target out_test.go src/x.go`,
			want: Command{Subcommand: "write", Spec: "tests for X", Target: "out_test.go", Paths: []string{"src/x.go"}},
		},
		{
			name: "extract input file with -o",
			in:   `lmsgo extract -o /tmp/chat.txt /home/user/.claude/projects/p/s.jsonl`,
			want: Command{Subcommand: "extract", Paths: []string{"/home/user/.claude/projects/p/s.jsonl"}},
		},
		{
			name: "extract with --last flag",
			in:   `lmsgo extract --last 5 session.jsonl`,
			want: Command{Subcommand: "extract", Paths: []string{"session.jsonl"}},
		},
		{
			name: "unknown subcommand rejected",
			in:   `lmsgo gain --history`,
			want: Command{},
		},
		{
			name: "quoted path with spaces",
			in:   `lmsgo ask --question "what types?" "C:/My Documents/code/main.go"`,
			want: Command{Subcommand: "ask", Question: "what types?", Paths: []string{"C:/My Documents/code/main.go"}},
		},
		{
			name: "single-quoted path",
			in:   `lmsgo ask --question 'find it' 'a b c.go'`,
			want: Command{Subcommand: "ask", Question: "find it", Paths: []string{"a b c.go"}},
		},
		{
			name: "empty input",
			in:   "",
			want: Command{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q)\n  got:  %+v\n  want: %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsLmsgo(t *testing.T) {
	if (Command{}).IsLmsgo() {
		t.Error("zero Command should not be lmsgo")
	}
	if !(Command{Subcommand: "ask"}).IsLmsgo() {
		t.Error("ask Command should be lmsgo")
	}
}

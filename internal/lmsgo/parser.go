// Package lmsgo parses lmsgo CLI invocations stored as Bash tool targets in
// the tool_calls table. Used to estimate tokens saved by delegating bulk I/O
// to a local LM Studio model instead of pulling files into Claude's context.
package lmsgo

import (
	"strings"
)

// Command is the parsed shape of one `lmsgo …` invocation.
type Command struct {
	Subcommand string   // ask | write | extract | "" if not lmsgo
	Paths      []string // positional file/dir arguments (non-flag, non-flag-value)
	Glob       string   // value of --glob if present
	Target     string   // value of --target (write subcommand output, excluded from input estimate)
	Question   string   // value of --question (used only for context, not a file)
	Spec       string   // value of --spec (write subcommand)
}

// IsLmsgo reports whether the parsed command was an lmsgo invocation.
func (c Command) IsLmsgo() bool { return c.Subcommand != "" }

// Parse tokenizes a Bash command line and returns the parsed lmsgo Command.
// Returns a zero Command (IsLmsgo()==false) for non-lmsgo commands.
func Parse(cmdline string) Command {
	tokens := tokenize(cmdline)
	if len(tokens) < 2 || tokens[0] != "lmsgo" {
		return Command{}
	}

	cmd := Command{Subcommand: tokens[1]}
	switch cmd.Subcommand {
	case "ask", "write", "extract":
	default:
		return Command{} // unknown subcommand — don't claim it
	}

	for i := 2; i < len(tokens); i++ {
		t := tokens[i]
		switch t {
		case "--question", "-q":
			if i+1 < len(tokens) {
				cmd.Question = tokens[i+1]
				i++
			}
		case "--spec":
			if i+1 < len(tokens) {
				cmd.Spec = tokens[i+1]
				i++
			}
		case "--target":
			if i+1 < len(tokens) {
				cmd.Target = tokens[i+1]
				i++
			}
		case "--glob":
			if i+1 < len(tokens) {
				cmd.Glob = tokens[i+1]
				i++
			}
		case "-o", "--last":
			// flags with a value: -o is the extract output path; --last is a tail count.
			// Either way, skip the value so it isn't mistaken for an input file.
			if i+1 < len(tokens) {
				i++
			}
		case "--dry-run":
			// boolean flag — no value to skip
		default:
			if strings.HasPrefix(t, "-") {
				continue // unknown flag; conservatively skip
			}
			cmd.Paths = append(cmd.Paths, t)
		}
	}
	return cmd
}

// tokenize splits a command line into argv-style tokens, honoring single and
// double quotes. Backslash escapes are passed through verbatim because the
// transcripts we parse have already been through one round of shell quoting.
func tokenize(s string) []string {
	var tokens []string
	var buf strings.Builder
	inSingle, inDouble := false, false
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	for _, r := range s {
		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				buf.WriteRune(r)
			}
		case inDouble:
			if r == '"' {
				inDouble = false
			} else {
				buf.WriteRune(r)
			}
		case r == '\'':
			inSingle = true
		case r == '"':
			inDouble = true
		case r == ' ' || r == '\t':
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return tokens
}

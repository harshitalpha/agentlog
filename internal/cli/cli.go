// Package cli parses subcommands, wires the other packages together and
// prints output. It holds no business logic.
package cli

import (
	"fmt"
	"io"
	"os"
)

// Version is set by main (and overridable at build time via -ldflags).
var Version = "dev"

// Exit codes (docs/04-cli.md).
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// command describes one subcommand. run receives the args after the
// command name. Stubs are replaced by real implementations in later tasks.
type command struct {
	name    string
	summary string
	usage   string
	run     func(args []string, s streams) int
}

// streams bundles the output writers so commands are testable.
type streams struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

func commands() []command {
	return []command{
		{"init", "enable agentlog in a project (git init, baseline commit, rules)",
			"agentlog init [path] [--no-agents-md] [--claude-md] [--no-baseline] [--standalone-hook]", initCmd},
		{"disable", "turn enforcement off for a repo (commits are kept)",
			"agentlog disable [path]", stub("disable")},
		{"status", "show whether agentlog is enabled and what it has committed",
			"agentlog status [path] [--json]", stub("status")},
		{"ui", "open the timeline UI",
			"agentlog ui [path] [--port N] [--no-open] [--detach] [--stop]", stub("ui")},
		{"watch", "auto-commit after edits settle (for agents without hooks)",
			"agentlog watch [path] [--quiet 20] [--interval 2]", stub("watch")},
		{"hook-session-start", "Claude Code SessionStart hook (plugin use)",
			"agentlog hook-session-start", hookSessionStart},
		{"hook-stop", "Claude Code Stop hook (plugin use)",
			"agentlog hook-stop", hookStop},
		{"uninstall", "remove standalone hooks and rules from a repo",
			"agentlog uninstall [path]", stub("uninstall")},
	}
}

// stub is a placeholder that fails with a clear message until implemented.
func stub(name string) func([]string, streams) int {
	return func(_ []string, s streams) int {
		fmt.Fprintf(s.err, "agentlog: %s is not implemented yet\n", name)
		return exitError
	}
}

func usageText() string {
	text := "agentlog: make AI coding agents commit locally, then browse the history.\n\n" +
		"Usage:\n  agentlog <command> [flags] [path]\n  agentlog --version | --help\n\nCommands:\n"
	for _, c := range commands() {
		text += fmt.Sprintf("  %-20s %s\n", c.name, c.summary)
	}
	return text + "\nRun 'agentlog <command> --help' for details.\n"
}

// Run executes the CLI and returns the process exit code
// (0 success, 1 runtime error, 2 usage error).
func Run(args []string) int {
	return run(args, streams{in: os.Stdin, out: os.Stdout, err: os.Stderr})
}

func run(args []string, s streams) int {
	if len(args) == 0 {
		fmt.Fprint(s.err, usageText())
		return exitUsage
	}
	switch args[0] {
	case "--version", "-version", "version":
		fmt.Fprintln(s.out, Version)
		return exitOK
	case "--help", "-h", "help":
		fmt.Fprint(s.out, usageText())
		return exitOK
	}
	for _, c := range commands() {
		if c.name != args[0] {
			continue
		}
		rest := args[1:]
		if len(rest) > 0 && (rest[0] == "--help" || rest[0] == "-h") {
			fmt.Fprintf(s.out, "Usage: %s\n\n%s\n", c.usage, c.summary)
			return exitOK
		}
		return c.run(rest, s)
	}
	fmt.Fprintf(s.err, "agentlog: unknown command %q\n\n%s", args[0], usageText())
	return exitUsage
}

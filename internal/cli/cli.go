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

const usage = `agentlog: make AI coding agents commit locally, then browse the history.

Usage:
  agentlog <command> [flags] [path]
  agentlog --version | --help
`

// Run executes the CLI and returns the process exit code
// (0 success, 1 runtime error, 2 usage error).
func Run(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "--version", "-version", "version":
		fmt.Fprintln(stdout, Version)
		return 0
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "agentlog: unknown command %q\n\n%s", args[0], usage)
	return 2
}

package cli

import (
	"flag"
	"fmt"

	"github.com/harshitalpha/agentlog/internal/agent"
)

func uninstallCmd(args []string, s streams) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	path, ok := parseArgs("uninstall", fs, args, s.err)
	if !ok {
		return exitUsage
	}
	repo, ok := openRepoArg("uninstall", path, s)
	if !ok {
		return exitError
	}
	if err := agent.Uninstall(repo, s.out); err != nil {
		fmt.Fprintf(s.err, "agentlog: uninstall: %v\n", err)
		return exitError
	}
	stopRunningUI(repo)
	fmt.Fprintf(s.out, "✓ removed agentlog from %s (your commits are untouched)\n", repo.Root)
	return exitOK
}

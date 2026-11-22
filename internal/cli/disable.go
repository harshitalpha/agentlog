package cli

import (
	"flag"
	"fmt"

	"github.com/harshitalpha/agentlog/internal/agent"
)

func disableCmd(args []string, s streams) int {
	fs := flag.NewFlagSet("disable", flag.ContinueOnError)
	path, ok := parseArgs("disable", fs, args, s.err)
	if !ok {
		return exitUsage
	}
	repo, ok := openRepoArg("disable", path, s)
	if !ok {
		return exitError
	}
	if err := repo.UnsetLocal(agent.ConfigKey); err != nil {
		fmt.Fprintf(s.err, "agentlog: disable: %v\n", err)
		return exitError
	}
	stopRunningUI(repo)
	fmt.Fprintf(s.out, "✓ agentlog disabled in %s (commits untouched)\n", repo.Root)
	return exitOK
}

package cli

import (
	"flag"
	"fmt"

	"github.com/harshitalpha/agentlog/internal/agent"
)

func initCmd(args []string, s streams) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var opts agent.InitOptions
	fs.BoolVar(&opts.NoAgentsMD, "no-agents-md", false, "don't write the rules block into AGENTS.md")
	fs.BoolVar(&opts.ClaudeMD, "claude-md", false, "also write the rules block into CLAUDE.md")
	fs.BoolVar(&opts.NoBaseline, "no-baseline", false, "don't commit a baseline snapshot")
	fs.BoolVar(&opts.StandaloneHook, "standalone-hook", false, "install hooks into .claude/settings.local.json")
	path, ok := parseArgs("init", fs, args, s.err)
	if !ok {
		return exitUsage
	}
	if _, err := agent.Init(path, opts, s.out); err != nil {
		fmt.Fprintf(s.err, "agentlog: %v\n", err)
		return exitError
	}
	return exitOK
}

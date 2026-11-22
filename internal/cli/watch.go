package cli

import (
	"flag"
	"fmt"
	"time"

	"github.com/OWNER/agentlog/internal/agent"
)

func watchCmd(args []string, s streams) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	quiet := fs.Float64("quiet", agent.DefaultWatchQuiet.Seconds(), "seconds without new edits before auto-committing")
	interval := fs.Float64("interval", agent.DefaultWatchInterval.Seconds(), "seconds between checks")
	path, ok := parseArgs("watch", fs, args, s.err)
	if !ok {
		return exitUsage
	}
	if *quiet <= 0 || *interval <= 0 {
		fmt.Fprintln(s.err, "agentlog: watch: --quiet and --interval must be greater than 0")
		return exitUsage
	}
	repo, ok := openRepoArg("watch", path, s)
	if !ok {
		return exitError
	}
	fmt.Fprintf(s.out, "watching %s  (auto-commit after %gs with no new edits, Ctrl+C to stop)\n", repo.Root, *quiet)
	opts := agent.WatchOptions{
		Quiet:    time.Duration(*quiet * float64(time.Second)),
		Interval: time.Duration(*interval * float64(time.Second)),
	}
	if err := agent.Watch(s.context(), repo, opts, s.out, s.err); err != nil {
		fmt.Fprintf(s.err, "agentlog: watch: %v\n", err)
		return exitError
	}
	return exitOK
}

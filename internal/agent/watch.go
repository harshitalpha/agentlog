package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/harshitalpha/agentlog/internal/gitx"
)

// WatchOptions configure Watch.
type WatchOptions struct {
	Quiet    time.Duration // how long the tree must stay unchanged before committing
	Interval time.Duration // how often to look at the tree
}

// Default watch timings (docs/04-cli.md).
const (
	DefaultWatchQuiet    = 20 * time.Second
	DefaultWatchInterval = 2 * time.Second
)

// Watch is the fallback for agents without hooks. Every Interval it takes a
// signature of the uncommitted state; once a non-empty signature has stayed
// unchanged for Quiet, everything is auto-committed and one line is written
// to w. It ignores the enabled flag (running it is the opt-in) and returns
// nil when ctx is cancelled.
func Watch(ctx context.Context, repo *gitx.Repo, opts WatchOptions, w io.Writer, errW io.Writer) error {
	if opts.Interval <= 0 {
		opts.Interval = DefaultWatchInterval
	}
	if opts.Quiet <= 0 {
		opts.Quiet = DefaultWatchQuiet
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	var last string
	var since time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			sig, err := repo.WorkingSignature()
			if err != nil {
				fmt.Fprintf(errW, "agentlog: watch: %v\n", err)
				continue
			}
			switch {
			case sig == "":
				last = ""
			case sig != last:
				last, since = sig, now
			case now.Sub(since) >= opts.Quiet:
				subject, committed, err := repo.AutoCommit("edits")
				if err != nil {
					fmt.Fprintf(errW, "agentlog: watch: auto-commit failed: %v\n", err)
					since = now // retry after another quiet period, not every tick
					continue
				}
				if committed {
					fmt.Fprintf(w, "%s %s\n", now.Format("15:04:05"), subject)
				}
				last = ""
			}
		}
	}
}

package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"github.com/OWNER/agentlog/internal/agent"
	"github.com/OWNER/agentlog/internal/gitx"
)

// openRepoArg resolves [path] to a repo, printing the standard error.
func openRepoArg(name, path string, s streams) (*gitx.Repo, bool) {
	repo, err := gitx.Open(path)
	if errors.Is(err, gitx.ErrNotRepo) {
		fmt.Fprintf(s.err, "agentlog: %s: %s is not inside a git repo (run agentlog init first)\n", name, path)
		return nil, false
	}
	if err != nil {
		fmt.Fprintf(s.err, "agentlog: %s: %v\n", name, err)
		return nil, false
	}
	return repo, true
}

func statusCmd(args []string, s streams) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the status as JSON")
	path, ok := parseArgs("status", fs, args, s.err)
	if !ok {
		return exitUsage
	}
	repo, ok := openRepoArg("status", path, s)
	if !ok {
		return exitError
	}
	st, err := agent.ComputeStatus(repo, agent.OSEnv, runningUIURL(repo))
	if err != nil {
		fmt.Fprintf(s.err, "agentlog: status: %v\n", err)
		return exitError
	}
	if *asJSON {
		enc := json.NewEncoder(s.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st); err != nil {
			fmt.Fprintf(s.err, "agentlog: status: %v\n", err)
			return exitError
		}
		return exitOK
	}
	st.Print(s.out)
	return exitOK
}

// runningUIURL returns the URL of this repo's running UI server, or "".
// Wired to uiproc in T3.4.
func runningUIURL(*gitx.Repo) string { return "" }

// stopRunningUI stops this repo's UI server if one is running.
// Wired to uiproc in T3.4.
func stopRunningUI(*gitx.Repo) {}

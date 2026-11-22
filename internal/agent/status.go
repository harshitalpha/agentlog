package agent

import (
	"fmt"
	"io"

	"github.com/harshitalpha/agentlog/internal/gitx"
)

// How a repo came to be enabled.
const (
	EnabledViaRepo     = "repo"
	EnabledViaEnforce  = "enforce_everywhere"
	EnabledViaNotEnabl = ""
)

// CommitCounts splits commits since the baseline by who made them.
type CommitCounts struct {
	Agent int `json:"agent"`
	Human int `json:"human"`
	Auto  int `json:"auto"`
}

// Total is the number of commits counted.
func (c CommitCounts) Total() int { return c.Agent + c.Human + c.Auto }

// Status is the result of `agentlog status` (also its --json shape).
type Status struct {
	Repo        string       `json:"repo"`
	Enabled     bool         `json:"enabled"`
	EnabledVia  string       `json:"enabled_via"` // "repo", "enforce_everywhere" or ""
	Branch      string       `json:"branch"`
	HasBaseline bool         `json:"has_baseline"`
	Commits     CommitCounts `json:"commits_since_baseline"`
	Uncommitted int          `json:"uncommitted_files"`
	UIURL       string       `json:"ui_url"` // "" when the UI is not running
}

// ComputeStatus gathers the status of repo. uiURL is the URL of the running
// UI server, or "".
func ComputeStatus(repo *gitx.Repo, env Env, uiURL string) (Status, error) {
	st := Status{Repo: repo.Root, Branch: repo.Branch(), UIURL: uiURL}

	if v, ok, err := repo.GetLocal(ConfigKey); err == nil && ok && truthy(v) {
		st.Enabled, st.EnabledVia = true, EnabledViaRepo
	} else if EnforceEverywhere(env) {
		st.Enabled, st.EnabledVia = true, EnabledViaEnforce
	}

	files, err := repo.DirtyFiles()
	if err != nil {
		return st, err
	}
	st.Uncommitted = len(files)

	commits, err := repo.Log(gitx.MaxLogLimit)
	if err != nil {
		return st, err
	}
	st.Commits, st.HasBaseline = CountSinceBaseline(commits)
	return st, nil
}

// CountSinceBaseline counts commits newer than the baseline snapshot
// (newest-first input), classified with DetectAgent. Without a baseline commit
// every commit counts. init's own rules commit is not counted.
func CountSinceBaseline(commits []gitx.Commit) (counts CommitCounts, hasBaseline bool) {
	for _, c := range commits {
		if c.Subject == BaselineSubject {
			return counts, true
		}
		if c.Subject == RulesSubject {
			continue
		}
		switch a := DetectAgent(c.Author, c.Body); a {
		case "":
			counts.Human++
		case "auto":
			counts.Auto++
		default:
			counts.Agent++
		}
	}
	return counts, false
}

// Print writes the human-readable status.
func (s Status) Print(w io.Writer) {
	fmt.Fprintf(w, "agentlog status for %s\n", s.Repo)
	switch s.EnabledVia {
	case EnabledViaRepo:
		fmt.Fprintln(w, "  enabled:      yes (this repo)")
	case EnabledViaEnforce:
		fmt.Fprintln(w, "  enabled:      yes (enforce_everywhere is on)")
	default:
		fmt.Fprintln(w, "  enabled:      no (run /agentlog:init or: agentlog init)")
	}
	fmt.Fprintf(w, "  branch:       %s\n", s.Branch)
	total := fmt.Sprintf("%d", s.Commits.Total())
	if s.HasBaseline {
		total += " since baseline"
	}
	fmt.Fprintf(w, "  commits:      %s (%d agent, %d human, %d auto)\n", total, s.Commits.Agent, s.Commits.Human, s.Commits.Auto)
	fmt.Fprintf(w, "  uncommitted:  %d file%s\n", s.Uncommitted, plural(s.Uncommitted))
	if s.UIURL != "" {
		fmt.Fprintf(w, "  ui:           %s\n", s.UIURL)
	} else {
		fmt.Fprintln(w, "  ui:           not running (run /agentlog:ui or: agentlog ui)")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

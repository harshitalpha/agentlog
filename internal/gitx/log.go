package gitx

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Log limits (docs/06-git-layer.md).
const (
	DefaultLogLimit = 1000
	MaxLogLimit     = 5000
)

// Commit is one entry of the first-parent history.
type Commit struct {
	Hash    string    `json:"hash"`
	Short   string    `json:"short"`
	Parents []string  `json:"parents"`
	Author  string    `json:"author"`
	Email   string    `json:"email"`
	Date    time.Time `json:"date"` // author date
	Subject string    `json:"subject"`
	Body    string    `json:"body"` // trimmed
	Files   int       `json:"files"`
	Ins     int       `json:"ins"`
	Dels    int       `json:"dels"`
	Agent   string    `json:"agent"` // filled by the caller (agent.DetectAgent); "" = human
}

const logFormat = "%x1e%H%x1f%h%x1f%P%x1f%an%x1f%ae%x1f%aI%x1f%s%x1f%b%x1f"

// Log returns up to limit commits on the first-parent line, newest first,
// with line totals from numstat. It returns an empty slice when the repo has
// no commits. limit <= 0 means DefaultLogLimit; it is capped at MaxLogLimit.
func (r *Repo) Log(limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = DefaultLogLimit
	}
	if limit > MaxLogLimit {
		limit = MaxLogLimit
	}
	if !r.HasCommits() {
		return []Commit{}, nil
	}
	out, err := r.run("log", "-n"+strconv.Itoa(limit), "--first-parent", "-M", "--numstat",
		"--format="+logFormat)
	if err != nil {
		return nil, err
	}
	return parseLog(out)
}

func parseLog(out string) ([]Commit, error) {
	commits := []Commit{}
	records := strings.Split(out, "\x1e")
	for _, rec := range records[1:] { // first chunk is empty
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 9 {
			return nil, fmt.Errorf("gitx: malformed log record (%d fields)", len(parts))
		}
		date, err := time.Parse(time.RFC3339, parts[5])
		if err != nil {
			return nil, fmt.Errorf("gitx: bad commit date %q: %w", parts[5], err)
		}
		c := Commit{
			Hash:    parts[0],
			Short:   parts[1],
			Parents: strings.Fields(parts[2]),
			Author:  parts[3],
			Email:   parts[4],
			Date:    date,
			Subject: parts[6],
			Body:    strings.TrimSpace(parts[7]),
		}
		c.Files, c.Ins, c.Dels = parseNumstat(parts[8])
		commits = append(commits, c)
	}
	return commits, nil
}

// parseNumstat sums "ins<TAB>dels<TAB>path" lines. Binary files show "-" and
// count as 0 lines; every line counts as one file.
func parseNumstat(s string) (files, ins, dels int) {
	for _, line := range strings.Split(s, "\n") {
		cols := strings.SplitN(line, "\t", 3)
		if len(cols) < 3 {
			continue
		}
		files++
		if n, err := strconv.Atoi(cols[0]); err == nil {
			ins += n
		}
		if n, err := strconv.Atoi(cols[1]); err == nil {
			dels += n
		}
	}
	return files, ins, dels
}

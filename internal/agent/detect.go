package agent

import (
	"regexp"
	"strings"
)

var (
	agentTrailer = regexp.MustCompile(`(?im)^Agent:[ \t]*(.+?)[ \t]*$`)
	coAuthor     = regexp.MustCompile(`(?im)^Co-Authored-By:[ \t]*(.+?)[ \t]*$`)
)

// coAuthorLabels maps a substring of a Co-Authored-By value to a display
// label. Order matters: first match wins.
var coAuthorLabels = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)claude`), "Claude"},
	{regexp.MustCompile(`(?i)codex|openai`), "Codex"},
	{regexp.MustCompile(`(?i)cursor`), "Cursor"},
	{regexp.MustCompile(`(?i)copilot`), "Copilot"},
	{regexp.MustCompile(`(?i)gemini`), "Gemini"},
	{regexp.MustCompile(`(?i)aider`), "Aider"},
	{regexp.MustCompile(`(?i)devin`), "Devin"},
}

// AutoAuthor is the reserved author name of auto-commits.
const AutoAuthor = "agentlog"

// DetectAgent returns the display label of the agent that made a commit, or
// "" for a human commit. First match wins:
//
//  1. an "Agent: <name>" trailer → <name>
//  2. author name exactly "agentlog" → "auto"
//  3. a Co-Authored-By value naming a known agent → its label
//  4. otherwise "" (the author name is never used to guess an agent)
func DetectAgent(author, body string) string {
	if m := agentTrailer.FindStringSubmatch(body); m != nil {
		if v := strings.TrimSpace(m[1]); v != "" {
			return v
		}
	}
	if author == AutoAuthor {
		return "auto"
	}
	for _, m := range coAuthor.FindAllStringSubmatch(body, -1) {
		for _, c := range coAuthorLabels {
			if c.pattern.MatchString(m[1]) {
				return c.label
			}
		}
	}
	return ""
}

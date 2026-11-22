// Package agent holds the agent-facing logic: the commit rules text, the
// per-repo enabled check, the SessionStart and Stop hook handlers, the
// standalone settings merge and agent detection from commit trailers.
package agent

import (
	_ "embed"
	"errors"
	"os"
	"strings"
)

// Markers that wrap the rules block inside AGENTS.md / CLAUDE.md so it can be
// updated or removed without touching the user's own content.
const (
	StartMarker = "<!-- agentlog:start -->"
	EndMarker   = "<!-- agentlog:end -->"
)

//go:embed rules.md
var rulesText string

// Rules returns the commit-discipline rules without markers. It is used as
// SessionStart additionalContext.
func Rules() string { return strings.TrimRight(rulesText, "\n") }

// Block returns the rules wrapped in the start/end markers.
func Block() string { return StartMarker + "\n" + Rules() + "\n" + EndMarker }

// findBlock returns the byte range [start, end) of an existing marker block,
// including both markers. ok is false when there is no complete block.
func findBlock(text string) (start, end int, ok bool) {
	s := strings.Index(text, StartMarker)
	if s < 0 {
		return 0, 0, false
	}
	e := strings.Index(text[s:], EndMarker)
	if e < 0 {
		return 0, 0, false
	}
	return s, s + e + len(EndMarker), true
}

// UpsertBlock returns text with the rules block added or refreshed. An
// existing block is replaced in place; otherwise the block is appended after
// one blank line. Applying it twice yields the same text.
func UpsertBlock(text string) string {
	if s, e, ok := findBlock(text); ok {
		return text[:s] + Block() + text[e:]
	}
	if strings.TrimSpace(text) == "" {
		return Block() + "\n"
	}
	return strings.TrimRight(text, "\r\n") + "\n\n" + Block() + "\n"
}

// RemoveBlock returns text without the rules block and without the blank
// lines that surrounded it. Text with no block is returned unchanged.
func RemoveBlock(text string) string {
	s, e, ok := findBlock(text)
	if !ok {
		return text
	}
	before := strings.TrimRight(text[:s], "\r\n")
	after := strings.TrimLeft(text[e:], "\r\n")
	switch {
	case before == "" && after == "":
		return ""
	case before == "":
		return after
	case after == "":
		return before + "\n"
	}
	return before + "\n\n" + after
}

// UpsertRules adds or refreshes the rules block in the file at path, creating
// the file if needed. changed is false when the file already held the
// current block.
func UpsertRules(path string) (changed bool, err error) {
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	updated := UpsertBlock(string(old))
	if err == nil && updated == string(old) {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

// RemoveRules removes the rules block from the file at path. A file that
// becomes empty is deleted. removed is false when the file is missing or has
// no block.
func RemoveRules(path string) (removed bool, err error) {
	old, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, _, ok := findBlock(string(old)); !ok {
		return false, nil
	}
	updated := RemoveBlock(string(old))
	if strings.TrimSpace(updated) == "" {
		return true, os.Remove(path)
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

package agent

import (
	"fmt"
	"strings"
)

// maxListedFiles caps the file list in the block reason.
const maxListedFiles = 40

type stopBlock struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// HandleStop implements the Stop hook. It returns the single JSON line to
// print, or "" to allow Claude to stop.
//
//	not a repo / not enabled / clean tree  → ""            (allow stop)
//	dirty, stop_hook_active == false       → block JSON    (make Claude commit)
//	dirty, stop_hook_active == true        → auto-commit, "" (don't loop)
//
// The enabled check comes first and everything else is best-effort: errors
// are logged and swallowed so the hook can never break Claude.
func HandleStop(stdin []byte, env Env) string {
	in, ok := ParseHookInput(stdin)
	if !ok {
		return ""
	}
	repo := openRepo(in, env)
	if !IsEnabled(repo, env) { // also false for a nil repo
		return ""
	}
	if !acquireLock(repo, stopLockKey(in)) {
		return "" // an identical hook (plugin + standalone) is already handling this
	}
	files, err := repo.DirtyFiles()
	if err != nil {
		LogError(env, "hook-stop: reading status: %v", err)
		return ""
	}
	if len(files) == 0 {
		return ""
	}
	if in.StopHookActive {
		// Claude already got one nudge and still left changes. Blocking again
		// risks a loop, so save the work with a clearly labelled commit.
		if _, _, err := repo.AutoCommit("uncommitted changes"); err != nil {
			LogError(env, "hook-stop: auto-commit failed: %v", err)
		}
		return ""
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return marshalLine(stopBlock{Decision: "block", Reason: blockReason(paths)})
}

// blockReason is the instruction Claude receives when it tries to stop with
// uncommitted changes.
func blockReason(paths []string) string {
	var b strings.Builder
	b.WriteString("agentlog: you have uncommitted changes. Before finishing, commit them locally.\n")
	b.WriteString("Split them into one commit per logical step if they cover more than one concern\n")
	b.WriteString("(use `git add <paths>` per commit). Each subject should say what was done in plain\n")
	b.WriteString("words, e.g. \"Add repo interface and impl for product DB\". Do NOT push, amend or rebase.\n")
	b.WriteString("End each commit body with the trailer \"Agent: Claude\".\n\n")
	b.WriteString("Uncommitted files:\n")
	for i, p := range paths {
		if i == maxListedFiles {
			fmt.Fprintf(&b, "  … and %d more", len(paths)-maxListedFiles)
			break
		}
		fmt.Fprintf(&b, "  %s\n", p)
	}
	return strings.TrimRight(b.String(), "\n")
}

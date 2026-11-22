package agent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OWNER/agentlog/internal/gitx"
)

// InitOptions are the flags of `agentlog init`.
type InitOptions struct {
	NoAgentsMD     bool   // don't write the rules block into AGENTS.md
	ClaudeMD       bool   // also write it into CLAUDE.md
	NoBaseline     bool   // don't commit a baseline snapshot
	StandaloneHook bool   // also install hooks into .claude/settings.local.json
	ExePath        string // binary path for standalone hooks; "" = this executable
}

// Commit messages written by init.
const (
	BaselineSubject = "Baseline snapshot before agent work"
	baselineBody    = "Created by agentlog init so later agent commits diff against this state."
	RulesSubject    = "Add agentlog commit rules for AI agents"
)

// gitignoreLines are appended to .gitignore when missing. They keep secrets
// and the machine-specific hook settings out of the baseline snapshot.
var gitignoreLines = []string{".claude/settings.local.json", ".env", ".env.*"}

// Init prepares the project at path and enables enforcement. It is
// idempotent. Progress is written to w. It returns the repo it enabled.
func Init(path string, opts InitOptions, w io.Writer) (*gitx.Repo, error) {
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}

	repo, err := gitx.Open(path)
	switch {
	case errors.Is(err, gitx.ErrNotRepo):
		if err := refuseUnsafeInit(path); err != nil {
			return nil, err
		}
		if repo, err = gitx.Init(path); err != nil {
			return nil, err
		}
		fmt.Fprintln(w, "✓ initialised local git repo")
	case err != nil:
		return nil, err
	}

	if changed, err := repo.EnsureIdentity(); err != nil {
		return nil, err
	} else if changed {
		fmt.Fprintf(w, "  note: no git identity found, set repo-local user.name=%s (global config untouched)\n", gitx.DefaultName)
	}

	// Decide on the baseline before .gitignore is touched, so a clean repo
	// doesn't get a "baseline" holding only our own edits.
	needBaseline := false
	if !opts.NoBaseline {
		dirty, err := repo.DirtyFiles()
		if err != nil {
			return nil, err
		}
		needBaseline = !repo.HasCommits() || len(dirty) > 0
	}

	// .gitignore must be updated BEFORE the baseline `git add -A`, otherwise
	// .env files would be committed.
	touched := []string{}
	if changed, err := ensureGitignore(repo.Root); err != nil {
		return nil, err
	} else if changed {
		touched = append(touched, ".gitignore")
	}

	if needBaseline {
		if err := repo.AddAll(); err != nil {
			return nil, err
		}
		empty, err := repo.StagedEmpty()
		if err != nil {
			return nil, err
		}
		if !empty {
			if err := repo.Commit(BaselineSubject, baselineBody, gitx.CommitOpts{}); err != nil {
				return nil, err
			}
			fmt.Fprintln(w, "✓ committed baseline snapshot")
		}
	}

	if err := repo.SetLocal(ConfigKey, "true"); err != nil {
		return nil, err
	}

	names := []string{}
	if !opts.NoAgentsMD {
		names = append(names, "AGENTS.md")
	}
	if opts.ClaudeMD {
		names = append(names, "CLAUDE.md")
	}
	for _, name := range names {
		changed, err := UpsertRules(filepath.Join(repo.Root, name))
		if err != nil {
			return nil, err
		}
		if changed {
			touched = append(touched, name)
			fmt.Fprintf(w, "✓ wrote commit rules to %s\n", name)
		}
	}

	if opts.StandaloneHook {
		exe := opts.ExePath
		if exe == "" {
			if exe, err = executablePath(); err != nil {
				return nil, err
			}
		}
		switch err := InstallStandaloneHooks(repo.Root, exe); {
		case errors.Is(err, ErrInvalidSettings):
			fmt.Fprintln(w, settingsWarning(exe))
		case err != nil:
			return nil, err
		default:
			fmt.Fprintln(w, "✓ installed Claude Code hooks in .claude/settings.local.json")
		}
	}

	// Commit only the files init changed, never the user's other edits.
	if len(touched) > 0 {
		if _, err := repo.CommitPaths(RulesSubject, "", touched, gitx.CommitOpts{}); err != nil {
			return nil, err
		}
	}

	fmt.Fprintf(w, "✓ agentlog enabled in %s\n", repo.Root)
	fmt.Fprintln(w, "  Every change your agent makes will now be committed locally.")
	fmt.Fprintln(w, "  View history: /agentlog:ui   (or: agentlog ui)")
	return repo, nil
}

// refuseUnsafeInit stops `git init` + `git add -A` from running over a whole
// home directory or the filesystem root.
func refuseUnsafeInit(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs, _ = filepath.EvalSymlinks(abs)
	if abs == filepath.VolumeName(abs)+string(filepath.Separator) {
		return errors.New("refusing to create a git repo in the filesystem root; cd into a project folder")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if h, _ := filepath.EvalSymlinks(home); h != "" && h == abs {
			return errors.New("refusing to create a git repo in your home directory; cd into a project folder")
		}
	}
	return nil
}

// ensureGitignore appends any missing agentlog lines to .gitignore (created
// if needed). Existing content is preserved.
func ensureGitignore(root string) (changed bool, err error) {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(data), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var missing []string
	for _, l := range gitignoreLines {
		if !have[l] {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(missing, "\n") + "\n"
	return true, os.WriteFile(path, []byte(text), 0o644)
}

// executablePath is the absolute, symlink-free path of the running binary.
func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// Uninstall removes what standalone init added: the hooks in
// .claude/settings.local.json, the rules blocks in AGENTS.md and CLAUDE.md
// (deleting files that become empty), and the enabled flag. Commits and
// .gitignore are never touched. Problems with one step are reported on w and
// do not stop the others.
func Uninstall(repo *gitx.Repo, w io.Writer) error {
	switch removed, err := RemoveStandaloneHooks(repo.Root); {
	case errors.Is(err, ErrInvalidSettings):
		fmt.Fprintf(w, "  ! %s is not valid JSON, left untouched\n", filepath.Join(".claude", "settings.local.json"))
	case err != nil:
		return err
	case removed:
		fmt.Fprintln(w, "✓ removed hooks from .claude/settings.local.json")
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		removed, err := RemoveRules(filepath.Join(repo.Root, name))
		if err != nil {
			return err
		}
		if removed {
			fmt.Fprintf(w, "✓ removed commit rules from %s\n", name)
		}
	}
	return repo.UnsetLocal(ConfigKey)
}

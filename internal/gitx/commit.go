package gitx

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

// Reserved identity used for auto-commits. It is passed with `git -c`, never
// written to any config, so the user's identity is never touched.
var autoIdentity = Identity{Name: "agentlog", Email: "agentlog@localhost"}

const (
	maxAutoSubject = 100
	autoBody       = "Committed automatically by agentlog because the agent left changes uncommitted.\n\nAgent: auto"
)

// CommitOpts tweaks a single Commit call.
type CommitOpts struct {
	Identity *Identity // override author/committer via `git -c` (nil = user's config)
	NoVerify bool      // skip pre-commit and commit-msg hooks
	NoSign   bool      // force commit.gpgsign=false for this commit
}

// AddAll stages everything, including deletions and untracked files
// (`git add -A`, which respects .gitignore).
func (r *Repo) AddAll() error {
	_, err := r.run("add", "-A")
	return err
}

// StagedEmpty reports whether nothing is staged (`diff --cached --quiet`).
func (r *Repo) StagedEmpty() (bool, error) {
	_, code, err := r.runCode("diff", "--cached", "--quiet")
	if err != nil {
		return false, err
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	}
	return false, &Error{Args: "diff --cached --quiet", Code: code}
}

// Commit commits what is staged. body may be empty.
func (r *Repo) Commit(subject, body string, opts CommitOpts) error {
	var args []string
	if opts.Identity != nil {
		args = append(args, "-c", "user.name="+opts.Identity.Name, "-c", "user.email="+opts.Identity.Email)
	}
	if opts.NoSign {
		args = append(args, "-c", "commit.gpgsign=false")
	}
	args = append(args, "commit", "-q")
	if opts.NoVerify {
		args = append(args, "--no-verify")
	}
	args = append(args, "-m", subject)
	if body != "" {
		args = append(args, "-m", body)
	}
	_, err := r.run(args...)
	return err
}

// AutoCommit stages everything and commits it as the reserved `agentlog`
// identity with subject "[auto] <label>: a.go, b.go, c.go (+N more)". It does
// nothing (committed=false) when there is nothing to commit. Signing and
// hooks are disabled so the safety net cannot fail on the user's setup.
func (r *Repo) AutoCommit(label string) (subject string, committed bool, err error) {
	files, err := r.DirtyFiles()
	if err != nil {
		return "", false, err
	}
	if len(files) == 0 {
		return "", false, nil
	}
	if err := r.AddAll(); err != nil {
		return "", false, err
	}
	empty, err := r.StagedEmpty()
	if err != nil || empty {
		return "", false, err
	}
	subject = autoSubject(label, files)
	id := autoIdentity
	if err := r.Commit(subject, autoBody, CommitOpts{Identity: &id, NoVerify: true, NoSign: true}); err != nil {
		return "", false, err
	}
	return subject, true, nil
}

// autoSubject builds "[auto] label: n1, n2, n3 (+N more)" from base names,
// truncated to 100 characters.
func autoSubject(label string, files []DirtyFile) string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = path.Base(f.Path)
	}
	shown := strings.Join(names[:min(3, len(names))], ", ")
	if len(names) > 3 {
		shown += fmt.Sprintf(" (+%d more)", len(names)-3)
	}
	s := fmt.Sprintf("[auto] %s: %s", label, shown)
	if utf8.RuneCountInString(s) > maxAutoSubject {
		s = string([]rune(s)[:maxAutoSubject])
	}
	return s
}

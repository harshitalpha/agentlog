package gitx

import (
	"os"
	"path/filepath"
	"strings"
)

// Open resolves the repository containing path (any folder inside the repo).
// It returns ErrNotRepo when path is not inside a git repository.
func Open(path string) (*Repo, error) {
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return nil, ErrNotRepo
	}
	res, err := execGit(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	if res.code != 0 {
		return nil, ErrNotRepo
	}
	root := strings.TrimSpace(string(res.stdout))
	if root == "" {
		return nil, ErrNotRepo
	}
	return &Repo{Root: root}, nil
}

// Init runs `git init` in path and returns the new repo.
func Init(path string) (*Repo, error) {
	res, err := execGit(path, "init", "-q")
	if err != nil {
		return nil, err
	}
	if res.code != 0 {
		return nil, &Error{Args: "init", Code: res.code, Stderr: res.stderr}
	}
	return Open(path)
}

// HasCommits reports whether HEAD points at a commit.
func (r *Repo) HasCommits() bool {
	_, code, err := r.runCode("rev-parse", "--verify", "-q", "HEAD")
	return err == nil && code == 0
}

// Head returns the full hash of HEAD, or "" when there are no commits.
func (r *Repo) Head() (string, error) {
	if !r.HasCommits() {
		return "", nil
	}
	out, err := r.run("rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

// Branch returns the current branch name. It works before the first commit
// (symbolic-ref) and falls back to "HEAD" when detached.
func (r *Repo) Branch() string {
	out, code, err := r.runCode("symbolic-ref", "--short", "-q", "HEAD")
	if err == nil && code == 0 {
		if b := strings.TrimSpace(out); b != "" {
			return b
		}
	}
	return "HEAD"
}

// GitPath returns the absolute path of rel inside the repo's git directory
// (e.g. "agentlog" → <repo>/.git/agentlog), correct for worktrees too.
func (r *Repo) GitPath(rel string) (string, error) {
	out, err := r.run("rev-parse", "--git-path", rel)
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(out)
	if p == "" {
		return "", ErrNotRepo
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Root, p)
	}
	return p, nil
}

package gitx

import (
	"os"
	"path/filepath"
	"strings"
)

// emptyTree is the well-known hash of git's empty tree, the diff base for a
// repo with no commits.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// Limits for the working-tree patch (docs/06-git-layer.md).
const (
	maxUntrackedFiles = 200
	maxUntrackedBytes = 1 << 20 // 1 MB
)

// CommitPatch returns the unified patch of one commit against its first
// parent (root commits show every file as added). An unknown commit returns
// ErrNotFound.
func (r *Repo) CommitPatch(hash string) (string, error) {
	if _, code, err := r.runCode("rev-parse", "--verify", "-q", "--end-of-options", hash+"^{commit}"); err != nil {
		return "", err
	} else if code != 0 {
		return "", ErrNotFound
	}
	return r.run("show", "--format=", "--patch", "-M", "--no-color", "--first-parent",
		"--no-ext-diff", "--end-of-options", hash)
}

// WorkingPatch returns the unified patch of everything uncommitted: tracked
// changes against HEAD (or the empty tree), plus each untracked, non-ignored
// file shown as added. At most 200 untracked files are included, and files
// over 1 MB are skipped.
func (r *Repo) WorkingPatch() (string, error) {
	base := emptyTree
	if r.HasCommits() {
		base = "HEAD"
	}
	patch, err := r.run("diff", base, "--patch", "-M", "--no-color", "--no-ext-diff")
	if err != nil {
		return "", err
	}

	out, err := r.run("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(patch)
	count := 0
	for _, f := range strings.Split(out, "\x00") {
		if f == "" {
			continue
		}
		if count >= maxUntrackedFiles {
			break
		}
		fi, err := os.Lstat(filepath.Join(r.Root, filepath.FromSlash(f)))
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxUntrackedBytes {
			continue
		}
		// `diff --no-index` exits 1 when the files differ; that is success here.
		text, code, err := r.runCode("diff", "--no-index", "--no-color", "--no-ext-diff",
			"--", "/dev/null", f)
		if err != nil {
			return "", err
		}
		if code > 1 {
			continue
		}
		b.WriteString(text)
		count++
	}
	return b.String(), nil
}

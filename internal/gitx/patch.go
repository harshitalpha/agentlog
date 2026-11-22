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

// diffFlags pin the patch format so the UI's parser sees the same headers on
// every machine, whatever the user's git config says: no external diff or
// textconv programs, and a/ and b/ prefixes even if diff.noprefix or
// diff.mnemonicPrefix (which yields i/, w/, c/) is set.
var diffFlags = []string{"--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/"}

// CommitPatch returns the unified patch of one commit against its first
// parent (root commits show every file as added). An unknown commit returns
// ErrNotFound.
func (r *Repo) CommitPatch(hash string) (string, error) {
	if _, code, err := r.runCode("rev-parse", "--verify", "-q", "--end-of-options", hash+"^{commit}"); err != nil {
		return "", err
	} else if code != 0 {
		return "", ErrNotFound
	}
	args := append([]string{"show", "--format=", "--patch", "-M", "--no-color", "--first-parent"}, diffFlags...)
	return r.run(append(args, "--end-of-options", hash)...)
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
	args := append([]string{"diff", base, "--patch", "-M", "--no-color"}, diffFlags...)
	patch, err := r.run(args...)
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
		args := append([]string{"diff", "--no-index", "--no-color"}, diffFlags...)
		text, code, err := r.runCode(append(args, "--", "/dev/null", f)...)
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

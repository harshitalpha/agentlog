package gitx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirtyFile is one entry of `git status`: a changed or untracked path.
type DirtyFile struct {
	Path   string // repo-relative, slash-separated; the new path for renames
	Status string // two-letter porcelain code, e.g. " M", "??", "R "
}

// DirtyFiles lists every changed, staged or untracked (not ignored) file.
func (r *Repo) DirtyFiles() ([]DirtyFile, error) {
	out, err := r.run("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parsePorcelainZ(out), nil
}

// parsePorcelainZ parses NUL-separated porcelain v1 output. Each entry is
// "XY <path>"; for renames and copies it is followed by one extra field with
// the original path, which is skipped.
func parsePorcelainZ(out string) []DirtyFile {
	var files []DirtyFile
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		code, path := f[:2], f[3:]
		files = append(files, DirtyFile{Path: path, Status: code})
		if code[0] == 'R' || code[0] == 'C' || code[1] == 'R' || code[1] == 'C' {
			i++ // skip the original path
		}
	}
	return files
}

// maxSignatureFiles bounds how many dirty files are stat'ed for a signature.
const maxSignatureFiles = 500

// WorkingSignature returns a string that changes whenever the uncommitted
// state changes, and is "" when the tree is clean. It combines the status
// listing, `git diff --stat` and each dirty file's size and mtime, so editing
// a file again is noticed even when the diff stat stays the same.
func (r *Repo) WorkingSignature() (string, error) {
	files, err := r.DirtyFiles()
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	stat, err := r.run("diff", "--stat", "--no-color", "--no-ext-diff")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, f := range files {
		fmt.Fprintf(&b, "%s %s", f.Status, f.Path)
		if i < maxSignatureFiles {
			if fi, err := os.Lstat(filepath.Join(r.Root, filepath.FromSlash(f.Path))); err == nil {
				fmt.Fprintf(&b, " %d %d", fi.Size(), fi.ModTime().UnixNano())
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString(stat)
	return b.String(), nil
}

package gitx

import (
	"errors"
	"strconv"
	"strings"
)

// Limits and errors for FileAt.
const (
	maxFileBytes = 2 << 20 // 2 MB
	binarySniff  = 8 << 10 // look for NUL in the first 8 KB
)

var (
	// ErrBinary means the file looks binary (NUL byte in its first 8 KB).
	ErrBinary = errors.New("binary file")
	// ErrNotFound means the commit or path does not exist.
	ErrNotFound = errors.New("not found")
	// ErrTooLarge means the file exceeds the 2 MB display cap.
	ErrTooLarge = errors.New("file too large")
)

// FileAt returns the content of path as it was at commit rev. It returns
// ErrNotFound for unknown revisions, missing paths or directories, ErrTooLarge
// above 2 MB and ErrBinary for binary content.
func (r *Repo) FileAt(rev, path string) (string, error) {
	spec := rev + ":" + path
	sizeOut, code, err := r.runCode("cat-file", "-s", "--end-of-options", spec)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", ErrNotFound
	}
	size, err := strconv.Atoi(strings.TrimSpace(sizeOut))
	if err != nil {
		return "", ErrNotFound
	}
	if size > maxFileBytes {
		return "", ErrTooLarge
	}
	content, code, err := r.runCode("cat-file", "blob", spec) // fails for directories
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", ErrNotFound
	}
	if sniff := content; strings.IndexByte(sniffPrefix(sniff), 0) >= 0 {
		return "", ErrBinary
	}
	return content, nil
}

func sniffPrefix(s string) string {
	if len(s) > binarySniff {
		return s[:binarySniff]
	}
	return s
}

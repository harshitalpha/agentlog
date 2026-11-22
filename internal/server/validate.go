package server

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// workingHash is the pseudo-hash for the uncommitted working tree.
const workingHash = "WORKING"

var hashRe = regexp.MustCompile(`^[0-9a-f]{4,64}$`)

// validHash accepts a lowercase hex commit hash (4-64 chars) or WORKING.
func validHash(h string) bool {
	return h == workingHash || hashRe.MatchString(h)
}

// cleanRepoPath validates a repo-relative path from a query string and
// returns its cleaned, slash-separated form. It rejects empty paths, a
// leading "-" (so it can never look like a git flag), NUL bytes, absolute
// paths and anything that climbs out of the repo.
func cleanRepoPath(p string) (string, bool) {
	if p == "" || strings.HasPrefix(p, "-") || strings.ContainsRune(p, 0) {
		return "", false
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || filepath.IsAbs(p) {
		return "", false
	}
	c := path.Clean(strings.ReplaceAll(p, `\`, "/"))
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || strings.HasPrefix(c, "-") {
		return "", false
	}
	return c, true
}

// allowedHost reports whether a Host header is exactly 127.0.0.1:<port> or
// localhost:<port>. Rejecting everything else blocks DNS-rebinding attacks,
// where a web page makes the browser talk to this server under another name.
func allowedHost(host string, port int) bool {
	h := strings.ToLower(host)
	return h == fmt.Sprintf("127.0.0.1:%d", port) || h == fmt.Sprintf("localhost:%d", port)
}

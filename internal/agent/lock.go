package agent

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OWNER/agentlog/internal/gitx"
)

// lockTTL is how long a hook invocation shadows an identical one. It dedupes
// the case where both the plugin and the standalone install fire the same hook.
const lockTTL = 5 * time.Second

// staleLockAge is when old lock files are swept away.
const staleLockAge = time.Minute

// sanitizeID makes a session id safe to use inside a file name.
func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

// acquireLock reports whether this invocation should proceed. The first
// caller for a given key wins; identical callers within lockTTL get false.
// Locks live in .git/agentlog/ and are created atomically. Any filesystem
// problem or an empty key means "proceed": deduping is best-effort and must
// never stop the hook from doing its job.
func acquireLock(repo *gitx.Repo, key string) bool {
	if key == "" {
		return true
	}
	dir, err := repo.GitPath("agentlog")
	if err != nil {
		return true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return true
	}
	sweepStaleLocks(dir)
	path := filepath.Join(dir, key)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return true
		}
		if !os.IsExist(err) {
			return true
		}
		fi, err := os.Stat(path)
		if err != nil {
			continue // vanished between the two calls; try to create it again
		}
		if time.Since(fi.ModTime()) < lockTTL {
			return false
		}
		_ = os.Remove(path) // expired: take it over on the next loop
	}
	return true
}

func sweepStaleLocks(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "stop-") && !strings.HasPrefix(e.Name(), "start-") {
			continue
		}
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > staleLockAge {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// stopLockKey identifies one Stop invocation. The stop_hook_active flag is
// part of the key so Claude's genuine second attempt (active=true) is never
// mistaken for a duplicate of the first (active=false).
func stopLockKey(in HookInput) string {
	if in.SessionID == "" {
		return ""
	}
	active := "0"
	if in.StopHookActive {
		active = "1"
	}
	return "stop-" + sanitizeID(in.SessionID) + "-" + active
}

func startLockKey(in HookInput) string {
	if in.SessionID == "" {
		return ""
	}
	return "start-" + sanitizeID(in.SessionID)
}

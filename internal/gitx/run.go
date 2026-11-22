// Package gitx is the only code in agentlog that runs git. It shells out to
// the git CLI with explicit argument slices (never through a shell) and
// returns typed results.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// callTimeout bounds every git invocation.
const callTimeout = 15 * time.Second

// ErrNotRepo is returned by Open when the path is not inside a git repo.
var ErrNotRepo = errors.New("not a git repository")

// Repo is an opened git repository. Root is the top-level directory.
type Repo struct {
	Root string
}

// result is the raw outcome of one git call.
type result struct {
	stdout []byte
	stderr string
	code   int // process exit code; -1 if git could not be started
}

// execGit runs `git -C dir args...` and returns the raw result. The error is
// non-nil only when git could not run at all (missing binary, timeout).
// A non-zero exit code is reported in result.code, not as an error.
func execGit(dir string, args ...string) (result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	res := result{stdout: stdout.Bytes(), stderr: strings.TrimSpace(stderr.String())}
	if err == nil {
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		res.code = exitErr.ExitCode()
		return res, nil
	}
	res.code = -1
	if ctx.Err() != nil {
		return res, fmt.Errorf("git %s: timed out after %s", args[0], callTimeout)
	}
	return res, fmt.Errorf("git %s: %w", args[0], err)
}

// run executes git in the repo root and returns stdout. A non-zero exit is an
// error that includes stderr.
func (r *Repo) run(args ...string) (string, error) {
	res, err := execGit(r.Root, args...)
	if err != nil {
		return "", err
	}
	if res.code != 0 {
		return "", fmt.Errorf("git %s: exit %d: %s", strings.Join(args, " "), res.code, res.stderr)
	}
	return string(res.stdout), nil
}

// runCode executes git and returns stdout plus the exit code, treating any
// exit code as non-fatal. Used for commands where the code carries meaning.
func (r *Repo) runCode(args ...string) (string, int, error) {
	res, err := execGit(r.Root, args...)
	if err != nil {
		return "", -1, err
	}
	return string(res.stdout), res.code, nil
}

// Error is a failed git invocation carrying its stderr.
type Error struct {
	Args   string
	Code   int
	Stderr string
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: exit %d: %s", e.Args, e.Code, e.Stderr)
}

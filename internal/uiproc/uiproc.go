package uiproc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/harshitalpha/agentlog/internal/gitx"
)

// File names inside .git/agentlog/.
const (
	stateFile = "ui.json"
	logFile   = "ui.log"
)

// Timing from docs/04-cli.md.
const (
	DefaultStartWait = 3 * time.Second
	stopGrace        = 3 * time.Second
	probeTimeout     = time.Second
)

// State is the content of .git/agentlog/ui.json.
type State struct {
	PID     int       `json:"pid"`
	Port    int       `json:"port"`
	URL     string    `json:"url"`
	Started time.Time `json:"started"`
}

func itoa(n int) string { return strconv.Itoa(n) }

func agentlogDir(repo *gitx.Repo) (string, error) { return repo.GitPath("agentlog") }

// ReadState loads ui.json. ok is false when it is missing or unreadable.
func ReadState(repo *gitx.Repo) (st State, ok bool) {
	dir, err := agentlogDir(repo)
	if err != nil {
		return State{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return State{}, false
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, false
	}
	return st, true
}

// writeState saves ui.json atomically.
func writeState(repo *gitx.Repo, st State) error {
	dir, err := agentlogDir(repo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, stateFile+".tmp-")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, stateFile))
}

func removeState(repo *gitx.Repo) {
	if dir, err := agentlogDir(repo); err == nil {
		_ = os.Remove(filepath.Join(dir, stateFile))
	}
}

// serving reports whether url answers /api/repo with this repo's path. It is
// what proves a pid really is our server (pids get reused).
func serving(url, repoRoot string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/repo", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var info struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false
	}
	return samePath(info.Path, repoRoot)
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// Running returns the live UI server for repo, if any: ui.json must exist,
// its pid must be alive and /api/repo at its URL must report this repo.
func Running(repo *gitx.Repo) (State, bool) {
	st, ok := ReadState(repo)
	if !ok || st.URL == "" || !pidAlive(st.PID) {
		return State{}, false
	}
	if !serving(st.URL, repo.Root) {
		return State{}, false
	}
	return st, true
}

// DetachOptions configure Detach.
type DetachOptions struct {
	Exe  string        // binary to re-exec; "" = this executable
	Port int           // first port to try (the child falls back across 10 ports)
	Wait time.Duration // how long to wait for the child to answer; 0 = 3s
}

var urlLine = regexp.MustCompile(`→ (http://127\.0\.0\.1:(\d+))`)

// Detach returns the running UI server for repo (reused=true) or starts one in
// the background: the binary re-executes itself as `ui --no-open --port N
// <repo>` in its own session with stdout/stderr in .git/agentlog/ui.log. It
// waits for the child to answer, records ui.json and returns its state.
func Detach(repo *gitx.Repo, opts DetachOptions) (st State, reused bool, err error) {
	if st, ok := Running(repo); ok {
		return st, true, nil
	}
	removeState(repo) // anything left over is stale

	dir, err := agentlogDir(repo)
	if err != nil {
		return State{}, false, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, false, err
	}
	exe := opts.Exe
	if exe == "" {
		if exe, err = os.Executable(); err != nil {
			return State{}, false, err
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
	}
	wait := opts.Wait
	if wait <= 0 {
		wait = DefaultStartWait
	}

	logPath := filepath.Join(dir, logFile)
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return State{}, false, err
	}
	cmd := exec.Command(exe, "ui", "--no-open", "--port", itoa(opts.Port), repo.Root)
	cmd.Stdout, cmd.Stderr = lf, lf
	cmd.SysProcAttr = detachAttr()
	if err := cmd.Start(); err != nil {
		lf.Close()
		return State{}, false, fmt.Errorf("starting %s: %w", exe, err)
	}
	lf.Close()
	pid := cmd.Process.Pid
	// Reap the child if it ends while we are still running (tests, long-lived
	// callers). When this process exits first, init adopts and reaps it.
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(logPath); err == nil {
			if m := urlLine.FindStringSubmatch(string(data)); m != nil && serving(m[1], repo.Root) {
				port, _ := strconv.Atoi(m[2])
				st = State{PID: pid, Port: port, URL: m[1], Started: time.Now().UTC().Truncate(time.Second)}
				if err := writeState(repo, st); err != nil {
					_ = terminate(pid, stopGrace)
					return State{}, false, err
				}
				return st, false, nil
			}
		}
		select {
		case <-exited:
			return State{}, false, fmt.Errorf("the UI server exited immediately: %s", lastLogLine(logPath))
		case <-time.After(40 * time.Millisecond):
		}
	}
	_ = terminate(pid, stopGrace)
	return State{}, false, fmt.Errorf("the UI server did not start within %s (see %s): %s", wait, logPath, lastLogLine(logPath))
}

func lastLogLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "no log"
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	return "log is empty"
}

// StopResult says what Stop did.
type StopResult int

const (
	// NotRunning: nothing was running (stale or missing ui.json was cleaned up).
	NotRunning StopResult = iota
	// Stopped: the server was running and has been terminated.
	Stopped
	// NotOurs: ui.json named a live process that is not this repo's agentlog
	// server (pid reuse). It was NOT signalled; the stale file was removed.
	NotOurs
)

// Stop terminates this repo's background UI server and deletes ui.json. It
// only signals a process after confirming it serves this repo, so a stale or
// corrupt ui.json can never get an unrelated process killed.
func Stop(repo *gitx.Repo) (StopResult, error) {
	st, ok := ReadState(repo)
	if !ok {
		removeState(repo)
		return NotRunning, nil
	}
	defer removeState(repo)
	if !pidAlive(st.PID) {
		return NotRunning, nil
	}
	if st.URL == "" || !serving(st.URL, repo.Root) {
		return NotOurs, nil
	}
	if err := terminate(st.PID, stopGrace); err != nil {
		return Stopped, err
	}
	if pidAlive(st.PID) {
		return Stopped, errors.New("the UI server did not exit")
	}
	return Stopped, nil
}

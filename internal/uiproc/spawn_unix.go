//go:build !windows

package uiproc

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// detachAttr starts the child in its own session so it survives the terminal
// (and Claude Code's shell) going away.
func detachAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// pidAlive reports whether a process with this pid exists. Pids <= 1 are never
// considered alive: kill(0) and kill(-1) would address whole groups.
func pidAlive(pid int) bool {
	if pid <= 1 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// terminate asks the process to exit (SIGTERM), waits up to grace, then kills it.
func terminate(pid int, grace time.Duration) error {
	if pid <= 1 {
		return errors.New("refusing to signal pid " + itoa(pid))
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
	return nil
}

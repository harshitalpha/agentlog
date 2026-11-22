//go:build windows

package uiproc

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	stillActive           = 259
	processQueryLimited   = 0x1000
)

// detachAttr starts the child detached from the console and in its own
// process group so closing the terminal does not stop it.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}

// pidAlive reports whether a process with this pid is still running.
func pidAlive(pid int) bool {
	if pid <= 1 {
		return false
	}
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// terminate kills the process and waits up to grace for it to disappear.
func terminate(pid int, grace time.Duration) error {
	if pid <= 1 {
		return errors.New("refusing to signal pid " + itoa(pid))
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = p.Kill()
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return nil
}

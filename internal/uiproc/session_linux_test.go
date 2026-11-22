//go:build linux

package uiproc

import "syscall"

// The syscall package has no Getsid on Linux (it does on macOS), so call it directly.
func sessionID(pid int) (int, error) {
	sid, _, errno := syscall.RawSyscall(syscall.SYS_GETSID, uintptr(pid), 0, 0)
	if errno != 0 {
		return 0, errno
	}
	return int(sid), nil
}

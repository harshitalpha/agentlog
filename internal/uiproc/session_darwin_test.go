//go:build darwin

package uiproc

import "syscall"

func sessionID(pid int) (int, error) { return syscall.Getsid(pid) }

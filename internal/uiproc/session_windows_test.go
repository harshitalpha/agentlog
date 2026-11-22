//go:build windows

package uiproc

import "errors"

func sessionID(pid int) (int, error) { return 0, errors.New("not supported on windows") }

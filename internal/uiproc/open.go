// Package uiproc manages the UI server process: opening the browser and,
// for --detach/--stop, reusing, spawning and stopping a background server
// tracked in .git/agentlog/ui.json.
package uiproc

import (
	"os/exec"
	"runtime"
)

// OpenURL opens url in the user's default browser. It is a variable so tests
// can replace it; the default starts the platform's opener and returns
// without waiting for it.
var OpenURL = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap the opener
	return nil
}

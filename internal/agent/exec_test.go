package agent

import "os/exec"

func execCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

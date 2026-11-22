package gitx

import "strings"

// GetLocal reads a repo-local config key. ok is false when it is not set.
func (r *Repo) GetLocal(key string) (value string, ok bool, err error) {
	out, code, err := r.runCode("config", "--local", "--get", key)
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		return strings.TrimSpace(out), true, nil
	case 1: // key not set
		return "", false, nil
	}
	return "", false, &Error{Args: "config --get " + key, Code: code}
}

// SetLocal writes a repo-local config key.
func (r *Repo) SetLocal(key, value string) error {
	_, err := r.run("config", "--local", key, value)
	return err
}

// UnsetLocal removes a repo-local config key. Removing an unset key is not an error.
func (r *Repo) UnsetLocal(key string) error {
	_, code, err := r.runCode("config", "--local", "--unset", key)
	if err != nil {
		return err
	}
	if code != 0 && code != 5 { // 5 = key was not set
		return &Error{Args: "config --unset " + key, Code: code}
	}
	return nil
}

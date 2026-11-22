package gitx

// Identity is a git author/committer identity.
type Identity struct {
	Name  string
	Email string
}

// Fallback identity used by init when the user has none configured.
const (
	DefaultName  = "local-dev"
	DefaultEmail = "local-dev@localhost"
)

// EnsureIdentity makes sure user.name and user.email resolve (any scope). If
// either is missing it sets a repo-local value and reports changed=true. The
// global config is never touched.
func (r *Repo) EnsureIdentity() (changed bool, err error) {
	for _, kv := range [][2]string{{"user.name", DefaultName}, {"user.email", DefaultEmail}} {
		out, code, err := r.runCode("config", "--get", kv[0])
		if err != nil {
			return changed, err
		}
		if code == 0 && len(out) > 1 {
			continue
		}
		if err := r.SetLocal(kv[0], kv[1]); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

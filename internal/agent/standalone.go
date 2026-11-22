package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidSettings means .claude/settings.local.json is not valid JSON and
// was left untouched.
var ErrInvalidSettings = errors.New("settings file is not valid JSON")

// SettingsPath is the standalone hook file inside a repo.
func SettingsPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "settings.local.json")
}

// hookSpec is one hook agentlog installs.
type hookSpec struct {
	Event   string
	Arg     string
	Timeout int
}

var standaloneHooks = []hookSpec{
	{"SessionStart", "hook-session-start", 10},
	{"Stop", "hook-stop", 30},
}

// StandaloneCommand is the hook command line for the binary at exe: the
// absolute path, quoted, plus the hook subcommand.
func StandaloneCommand(exe, hook string) string {
	return `"` + exe + `" ` + hook
}

// Snippet returns the JSON users can paste into their settings by hand.
func Snippet(exe string) string {
	hooks := map[string]any{}
	for _, h := range standaloneHooks {
		hooks[h.Event] = []any{map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": StandaloneCommand(exe, h.Arg), "timeout": h.Timeout,
		}}}}
	}
	b, _ := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	return string(b)
}

// isAgentlogHook matches hook commands that agentlog installed (here or in an
// older version): they contain "agentlog" and " hook-".
func isAgentlogHook(command string) bool {
	return strings.Contains(command, "agentlog") && strings.Contains(command, " hook-")
}

// readSettings loads the settings object. A missing or empty file is an empty
// object; invalid JSON returns ErrInvalidSettings.
func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep numbers exactly as the user wrote them
	var v map[string]any
	if err := dec.Decode(&v); err != nil || v == nil {
		return nil, ErrInvalidSettings
	}
	if dec.More() {
		return nil, ErrInvalidSettings
	}
	return v, nil
}

func writeSettings(path string, v map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// stripAgentlogHooks removes agentlog's hook entries from the settings'
// "hooks" object, drops groups left empty and prunes empty events and an
// empty "hooks" key. It reports whether anything was removed.
func stripAgentlogHooks(settings map[string]any) (removed bool) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}
	for event, raw := range hooks {
		groups, ok := raw.([]any)
		if !ok {
			continue
		}
		var keptGroups []any
		for _, g := range groups {
			group, ok := g.(map[string]any)
			if !ok {
				keptGroups = append(keptGroups, g)
				continue
			}
			list, ok := group["hooks"].([]any)
			if !ok {
				keptGroups = append(keptGroups, g)
				continue
			}
			var kept []any
			for _, h := range list {
				if hm, ok := h.(map[string]any); ok {
					if cmd, _ := hm["command"].(string); isAgentlogHook(cmd) {
						removed = true
						continue
					}
				}
				kept = append(kept, h)
			}
			if len(kept) == 0 && len(list) > 0 {
				continue // group became empty because of us: drop it
			}
			if len(kept) != len(list) {
				group["hooks"] = kept
			}
			keptGroups = append(keptGroups, group)
		}
		if len(keptGroups) == 0 && len(groups) > 0 {
			delete(hooks, event)
		} else if len(keptGroups) != len(groups) {
			hooks[event] = keptGroups
		}
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	return removed
}

// InstallStandaloneHooks merges agentlog's SessionStart and Stop hooks into
// <repo>/.claude/settings.local.json for the binary at exe. Every other key
// and hook is preserved, and any older agentlog hooks are replaced rather
// than duplicated. If the file is not valid JSON it is not touched and
// ErrInvalidSettings is returned (callers print Snippet).
func InstallStandaloneHooks(repoRoot, exe string) error {
	path := SettingsPath(repoRoot)
	settings, err := readSettings(path)
	if err != nil {
		return err
	}
	stripAgentlogHooks(settings)

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, h := range standaloneHooks {
		groups, _ := hooks[h.Event].([]any)
		groups = append(groups, map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": StandaloneCommand(exe, h.Arg), "timeout": h.Timeout,
		}}})
		hooks[h.Event] = groups
	}
	settings["hooks"] = hooks
	return writeSettings(path, settings)
}

// RemoveStandaloneHooks removes agentlog's hooks from the settings file and
// keeps everything else. A file that ends up as an empty object is deleted.
// A missing file or one without agentlog hooks is a no-op (removed=false);
// invalid JSON returns ErrInvalidSettings and is left untouched.
func RemoveStandaloneHooks(repoRoot string) (removed bool, err error) {
	path := SettingsPath(repoRoot)
	settings, err := readSettings(path)
	if err != nil {
		return false, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return false, nil
	}
	if !stripAgentlogHooks(settings) {
		return false, nil
	}
	if len(settings) == 0 {
		if err := os.Remove(path); err != nil {
			return true, err
		}
		_ = os.Remove(filepath.Dir(path)) // only succeeds when .claude is now empty
		return true, nil
	}
	return true, writeSettings(path, settings)
}

// settingsWarning is printed when the settings file can't be edited.
func settingsWarning(exe string) string {
	return fmt.Sprintf("  ! %s is not valid JSON, left untouched. Add this by hand:\n%s",
		filepath.Join(".claude", "settings.local.json"), Snippet(exe))
}

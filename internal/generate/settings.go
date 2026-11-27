package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	projectconfig "github.com/silver2dream/ai-workflow-kit/internal/config"
)

// The local settings file of Claude Code (.claude/settings.local.json) is the
// person's own: Claude Code adds to it every time they allow a command for
// good. The generator used to write the whole file, so every `awkit generate`
// and `awkit upgrade` threw those away. It now adds what a run needs to the
// list that is there and leaves the rest of the file alone.
//
// It does not hold the project's sessions to a tool policy. It used to put
// the Principal's command-policy hook here, and the hook then held every
// Claude Code session of the project, a person's own among them, and
// stopped every tool call of them all when the awkit on PATH could not
// check. The Principal of AWKIT_ENGINE=principal is now given the hook in
// the settings it is started with, as a Worker and the engine's agents are
// (internal/guard/hook.go); the hook awkit put here before is taken out.

// neededPermissions lists what the Principal and its subagents are allowed
// without being asked: the skill, the subagents, the tools of the workflow,
// and the tools the configuration's build and test commands start with (the
// review runs them).
func neededPermissions(config *Config) []string {
	needed := []string{
		"Skill(principal-workflow)",
		"Task(pr-reviewer)",
		"Task(conflict-resolver)",
	}
	for _, agent := range config.Agents.Custom {
		needed = append(needed, fmt.Sprintf("Task(%s)", agent.Name))
	}
	for _, tool := range []string{"gh", "git", "awkit", "bash", "codex"} {
		needed = append(needed, fmt.Sprintf("Bash(%s:*)", tool))
	}

	seen := map[string]bool{"gh": true, "git": true, "awkit": true, "bash": true, "codex": true}
	var tools []string
	for _, repo := range config.Repos {
		for _, command := range []string{repo.Verify.Build, repo.Verify.Test} {
			if tool := commandTool(command); tool != "" && !seen[tool] {
				seen[tool] = true
				tools = append(tools, tool)
			}
		}
	}
	sort.Strings(tools)
	for _, tool := range tools {
		needed = append(needed, fmt.Sprintf("Bash(%s:*)", tool))
	}
	return needed
}

// commandTool returns the program a command line starts: "go" for
// "go test ./...", "dotnet" for "dotnet build". Assignments in front of it
// (CGO_ENABLED=0 go build) are skipped. A command that starts with anything
// but a plain name (a path, a subshell) names no tool.
func commandTool(command string) string {
	for _, word := range strings.Fields(command) {
		if strings.Contains(word, "=") && !strings.HasPrefix(word, "=") {
			continue
		}
		for _, r := range word {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return ""
			}
		}
		return word
	}
	return ""
}

// generateClaudeSettings makes sure the local settings file allows what a run
// needs. A file that is there keeps everything it has and gets the
// permissions it lacks appended to its list; a file that is not valid JSON is
// left as it is and reported, since awkit cannot tell what the person meant
// by it.
// LocalSettings adds what a run of the project at stateRoot needs to Claude
// Code's local settings, and writes nothing else: of what generate writes, it
// is the one file git does not keep, so a working tree brought up to a
// branch that has the rest is brought up to its configuration by it.
func LocalSettings(stateRoot string) error {
	config, err := loadConfig(projectconfig.Path(stateRoot))
	if err != nil {
		return err
	}
	_, err = generateClaudeSettings(filepath.Join(stateRoot, ".claude", "settings.local.json"), config)
	return err
}

func generateClaudeSettings(path string, config *Config) (changed bool, err error) {
	needed := neededPermissions(config)

	var settings map[string]any
	data, readErr := os.ReadFile(path)
	switch {
	case os.IsNotExist(readErr):
		settings, changed = map[string]any{}, true
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
	case readErr != nil:
		return false, readErr
	default:
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, fmt.Errorf("%s is not valid JSON (%v); it is left as it is, and a run may ask for permissions nobody is there to give", filepath.ToSlash(filepath.Base(filepath.Dir(path)))+"/"+filepath.Base(path), err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	if removeGuardHook(settings) {
		changed = true
	}
	permissions, _ := settings["permissions"].(map[string]any)
	if permissions == nil {
		permissions = map[string]any{}
		settings["permissions"] = permissions
	}
	allow, _ := permissions["allow"].([]any)
	has := map[string]bool{}
	for _, entry := range allow {
		if s, ok := entry.(string); ok {
			has[s] = true
		}
	}
	for _, permission := range needed {
		if !has[permission] {
			allow = append(allow, permission)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	permissions["allow"] = allow

	// Written as Claude Code writes it: two spaces, and the characters of a
	// command (&, <, >) as themselves.
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out.Bytes(), 0o644)
}

// guardHookPrefix starts the command of the hook awkit put in the settings
// before: a hook whose command starts so is awkit's.
const guardHookPrefix = "awkit guard tool"

// removeGuardHook takes the hook awkit put among the PreToolUse hooks out of
// the settings, and leaves every other hook as it is. It reports whether the
// settings changed.
func removeGuardHook(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	var kept []any
	for _, e := range pre {
		if !isGuardEntry(e) {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(pre) {
		return false
	}
	if len(kept) > 0 {
		hooks["PreToolUse"] = kept
	} else {
		delete(hooks, "PreToolUse")
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	return true
}

func isGuardEntry(e any) bool {
	m, _ := e.(map[string]any)
	list, _ := m["hooks"].([]any)
	for _, h := range list {
		hm, _ := h.(map[string]any)
		if command, _ := hm["command"].(string); strings.HasPrefix(command, guardHookPrefix) {
			return true
		}
	}
	return false
}

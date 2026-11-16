package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The local settings file of Claude Code (.claude/settings.local.json) is the
// person's own: Claude Code adds to it every time they allow a command for
// good. The generator used to write the whole file, so every `awkit generate`
// and `awkit upgrade` threw those away. It now adds what a run needs to the
// list that is there and leaves the rest of the file alone.

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
func generateClaudeSettings(path string, config *Config) (changed bool, err error) {
	needed := neededPermissions(config)

	data, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		entries := make([]string, len(needed))
		for i, permission := range needed {
			entries[i] = "      " + quoteJSON(permission)
		}
		content := "{\n  \"permissions\": {\n    \"allow\": [\n" + strings.Join(entries, ",\n") + "\n    ]\n  }\n}\n"
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
		return true, os.WriteFile(path, []byte(content), 0o644)
	}
	if readErr != nil {
		return false, readErr
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, fmt.Errorf("%s is not valid JSON (%v); it is left as it is, and a run may ask for permissions nobody is there to give", filepath.ToSlash(filepath.Base(filepath.Dir(path)))+"/"+filepath.Base(path), err)
	}
	if settings == nil {
		settings = map[string]any{}
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

// quoteJSON writes a string as JSON does.
func quoteJSON(s string) string {
	out, _ := json.Marshal(s)
	return string(out)
}

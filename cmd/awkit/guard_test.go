package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runGuard runs `awkit guard tool` with a hook's input on standard input.
func runGuard(t *testing.T, input string, args ...string) (int, string) {
	t.Helper()
	in, err := os.CreateTemp(t.TempDir(), "hook-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := in.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = origStdin; in.Close() }()
	var code int
	stderr := captureStderr(func() { code = cmdGuard(append([]string{"tool"}, args...)) })
	return code, stderr
}

// A call the role's policy allows passes quietly; one it refuses ends with
// status 2 and the reason, which Claude Code gives the agent, and the
// verdict is recorded with the issue of the Worker's worktree.
func TestGuardTool(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.ToSlash(filepath.Join(root, ".worktrees", "issue-12"))
	if code, out := runGuard(t, `{"tool_name":"Bash","tool_input":{"command":"go test ./..."},"cwd":"`+cwd+`"}`, "--role", "implementer", "--state-root", root); code != 0 || out != "" {
		t.Errorf("allowed call: %d %q", code, out)
	}
	code, out := runGuard(t, `{"tool_name":"Bash","tool_input":{"command":"gh pr merge 41"},"cwd":"`+cwd+`"}`, "--role", "implementer", "--state-root", root)
	if code != 2 || !strings.Contains(out, "AWK guard:") || !strings.Contains(out, "tool-policy/1#github") {
		t.Errorf("refused call: %d %q", code, out)
	}
	events, err := os.ReadFile(filepath.Join(root, ".ai", "state", "events", "guard.jsonl"))
	if err != nil || !strings.Contains(string(events), `"issue_id":12`) || !strings.Contains(string(events), "tool-policy/1#github") {
		t.Errorf("events = %s, %v", events, err)
	}

	// The project's branches are protected as given.
	code, out = runGuard(t, `{"tool_name":"Bash","tool_input":{"command":"git push origin stable"}}`, "--role", "principal", "--protect", "stable", "--state-root", root)
	if code != 2 || !strings.Contains(out, "push-protected") {
		t.Errorf("push to a protected branch: %d %q", code, out)
	}
}

// What cannot be read is not allowed.
func TestGuardTool_RefusesWhatItCannotRead(t *testing.T) {
	for _, input := range []string{"", "not json", `{"tool_input":{}}`} {
		if code, out := runGuard(t, input, "--role", "implementer"); code != 2 || !strings.Contains(out, "cannot be read") {
			t.Errorf("%q: %d %q", input, code, out)
		}
	}
	if code, _ := runGuard(t, `{"tool_name":"Bash","tool_input":{"command":"ls"}}`, "--role", "stranger"); code != 2 {
		t.Errorf("an unknown role: %d", code)
	}
	if code := cmdGuard(nil); code != 2 {
		t.Errorf("no subcommand: %d", code)
	}
}

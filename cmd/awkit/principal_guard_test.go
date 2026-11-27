package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

func stubPrincipalClaude(t *testing.T, settings, atLeast bool) {
	t.Helper()
	exe, supports, at := principalAwkit, principalSupports, principalAtLeast
	t.Cleanup(func() { principalAwkit, principalSupports, principalAtLeast = exe, supports, at })
	principalAwkit = func() (string, error) { return "C:/tools/awkit.exe", nil }
	principalSupports = func(option string) bool { return settings && option == "--settings" }
	principalAtLeast = func(int, int, int) bool { return atLeast }
}

// The Principal of AWKIT_ENGINE=principal is started with settings of its
// own: every tool call goes past this awkit's `guard tool`, by its full path,
// with the principal's policy, the project and its branches. Nothing else is
// in them: the project's settings stay the session's.
func TestPrincipalGuardArgs(t *testing.T) {
	stubPrincipalClaude(t, true, true)
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch = "develop", "main"

	args, err := principalGuardArgs(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	file := principalSettingsFile(root)
	if len(args) != 2 || args[0] != "--settings" || args[1] != file {
		t.Fatalf("args = %q, want --settings %s", args, file)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 || settings["hooks"] == nil {
		t.Errorf("the settings hold more than the hook: %s", data)
	}
	var parsed struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string
				Hooks   []struct {
					Type, Command, OnFailure string
					Timeout                  int
				}
			}
		}
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Hooks.PreToolUse) != 1 || parsed.Hooks.PreToolUse[0].Matcher != "*" || len(parsed.Hooks.PreToolUse[0].Hooks) != 1 {
		t.Fatalf("hooks = %s", data)
	}
	hook := parsed.Hooks.PreToolUse[0].Hooks[0]
	want := `"C:/tools/awkit.exe" guard tool --role principal --state-root "` + filepath.ToSlash(root) + `" --protect "develop" --protect "main"`
	if hook.Type != "command" || hook.Command != want || hook.OnFailure != "block" || hook.Timeout != 30 {
		t.Errorf("hook = %+v\nwant the command %s", hook, want)
	}
}

// A claude CLI that cannot be given settings is refused: the Principal does
// not run unchecked. One older than 2.1.295 runs, and is warned of: a check
// that fails lets its call through.
func TestPrincipalGuardArgs_OlderClaude(t *testing.T) {
	stubPrincipalClaude(t, false, true)
	root := t.TempDir()
	if args, err := principalGuardArgs(root, nil); err == nil || !strings.Contains(err.Error(), "lacks --settings") {
		t.Errorf("a CLI without --settings: %q, %v", args, err)
	}
	if _, err := os.Stat(principalSettingsFile(root)); !os.IsNotExist(err) {
		t.Errorf("settings were written for a CLI that cannot take them: %v", err)
	}

	stubPrincipalClaude(t, true, false)
	var args []string
	out := captureStderr(func() {
		var err error
		if args, err = principalGuardArgs(root, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "older than 2.1.295") {
		t.Errorf("an older CLI is not warned of: %q", out)
	}
	if data, _ := os.ReadFile(args[1]); strings.Contains(string(data), "onFailure") {
		t.Errorf("an older CLI is given onFailure: %s", data)
	}
}

package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubClaudeCLI(t *testing.T, options []string, mentions []string, atLeast bool) {
	t.Helper()
	origSupports, origMentions, origAtLeast, origExe := claudeSupports, claudeMentions, claudeAtLeast, awkitExecutable
	t.Cleanup(func() {
		claudeSupports, claudeMentions, claudeAtLeast, awkitExecutable = origSupports, origMentions, origAtLeast, origExe
	})
	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}
	claudeSupports = func(option string) bool { return has(options, option) }
	claudeMentions = func(word string) bool { return has(mentions, word) }
	claudeAtLeast = func(int, int, int) bool { return atLeast }
	// The path as the platform writes it: the hook command writes it with
	// forward slashes on every platform.
	awkitExecutable = func() (string, error) { return filepath.FromSlash("C:/tools/awkit.exe"), nil }
}

// The Worker's claude runs with settings of its own for the run, none of the
// person's or the project's, refusing what is not allowed without asking,
// and with every call checked by the implementer's policy.
func TestGuardArgs(t *testing.T) {
	stubClaudeCLI(t, []string{"--settings", "--setting-sources", "--strict-mcp-config"}, []string{"dontAsk"}, true)
	settingsFile := filepath.Join(t.TempDir(), "run", "claude-settings.json")
	g := &WorkerGuard{StateRoot: filepath.FromSlash("D:/games/rally"), Protected: []string{"develop", "main", ""}, AllowControl: true, SettingsFile: settingsFile}

	args, err := (&ClaudeCodeBackend{Model: "sonnet"}).guardArgs(g)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--settings", settingsFile, "--setting-sources", "", "--strict-mcp-config", "--permission-mode", "dontAsk"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Errorf("args = %q\nwant  %q", args, want)
	}

	data, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Permissions struct{ Allow, Deny []string }
		Hooks       struct {
			PreToolUse []struct {
				Matcher string
				Hooks   []struct {
					Type, Command, OnFailure string
					Timeout                  int
				}
			}
		}
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(settings.Permissions.Allow, ","), "Edit") || !strings.Contains(strings.Join(settings.Permissions.Deny, ","), "Bash(gh:*)") {
		t.Errorf("permissions = %+v", settings.Permissions)
	}
	if len(settings.Hooks.PreToolUse) != 1 || settings.Hooks.PreToolUse[0].Matcher != "*" || len(settings.Hooks.PreToolUse[0].Hooks) != 1 {
		t.Fatalf("hooks = %+v", settings.Hooks)
	}
	hook := settings.Hooks.PreToolUse[0].Hooks[0]
	wantCommand := `"C:/tools/awkit.exe" guard tool --role implementer --state-root "D:/games/rally" --protect "develop" --protect "main" --allow-control`
	if hook.Type != "command" || hook.Command != wantCommand || hook.OnFailure != "block" || hook.Timeout != hookTimeoutSeconds {
		t.Errorf("hook = %+v\nwant command %s", hook, wantCommand)
	}
}

// Where the person set dangerously_skip_permissions, nothing is asked anyway;
// the hook still checks every call.
func TestGuardArgs_SkippingPermissionsKeepsTheHook(t *testing.T) {
	stubClaudeCLI(t, []string{"--settings", "--setting-sources"}, nil, false)
	g := &WorkerGuard{StateRoot: t.TempDir(), SettingsFile: filepath.Join(t.TempDir(), "s.json")}
	args, err := (&ClaudeCodeBackend{DangerouslySkipPermissions: true}).guardArgs(g)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(args, " "), "--permission-mode") {
		t.Errorf("args = %q", args)
	}
	data, _ := os.ReadFile(g.SettingsFile)
	if !strings.Contains(string(data), "guard tool --role implementer") || strings.Contains(string(data), "onFailure") {
		t.Errorf("settings of an older CLI:\n%s", data)
	}
}

// A claude CLI that cannot be held to the settings is not run.
func TestGuardArgs_AnOldCLIIsRefused(t *testing.T) {
	stubClaudeCLI(t, []string{"--settings"}, nil, false)
	_, err := (&ClaudeCodeBackend{}).guardArgs(&WorkerGuard{SettingsFile: filepath.Join(t.TempDir(), "s.json")})
	if err == nil || !strings.Contains(err.Error(), "--setting-sources") || !strings.Contains(err.Error(), "dontAsk") || !strings.Contains(err.Error(), "update Claude Code") {
		t.Errorf("err = %v", err)
	}

	// And the Worker reports it, without running.
	result := (&ClaudeCodeBackend{}).Execute(context.Background(), BackendOptions{
		Guard: &WorkerGuard{SettingsFile: filepath.Join(t.TempDir(), "s.json")},
	})
	if result.ExitCode == 0 || (result.FailureStage != "claude_guard" && result.FailureStage != "claude_exec") {
		t.Errorf("result = %+v", result)
	}
}

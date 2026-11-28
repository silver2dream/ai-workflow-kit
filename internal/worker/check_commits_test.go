package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commitsRepo is a repository with a base commit and, on top of it, a
// commit that writes files.
func commitsRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the PATH")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.invalid", "-c", "user.name=t", "-c", "core.autocrlf=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "base")
	git("tag", "base")
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-qm", "change")
	return dir
}

// The merge queue holds a change of commits to the rules a staged change is
// held to, with what the ticket allows.
func TestCheckCommits(t *testing.T) {
	ctx := context.Background()
	clean := commitsRepo(t, map[string]string{"score.go": "package main\n\nfunc Score() int { return 1 }\n"})
	if err := CheckCommits(ctx, clean, "base...HEAD", nil); err != nil {
		t.Errorf("a clean change: %v", err)
	}

	risky := commitsRepo(t, map[string]string{
		".claude/settings.json": "{}\n",
		"keys.go":               "package main\n\nconst apiKey = \"x\" // api_key = \"abc123\"\n",
	})
	err := CheckCommits(ctx, risky, "base...HEAD", &TicketMetadata{})
	var refused *PolicyError
	if !errors.As(err, &refused) || strings.Join(refused.Protected, ",") != ".claude/settings.json" || len(refused.Secrets) == 0 {
		t.Fatalf("refused: %+v (%v)", refused, err)
	}
	if !strings.Contains(err.Error(), "allow_script_changes") || !strings.Contains(err.Error(), "allow_secrets") {
		t.Errorf("said: %v", err)
	}

	err = CheckCommits(ctx, risky, "base...HEAD", &TicketMetadata{AllowScriptChanges: true})
	if !errors.As(err, &refused) || len(refused.Protected) != 0 || len(refused.Secrets) == 0 || strings.Contains(err.Error(), "allow_script_changes") {
		t.Errorf("AWK's files allowed: %+v (%v)", refused, err)
	}
	if err := CheckCommits(ctx, risky, "base...HEAD", &TicketMetadata{AllowScriptChanges: true, AllowSecrets: true}); err != nil {
		t.Errorf("both allowed: %v", err)
	}
	if err := CheckCommits(ctx, risky, "base...HEAD", &TicketMetadata{AllowSecrets: true, ScriptWhitelist: ".claude/settings.json"}); err != nil {
		t.Errorf("a legacy ticket's whitelist: %v", err)
	}

	// git that cannot show the change is not a refusal.
	err = CheckCommits(ctx, clean, "nothing...HEAD", nil)
	if err == nil || errors.As(err, &refused) {
		t.Errorf("a range git does not know: %v", err)
	}
}

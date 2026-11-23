package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/landing"
	"github.com/silver2dream/ai-workflow-kit/internal/settings"
)

const settingsConfig = `version: "1.2"

project:
  name: "game"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: ""
      test: ""

git:
  integration_branch: develop
  release_branch: main
`

// settingsProject is a project cloned from a bare origin whose develop has
// a configuration and no roster.
func settingsProject(t *testing.T) (root, origin string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	root = filepath.Join(base, "game")
	mergeGit(t, base, "init", "--quiet", "--bare", "--initial-branch=develop", origin)
	mergeGit(t, base, "clone", "--quiet", "-c", "core.autocrlf=false", origin, root)
	mergeConfigure(t, root)
	mergeGit(t, root, "checkout", "--quiet", "-B", "develop")
	for file, text := range map[string]string{".gitignore": ".ai/state/\n", ".ai/config.yaml": settingsConfig} {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mergeGit(t, root, "add", "-A")
	mergeGit(t, root, "commit", "--quiet", "-m", "start")
	mergeGit(t, root, "push", "--quiet", "-u", "origin", "develop")
	return root, origin
}

func stubSettings(t *testing.T, gh landing.GitHub, generated *[]string) {
	t.Helper()
	origGitHub, origLanding, origGenerate, origLocal, origRunning := settingsGitHub, settingsLanding, settingsGenerate, settingsLocal, settingsRunGoing
	t.Cleanup(func() {
		settingsGitHub, settingsLanding, settingsGenerate, settingsLocal, settingsRunGoing = origGitHub, origLanding, origGenerate, origLocal, origRunning
	})
	settingsGitHub = gh
	settingsRunGoing = func(string) bool { return false }
	settingsGenerate = func(root string) error {
		*generated = append(*generated, "generate "+root)
		return nil
	}
	settingsLocal = func(root string) error {
		*generated = append(*generated, "local "+root)
		return nil
	}
	settingsLanding = func(o *landing.Options) {
		clock := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
		o.Poll, o.Grace, o.Absent, o.Wait = time.Second, 3*time.Second, 10*time.Second, time.Minute
		o.Sleep = func(d time.Duration) { clock = clock.Add(d) }
		o.Now = func() time.Time { return clock }
	}
}

// `awkit settings apply` lands the change on develop, writes its record as it
// goes, generates in the worktree and then in the project, and leaves the
// working tree up to develop.
func TestSettingsApply_LandsTheRequest(t *testing.T) {
	root, origin := settingsProject(t)
	gh := &originGitHub{t: t, origin: origin, checks: []landing.Check{{Name: "build", Bucket: "pass"}}}
	var generated []string
	stubSettings(t, gh, &generated)

	files, err := settings.ReadFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := settings.ReadConfig(files.Config)
	if err != nil {
		t.Fatal(err)
	}
	was := *cfg
	cfg.Repos = append([]settings.RepoCommands{}, cfg.Repos...)
	cfg.Repos[0].Test = "go test ./..."
	file, err := settings.WriteRequest(root, settings.Request{Config: cfg, WasConfig: &was})
	if err != nil {
		t.Fatal(err)
	}

	var code int
	out := captureStdout(t, func() { code = cmdSettings([]string{"apply", "--request", file, "--state-root", root}) })
	if code != 0 {
		t.Fatalf("awkit settings apply = %d:\n%s", code, out)
	}
	there, err := exec.Command("git", "--git-dir", origin, "show", "develop:.ai/config.yaml").Output()
	if err != nil || !strings.Contains(string(there), "test: go test ./...") {
		t.Errorf("develop has:\n%s", there)
	}
	if gh.merged != 1 {
		t.Errorf("%d merges", gh.merged)
	}
	here, _ := os.ReadFile(filepath.Join(root, ".ai", "config.yaml"))
	if string(here) != string(there) {
		t.Error("the working tree was not brought up to develop")
	}
	// Generated in the worktree the change was made in; here only what git
	// does not keep.
	if len(generated) != 2 || !strings.HasPrefix(generated[0], "generate ") || generated[0] == "generate "+root || generated[1] != "local "+root {
		t.Errorf("generated: %v", generated)
	}
	record, err := settings.ReadRecord(root)
	if err != nil || record.State != settings.Done || record.PR != 41 || record.Left != "" {
		t.Errorf("record = %+v, %v", record, err)
	}
}

func TestSettingsApply_RefusesWhatIsWrong(t *testing.T) {
	if code := cmdSettings(nil); code != 2 {
		t.Errorf("no subcommand = %d", code)
	}
	if code := cmdSettings([]string{"show"}); code != 2 {
		t.Errorf("unknown subcommand = %d", code)
	}
	if code := cmdSettings([]string{"apply"}); code != 2 {
		t.Errorf("no request = %d", code)
	}
	if code := cmdSettings([]string{"--help"}); code != 0 {
		t.Errorf("--help = %d", code)
	}

	root, origin := settingsProject(t)
	gh := &originGitHub{t: t, origin: origin}
	var generated []string
	stubSettings(t, gh, &generated)
	file, err := settings.WriteRequest(root, settings.Request{Roster: &settings.Roster{Roles: []settings.Role{{ID: "only", Stance: "alone", Decides: true, Plans: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	var code int
	_ = captureStdout(t, func() { code = cmdSettings([]string{"apply", "--request", file, "--state-root", root}) })
	if code != 1 {
		t.Errorf("a roster of one role = %d", code)
	}
	if record, err := settings.ReadRecord(root); err != nil || record.Why != "invalid" {
		t.Errorf("record = %+v, %v", record, err)
	}
	if gh.open != nil || len(generated) != 0 {
		t.Error("something was landed")
	}
}

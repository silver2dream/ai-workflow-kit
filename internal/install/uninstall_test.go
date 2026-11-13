package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"testing/fstest"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
)

// installedProject is a project after `awkit init`, a few runs, and a person
// who keeps things of their own under .claude.
func installedProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := Install(nestedKit(), dir, Options{Preset: "generic", NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for rel, content := range map[string]string{
		// what the generator wrote
		"CLAUDE.md":                   "# CLAUDE.md\n",
		"AGENTS.md":                   "# AGENTS.md\n",
		".claude/agents/sec-check.md": "---\nname: sec-check\n---\n\n" + generate.GeneratedAgentMark + "\n\nYou are the sec-check agent.\n",
		".claude/settings.local.json": "{}",
		// what runs left behind
		".ai/state/results/issue-3.json": "{}",
		".ai/state/loop_count":           "4",
		".worktrees/issue-4/main.go":     "package main\n",
		// the project's own
		".claude/rules/backend.md":              "the project's rule",
		".claude/skills/release-notes/SKILL.md": "the project's skill",
		".claude/agents/designer.md":            "---\nname: designer\n---\n\nThe project's own.\n",
		".claude/settings.json":                 "{}",
		"main.go":                               "package main\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// nestedKit is a kit with a file two directories below a skill, as the real
// kit has: removing it leaves a chain of directories to remove, the deepest
// first.
func nestedKit() fstest.MapFS {
	kit := createMinimalMockFS()
	kit[".claude/skills/principal-workflow/phases/review/checklist.md"] = &fstest.MapFile{Data: []byte("# checklist\n")}
	return kit
}

func filesOf(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		found = append(found, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(found)
	return found
}

// An uninstall removes what AWK put into the project and nothing else. The
// .claude directory is Claude Code's: what the project keeps there stays, and
// so do the guides and the settings of this machine. It used to remove all of
// .claude.
func TestUninstall(t *testing.T) {
	dir := installedProject(t)

	removed, err := Uninstall(nestedKit(), dir, false, false)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	left := []string{
		".claude/agents/designer.md",
		".claude/rules/backend.md",
		".claude/settings.json",
		".claude/settings.local.json",
		".claude/skills/release-notes/SKILL.md",
		".gitattributes",
		".gitignore",
		"AGENTS.md",
		"CLAUDE.md",
		"main.go",
	}
	if got := filesOf(t, dir); !reflect.DeepEqual(got, left) {
		t.Errorf("left in the project:\n%v\nwant\n%v", got, left)
	}
	// A directory that held only what was removed is gone; one that holds
	// something of the project's is not.
	for _, gone := range []string{".ai", ".worktrees", ".claude/rules/_kit", ".claude/skills/principal-workflow"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("%s is still there (err %v)", gone, err)
		}
	}

	want := []string{
		".ai/config.yaml",
		".ai/roster.yaml",
		".ai/state",
		".claude/agents/pr-reviewer.md",
		".claude/agents/sec-check.md",
		".claude/rules/_kit/git-workflow.md",
		".claude/skills/principal-workflow/SKILL.md",
		".claude/skills/principal-workflow/phases/review/checklist.md",
		".worktrees",
	}
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, want) {
		t.Errorf("removed\n%v\nwant\n%v", removed, want)
	}

	// Nothing of AWK's is left to remove.
	again, err := Uninstall(nestedKit(), dir, false, false)
	if err != nil || len(again) != 0 {
		t.Errorf("a second uninstall removed %v (err %v)", again, err)
	}
}

// With keepConfig the project keeps what it says about itself: the
// configuration and the roster.
func TestUninstall_KeepConfig(t *testing.T) {
	dir := installedProject(t)

	removed, err := Uninstall(nestedKit(), dir, true, false)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	for _, kept := range []string{".ai/config.yaml", ".ai/roster.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(kept))); err != nil {
			t.Errorf("%s was not kept: %v", kept, err)
		}
		for _, location := range removed {
			if location == kept {
				t.Errorf("%s is listed as removed", kept)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".ai", "state")); !os.IsNotExist(err) {
		t.Errorf("what the runs left behind is still there (err %v)", err)
	}
}

// A dry run lists what would go and removes none of it.
func TestUninstall_DryRun(t *testing.T) {
	dir := installedProject(t)
	before := filesOf(t, dir)

	would, err := Uninstall(nestedKit(), dir, false, true)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	if after := filesOf(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("a dry run removed files:\n%v\nbefore\n%v", after, before)
	}
	done, err := Uninstall(nestedKit(), dir, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(would, done) || len(would) == 0 {
		t.Errorf("the dry run listed\n%v\nthe uninstall removed\n%v", would, done)
	}
}

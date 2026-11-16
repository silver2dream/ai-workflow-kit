package awkit

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestKitFSPythonLibModuleComplete was removed - Python lib module migrated to Go implementations.

// A path on the deprecated list is removed from a project on every init and
// upgrade. If the kit still shipped it, it would be installed and deleted in
// the same run, each time.
func TestKitFSShipsNothingOnTheDeprecatedList(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("internal", "install", "deprecated.txt"))
	if err != nil {
		t.Fatal(err)
	}
	listed := 0
	for _, line := range strings.Split(string(data), "\n") {
		path := strings.TrimSpace(line)
		if path == "" || strings.HasPrefix(path, "#") {
			continue
		}
		listed++
		if _, err := fs.Stat(KitFS, path); err == nil {
			t.Errorf("%s is on the deprecated list, but the kit still ships it", path)
		}
	}
	if listed == 0 {
		t.Error("the deprecated list was read as empty")
	}
}

// What the kit ships is what an install writes into a project, at the same
// paths: the kit's rules, skills and subagents, where Claude Code looks for
// them.
func TestKitFSHoldsTheKit(t *testing.T) {
	required := []string{
		".claude/rules/_kit/git-workflow.md",
		".claude/skills/principal-workflow/SKILL.md",
		".claude/skills/principal-workflow/phases/main-loop.md",
		".claude/skills/principal-workflow/phases/dispatch.md",
		".claude/skills/principal-workflow/phases/review.md",
		".claude/skills/principal-workflow/tasks/create-task.md",
		".claude/skills/principal-workflow/references/contracts.md",
		".claude/skills/post-mortem/SKILL.md",
		".claude/skills/release-checklist/SKILL.md",
		".claude/agents/pr-reviewer.md",
		".claude/agents/conflict-resolver.md",
	}
	for _, path := range required {
		if _, err := fs.Stat(KitFS, path); err != nil {
			t.Errorf("the kit does not hold %s", path)
		}
	}
}

// Only the kit ships. The repository's own configuration and roster, the
// skills that are of use here alone and the rule examples stay behind: a
// project gets its configuration from a preset, and Claude Code would read
// an example under .claude/rules as a rule of the project.
func TestKitFSHoldsNothingElse(t *testing.T) {
	kit := []string{
		".claude/rules/_kit/",
		".claude/skills/principal-workflow/",
		".claude/skills/post-mortem/",
		".claude/skills/release-checklist/",
		".claude/agents/",
	}
	files := 0
	err := fs.WalkDir(KitFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		for _, prefix := range kit {
			if strings.HasPrefix(path, prefix) {
				return nil
			}
		}
		t.Errorf("the kit ships %s, which is not part of it", path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Error("the kit is empty")
	}
}

// The rule examples are documents of this repository. The ones there are the
// ones their README lists, and the other way round.
func TestRuleExamplesMatchTheirReadme(t *testing.T) {
	dir := filepath.Join("docs", "examples", "rules")
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{}
	for _, entry := range entries {
		if name := entry.Name(); name != "README.md" {
			kept[name] = true
			if !strings.Contains(string(readme), "`"+name+"`") {
				t.Errorf("%s is there but the README does not list it", name)
			}
		}
	}
	for _, match := range regexp.MustCompile("- `([a-z0-9-]+[.]md)`").FindAllStringSubmatch(string(readme), -1) {
		if !kept[match[1]] {
			t.Errorf("the README lists %s, which is not there", match[1])
		}
	}
	if len(kept) == 0 {
		t.Error("there are no rule examples")
	}
}

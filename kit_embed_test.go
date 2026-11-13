package awkit

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestKitFSContainsRequiredFiles verifies that all required files are embedded.
// This prevents issues where awkit init fails because files are missing.
func TestKitFSContainsRequiredFiles(t *testing.T) {
	requiredFiles := []string{
		// Config files
		".ai/config/workflow.schema.json",

		// Rules
		".ai/rules/_kit/git-workflow.md",

		// Skills
		".ai/skills/principal-workflow/SKILL.md",
		".ai/skills/principal-workflow/phases/main-loop.md",

		// Subagents
		".claude/agents/pr-reviewer.md",
	}

	for _, path := range requiredFiles {
		_, err := fs.Stat(KitFS, path)
		if err != nil {
			t.Errorf("required file not embedded: %s", path)
		}
	}
}

// TestKitFSContainsRequiredDirectories verifies directory structure is embedded.
func TestKitFSContainsRequiredDirectories(t *testing.T) {
	requiredDirs := []string{
		".ai/config",
		".ai/rules/_kit",
		".ai/rules/_examples",
		// Skills
		".ai/skills/principal-workflow",
		".ai/skills/principal-workflow/phases",
		".ai/skills/principal-workflow/tasks",
		".ai/skills/principal-workflow/references",
		".ai/skills/post-mortem",
		".ai/skills/post-mortem/phases",
		".ai/skills/release-checklist",
		".ai/skills/release-checklist/phases",
		// Subagents
		".claude/agents",
	}

	for _, dir := range requiredDirs {
		entries, err := fs.ReadDir(KitFS, dir)
		if err != nil {
			t.Errorf("required directory not embedded or empty: %s (error: %v)", dir, err)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("required directory is empty: %s", dir)
		}
	}
}

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

// The rule examples the kit ships are the ones their README lists, and the
// other way round.
func TestRuleExamplesMatchTheirReadme(t *testing.T) {
	const dir = ".ai/rules/_examples"
	readme, err := fs.ReadFile(KitFS, dir+"/README.md")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(KitFS, dir)
	if err != nil {
		t.Fatal(err)
	}
	shipped := map[string]bool{}
	for _, entry := range entries {
		if name := entry.Name(); name != "README.md" {
			shipped[name] = true
			if !strings.Contains(string(readme), "`"+name+"`") {
				t.Errorf("%s is shipped but the README does not list it", name)
			}
		}
	}
	for _, match := range regexp.MustCompile("- `([a-z0-9-]+[.]md)`").FindAllStringSubmatch(string(readme), -1) {
		if !shipped[match[1]] {
			t.Errorf("the README lists %s, which the kit does not ship", match[1])
		}
	}
	if len(shipped) == 0 {
		t.Error("no rule examples are shipped")
	}
}

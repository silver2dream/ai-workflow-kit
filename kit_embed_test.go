package awkit

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
		".claude/skills/principal-workflow/references/conflict-resolution.md",
		".claude/skills/principal-workflow/references/ticket.md",
		".claude/skills/principal-workflow/references/contracts.md",
		".claude/skills/principal-workflow/references/after-compaction.md",
		".claude/skills/post-mortem/SKILL.md",
		".claude/skills/post-mortem/references/analyze-failure.md",
		".claude/skills/release-checklist/SKILL.md",
		".claude/skills/release-checklist/references/verify-release.md",
		".claude/agents/pr-reviewer.md",
		".claude/agents/conflict-resolver.md",
	}
	for _, path := range required {
		if _, err := fs.Stat(KitFS, path); err != nil {
			t.Errorf("the kit does not hold %s", path)
		}
	}
}

// Only the kit ships. The repository's own configuration and roster stay
// behind: a project gets its configuration from a preset.
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

// kitSkills lists the skills the kit ships, by the name of their directory.
func kitSkills(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(KitFS, ".claude/skills")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		t.Fatal("the kit ships no skill")
	}
	return names
}

// Claude Code decides from a skill's frontmatter whether to load it: the name
// is how it is called, the description is what the model reads to choose. A
// description that is not valid YAML (a colon followed by a space in a plain
// value is enough) is read differently by every tool that reads it.
func TestKitSkillsSayWhatTheyAre(t *testing.T) {
	for _, skill := range kitSkills(t) {
		data, err := fs.ReadFile(KitFS, ".claude/skills/"+skill+"/SKILL.md")
		if err != nil {
			t.Errorf("the skill %s has no SKILL.md", skill)
			continue
		}
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		rest, found := strings.CutPrefix(text, "---\n")
		head, body, closed := strings.Cut(rest, "\n---\n")
		if !found || !closed {
			t.Errorf("%s: SKILL.md does not start with a frontmatter block", skill)
			continue
		}
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
			t.Errorf("%s: the frontmatter is not valid YAML: %v", skill, err)
			continue
		}
		if meta.Name != skill {
			t.Errorf("%s: the frontmatter names the skill %q", skill, meta.Name)
		}
		if n := len([]rune(meta.Description)); n < 40 || n > 1024 {
			t.Errorf("%s: the description is %d characters; it says what the skill does and when to use it, in at most 1024", skill, n)
		}
		if strings.TrimSpace(body) == "" {
			t.Errorf("%s: SKILL.md has nothing after its frontmatter", skill)
		}
	}
}

// kitPath matches a file of the kit written in backticks: a path under
// .claude, or a skill's own references/<file>.md.
var kitPath = regexp.MustCompile("`((?:[.]claude/(?:skills|agents|rules/_kit)/|references/)[A-Za-z0-9_./-]+[.]md)`")

// A skill is short because the rest is in files it names. A name that leads
// nowhere leaves the agent without the step it was sent to read.
func TestKitTextsNameFilesThatAreThere(t *testing.T) {
	named := 0
	err := fs.WalkDir(KitFS, ".claude", func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(KitFS, file)
		if err != nil {
			return err
		}
		for _, match := range kitPath.FindAllStringSubmatch(string(data), -1) {
			target := match[1]
			if strings.HasPrefix(target, "references/") {
				// Relative to the skill the text belongs to.
				parts := strings.Split(file, "/")
				if len(parts) < 4 || parts[1] != "skills" {
					t.Errorf("%s names %s, which is relative to a skill, and is not part of one", file, target)
					continue
				}
				target = strings.Join(parts[:3], "/") + "/" + target
			}
			named++
			if _, err := fs.Stat(KitFS, target); err != nil {
				t.Errorf("%s names %s, which the kit does not hold", file, match[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if named < 8 {
		t.Errorf("only %d file names were found in the kit's texts: the pattern no longer sees them", named)
	}
}

// Every reference text of a skill is named by that skill's SKILL.md: a text
// nothing points to is never read.
func TestKitSkillsNameEveryReference(t *testing.T) {
	for _, skill := range kitSkills(t) {
		dir := ".claude/skills/" + skill
		data, err := fs.ReadFile(KitFS, dir+"/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		entries, err := fs.ReadDir(KitFS, dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() != "references" {
				t.Errorf("%s keeps texts in %s/; a skill keeps them in references/", skill, entry.Name())
				continue
			}
			files, err := fs.ReadDir(KitFS, dir+"/references")
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				if name := "`references/" + f.Name() + "`"; !strings.Contains(string(data), name) {
					t.Errorf("%s: SKILL.md does not name %s", skill, name)
				}
			}
		}
	}
}

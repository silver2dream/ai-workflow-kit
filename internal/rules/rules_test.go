package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		data  string
		title string
		paths []string
		text  string
	}{
		{
			name:  "paths as a list",
			file:  ".claude/rules/backend.md",
			data:  "---\npaths:\n  - \"backend/**\"\n  - cmd/**/*.go\n---\n\n# Backend Rules\n\nBody.\n",
			title: "Backend Rules", paths: []string{"backend/**", "cmd/**/*.go"}, text: "# Backend Rules\n\nBody.\n",
		},
		{
			name:  "paths as one string",
			file:  ".claude/rules/web.md",
			data:  "---\npaths: \"src/**/*.ts, src/**/*.tsx\"\n---\n# Web\n",
			title: "Web", paths: []string{"src/**/*.ts", "src/**/*.tsx"}, text: "# Web\n",
		},
		{
			name:  "no frontmatter: about everything",
			file:  ".claude/rules/agents.md",
			data:  "# Agent Orchestration\n\nText with --- in it.\n",
			title: "Agent Orchestration", text: "# Agent Orchestration\n\nText with --- in it.\n",
		},
		{
			name:  "frontmatter without paths",
			file:  ".claude/rules/x.md",
			data:  "---\ndescription: a rule\n---\n# X\n",
			title: "X", text: "# X\n",
		},
		{
			name:  "empty frontmatter",
			file:  ".claude/rules/x.md",
			data:  "---\n---\n# X\n",
			title: "X", text: "# X\n",
		},
		{
			name:  "no title: the name of the file",
			file:  ".claude/rules/_kit/git-workflow.md",
			data:  "Rules without a heading.\n## A section is not a title\n",
			title: "git-workflow", text: "Rules without a heading.\n## A section is not a title\n",
		},
		{
			name:  "CRLF and a byte order mark",
			file:  ".claude/rules/win.md",
			data:  "\xef\xbb\xbf---\r\npaths:\r\n  - \"a/**\"\r\n---\r\n\r\n# Windows\r\n\r\nBody.\r\n",
			title: "Windows", paths: []string{"a/**"}, text: "# Windows\n\nBody.\n",
		},
		{
			name:  "a pattern that is empty is none",
			file:  ".claude/rules/e.md",
			data:  "---\npaths: [\"\", \" a/** \"]\n---\n# E\n",
			title: "E", paths: []string{"a/**"}, text: "# E\n",
		},
		{
			name:  "two headings: the first is the title",
			file:  ".claude/rules/two.md",
			data:  "Before.\n# First\n\n# Second\n",
			title: "First", text: "Before.\n# First\n\n# Second\n",
		},
		{
			name:  "a heading without words is no title",
			file:  ".claude/rules/blank.md",
			data:  "#  \n# Real\n",
			title: "Real", text: "#  \n# Real\n",
		},
		{
			name:  "frontmatter and nothing after it",
			file:  ".claude/rules/only.md",
			data:  "---\npaths: [\"a/**\"]\n---",
			title: "only", paths: []string{"a/**"}, text: "",
		},
	}
	for _, c := range cases {
		rule := Parse(c.file, []byte(c.data))
		if rule.Problem != "" {
			t.Errorf("%s: %s", c.name, rule.Problem)
			continue
		}
		if rule.File != c.file || rule.Title != c.title || rule.Text != c.text || !reflect.DeepEqual(rule.Paths, c.paths) {
			t.Errorf("%s: got %+v\nwant title %q, paths %v, text %q", c.name, rule, c.title, c.paths, c.text)
		}
	}
}

// Frontmatter that cannot be read does not take the rule out of force:
// Claude Code loads such a rule as one that names no file, into every
// session. So it is a rule about everything here too, and it says what is
// wrong with it, for `awkit generate` to tell the person.
func TestParse_FrontmatterThatCannotBeRead(t *testing.T) {
	cases := []struct {
		name, data, problem, text string
	}{
		{"never closed", "---\npaths:\n  - a\n# Title\n", "never closed", "---\npaths:\n  - a\n# Title\n"},
		{"not YAML", "---\npaths: [a\n---\n# Title\n", "not valid YAML", "# Title\n"},
		{"paths is a mapping", "---\npaths:\n  a: b\n---\n# Title\n", "not a list of patterns", "# Title\n"},
	}
	for _, c := range cases {
		rule := Parse(".claude/rules/r.md", []byte(c.data))
		if !strings.Contains(rule.Problem, c.problem) {
			t.Errorf("%s: the problem is %q, want it to say %q", c.name, rule.Problem, c.problem)
		}
		if !rule.Everywhere() || rule.Scope() != "everything" {
			t.Errorf("%s: the rule is about %v, want everything", c.name, rule.Paths)
		}
		if rule.Title != "Title" || rule.Text != c.text {
			t.Errorf("%s: title %q, text %q; want the title found and the text %q", c.name, rule.Title, rule.Text, c.text)
		}
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, ".claude", "rules", "backend.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("---\npaths:\n  - \"backend/**\"\n---\n\n# Backend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rule, err := Load(root, ".claude/rules/backend.md")
	if err != nil || rule.Title != "Backend" || !reflect.DeepEqual(rule.Paths, []string{"backend/**"}) {
		t.Errorf("Load = %+v, %v", rule, err)
	}
	// What is wrong inside a file is not a failure to read it.
	if err := os.WriteFile(file, []byte("---\npaths: [a\n---\n# Broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rule, err := Load(root, ".claude/rules/backend.md"); err != nil || rule.Problem == "" || rule.Title != "Broken" {
		t.Errorf("Load of a rule with broken frontmatter = %+v, %v", rule, err)
	}
	missing, err := Load(root, ".claude/rules/nothing.md")
	if !os.IsNotExist(err) || missing.File != ".claude/rules/nothing.md" {
		t.Errorf("Load of a file that is not there = %+v, %v", missing, err)
	}
}

func TestScope(t *testing.T) {
	if got := (Rule{}).Scope(); got != "everything" || !(Rule{}).Everywhere() {
		t.Errorf("Scope of a rule that names no file = %q", got)
	}
	rule := Rule{Paths: []string{"backend/**", "**/*.md"}}
	if got, want := rule.Scope(), "`backend/**`, `**/*.md`"; got != want || rule.Everywhere() {
		t.Errorf("Scope = %q, want %q", got, want)
	}
}

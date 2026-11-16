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
		rule, err := Parse(c.file, []byte(c.data))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if rule.File != c.file || rule.Title != c.title || rule.Text != c.text || !reflect.DeepEqual(rule.Paths, c.paths) {
			t.Errorf("%s: got %+v\nwant title %q, paths %v, text %q", c.name, rule, c.title, c.paths, c.text)
		}
	}
}

func TestParse_WhatCannotBeRead(t *testing.T) {
	for name, data := range map[string]string{
		"never closed":       "---\npaths:\n  - a\n# Title\n",
		"not YAML":           "---\npaths: [a\n---\n# Title\n",
		"paths is a mapping": "---\npaths:\n  a: b\n---\n# Title\n",
	} {
		rule, err := Parse(".claude/rules/r.md", []byte(data))
		if err == nil {
			t.Errorf("%s: read as %+v, want an error", name, rule)
			continue
		}
		if !strings.Contains(err.Error(), ".claude/rules/r.md") && !strings.Contains(err.Error(), "paths") {
			t.Errorf("%s: the error does not say where: %v", name, err)
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

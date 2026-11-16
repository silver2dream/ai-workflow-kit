package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The patterns of a rule's `paths` are read the way Claude Code reads them.
// The first group is the table of its documentation; the rest is what that
// table leaves to be inferred, and the patterns the kit's own rules use.
func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		yes     []string
		no      []string
	}{
		// The documentation's table.
		{"**/*.ts", []string{"a.ts", "src/a.ts", "src/api/v1/a.ts"}, []string{"a.tsx", "src/a.go", "ts"}},
		{"src/**/*", []string{"src/a.ts", "src/api/a.ts", "src/api/v1/a.go"}, []string{"a.ts", "lib/src/a.ts", "src"}},
		{"*.md", []string{"README.md", "docs/a.md", "backend/docs/x/a.md"}, []string{"a.mdx", "md", "docs/a.txt"}},
		{"/*.md", []string{"README.md", "CLAUDE.md"}, []string{"docs/a.md", "a/b/c.md"}},
		{"src/components/*.tsx", []string{"src/components/Button.tsx"}, []string{"src/components/ui/Button.tsx", "components/Button.tsx", "src/Button.tsx"}},
		// A pattern is used up when the path is: a path that ends before the
		// pattern does is not matched, and neither is one that goes on after it.
		{"a/b/c", []string{"a/b/c"}, []string{"a/b", "a", "a/b/c/d", "b/c"}},
		{"src/components/*.tsx", nil, []string{"src/components", "src"}},
		// Braces.
		{"src/**/*.{ts,tsx}", []string{"src/a.ts", "src/x/y/a.tsx"}, []string{"src/a.js", "lib/a.ts"}},
		{"{a,b}/{c,d}/*.{ts,tsx}", []string{"a/c/x.ts", "b/d/x.tsx", "a/d/x.ts"}, []string{"a/e/x.ts", "c/a/x.ts", "a/c/x.js"}},
		{"docs/{design,tech/{api,db}}/*.md", []string{"docs/design/a.md", "docs/tech/api/a.md", "docs/tech/db/a.md"}, []string{"docs/tech/a.md", "docs/tech/ui/a.md"}},
		{"file{1}.md", []string{"file{1}.md"}, []string{"file1.md"}},   // no comma: no alternatives
		{"a/\\{b,c\\}.md", []string{"a/{b,c}.md"}, []string{"a/b.md"}}, // escaped
		{"a/{b,c.md", []string{"a/{b,c.md"}, []string{"a/b"}},          // never closed
		// Sets, and one character.
		{"backend/[abc]*.go", []string{"backend/a.go", "backend/cmd.go"}, []string{"backend/d.go", "backend/x/a.go"}},
		{"v?/api.md", []string{"v1/api.md", "v2/api.md"}, []string{"v10/api.md", "v/api.md"}},
		{"photos [2024/**", nil, []string{"photos [2024/a.png", "photos 2024/a.png"}}, // a set that is never closed: no file
		{"photos \\[2024/**", []string{"photos [2024/a.png"}, nil},
		// The patterns of the rules awkit writes.
		{"backend/**", []string{"backend/main.go", "backend/internal/x/y.go", "backend/docs/architecture.md"}, []string{"backend", "frontend/a.cs", "x/backend/a.go"}},
		{"docs/**", []string{"docs/README.md", "docs/design/a.md"}, []string{"backend/docs/a.md", "docs"}},
		{"**/docs/**", []string{"docs/a.md", "backend/docs/a.md", "a/b/docs/c/d.md"}, []string{"backend/docsx/a.md", "docs"}},
		// A directory.
		{"docs/", []string{"docs/a.md", "docs/x/y.md"}, []string{"docs", "backend/docs/a.md"}},
		{"node_modules", []string{"node_modules", "a/node_modules"}, []string{"a/node_modules/x.js"}},
		// One star stays within a name.
		{"src/*", []string{"src/a.ts"}, []string{"src/x/a.ts"}},
		{"**", []string{"a", "a/b/c"}, nil},
		{"", nil, []string{"a", ""}},
	}
	for _, c := range cases {
		for _, file := range c.yes {
			if !Match(c.pattern, file) {
				t.Errorf("%q does not match %q, and should", c.pattern, file)
			}
		}
		for _, file := range c.no {
			if Match(c.pattern, file) {
				t.Errorf("%q matches %q, and should not", c.pattern, file)
			}
		}
	}
}

// Braces are written out before a pattern is matched, and what they would
// expand to is bounded: a pattern past the bound is taken as it is written.
func TestExpandBraces(t *testing.T) {
	for pattern, want := range map[string][]string{
		"a":               {"a"},
		"a{b,c}d":         {"abd", "acd"},
		"{a,b}{c,d}":      {"ac", "ad", "bc", "bd"},
		"{a,{b,c}}":       {"a", "b", "c"},
		"x{,y}":           {"x", "xy"},
		"{a}":             {"{a}"},
		"{a}{b,c}":        {"{a}b", "{a}c"},
		"\\{a,b\\}":       {"\\{a,b\\}"},
		"{a,b":            {"{a,b"},
		"a}{b,c}":         {"a}b", "a}c"},
		"{a\\,b,c}":       {"a\\,b", "c"},
		"{{a,b},{c,d}}.x": {"a.x", "b.x", "c.x", "d.x"},
		// A group without a comma holds no alternatives; the one inside it does.
		"{{a,b}}": {"{a}", "{b}"},
		// A brace with a backslash in front opens nothing and closes nothing,
		// and a comma with one separates nothing.
		"\\{a,b}":   {"\\{a,b}"},
		"{a\\,b}":   {"{a\\,b}"},
		"{a,b\\}c}": {"a", "b\\}c"},
		"{a,b}\\":   {"a\\", "b\\"},
	} {
		if got := expandBraces(pattern); !reflect.DeepEqual(got, want) {
			t.Errorf("expandBraces(%q) = %q, want %q", pattern, got, want)
		}
	}

	// Ten groups of two are 1024 patterns: past the bound, so the pattern is
	// one pattern, and its braces match themselves only.
	many := "{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}"
	if got := expandBraces(many); len(got) != 1 || got[0] != many {
		t.Errorf("a pattern that expands to 1024 was expanded to %d", len(got))
	}
	if Match(many, "aaaaaaaaaa") {
		t.Error("a pattern past the bound still matches through its braces")
	}
	nine := "{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}{a,b}"
	if got := expandBraces(nine); len(got) != 512 || !Match(nine, "ababababa") {
		t.Errorf("a pattern that expands to 512 was expanded to %d", len(got))
	}
	// The bound is 1000: that many are written out, one more is not.
	const ten = "{0,1,2,3,4,5,6,7,8,9}"
	if got := expandBraces(ten + ten + ten); len(got) != 1000 || got[0] != "000" || got[1] != "001" || got[999] != "999" {
		t.Errorf("a pattern that expands to 1000 was expanded to %d", len(got))
	}
	over := "{a,b,c,d,e,f,g}{a,b,c,d,e,f,g,h,i,j,k}{a,b,c,d,e,f,g,h,i,j,k,l,m}" // 7 x 11 x 13 = 1001
	if got := expandBraces(over); len(got) != 1 || got[0] != over {
		t.Errorf("a pattern that expands to 1001 was expanded to %d", len(got))
	}
	// Past the bound a pattern is taken whole, not half written out: a part
	// of it that is too long by itself does not leave the rest expanded.
	deep := "{x,y}" + many
	if got := expandBraces(deep); len(got) != 1 || got[0] != deep {
		t.Errorf("a pattern whose end expands past the bound came back as %d patterns: %.60q", len(got), got)
	}
	inside := "{" + many + ",z}"
	if got := expandBraces(inside); len(got) != 1 || got[0] != inside {
		t.Errorf("a pattern whose alternative expands past the bound came back as %d patterns", len(got))
	}
}

// A rule is about a file when it names none, or when one of its patterns
// matches it; the file is named from the project's root, however it is
// written.
func TestAppliesTo(t *testing.T) {
	backend := Rule{File: ".claude/rules/backend.md", Paths: []string{"backend/**", "cmd/*.go"}}
	everywhere := Rule{File: ".claude/rules/agents.md"}
	docs := Rule{File: ".claude/rules/docs.md", Paths: []string{"docs/**", "**/docs/**", "**/*.md"}}

	for file, want := range map[string][3]bool{
		"backend/main.go":              {true, true, false},
		"cmd/tool.go":                  {true, true, false},
		"cmd/sub/tool.go":              {false, true, false},
		"frontend/Game.cs":             {false, true, false},
		"backend/docs/architecture.md": {true, true, true},
		"docs/design/game.md":          {false, true, true},
		"README.md":                    {false, true, true},
		"./backend/main.go":            {true, true, false},
		"backend\\internal\\a.go":      {true, true, false},
		"backend//internal/../a.go":    {true, true, false},
	} {
		got := [3]bool{backend.AppliesTo(file), everywhere.AppliesTo(file), docs.AppliesTo(file)}
		if got != want {
			t.Errorf("%q: the rules about the backend, about everything and about documents apply %v, want %v", file, got, want)
		}
	}

	all := []Rule{backend, everywhere, docs}
	if got := For(all, "frontend/Game.cs"); len(got) != 1 || got[0].File != everywhere.File {
		t.Errorf("For(a frontend file) = %v", files(got))
	}
	if got, want := files(For(all, "docs/a.md", "backend/b.go")), []string{backend.File, everywhere.File, docs.File}; !reflect.DeepEqual(got, want) {
		t.Errorf("For(a document and a backend file) = %v, want every rule once, in their order %v", got, want)
	}
	if got := For(all); got != nil {
		t.Errorf("For(no file) = %v, want none", files(got))
	}
	if got := For(nil, "a.go"); got != nil {
		t.Errorf("For(no rule) = %v", files(got))
	}
}

func files(rules []Rule) []string {
	var names []string
	for _, rule := range rules {
		names = append(names, rule.File)
	}
	return names
}

// Every rule file under the rules directory is a rule of the project, at any
// depth, as Claude Code reads them; what is no Markdown file is not.
func TestLoadAll(t *testing.T) {
	root := t.TempDir()
	if got, err := LoadAll(root); err != nil || len(got) != 0 {
		t.Errorf("a project without a rules directory: %v, %v; want no rule and no error", files(got), err)
	}

	for file, content := range map[string]string{
		".claude/rules/testing.md":           "# Testing\n",
		".claude/rules/_kit/git-workflow.md": "# Git\n",
		".claude/rules/backend.md":           "# Backend\n",
		".claude/rules/backend/api.md":       "---\npaths:\n  - \"backend/api/**\"\n---\n\n# API\n",
		".claude/rules/broken.md":            "---\npaths: [a\n---\n# Broken\n",
		".claude/rules/notes.txt":            "not a rule",
		".claude/skills/x/SKILL.md":          "not a rule either",
	} {
		full := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	// In the order of their paths: a file comes before the folder of its name.
	want := []string{".claude/rules/_kit/git-workflow.md", ".claude/rules/backend.md", ".claude/rules/backend/api.md", ".claude/rules/broken.md", ".claude/rules/testing.md"}
	if !reflect.DeepEqual(files(got), want) {
		t.Fatalf("LoadAll = %v, want %v", files(got), want)
	}
	if got[2].Title != "API" || !reflect.DeepEqual(got[2].Paths, []string{"backend/api/**"}) {
		t.Errorf("the rule in a folder: %+v", got[2])
	}
	if got[3].Problem == "" || !got[3].Everywhere() {
		t.Errorf("the rule with broken frontmatter: %+v, want it loaded as a rule about everything, with what is wrong", got[3])
	}

	// A rule file that cannot be read is an error, not a rule that is quietly
	// missing: whoever works by the rules would work by fewer than there are.
	if err := os.Symlink(filepath.Join(root, "nowhere.md"), filepath.Join(root, ".claude", "rules", "gone.md")); err != nil {
		t.Skipf("this system does not let the test make a link: %v", err)
	}
	if got, err := LoadAll(root); err == nil {
		t.Errorf("a rule file that leads nowhere: %v, want an error", files(got))
	}
}

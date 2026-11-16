// Package rules reads the rule files of a project: the files under
// .claude/rules that say how work is done there.
//
// A rule may name the files it is about, as glob patterns under `paths:` in
// its frontmatter. Claude Code loads such a rule when a file that matches is
// worked on, and a rule that names none into every session. Whoever is not a
// Claude Code session in the project's root (a Worker played by another
// tool, a role of the planning council, a review of a worktree) is told what
// applies by this package instead.
package rules

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule is one rule file.
type Rule struct {
	// File is the rule's path from the project's root, with forward slashes.
	File string
	// Title is the rule's first heading, or the name of its file when it has
	// none.
	Title string
	// Paths are the files the rule is about, as glob patterns. A rule that
	// names none is about everything.
	Paths []string
	// Text is the rule without its frontmatter.
	Text string
}

// Load reads a rule file of a project. file is its path from the project's
// root, with forward slashes.
func Load(root, file string) (Rule, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return Rule{File: file}, err
	}
	return Parse(file, data)
}

// Parse reads a rule from the content of its file.
func Parse(file string, data []byte) (Rule, error) {
	rule := Rule{File: file}
	text := strings.ReplaceAll(string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))), "\r\n", "\n")

	if rest, found := strings.CutPrefix(text, "---\n"); found {
		head, body, closed := cutFrontmatter(rest)
		if !closed {
			return rule, fmt.Errorf("%s: the frontmatter is opened with --- and never closed", file)
		}
		var meta struct {
			Paths pathList `yaml:"paths"`
		}
		if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
			return rule, fmt.Errorf("%s: the frontmatter cannot be read: %w", file, err)
		}
		rule.Paths = meta.Paths
		text = body
	}

	rule.Text = strings.TrimLeft(text, "\n")
	rule.Title = strings.TrimSuffix(path.Base(file), ".md")
	for _, line := range strings.Split(rule.Text, "\n") {
		if title, found := strings.CutPrefix(line, "# "); found && strings.TrimSpace(title) != "" {
			rule.Title = strings.TrimSpace(title)
			break
		}
	}
	return rule, nil
}

// cutFrontmatter splits what follows the opening --- at the closing one.
func cutFrontmatter(rest string) (head, body string, closed bool) {
	if body, found := strings.CutPrefix(rest, "---\n"); found {
		return "", body, true
	}
	if head, body, found := strings.Cut(rest, "\n---\n"); found {
		return head, body, true
	}
	if head, found := strings.CutSuffix(rest, "\n---"); found {
		return head, "", true
	}
	return "", "", false
}

// pathList reads `paths:` written as a list or as one string of patterns
// separated by commas.
type pathList []string

func (p *pathList) UnmarshalYAML(node *yaml.Node) error {
	var list []string
	switch node.Kind {
	case yaml.ScalarNode:
		list = strings.Split(node.Value, ",")
	case yaml.SequenceNode:
		if err := node.Decode(&list); err != nil {
			return err
		}
	default:
		return fmt.Errorf("line %d: paths is a list of patterns", node.Line)
	}
	*p = nil
	for _, pattern := range list {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			*p = append(*p, pattern)
		}
	}
	return nil
}

// Everywhere reports whether the rule is about every file.
func (r Rule) Everywhere() bool {
	return len(r.Paths) == 0
}

// Scope says what the rule is about, for a list of rules: its patterns, or
// "everything".
func (r Rule) Scope() string {
	if r.Everywhere() {
		return "everything"
	}
	quoted := make([]string, len(r.Paths))
	for i, pattern := range r.Paths {
		quoted[i] = "`" + pattern + "`"
	}
	return strings.Join(quoted, ", ")
}

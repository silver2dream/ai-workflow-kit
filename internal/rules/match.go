package rules

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// LoadAll reads every rule file of a project: the .md files under its rules
// directory, at any depth, in the order of their paths. Claude Code reads the
// same files. A project without the directory has no rule.
func LoadAll(root string) ([]Rule, error) {
	var files []string
	dir := layout.Rules(root)
	err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			if file == dir && os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	loaded := make([]Rule, 0, len(files))
	for _, file := range files {
		rule, err := Load(root, file)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, rule)
	}
	return loaded, nil
}

// AppliesTo reports whether the rule is about a file: it names no file, or
// one of its patterns matches. file is the file's path from the project's
// root, with forward slashes.
func (r Rule) AppliesTo(file string) bool {
	if r.Everywhere() {
		return true
	}
	file = path.Clean(strings.ReplaceAll(file, "\\", "/"))
	for _, pattern := range r.Paths {
		if Match(pattern, file) {
			return true
		}
	}
	return false
}

// For returns the rules that are about at least one of the files, in the
// order the rules are given.
func For(all []Rule, files ...string) []Rule {
	var applying []Rule
	for _, rule := range all {
		if slices.ContainsFunc(files, rule.AppliesTo) {
			applying = append(applying, rule)
		}
	}
	return applying
}

// maxExpanded bounds what the braces of one pattern expand to. A pattern
// that would expand to more is taken as it is written, whole, and its braces
// then match themselves only.
const maxExpanded = 1000

// Match reports whether a pattern of a rule's `paths` matches a file, the way
// Claude Code reads such a pattern:
//
//   - a pattern without a slash matches a file of that name at any depth:
//     *.md is every Markdown file of the project
//   - a pattern with a slash is read from the project's root, and a slash in
//     front only says so: /*.md is the Markdown files in the root itself
//   - ** stands for any number of directories; * and ? stay within one name;
//     [abc] is one of a set
//   - {a,b} stands for either
//   - a pattern that ends in a slash is everything under that directory
//
// A pattern that cannot be read matches nothing.
func Match(pattern, file string) bool {
	for _, expanded := range expandBraces(pattern) {
		if matchOne(expanded, file) {
			return true
		}
	}
	return false
}

func matchOne(pattern, file string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	rooted := strings.Contains(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	if !rooted {
		pattern = "**/" + pattern
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

// matchSegments matches the names of a pattern against the names of a path.
func matchSegments(pattern, names []string) bool {
	if len(pattern) == 0 {
		return len(names) == 0
	}
	if pattern[0] == "**" {
		// Any number of directories, none included. As the last name of a
		// pattern it is everything below, which is at least one name.
		if len(pattern) == 1 {
			return len(names) > 0
		}
		for skip := 0; skip < len(names); skip++ {
			if matchSegments(pattern[1:], names[skip:]) {
				return true
			}
		}
		return false
	}
	if len(names) == 0 {
		return false
	}
	if ok, err := path.Match(pattern[0], names[0]); err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], names[1:])
}

// expandBraces writes out the alternatives of a pattern: a{b,c}d is abd and
// acd. A brace that is escaped, or never closed, or holds no comma, is a
// character like any other. A pattern with more alternatives than
// maxExpanded is returned as it is written.
func expandBraces(pattern string) []string {
	if expanded, ok := expand(pattern); ok {
		return expanded
	}
	return []string{pattern}
}

// expand writes out the alternatives of a pattern, the first group's first.
// It reports false when there are more than maxExpanded, at whatever depth.
func expand(pattern string) ([]string, bool) {
	open, shut := braces(pattern)
	if open < 0 {
		return []string{pattern}, true
	}
	rests, ok := expand(pattern[shut+1:])
	if !ok {
		return nil, false
	}
	var out []string
	for _, alternative := range splitAlternatives(pattern[open+1 : shut]) {
		heads, ok := expand(pattern[:open] + alternative)
		if !ok {
			return nil, false
		}
		for _, head := range heads {
			for _, rest := range rests {
				out = append(out, head+rest)
				if len(out) > maxExpanded {
					return nil, false
				}
			}
		}
	}
	return out, true
}

// braces finds the first group of braces that holds alternatives: where it
// opens and where it shuts. It returns -1 when there is none.
func braces(pattern string) (open, shut int) {
	for start := 0; start < len(pattern); start++ {
		switch pattern[start] {
		case '\\':
			start++
			continue
		case '{':
		default:
			continue
		}
		depth, comma := 0, false
		for i := start; i < len(pattern); i++ {
			switch pattern[i] {
			case '\\':
				i++
			case '{':
				depth++
			case ',':
				comma = comma || depth == 1
			case '}':
				depth--
				if depth == 0 {
					if comma {
						return start, i
					}
					i = len(pattern) // a group without a comma: look for the next
				}
			}
		}
	}
	return -1, -1
}

// splitAlternatives splits what is between two braces at its commas, leaving
// the commas of a group inside it alone.
func splitAlternatives(inside string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(inside); i++ {
		switch inside[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, inside[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, inside[start:])
}

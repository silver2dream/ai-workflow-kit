package ticket

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// The paths of a ticket name what its work may change: globs written from the
// root of the project, with / between folders. The engine keeps two tickets
// whose paths overlap from being worked on at the same time, and checks a
// pull request's files against the paths of its ticket.
//
// The globs are read in one way only, so that a person, the planning role and
// the engine read them alike:
//
//   - * stands for any characters within one name, a name that starts with a
//     dot included; ? stands for one character;
//   - ** alone between two / (or at either end) stands for any number of
//     folders, none included;
//   - every other character stands for itself. [ ] { } and a leading ! are
//     refused: other tools read them as glob syntax, each in its own way, and
//     reading them either way would be a guess.
const (
	// MaxPaths bounds the paths of one ticket.
	MaxPaths = 50
	// MaxPathLength bounds one path, in characters.
	MaxPathLength = 256
)

// The risks a ticket can carry.
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// Risks lists the risks a ticket can carry, from the least.
var Risks = []string{RiskLow, RiskMedium, RiskHigh}

// CheckGlob says why a path cannot be a path of a ticket, or nil when it can.
func CheckGlob(glob string) error {
	switch {
	case glob == "":
		return errors.New("a path is empty")
	case utf8.RuneCountInString(glob) > MaxPathLength:
		return fmt.Errorf("the path %.40q... is longer than %d characters", glob, MaxPathLength)
	case strings.TrimSpace(glob) != glob:
		return fmt.Errorf("the path %q has spaces at its start or end", glob)
	case strings.HasPrefix(glob, "/"):
		return fmt.Errorf("the path %q starts with /: write it from the project's root, without the /", glob)
	case strings.HasPrefix(glob, "./"):
		return fmt.Errorf("the path %q starts with ./: write it from the project's root, without the ./", glob)
	case strings.HasSuffix(glob, "/"):
		return fmt.Errorf("the path %q ends in /: name files; %s** is everything in the folder", glob, glob)
	case strings.Contains(glob, `\`):
		return fmt.Errorf(`the path %q has a \: folders are separated with /`, glob)
	case strings.HasPrefix(glob, "!"):
		return fmt.Errorf("the path %q starts with !: paths name what the ticket changes, and nothing is taken out of them", glob)
	}
	for _, r := range glob {
		if unicode.IsControl(r) {
			return fmt.Errorf("the path %q has a control character", glob)
		}
	}
	if i := strings.IndexAny(glob, "[]{}"); i >= 0 {
		return fmt.Errorf("the path %q has %c: a path is a glob of *, ? and ** only (write * or ? where a name varies)", glob, glob[i])
	}
	for _, name := range strings.Split(glob, "/") {
		switch {
		case name == "":
			return fmt.Errorf("the path %q has an empty folder name (//)", glob)
		case name == "." || name == "..":
			return fmt.Errorf("the path %q has %s in it: write it from the project's root, without . or ..", glob, name)
		case name != "**" && strings.Contains(name, "**"):
			return fmt.Errorf("the path %q has ** within a name: ** stands for any number of folders, and stands alone between two /", glob)
		}
	}
	return nil
}

// Overlap reports whether some path is matched by both globs: two tickets
// whose paths overlap may change the same file. Both globs are ones
// CheckGlob takes. Case counts; compare lowered globs to ignore it.
func Overlap(a, b string) bool {
	return namesOverlap(strings.Split(a, "/"), strings.Split(b, "/"))
}

// namesOverlap reports whether some list of names is matched by both lists
// of glob names, where ** matches any number of names.
func namesOverlap(a, b []string) bool {
	known := make(map[[2]int]bool)
	var from func(i, j int) bool
	from = func(i, j int) (overlap bool) {
		key := [2]int{i, j}
		if result, ok := known[key]; ok {
			return result
		}
		defer func() { known[key] = overlap }()
		switch {
		case i == len(a) && j == len(b):
			return true
		case i < len(a) && a[i] == "**":
			// It matches no name, or the next name of b's path (and maybe more).
			return from(i+1, j) || (j < len(b) && from(i, j+1))
		case j < len(b) && b[j] == "**":
			return from(i, j+1) || (i < len(a) && from(i+1, j))
		case i == len(a) || j == len(b):
			return false
		}
		return nameOverlap([]rune(a[i]), []rune(b[j])) && from(i+1, j+1)
	}
	return from(0, 0)
}

// nameOverlap reports whether some name is matched by both glob names, of *
// and ? and characters that stand for themselves. A name that only the empty
// string would match does not occur: such a glob is all *, and both then
// match "x" as well.
func nameOverlap(a, b []rune) bool {
	known := make(map[[2]int]bool)
	var from func(i, j int) bool
	from = func(i, j int) (overlap bool) {
		key := [2]int{i, j}
		if result, ok := known[key]; ok {
			return result
		}
		defer func() { known[key] = overlap }()
		switch {
		case i == len(a) && j == len(b):
			return true
		case i < len(a) && a[i] == '*':
			// It matches nothing more, or the next character b's glob stands for.
			return from(i+1, j) || (j < len(b) && from(i, j+1))
		case j < len(b) && b[j] == '*':
			return from(i, j+1) || (i < len(a) && from(i+1, j))
		case i == len(a) || j == len(b):
			return false
		case a[i] == '?' || b[j] == '?' || a[i] == b[j]:
			return from(i+1, j+1)
		}
		return false
	}
	return from(0, 0)
}

// protectedGlobs are the workflow's own files (layout.ProtectedDirs and
// layout.ProtectedFiles) as globs, lowered: they are compared without case.
// A folder's glob is every file in it, not the folder's own name.
func protectedGlobs() []string {
	var globs []string
	for _, dir := range layout.ProtectedDirs {
		globs = append(globs, strings.ToLower(strings.TrimSuffix(dir, "/"))+"/*/**")
	}
	for _, file := range layout.ProtectedFiles {
		globs = append(globs, strings.ToLower(file))
	}
	return globs
}

// protectedIn returns a file of the workflow's own that a path may take in,
// or "" when it takes in none. Case does not count.
func protectedIn(glob string) string {
	lowered := strings.ToLower(glob)
	for i, protected := range protectedGlobs() {
		if Overlap(lowered, protected) {
			if i < len(layout.ProtectedDirs) {
				return layout.ProtectedDirs[i]
			}
			return layout.ProtectedFiles[i-len(layout.ProtectedDirs)]
		}
	}
	return ""
}

// checkPaths checks the paths and the risk of a schema 2 block, in the order
// a person fills them in.
func (m Meta) checkPaths() error {
	if len(m.Paths) == 0 {
		return errors.New("paths is missing: list what the ticket changes, as globs from the project's root (backend/internal/match/**)")
	}
	if len(m.Paths) > MaxPaths {
		return fmt.Errorf("paths has %d entries; at most %d (name folders with ** instead of their files)", len(m.Paths), MaxPaths)
	}
	seen := map[string]bool{}
	for _, path := range m.Paths {
		if err := CheckGlob(path); err != nil {
			return fmt.Errorf("paths: %w", err)
		}
		if seen[path] {
			return fmt.Errorf("paths lists %q twice", path)
		}
		seen[path] = true
		if protected := protectedIn(path); protected != "" && !m.AllowScriptChanges {
			return fmt.Errorf("paths has %q, which takes in %s, one of the workflow's own files: name narrower paths, or set allow_script_changes: true when the ticket is to change them", path, protected)
		}
	}
	switch m.Risk {
	case RiskLow, RiskMedium, RiskHigh:
		return nil
	case "":
		return errors.New("risk is missing: low, medium or high")
	}
	return fmt.Errorf("risk %q is not one of low, medium, high", m.Risk)
}

// RepoDir returns the folder of a repo from the project's root, with forward
// slashes and no slash at either end, the way a configuration writes it
// ("backend/", "./backend", "./"): "" for the root of the project.
func RepoDir(dir string) string {
	dir = strings.ReplaceAll(strings.TrimSpace(dir), `\`, "/")
	for strings.HasPrefix(dir, "./") {
		dir = strings.TrimPrefix(dir, "./")
	}
	dir = strings.Trim(dir, "/")
	if dir == "." {
		return ""
	}
	return dir
}

// CheckRepo checks that the paths of a ticket lie in the folder of its repo,
// dir as the configuration writes it: a ticket is worked on in its repo. The
// root of the project takes in every path, and so does a ticket that may
// change files outside its repo (allow_parent_changes).
func (m Meta) CheckRepo(dir string) error {
	dir = RepoDir(dir)
	if dir == "" || m.AllowParentChanges {
		return nil
	}
	folder := strings.Split(dir, "/")
	for _, path := range m.Paths {
		names := strings.Split(path, "/")
		inside := len(names) > len(folder)
		for k := 0; inside && k < len(folder); k++ {
			inside = names[k] == folder[k]
		}
		if !inside {
			return fmt.Errorf("paths has %q, which is not in the folder of repo %s: write it from the project's root, starting with %s/", path, m.Repo, dir)
		}
	}
	return nil
}

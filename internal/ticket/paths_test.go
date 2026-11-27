package ticket

import (
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCheckGlob(t *testing.T) {
	for _, glob := range []string{
		"backend/internal/match/**", "backend/docs/match.md", "**/*.go", "src/**/test/*_test.go", "x/?.txt", "*",
		"**", ".github/workflows/ci.yml", "docs/a b.md", "名稱/檔案.md", "a!b", "a/!b", "**/**", "x/**/y/**/z",
		strings.Repeat("a", MaxPathLength),
	} {
		if err := CheckGlob(glob); err != nil {
			t.Errorf("%q was refused: %v", glob, err)
		}
	}
	for glob, want := range map[string]string{
		"":                                   "empty",
		strings.Repeat("a", MaxPathLength+1): "longer than 256 characters",
		strings.Repeat("名", MaxPathLength+1): "longer than 256 characters",
		" backend/x.go":                      "spaces at its start or end",
		"backend/x.go\t":                     "spaces at its start or end",
		"/backend/x.go":                      "starts with /",
		"./backend/x.go":                     "starts with ./",
		"backend/":                           "ends in /: name files; backend/** is everything in the folder",
		`backend\x.go`:                       `has a \`,
		"!backend/x.go":                      "starts with !",
		"backend/x\x00.go":                   "control character",
		"backend/x\u0085.go":                 "control character",
		"backend/[ab].go":                    "has [",
		"backend/a].go":                      "has ]",
		"backend/{a,b}.go":                   "has {",
		"backend/a}.go":                      "has }",
		"backend//x.go":                      "empty folder name",
		"backend/./x.go":                     "has . in it",
		"backend/../x.go":                    "has .. in it",
		"..":                                 "has .. in it",
		"backend/**.go":                      "** within a name",
		"backend/a**":                        "** within a name",
		"***":                                "** within a name",
	} {
		err := CheckGlob(glob)
		if err == nil {
			t.Errorf("%q was taken", glob)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %q, want it to say %q", glob, err, want)
		}
	}
}

func TestOverlap(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"backend/x.go", "backend/x.go", true},
		{"backend/x.go", "backend/y.go", false},
		{"backend/**", "backend/internal/match/x.go", true},
		{"backend/**", "frontend/**", false},
		{"backend/**", "**/match/**", true},
		{"backend/*.go", "backend/internal/x.go", false},
		{"backend/*.go", "backend/x.*", true},
		{"backend/a*.go", "backend/*b.go", true},
		{"backend/a*.go", "backend/b*.go", false},
		{"backend/?.go", "backend/ab.go", false},
		{"backend/??.go", "backend/ab.go", true},
		{"backend/*", "backend", false},
		{"backend/**", "backend", true}, // ** matches no folder: conservative
		{"**", "anything/at/all", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/c", false},
		{"a/b/**", "**/c/d", true}, // a/b/c/d
		{"*/x", "y/*", true},
		{"*.md", "CLAUDE.md", true},
		{"*.md", "docs/a.md", false},
		{".claude/**", "*/settings.json", true},
		{"名?/x", "名稱/x", true},
		{"名?/x", "名稱詞/x", false},
		{"Backend/x.go", "backend/x.go", false}, // case counts
	} {
		if got := Overlap(tt.a, tt.b); got != tt.want {
			t.Errorf("Overlap(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
		if got := Overlap(tt.b, tt.a); got != tt.want {
			t.Errorf("Overlap(%q, %q) = %v, want %v", tt.b, tt.a, got, tt.want)
		}
	}
}

// Overlap says whether some path matches both globs. Checked against a
// matcher written separately, over every glob of at most two names (each a
// name of at most two of a, b, * and ?, or **) and every path of at most four
// names of at most two of a and b: a path both globs match, if there is one,
// is among those.
func TestOverlap_EveryPairOfSmallGlobs(t *testing.T) {
	var names []string
	for _, x := range []string{"a", "b"} {
		names = append(names, x)
		for _, y := range []string{"a", "b"} {
			names = append(names, x+y)
		}
	}
	paths := [][]string{{}}
	var all [][]string
	for depth := 1; depth <= 4; depth++ {
		var next [][]string
		for _, path := range paths {
			for _, name := range names {
				next = append(next, append(append([]string{}, path...), name))
			}
		}
		all = append(all, next...)
		paths = next
	}

	var segments []string
	for _, x := range []string{"a", "b", "*", "?"} {
		segments = append(segments, x)
		for _, y := range []string{"a", "b", "*", "?"} {
			segments = append(segments, x+y)
		}
	}
	segments = append(segments, "**")
	var globs []string
	for _, first := range segments {
		globs = append(globs, first)
		for _, second := range segments {
			globs = append(globs, first+"/"+second)
		}
	}

	words := (len(all) + 63) / 64
	matched := make([][]uint64, len(globs))
	for i, glob := range globs {
		if err := CheckGlob(glob); err != nil {
			t.Fatalf("%q: %v", glob, err)
		}
		matched[i] = make([]uint64, words)
		for k, path := range all {
			if referenceMatch(strings.Split(glob, "/"), path) {
				matched[i][k/64] |= 1 << (k % 64)
			}
		}
	}
	wrong := 0
	for i := range globs {
		for j := i; j < len(globs); j++ {
			common := 0
			for w := 0; w < words; w++ {
				common += bits.OnesCount64(matched[i][w] & matched[j][w])
			}
			if got := Overlap(globs[i], globs[j]); got != (common > 0) {
				if wrong++; wrong <= 10 {
					t.Errorf("Overlap(%q, %q) = %v; %d paths match both", globs[i], globs[j], got, common)
				}
			}
		}
	}
	if wrong > 0 {
		t.Errorf("%d pairs of globs are wrong", wrong)
	}
}

// referenceMatch matches a path against a glob, ** by recursion and a name
// by a regular expression.
func referenceMatch(glob, path []string) bool {
	if len(glob) == 0 {
		return len(path) == 0
	}
	if glob[0] == "**" {
		return referenceMatch(glob[1:], path) || (len(path) > 0 && referenceMatch(glob, path[1:]))
	}
	if len(path) == 0 {
		return false
	}
	return nameMatcher(glob[0]).MatchString(path[0]) && referenceMatch(glob[1:], path[1:])
}

var nameMatchers = map[string]*regexp.Regexp{}

// nameMatcher is the regular expression of a glob name.
func nameMatcher(name string) *regexp.Regexp {
	if matcher, ok := nameMatchers[name]; ok {
		return matcher
	}
	var pattern strings.Builder
	for _, r := range name {
		switch r {
		case '*':
			pattern.WriteString(".*")
		case '?':
			pattern.WriteString(".")
		default:
			pattern.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	nameMatchers[name] = regexp.MustCompile("^(?:" + pattern.String() + ")$")
	return nameMatchers[name]
}

// A path takes in a file of the workflow's own when some file of that name
// would be matched by it, whatever the case.
func TestProtectedIn(t *testing.T) {
	for glob, want := range map[string]string{
		"**":                    ".claude/",
		"**/x.go":               ".claude/",
		".claude/**":            ".claude/",
		".Claude/settings.json": ".claude/",
		"*/settings.json":       ".claude/",
		".*/rules/x.md":         ".claude/",
		"*.md":                  "CLAUDE.md",
		"claude.md":             "CLAUDE.md",
		"AGENTS.md":             "AGENTS.md",
		"*":                     "CLAUDE.md",
		".ai/**":                ".ai/config.yaml",
		".ai/*.yaml":            ".ai/config.yaml",
		".ai/roster.yaml":       ".ai/roster.yaml",
		".mcp.*":                ".mcp.json",
	} {
		if got := protectedIn(glob); got != want {
			t.Errorf("protectedIn(%q) = %q, want %q", glob, got, want)
		}
	}
	for _, glob := range []string{"docs/**", "backend/**", "src/CLAUDE.md", ".ai/state/**", ".github/**", "*.go", "src/**/*.go", ".claudex/x", "x/.claude/y", ".claude"} {
		if got := protectedIn(glob); got != "" {
			t.Errorf("protectedIn(%q) = %q; it takes in none", glob, got)
		}
	}
}

func TestCheckRepo(t *testing.T) {
	meta := func(paths ...string) Meta {
		return Meta{Schema: 2, Repo: "backend", Paths: paths, Risk: "low"}
	}
	for _, dir := range []string{"backend", "backend/", "./backend", "./backend/", `backend\`, " backend/ "} {
		if err := meta("backend/**", "backend/internal/x.go", "backend/*.md").CheckRepo(dir); err != nil {
			t.Errorf("dir %q: paths in the folder were refused: %v", dir, err)
		}
	}
	for _, path := range []string{"backend", "frontend/x.go", "*/x.go", "**/x.go", "**", "backendx/y.go", "Backend/x.go", "backend*/x.go"} {
		err := meta(path).CheckRepo("backend/")
		if err == nil || !strings.Contains(err.Error(), "not in the folder of repo backend") || !strings.Contains(err.Error(), "starting with backend/") {
			t.Errorf("path %q: %v; want it refused as outside backend/", path, err)
		}
	}
	nested := Meta{Schema: 2, Repo: "api", Paths: []string{"services/api/**"}, Risk: "low"}
	if err := nested.CheckRepo("services/api"); err != nil {
		t.Errorf("a repo two folders down: %v", err)
	}
	if err := (Meta{Schema: 2, Repo: "api", Paths: []string{"services/x/**"}, Risk: "low"}).CheckRepo("services/api"); err == nil {
		t.Error("a path in the parent folder of a repo two folders down was taken")
	}
	// The root of the project takes in everything, and so does a ticket
	// that may change files outside its repo.
	for _, dir := range []string{"", "./", ".", "/", " "} {
		if err := meta("frontend/x.go", "**").CheckRepo(dir); err != nil {
			t.Errorf("root dir %q: %v", dir, err)
		}
	}
	outside := meta("frontend/x.go")
	outside.AllowParentChanges = true
	if err := outside.CheckRepo("backend/"); err != nil {
		t.Errorf("allow_parent_changes: %v", err)
	}
	// A schema 1 ticket has no paths to check.
	if err := (Meta{Schema: 1, Repo: "backend", Risk: "medium"}).CheckRepo("backend/"); err != nil {
		t.Errorf("schema 1: %v", err)
	}
}

func TestRepoDir(t *testing.T) {
	for dir, want := range map[string]string{
		"": "", "./": "", ".": "", "/": "", "backend": "backend", "backend/": "backend", "./backend/": "backend",
		`services\api\`: "services/api", " ././x/ ": "x",
	} {
		if got := RepoDir(dir); got != want {
			t.Errorf("RepoDir(%q) = %q, want %q", dir, got, want)
		}
	}
}

// A ticket may have as many as MaxPaths paths, and no more.
func TestValidate_AsManyPathsAsAllowed(t *testing.T) {
	var paths []string
	for i := 0; i < MaxPaths; i++ {
		paths = append(paths, "backend/"+strconv.Itoa(i)+".go")
	}
	meta := Meta{Schema: 2, Repo: "backend", Paths: paths, Risk: "low"}
	if err := meta.Validate(); err != nil {
		t.Errorf("%d paths were refused: %v", MaxPaths, err)
	}
	meta.Paths = append(meta.Paths, "backend/one-more.go")
	if err := meta.Validate(); err == nil {
		t.Errorf("%d paths were taken", MaxPaths+1)
	}
}

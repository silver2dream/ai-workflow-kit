package selfheal

import (
	"testing"
)

func TestInferPreset(t *testing.T) {
	cases := []struct {
		name  string
		repos []Repo
		want  string
	}{
		{"react-go", []Repo{{Language: "go"}, {Language: "node"}}, "react-go"},
		{"react-python", []Repo{{Language: "python"}, {Language: "node"}}, "react-python"},
		{"go only", []Repo{{Language: "go"}}, "go"},
		{"node only", []Repo{{Language: "node"}}, "node"},
		{"unknown", []Repo{{Language: "rust"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferPreset(tc.repos); got != tc.want {
				t.Errorf("inferPreset = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRepoBaseFile(t *testing.T) {
	cases := map[string]Repo{
		"backend/go.mod":         {Path: "backend/", Language: "go"},
		"frontend/package.json":  {Path: "frontend/", Language: "node"},
		"backend/pyproject.toml": {Path: "backend/", Language: "python"},
		"go.mod":                 {Path: ".", Language: "go"},
	}
	for want, repo := range cases {
		if got := repoBaseFile(repo); got != want {
			t.Errorf("repoBaseFile(%+v) = %q, want %q", repo, got, want)
		}
	}
	if got := repoBaseFile(Repo{Path: "x", Language: "rust"}); got != "" {
		t.Errorf("unknown language should yield no base file, got %q", got)
	}
}

func TestMissingRepoScaffold(t *testing.T) {
	repos := []Repo{
		{Path: "backend/", Language: "go"},
		{Path: "frontend/", Language: "node"},
	}

	// Only backend present -> frontend missing.
	present := map[string]bool{"backend/go.mod": true}
	got := missingRepoScaffold("origin/main", repos, func(_, p string) bool { return present[p] })
	if len(got) != 1 || got[0] != "frontend/" {
		t.Errorf("want [frontend/], got %v", got)
	}

	// Nothing present -> both missing (this is the tennis-arena "empty main" case).
	got = missingRepoScaffold("origin/main", repos, func(_, _ string) bool { return false })
	if len(got) != 2 {
		t.Errorf("want both repos missing, got %v", got)
	}

	// Everything present -> healthy.
	got = missingRepoScaffold("origin/main", repos, func(_, _ string) bool { return true })
	if len(got) != 0 {
		t.Errorf("want nothing missing, got %v", got)
	}
}

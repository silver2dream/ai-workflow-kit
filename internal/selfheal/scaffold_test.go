package selfheal

import (
	"reflect"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/install"
)

func TestInferPreset(t *testing.T) {
	cases := []struct {
		name  string
		repos []Repo
		want  string
	}{
		{"go backend and csharp frontend", []Repo{{Language: "go"}, {Language: "csharp"}}, install.PresetGodotGo},
		{"spelled differently", []Repo{{Language: "Golang"}, {Language: "Godot"}}, install.PresetGodotGo},
		{"go only", []Repo{{Language: "go"}}, ""},
		{"csharp only", []Repo{{Language: "csharp"}}, ""},
		{"a stack without a preset", []Repo{{Language: "go"}, {Language: "node"}}, ""},
		{"no stack named", []Repo{{Language: "generic"}}, ""},
		{"no repos", nil, ""},
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
		"backend/go.mod": {Path: "backend/", Language: "go"},
		"go.mod":         {Path: ".", Language: "go"},
	}
	for want, repo := range cases {
		if got := repoBaseFile(repo); got != want {
			t.Errorf("repoBaseFile(%+v) = %q, want %q", repo, got, want)
		}
	}
	// A C# repo names its own solution and project files: no file of the
	// skeleton proves or disproves that the repo has been set up.
	for _, lang := range []string{"csharp", "godot", "generic", "rust"} {
		if got := repoBaseFile(Repo{Path: "frontend/", Language: lang}); got != "" {
			t.Errorf("language %q should have no base file, got %q", lang, got)
		}
	}
}

func TestMissingRepoScaffold(t *testing.T) {
	repos := []Repo{
		{Path: "backend/", Language: "go"},
		{Path: "frontend/", Language: "csharp"},
	}

	// The backend is there: nothing can be said to be missing.
	present := map[string]bool{"backend/go.mod": true}
	got := missingRepoScaffold("origin/main", repos, func(_, p string) bool { return present[p] })
	if len(got) != 0 {
		t.Errorf("want nothing missing, got %v", got)
	}

	// Nothing present -> the backend is missing (the "empty main" case). The
	// frontend has no file that could prove it, so it is not reported.
	got = missingRepoScaffold("origin/main", repos, func(_, _ string) bool { return false })
	if !reflect.DeepEqual(got, []string{"backend/"}) {
		t.Errorf("want [backend/], got %v", got)
	}

	// Only the probed file is asked about.
	var asked []string
	missingRepoScaffold("origin/main", repos, func(_, p string) bool { asked = append(asked, p); return true })
	if !reflect.DeepEqual(asked, []string{"backend/go.mod"}) {
		t.Errorf("probed %v, want only backend/go.mod", asked)
	}
}

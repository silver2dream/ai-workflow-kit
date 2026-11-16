package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"gopkg.in/yaml.v3"
)

// gitIn runs git in dir and fails the test when git does.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// repoOn makes a repository in a new directory with one commit on each of the
// named branches, and leaves the last one checked out. Without a branch it is
// not a repository at all.
func repoOn(t *testing.T, branches ...string) string {
	t.Helper()
	dir := t.TempDir()
	// A temporary directory may itself be inside a repository; git must not
	// look above it.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	for i, branch := range branches {
		if i == 0 {
			gitIn(t, dir, "init", "-q", "-b", branch)
			gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
			continue
		}
		gitIn(t, dir, "checkout", "-q", "-b", branch)
	}
	return dir
}

// The configuration of a new project names branches that are there. One that
// named a branch nobody had created (the presets wrote `feat/example`) failed
// at the first ticket, where the Worker's worktree is branched from it.
func TestDetectBranches(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		if got := DetectBranches(repoOn(t)); got != DefaultBranches() {
			t.Errorf("DetectBranches = %+v, want one trunk named main", got)
		}
	})

	t.Run("a repository without a commit", func(t *testing.T) {
		dir := repoOn(t)
		gitIn(t, dir, "init", "-q", "-b", "trunk")
		if got, want := DetectBranches(dir), (Branches{"trunk", "trunk"}); got != want {
			t.Errorf("DetectBranches = %+v, want %+v: the branch that is checked out, though it has no commit yet", got, want)
		}
	})

	for _, tc := range []struct {
		name     string
		branches []string
		want     Branches
	}{
		{"one trunk", []string{"main"}, Branches{"main", "main"}},
		{"master is the trunk", []string{"master"}, Branches{"master", "master"}},
		{"a trunk of another name", []string{"trunk"}, Branches{"trunk", "trunk"}},
		{"develop and main", []string{"main", "develop"}, Branches{"develop", "main"}},
		{"develop and main, main checked out", []string{"develop", "main"}, Branches{"develop", "main"}},
		{"a feature branch is checked out", []string{"main", "feat/login"}, Branches{"main", "main"}},
		{"a feature branch is checked out, master is the trunk", []string{"master", "feat/login"}, Branches{"master", "master"}},
		{"main is preferred to master", []string{"master", "main", "feat/login"}, Branches{"main", "main"}},
		{"a feature branch and develop", []string{"main", "develop", "feat/login"}, Branches{"develop", "main"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectBranches(repoOn(t, tc.branches...)); got != tc.want {
				t.Errorf("DetectBranches = %+v, want %+v", got, tc.want)
			}
		})
	}

	// What the remote calls its default branch is the release branch, whatever
	// its name and whatever is checked out; a develop that is only on the
	// remote counts.
	t.Run("the default branch of the remote", func(t *testing.T) {
		origin := repoOn(t, "stable", "develop", "main")
		gitIn(t, origin, "checkout", "-q", "stable")
		clone := filepath.Join(t.TempDir(), "clone")
		gitIn(t, filepath.Dir(clone), "clone", "-q", origin, clone)
		gitIn(t, clone, "checkout", "-q", "-b", "feat/login")
		if got, want := DetectBranches(clone), (Branches{"develop", "stable"}); got != want {
			t.Errorf("DetectBranches = %+v, want %+v", got, want)
		}
	})
}

// gitOf reads the git section of the configuration an install wrote.
func gitOf(t *testing.T, dir string) (integration, release string) {
	t.Helper()
	data, err := os.ReadFile(layout.Config(dir))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Git struct {
			Integration string `yaml:"integration_branch"`
			Release     string `yaml:"release_branch"`
		} `yaml:"git"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("the configuration is not valid YAML: %v\n%s", err, data)
	}
	if strings.Contains(string(data), "feat/example") {
		t.Errorf("the configuration names the placeholder branch feat/example")
	}
	return cfg.Git.Integration, cfg.Git.Release
}

// A preset writes the branches it is given, in both presets, and one trunk
// named main when it is given none.
func TestInstall_TheConfigurationNamesTheGivenBranches(t *testing.T) {
	for _, preset := range Presets {
		for _, tc := range []struct {
			name                     string
			given                    Branches
			wantIntegration, wantRel string
		}{
			{"none given", Branches{}, "main", "main"},
			{"both given", Branches{"develop", "main"}, "develop", "main"},
			{"one trunk", Branches{"trunk", "trunk"}, "trunk", "trunk"},
			{"only the release branch", Branches{Release: "master"}, "master", "master"},
			{"only the integration branch", Branches{Integration: "develop"}, "develop", "develop"},
			{"names YAML would read otherwise", Branches{"feat/#12: next", "release/1.0"}, "feat/#12: next", "release/1.0"},
		} {
			t.Run(preset+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				if _, err := Install(createMinimalMockFS(), dir, Options{Preset: preset, NoGenerate: true, Branches: tc.given}); err != nil {
					t.Fatalf("Install: %v", err)
				}
				integration, release := gitOf(t, dir)
				if integration != tc.wantIntegration || release != tc.wantRel {
					t.Errorf("git = %q, %q; want %q, %q", integration, release, tc.wantIntegration, tc.wantRel)
				}
			})
		}
	}
}

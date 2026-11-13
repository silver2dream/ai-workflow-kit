package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ciWorkflow is the part of a generated workflow the tests look at.
type ciWorkflow struct {
	Name string `yaml:"name"`
	On   struct {
		Push struct {
			Branches []string `yaml:"branches"`
		} `yaml:"push"`
		PullRequest struct {
			Branches []string `yaml:"branches"`
		} `yaml:"pull_request"`
	} `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Defaults struct {
			Run struct {
				WorkingDirectory string `yaml:"working-directory"`
			} `yaml:"run"`
		} `yaml:"defaults"`
		Steps []ciStep `yaml:"steps"`
	} `yaml:"jobs"`
}

type ciStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
	Run  string            `yaml:"run"`
}

// commands are the scripts of the run steps, in order, as the shell gets them
// minus the final newline of the block they are written in.
func (w ciWorkflow) commands() []string {
	var out []string
	for _, step := range w.Jobs["build"].Steps {
		if step.Run != "" {
			out = append(out, strings.TrimSuffix(step.Run, "\n"))
		}
	}
	return out
}

// actions are the actions of the job in order, without their versions.
func (w ciWorkflow) actions() []string {
	var out []string
	for _, step := range w.Jobs["build"].Steps {
		if step.Uses != "" {
			out = append(out, strings.SplitN(step.Uses, "@", 2)[0])
		}
	}
	return out
}

// setup is the step that uses the named action.
func (w ciWorkflow) setup(t *testing.T, action string) ciStep {
	t.Helper()
	for _, step := range w.Jobs["build"].Steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			return step
		}
	}
	t.Fatalf("no step uses %s", action)
	return ciStep{}
}

func parseWorkflowText(t *testing.T, name, content string) ciWorkflow {
	t.Helper()
	var w ciWorkflow
	if err := yaml.Unmarshal([]byte(content), &w); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", name, err, content)
	}
	return w
}

func parseWorkflow(t *testing.T, path string) ciWorkflow {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parseWorkflowText(t, filepath.Base(path), string(data))
}

var ciGit = GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}

func ciContext(repos ...RepoConfig) *TemplateContext {
	return &TemplateContext{Config: Config{
		Repos:   repos,
		Git:     ciGit,
		Project: ProjectConfig{Type: "monorepo"},
	}}
}

func writeWorkflowFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, ".github", "workflows", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

var (
	goBackend = RepoConfig{Name: "backend", Path: "backend/", Type: "directory", Language: "go",
		Verify: VerifyConfig{Build: "go build ./...", Test: "go test ./..."}}
	csharpFrontend = RepoConfig{Name: "frontend", Path: "frontend/", Type: "directory", Language: "csharp",
		Verify: VerifyConfig{Build: "dotnet build", Test: "dotnet test"}}
)

// The Go version is the module's own, wherever the module sits.
func TestCIContent_GoVersionComesFromTheModule(t *testing.T) {
	tests := []struct {
		name               string
		repo               RepoConfig
		wantMod, wantCache string
	}{
		{"directory repo", RepoConfig{Name: "backend", Path: "backend/", Type: "directory"}, "backend/go.mod", "backend/go.mod\nbackend/go.sum\n"},
		{"nested directory", RepoConfig{Name: "api", Path: "services/api/", Type: "directory"}, "services/api/go.mod", "services/api/go.mod\nservices/api/go.sum\n"},
		{"root repo", RepoConfig{Name: "root", Path: "./", Type: "root"}, "go.mod", "go.mod\ngo.sum\n"},
		// A submodule's workflow runs in the submodule's own repository.
		{"submodule", RepoConfig{Name: "backend", Path: "backend/", Type: "submodule"}, "go.mod", "go.mod\ngo.sum\n"},
	}
	for _, tt := range tests {
		tt.repo.Language = "go"
		tt.repo.Verify = VerifyConfig{Test: "go test ./..."}

		w := parseWorkflowText(t, tt.name, generateCIContent(tt.repo, ciGit))

		with := w.setup(t, "actions/setup-go").With
		if with["go-version-file"] != tt.wantMod {
			t.Errorf("%s: go-version-file = %q, want %q", tt.name, with["go-version-file"], tt.wantMod)
		}
		if _, pinned := with["go-version"]; pinned {
			t.Errorf("%s: the workflow pins a Go version of its own next to the module's", tt.name)
		}
		// go.mod is part of the cache key: a module without dependencies has no go.sum.
		if with["cache-dependency-path"] != tt.wantCache {
			t.Errorf("%s: cache-dependency-path = %q, want %q", tt.name, with["cache-dependency-path"], tt.wantCache)
		}
	}
}

// The workflow runs on Linux: a repo path keeps its case and uses forward
// slashes whatever machine generated it.
func TestCIContent_WorkingDirectory(t *testing.T) {
	tests := []struct {
		repoType, path, want string
	}{
		{"directory", "backend/", "backend"},
		{"directory", "./backend/", "backend"},
		{"directory", "Client/Game/", "Client/Game"},
		{"directory", `Client\Game\`, "Client/Game"},
		{"directory", "my #1 app/", "my #1 app"},
		{"directory", "key: value/", "key: value"},
		{"directory", "./", ""},
		{"directory", ".", ""},
		{"root", "./", ""},
		{"submodule", "backend/", ""},
	}
	for _, tt := range tests {
		repo := RepoConfig{Name: "app", Path: tt.path, Type: tt.repoType, Language: "generic", Verify: VerifyConfig{Test: "make test"}}

		w := parseWorkflowText(t, tt.path, generateCIContent(repo, ciGit))

		if got := w.Jobs["build"].Defaults.Run.WorkingDirectory; got != tt.want {
			t.Errorf("%s repo at %q: working directory = %q, want %q", tt.repoType, tt.path, got, tt.want)
		}
	}
}

// Whatever a command contains, the workflow stays valid and the shell gets
// the command unchanged.
func TestCIContent_CommandsReachTheShellUnchanged(t *testing.T) {
	commands := []string{
		`go test ./...`,
		`dotnet test --logger "console;verbosity=normal"`,
		`echo "key: value"`,
		`make test # not a comment to YAML`,
		`FOO='a b' make "test"`,
		`*.sh`,
		`& wait`,
		`! false`,
		`{ make a; make b; }`,
		`[ -f Makefile ] && make`,
		`| tee log`,
		`> out.txt echo hi`,
		`%cmd%`,
		`@echo off`,
		"`make flags`",
		`- dash first`,
		`? question`,
		`yes: no`,
		`true`,
		`123`,
		`~`,
		`null`,
		`echo \n 'it''s' "\"q\""`,
		"make a\nmake b",
		"if [ -d sub ]; then\n  cd sub\n\tmake\nfi",
		"make a\n\nmake b",
		"# a leading comment line\nmake",
		`echo 測試`,
	}
	for _, command := range commands {
		repo := RepoConfig{Name: "app", Path: "./", Type: "root", Language: "generic",
			Verify: VerifyConfig{Setup: command, Build: command, Test: command}}

		content := generateCIContent(repo, ciGit)
		w := parseWorkflowText(t, command, content)

		if got := w.commands(); !reflect.DeepEqual(got, []string{command, command, command}) {
			t.Errorf("command %q reached the workflow as %q", command, got)
		}
		for _, line := range strings.Split(content, "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("command %q: the workflow has a line with trailing whitespace: %q", command, line)
			}
		}
	}
}

func TestCIContent_CommandWhitespaceIsNormalized(t *testing.T) {
	repo := RepoConfig{Name: "app", Path: "./", Type: "root", Language: "generic",
		Verify: VerifyConfig{Build: "  make a\r\nmake b\r\n", Test: "\n\tmake test\n\n"}}

	content := generateCIContent(repo, ciGit)
	w := parseWorkflowText(t, "ci.yml", content)

	if got := w.commands(); !reflect.DeepEqual(got, []string{"make a\nmake b", "make test"}) {
		t.Errorf("commands = %q, want them trimmed and with plain line feeds", got)
	}
	if strings.Contains(content, "\r") {
		t.Errorf("a carriage return from the config ended up in the workflow:\n%q", content)
	}
}

// The Worker runs verify.setup before it builds; CI does the same, or a build
// that passes for the Worker could fail there.
func TestCIContent_SetupRunsBeforeBuildAndTest(t *testing.T) {
	repo := RepoConfig{Name: "frontend", Path: "frontend/", Type: "directory", Language: "csharp",
		Verify: VerifyConfig{Setup: "dotnet tool restore", Build: "dotnet build", Test: "dotnet test"}}

	w := parseWorkflowText(t, "ci.yml", generateCIContent(repo, ciGit))

	if got := w.commands(); !reflect.DeepEqual(got, []string{"dotnet tool restore", "dotnet build", "dotnet test"}) {
		t.Errorf("commands = %q, want setup, build, test in that order", got)
	}
	var names []string
	for _, step := range w.Jobs["build"].Steps {
		if step.Run != "" {
			names = append(names, step.Name)
		}
	}
	if !reflect.DeepEqual(names, []string{"Setup", "Build", "Test"}) {
		t.Errorf("run steps are named %v", names)
	}
}

func TestCIContent_OnlyTheCommandsThatAreSet(t *testing.T) {
	repo := RepoConfig{Name: "lib", Path: "./", Type: "root", Language: "go", Verify: VerifyConfig{Build: " \t", Test: "go test ./..."}}

	w := parseWorkflowText(t, "ci.yml", generateCIContent(repo, ciGit))

	if got := w.commands(); !reflect.DeepEqual(got, []string{"go test ./..."}) {
		t.Errorf("commands = %q, want only the test command", got)
	}
	// A step that neither uses an action nor runs anything is not a valid step.
	for _, step := range w.Jobs["build"].Steps {
		if step.Uses == "" && step.Run == "" {
			t.Errorf("step %q does nothing", step.Name)
		}
	}
}

func TestCIContent_Triggers(t *testing.T) {
	tests := []struct {
		name            string
		git             GitConfig
		wantPush, wantP []string
	}{
		{"integration and release", GitConfig{IntegrationBranch: "develop", ReleaseBranch: "main"}, []string{"develop", "main"}, []string{"develop"}},
		{"one branch for both", GitConfig{IntegrationBranch: "main", ReleaseBranch: "main"}, []string{"main"}, []string{"main"}},
		{"no release branch", GitConfig{IntegrationBranch: "main"}, []string{"main"}, []string{"main"}},
		{"names YAML would misread", GitConfig{IntegrationBranch: "release/1.0", ReleaseBranch: "on"}, []string{"release/1.0", "on"}, []string{"release/1.0"}},
		{"names with YAML syntax in them", GitConfig{IntegrationBranch: "feat#1, x", ReleaseBranch: "[main]"}, []string{"feat#1, x", "[main]"}, []string{"feat#1, x"}},
	}
	for _, tt := range tests {
		repo := RepoConfig{Name: "app", Path: "./", Type: "root", Verify: VerifyConfig{Test: "make test"}}

		w := parseWorkflowText(t, tt.name, generateCIContent(repo, tt.git))

		if !reflect.DeepEqual(w.On.Push.Branches, tt.wantPush) {
			t.Errorf("%s: push branches = %q, want %q", tt.name, w.On.Push.Branches, tt.wantPush)
		}
		if !reflect.DeepEqual(w.On.PullRequest.Branches, tt.wantP) {
			t.Errorf("%s: pull_request branches = %q, want %q", tt.name, w.On.PullRequest.Branches, tt.wantP)
		}
	}
}

func TestCIContent_RepoNameIsQuoted(t *testing.T) {
	repo := RepoConfig{Name: `core: "api" #1`, Path: "./", Type: "root", Verify: VerifyConfig{Test: "make test"}}

	w := parseWorkflowText(t, "ci.yml", generateCIContent(repo, ciGit))

	if w.Name != `core: "api" #1 CI` {
		t.Errorf("workflow name = %q", w.Name)
	}
}

// Every action a workflow uses is one of the versions declared in ci.go.
func TestCIContent_UsesTheDeclaredActionVersions(t *testing.T) {
	declared := map[string]bool{actionCheckout: true, actionSetupGo: true, actionSetupDotnet: true}
	for _, language := range []string{"go", "csharp", "generic"} {
		repo := RepoConfig{Name: "app", Path: "./", Type: "root", Language: language, Verify: VerifyConfig{Test: "make test"}}

		w := parseWorkflowText(t, language, generateCIContent(repo, ciGit))

		for _, step := range w.Jobs["build"].Steps {
			if step.Uses != "" && !declared[step.Uses] {
				t.Errorf("language %q: step uses %q, which is not a declared action version", language, step.Uses)
			}
		}
	}
}

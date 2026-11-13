package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// presetConfig is the part of workflow.yaml the presets differ in.
type presetConfig struct {
	Project struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
	} `yaml:"project"`
	Repos []struct {
		Name     string `yaml:"name"`
		Path     string `yaml:"path"`
		Type     string `yaml:"type"`
		Language string `yaml:"language"`
		Verify   struct {
			Build string `yaml:"build"`
			Test  string `yaml:"test"`
		} `yaml:"verify"`
	} `yaml:"repos"`
	Rules struct {
		Custom []string `yaml:"custom"`
	} `yaml:"rules"`
	Worker struct {
		Backend string `yaml:"backend"`
	} `yaml:"worker"`
}

// installedConfig installs a preset into a fresh directory and parses the
// workflow config it wrote.
func installedConfig(t *testing.T, preset string) presetConfig {
	t.Helper()
	dir := t.TempDir()
	if _, err := Install(createMinimalMockFS(), dir, Options{Preset: preset, ProjectName: "demo", NoGenerate: true}); err != nil {
		t.Fatalf("Install(%q): %v", preset, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".ai", "config", "workflow.yaml"))
	if err != nil {
		t.Fatalf("read workflow.yaml: %v", err)
	}
	var cfg presetConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("preset %q wrote a config that is not valid YAML: %v\n%s", preset, err, data)
	}
	return cfg
}

func TestPreset_GenericAssumesNoStack(t *testing.T) {
	for _, preset := range []string{PresetGeneric, ""} {
		cfg := installedConfig(t, preset)

		if cfg.Project.Name != "demo" || cfg.Project.Type != "single-repo" {
			t.Errorf("preset %q: project = %+v, want a single repo named demo", preset, cfg.Project)
		}
		if len(cfg.Repos) != 1 {
			t.Fatalf("preset %q: %d repos, want 1", preset, len(cfg.Repos))
		}
		repo := cfg.Repos[0]
		if repo.Path != "./" || repo.Type != "root" || repo.Language != "generic" {
			t.Errorf("preset %q: repo = %+v, want the repository root with no stack named", preset, repo)
		}
		if repo.Verify.Build != "" || repo.Verify.Test != "" {
			t.Errorf("preset %q: verify = %+v; the generic preset must not guess the project's commands", preset, repo.Verify)
		}
		if len(cfg.Rules.Custom) != 0 {
			t.Errorf("preset %q: rules.custom = %v, want none", preset, cfg.Rules.Custom)
		}
	}
}

func TestPreset_GodotGoIsGoBackendAndCSharpFrontend(t *testing.T) {
	cfg := installedConfig(t, PresetGodotGo)

	if cfg.Project.Type != "monorepo" || len(cfg.Repos) != 2 {
		t.Fatalf("project type %q with %d repos, want a monorepo of 2", cfg.Project.Type, len(cfg.Repos))
	}
	backend, frontend := cfg.Repos[0], cfg.Repos[1]
	if backend.Name != "backend" || backend.Path != "backend/" || backend.Type != "directory" || backend.Language != "go" {
		t.Errorf("backend = %+v", backend)
	}
	if backend.Verify.Build != "go build ./..." || backend.Verify.Test != "go test ./..." {
		t.Errorf("backend verify = %+v", backend.Verify)
	}
	if frontend.Name != "frontend" || frontend.Path != "frontend/" || frontend.Type != "directory" || frontend.Language != "csharp" {
		t.Errorf("frontend = %+v, want a C# repo (not gdscript)", frontend)
	}
	if frontend.Verify.Build != "dotnet build" || frontend.Verify.Test != "dotnet test" {
		t.Errorf("frontend verify = %+v, want the dotnet commands", frontend.Verify)
	}
	// A listed rule that does not exist makes the Worker stop; none ship yet.
	if len(cfg.Rules.Custom) != 0 {
		t.Errorf("rules.custom = %v, want none until the rule files exist", cfg.Rules.Custom)
	}
	if cfg.Worker.Backend != "codex" {
		t.Errorf("worker.backend = %q, want the shared default", cfg.Worker.Backend)
	}
}

func TestPreset_RemovedAndUnknownNamesAreRejected(t *testing.T) {
	for _, preset := range []string{"react-go", "unity-go", "node", "go", "no-such-preset"} {
		dir := t.TempDir()
		_, err := Install(createMinimalMockFS(), dir, Options{Preset: preset, NoGenerate: true})
		if !errors.Is(err, ErrUnknownPreset) {
			t.Errorf("Install(%q) error = %v, want ErrUnknownPreset", preset, err)
			continue
		}
		for _, name := range Presets {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("Install(%q) error %q should name the available preset %q", preset, err, name)
			}
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".ai", "config", "workflow.yaml")); !os.IsNotExist(statErr) {
			t.Errorf("Install(%q) wrote a config for a preset that does not exist", preset)
		}
	}
}

func TestPresets_EveryTableUsesListedNames(t *testing.T) {
	listed := map[string]bool{}
	for _, name := range Presets {
		listed[name] = true
		if _, ok := presetConfigs[name]; !ok {
			t.Errorf("preset %q is listed but has no config", name)
		}
	}
	if len(presetConfigs) != len(Presets) {
		t.Errorf("presetConfigs has %d entries, Presets lists %d", len(presetConfigs), len(Presets))
	}
	for name := range scaffoldRoots {
		if !listed[name] {
			t.Errorf("scaffoldRoots has %q, which is not a listed preset", name)
		}
	}
	for name := range ciWorkflows {
		if !listed[name] {
			t.Errorf("ciWorkflows has %q, which is not a listed preset", name)
		}
	}
	if !reflect.DeepEqual(Presets, []string{PresetGeneric, PresetGodotGo}) {
		t.Errorf("Presets = %v", Presets)
	}
}

// ---------------------------------------------------------------------------
// CI workflow
// ---------------------------------------------------------------------------

func installedCI(t *testing.T, preset string) (content string, exists bool) {
	t.Helper()
	dir := t.TempDir()
	if _, err := Install(createMinimalMockFS(), dir, Options{Preset: preset, NoGenerate: true, WithCI: true}); err != nil {
		t.Fatalf("Install(%q): %v", preset, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "ci.yml"))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func TestCI_GodotGoBuildsAndTestsBothSides(t *testing.T) {
	content, exists := installedCI(t, PresetGodotGo)
	if !exists {
		t.Fatal("the godot-go preset should write a CI workflow")
	}
	var workflow struct {
		Jobs map[string]struct {
			Defaults struct {
				Run struct {
					WorkingDirectory string `yaml:"working-directory"`
				} `yaml:"run"`
			} `yaml:"defaults"`
			Steps []struct {
				Uses string `yaml:"uses"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(content), &workflow); err != nil {
		t.Fatalf("ci.yml is not valid YAML: %v\n%s", err, content)
	}

	commands := func(job string) []string {
		var out []string
		for _, step := range workflow.Jobs[job].Steps {
			if step.Run != "" {
				out = append(out, step.Run)
			}
		}
		return out
	}
	if dir := workflow.Jobs["backend"].Defaults.Run.WorkingDirectory; dir != "backend" {
		t.Errorf("backend job runs in %q, want backend", dir)
	}
	if got, want := commands("backend"), []string{"go build ./...", "go vet ./...", "go test ./..."}; !reflect.DeepEqual(got, want) {
		t.Errorf("backend job runs %v, want %v", got, want)
	}
	if dir := workflow.Jobs["frontend"].Defaults.Run.WorkingDirectory; dir != "frontend" {
		t.Errorf("frontend job runs in %q, want frontend", dir)
	}
	if got, want := commands("frontend"), []string{"dotnet build", "dotnet test --no-build"}; !reflect.DeepEqual(got, want) {
		t.Errorf("frontend job runs %v, want %v", got, want)
	}
	for _, leftover := range []string{"npm", "node", "unity", "awk:"} {
		if strings.Contains(strings.ToLower(content), leftover) {
			t.Errorf("ci.yml still mentions %q", leftover)
		}
	}
}

func TestCI_GenericWritesNoWorkflow(t *testing.T) {
	if content, exists := installedCI(t, PresetGeneric); exists {
		t.Errorf("the generic preset knows nothing about the build, so it must not write a CI workflow; got:\n%s", content)
	}
}

func TestCI_ExistingWorkflowIsKeptUnlessForced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	own := "name: mine\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n"
	if err := os.WriteFile(path, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureCIWorkflow(dir, PresetGodotGo, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != own {
		t.Errorf("an existing workflow was replaced without force:\n%s", got)
	}

	if err := ensureCIWorkflow(dir, PresetGodotGo, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != godotGoCI {
		t.Errorf("force should write the preset's workflow, got:\n%s", got)
	}

	// Force with a preset that has no workflow leaves the project's own alone.
	if err := os.WriteFile(path, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCIWorkflow(dir, PresetGeneric, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != own {
		t.Errorf("the generic preset has no workflow to write, the file must stay:\n%s", got)
	}
}

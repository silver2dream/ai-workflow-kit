package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
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
	data, err := os.ReadFile(layout.Config(dir))
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
		if _, statErr := os.Stat(layout.Config(dir)); !os.IsNotExist(statErr) {
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
	if !reflect.DeepEqual(Presets, []string{PresetGeneric, PresetGodotGo}) {
		t.Errorf("Presets = %v", Presets)
	}
}

// ---------------------------------------------------------------------------
// CI workflows: init gets them from the generator, one per repo
// ---------------------------------------------------------------------------

// installCI installs with the given options and returns the project's
// workflow files by name.
func installCI(t *testing.T, dir string, opts Options) map[string]string {
	t.Helper()
	result, err := Install(createMinimalMockFS(), dir, opts)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.GenerateError != nil {
		t.Fatalf("the generated files were not written: %v", result.GenerateError)
	}
	return workflowFiles(t, dir)
}

func workflowFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	workflows := filepath.Join(dir, ".github", "workflows")
	entries, err := os.ReadDir(workflows)
	if os.IsNotExist(err) {
		return files
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(workflows, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(data)
	}
	return files
}

func workflowNames(files map[string]string) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func writeWorkflow(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, ".github", "workflows", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// builtWorkflow is what a generated workflow builds: where, and with which
// commands.
type builtWorkflow struct {
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

func parseBuiltWorkflow(t *testing.T, name, content string) (dir string, commands []string) {
	t.Helper()
	var w builtWorkflow
	if err := yaml.Unmarshal([]byte(content), &w); err != nil {
		t.Fatalf("%s is not valid YAML: %v\n%s", name, err, content)
	}
	if len(w.Jobs) != 1 {
		t.Fatalf("%s has %d jobs, want the one build job:\n%s", name, len(w.Jobs), content)
	}
	job, ok := w.Jobs["build"]
	if !ok {
		t.Fatalf("%s has no build job:\n%s", name, content)
	}
	for _, step := range job.Steps {
		if step.Run != "" {
			commands = append(commands, strings.TrimSuffix(step.Run, "\n"))
		}
	}
	return job.Defaults.Run.WorkingDirectory, commands
}

// The workflows build and test each side with the very commands the preset
// put into the config, the ones the Worker verifies with.
func TestInitCI_GodotGoGetsAWorkflowPerRepo(t *testing.T) {
	dir := t.TempDir()

	files := installCI(t, dir, Options{Preset: PresetGodotGo, CI: generate.CISync})

	if got := workflowNames(files); !reflect.DeepEqual(got, []string{"ci-backend.yml", "ci-frontend.yml"}) {
		t.Fatalf("workflows = %v, want one per repo", got)
	}
	cfg := installedConfig(t, PresetGodotGo)
	for _, repo := range cfg.Repos {
		name := "ci-" + repo.Name + ".yml"
		workDir, commands := parseBuiltWorkflow(t, name, files[name])
		if workDir != repo.Name {
			t.Errorf("%s runs in %q, want %q", name, workDir, repo.Name)
		}
		if want := []string{repo.Verify.Build, repo.Verify.Test}; !reflect.DeepEqual(commands, want) {
			t.Errorf("%s runs %q, want the config's verify commands %q", name, commands, want)
		}
	}
	if !strings.Contains(files["ci-backend.yml"], "actions/setup-go@") {
		t.Errorf("the backend workflow should set up Go:\n%s", files["ci-backend.yml"])
	}
	if !strings.Contains(files["ci-frontend.yml"], "actions/setup-dotnet@") {
		t.Errorf("the frontend workflow should set up .NET:\n%s", files["ci-frontend.yml"])
	}
}

// The generic preset names no commands, so there is nothing a workflow could
// run.
func TestInitCI_GenericGetsNoWorkflow(t *testing.T) {
	files := installCI(t, t.TempDir(), Options{Preset: PresetGeneric, CI: generate.CISync})

	if len(files) != 0 {
		t.Errorf("the generic preset wrote %v", workflowNames(files))
	}
}

func TestInitCI_OnlyWhenAsked(t *testing.T) {
	without := installCI(t, t.TempDir(), Options{Preset: PresetGodotGo})
	if len(without) != 0 {
		t.Errorf("no CI policy was given, yet %v were written", workflowNames(without))
	}

	// CI workflows are generated files: --no-generate skips them too.
	noGenerate := installCI(t, t.TempDir(), Options{Preset: PresetGodotGo, CI: generate.CISync, NoGenerate: true})
	if len(noGenerate) != 0 {
		t.Errorf("NoGenerate is on, yet %v were written", workflowNames(noGenerate))
	}
}

func TestInitCI_ExistingWorkflowIsKeptUnlessForced(t *testing.T) {
	dir := t.TempDir()
	const own = "name: mine\non: push\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: make\n"
	writeWorkflow(t, dir, "ci-backend.yml", own)

	files := installCI(t, dir, Options{Preset: PresetGodotGo, CI: generate.CISync})
	if files["ci-backend.yml"] != own {
		t.Errorf("the project's own workflow was replaced without force:\n%s", files["ci-backend.yml"])
	}
	if _, ok := files["ci-frontend.yml"]; !ok {
		t.Errorf("the missing workflow should still be written, got %v", workflowNames(files))
	}

	files = installCI(t, dir, Options{Preset: PresetGodotGo, CI: generate.CIRewrite})
	if files["ci-backend.yml"] == own {
		t.Error("force should rewrite the workflow from the config")
	}
	if _, commands := parseBuiltWorkflow(t, "ci-backend.yml", files["ci-backend.yml"]); !reflect.DeepEqual(commands, []string{"go build ./...", "go test ./..."}) {
		t.Errorf("the rewritten workflow runs %q", commands)
	}
}

// oldKitCI is the ci.yml a long-gone version of the kit wrote: one file for
// every repo, with an awk job that ran the kit's own tests.
const oldKitCI = `name: ci

on:
  push:
    branches: ["develop"]

jobs:
  awk:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run AWK tests
        run: bash .ai/scripts/evaluate.sh

  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Go test
        working-directory: backend
        run: go test ./...
`

// A project that already has the old single ci.yml keeps it as its CI. The awk
// job goes, the project's jobs stay, and no per-repo workflow is put next to
// it to run everything a second time.
func TestInitCI_OldKitWorkflowIsMigratedNotDuplicated(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "ci.yml", oldKitCI)

	files := installCI(t, dir, Options{Preset: PresetGodotGo, CI: generate.CISync})

	if got := workflowNames(files); !reflect.DeepEqual(got, []string{"ci.yml"}) {
		t.Fatalf("workflows = %v, want only the project's ci.yml", got)
	}
	ci := files["ci.yml"]
	if strings.Contains(ci, "awk:") || strings.Contains(ci, "evaluate.sh") {
		t.Errorf("the awk job is still there:\n%s", ci)
	}
	if !strings.Contains(ci, "backend:") || !strings.Contains(ci, "go test ./...") {
		t.Errorf("the project's own job was lost:\n%s", ci)
	}
}

// The old awk job goes whatever the CI policy is, even when nothing is
// generated at all.
func TestInstall_OldAwkJobGoesWhateverThePolicy(t *testing.T) {
	for name, opts := range map[string]Options{
		"no policy":   {Preset: PresetGodotGo},
		"refresh":     {Preset: PresetGodotGo, CI: generate.CIRefresh},
		"no generate": {Preset: PresetGodotGo, CI: generate.CISync, NoGenerate: true},
	} {
		dir := t.TempDir()
		writeWorkflow(t, dir, "ci.yml", oldKitCI)

		files := installCI(t, dir, opts)

		if got := workflowNames(files); !reflect.DeepEqual(got, []string{"ci.yml"}) {
			t.Fatalf("%s: workflows = %v, want only the project's ci.yml", name, got)
		}
		if strings.Contains(files["ci.yml"], "awk:") {
			t.Errorf("%s: the awk job is still there:\n%s", name, files["ci.yml"])
		}
	}
}

// What `awkit upgrade` asks for. The workflows awkit generated follow the new
// kit; one the project took over stays as it is; none is added.
func TestUpgradeCI_RefreshesAwkitsWorkflowsAndAddsNone(t *testing.T) {
	dir := t.TempDir()
	fresh := installCI(t, dir, Options{Preset: PresetGodotGo, CI: generate.CISync})

	// The backend workflow as an older kit would have written it; the
	// frontend workflow taken over by the project, the way its header says.
	older := strings.Replace(fresh["ci-backend.yml"], "go test ./...", "go test -short ./...", 1)
	if older == fresh["ci-backend.yml"] {
		t.Fatal("could not make the backend workflow look out of date")
	}
	writeWorkflow(t, dir, "ci-backend.yml", older)
	headerEnd := strings.Index(fresh["ci-frontend.yml"], "\n\n")
	if headerEnd < 0 || !strings.HasPrefix(fresh["ci-frontend.yml"], "# Generated by awkit") {
		t.Fatalf("the generated workflow has no header to delete:\n%s", fresh["ci-frontend.yml"])
	}
	mine := fresh["ci-frontend.yml"][headerEnd+2:] + "# our own step list\n"
	writeWorkflow(t, dir, "ci-frontend.yml", mine)

	result, err := Install(createMinimalMockFS(), dir, Options{Preset: PresetGodotGo, Force: true, SkipConfig: true, CI: generate.CIRefresh})
	if err != nil || result.GenerateError != nil {
		t.Fatalf("Install: %v / %v", err, result.GenerateError)
	}
	files := workflowFiles(t, dir)

	if files["ci-backend.yml"] != fresh["ci-backend.yml"] {
		t.Errorf("awkit's own workflow should be brought back in step with the config:\n%s", files["ci-backend.yml"])
	}
	if files["ci-frontend.yml"] != mine {
		t.Errorf("the workflow the project took over was changed:\n%s", files["ci-frontend.yml"])
	}
	if len(result.CIWritten) != 1 || filepath.Base(result.CIWritten[0]) != "ci-backend.yml" {
		t.Errorf("CIWritten = %v, want the one workflow that was refreshed", result.CIWritten)
	}

	// A project without workflows gets none from an upgrade.
	bare := t.TempDir()
	if files := installCI(t, bare, Options{Preset: PresetGodotGo, CI: generate.CIRefresh}); len(files) != 0 {
		t.Errorf("an upgrade added %v", workflowNames(files))
	}
}

// A repo that leaves the config takes awkit's workflow for it along, and the
// install says which file went.
func TestInstallCI_ReportsTheWorkflowsItRemoved(t *testing.T) {
	dir := t.TempDir()
	installCI(t, dir, Options{Preset: PresetGodotGo, CI: generate.CISync})

	// The project becomes a single generic repo: neither workflow has a repo.
	result, err := Install(createMinimalMockFS(), dir, Options{Preset: PresetGeneric, ForceConfig: true, CI: generate.CISync})
	if err != nil || result.GenerateError != nil {
		t.Fatalf("Install: %v / %v", err, result.GenerateError)
	}

	if files := workflowFiles(t, dir); len(files) != 0 {
		t.Errorf("workflows left behind: %v", workflowNames(files))
	}
	var removed []string
	for _, file := range result.CIRemoved {
		removed = append(removed, filepath.Base(file))
	}
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, []string{"ci-backend.yml", "ci-frontend.yml"}) {
		t.Errorf("CIRemoved = %v, want both workflows", removed)
	}
}

// The kit is installed even when the files derived from the config cannot be
// written, and the caller is told why.
func TestInstall_ReportsAFailedGenerate(t *testing.T) {
	dir := t.TempDir()
	config := layout.Config(dir)
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	// The project's own config is kept, and this one is not valid.
	if err := os.WriteFile(config, []byte("project:\n  name: demo\n  type: no-such-type\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Install(createMinimalMockFS(), dir, Options{Preset: PresetGodotGo, CI: generate.CISync})

	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.ConfigSkipped {
		t.Fatal("the existing config should have been kept")
	}
	if result.GenerateError == nil {
		t.Fatal("GenerateError is nil although the config is not valid")
	}
	if !strings.Contains(result.GenerateError.Error(), "project.type") {
		t.Errorf("the error should say what is wrong with the config, got: %v", result.GenerateError)
	}
	if _, err := os.Stat(filepath.Join(layout.KitRules(dir), "git-workflow.md")); err != nil {
		t.Errorf("the kit files should be installed all the same: %v", err)
	}
	if files := workflowFiles(t, dir); len(files) != 0 {
		t.Errorf("no workflow can come from a config that is not valid, got %v", workflowNames(files))
	}
}

func TestInstall_GenerateErrorIsNilWhenGenerateIsSkipped(t *testing.T) {
	result, err := Install(createMinimalMockFS(), t.TempDir(), Options{Preset: PresetGeneric, NoGenerate: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.GenerateError != nil {
		t.Errorf("GenerateError = %v for an install that did not generate", result.GenerateError)
	}
}

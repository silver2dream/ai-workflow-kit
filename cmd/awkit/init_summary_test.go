package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/generate"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
)

// summaryText is what init prints for a summary, with every run of spaces
// inside a line as one: the tests are about the words, not the columns.
func summaryText(s initSummary) string {
	var out bytes.Buffer
	printInitSummary(&out, s)
	return oneSpace(out.String())
}

var spaces = regexp.MustCompile(`([^ 
]) {2,}`)

func oneSpace(s string) string { return spaces.ReplaceAllString(s, "$1 ") }

// Init ends with what it did, in the terms a person thinks in, and with what
// to do next. The step it used to leave out is the commit: a run starts from
// a clean working tree, and init has just written two dozen files.
func TestPrintInitSummary(t *testing.T) {
	out := summaryText(initSummary{
		Target: "this directory", Preset: "godot-go",
		Integration: "develop", Release: "main", Repos: []string{"backend", "frontend"},
		Skills: 3, Subagents: 2, Rules: 4,
		Workflows:    []string{"ci-backend.yml", "ci-frontend.yml"},
		SkeletonDirs: []string{"backend/", "frontend/"}, SkeletonFiles: 15,
		InRepo: true,
	})
	for _, want := range []string{
		"✓ AWK is set up in this directory (preset godot-go)\n",
		".ai/config.yaml the configuration: pull requests go to develop, releases are cut from main\n",
		".ai/roster.yaml who sits on the planning council (awkit plan)\n",
		".claude/ what agents read: 3 skills, 2 subagents, 4 rule files\n",
		".github/workflows/ ci-backend.yml, ci-frontend.yml\n",
		"backend/, frontend/ the project skeleton, 15 files\n",
		"  1. Look over .ai/config.yaml\n",
		"  2. git add -A && git commit -m \"[chore] set up awkit\"\n     a run starts from a clean working tree\n",
		"  3. awkit doctor\n",
		"  4. awkit plan \"<what you want built>\"\n",
		"  5. awkit run\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary lacks %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"awkit generate", "[generate]", "cd ", "preset was not applied", "⚠"} {
		if strings.Contains(out, gone) {
			t.Errorf("the summary says %q:\n%s", gone, out)
		}
	}
}

// What the summary says differently: one trunk, one of each, a project that
// has no build and test commands yet, one that is not a repository, a
// configuration that was kept, and what went wrong on the way.
func TestPrintInitSummary_Variations(t *testing.T) {
	oneTrunk := summaryText(initSummary{Target: "demo", Preset: "generic", Integration: "main", Release: "main", Skills: 1, Subagents: 1, Rules: 1})
	for _, want := range []string{
		"the configuration: pull requests go to main\n",
		"what agents read: 1 skill, 1 subagent, 1 rule file\n",
		"git init && git add -A && git commit",
		"not a git repository yet",
	} {
		if !strings.Contains(oneTrunk, want) {
			t.Errorf("the summary lacks %q:\n%s", want, oneTrunk)
		}
	}
	for _, gone := range []string{"releases are cut from", ".github/workflows/", "skeleton"} {
		if strings.Contains(oneTrunk, gone) {
			t.Errorf("the summary of a project with one trunk, no workflow and no skeleton says %q:\n%s", gone, oneTrunk)
		}
	}

	noCommands := summaryText(initSummary{Target: "demo", Preset: "generic", NoCommands: []string{"root"}, InRepo: true})
	if !strings.Contains(noCommands, "  1. Edit .ai/config.yaml\n     set the build and test commands (verify:) of root: the Worker and the review run them, and they are empty\n") {
		t.Errorf("a project without build and test commands is not told to set them first:\n%s", noCommands)
	}

	kept := initSummary{Target: "demo", Preset: "godot-go", ConfigKept: true, Integration: "main", Release: "main", InRepo: true}
	if out := summaryText(kept); !strings.Contains(out, "✓ AWK is set up in demo\n") || !strings.Contains(out, "the configuration that was there, kept as it is: pull requests go to main\n") || strings.Contains(out, "preset") {
		t.Errorf("a configuration that was kept, and no preset named:\n%s", out)
	}
	kept.PresetNamed = true
	if out := summaryText(kept); !strings.Contains(out, "The preset was not applied: the project has a configuration.") || !strings.Contains(out, "awkit init --preset godot-go --force-config\n") {
		t.Errorf("a configuration that was kept though a preset was named:\n%s", out)
	}

	skeleton := summaryText(initSummary{Target: "demo", Preset: "godot-go", SkeletonDirs: []string{"backend/"}, SkeletonFiles: 1, SkeletonKept: 14, InRepo: true})
	if !strings.Contains(skeleton, "backend/ the project skeleton, 1 file (14 that were there are kept; --force writes over them)\n") {
		t.Errorf("a skeleton that was mostly there:\n%s", skeleton)
	}

	warned := summaryText(initSummary{Target: "demo", Preset: "generic", Warnings: []string{"custom rule 'backend' not found: .claude/rules/backend.md"}, InRepo: true})
	if !strings.Contains(warned, "⚠ custom rule 'backend' not found: .claude/rules/backend.md\n") {
		t.Errorf("a warning of the generator is not shown:\n%s", warned)
	}
}

// The summary is read from what the install left behind.
func TestCollectInitSummary(t *testing.T) {
	dir := t.TempDir()
	result, err := install.Install(awkit.KitFS, dir, install.Options{
		Preset: install.PresetGodotGo, ProjectName: "demo", Scaffold: true, CI: generate.CISync,
		Branches: install.Branches{Integration: "develop", Release: "main"},
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := collectInitSummary(dir, install.PresetGodotGo, result)
	want := initSummary{
		Target: dir, Preset: install.PresetGodotGo,
		Integration: "develop", Release: "main", Repos: []string{"backend", "frontend"},
		Skills: 3, Subagents: 2, Rules: 4,
		Workflows:    []string{"ci-backend.yml", "ci-frontend.yml"},
		SkeletonDirs: []string{"backend/", "frontend/"}, SkeletonFiles: 15,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectInitSummary =\n%+v\nwant\n%+v", got, want)
	}

	// A second init keeps the configuration and the skeleton, and says so.
	again, err := install.Install(awkit.KitFS, dir, install.Options{Preset: install.PresetGodotGo, Scaffold: true, CI: generate.CISync})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	second := collectInitSummary(dir, install.PresetGodotGo, again)
	if !second.ConfigKept || second.SkeletonFiles != 0 || second.SkeletonKept != 15 || second.Integration != "develop" {
		t.Errorf("a second init: %+v, want the configuration and the 15 skeleton files kept", second)
	}

	generic := t.TempDir()
	if err := os.Mkdir(filepath.Join(generic, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err = install.Install(awkit.KitFS, generic, install.Options{Preset: install.PresetGeneric, CI: generate.CISync})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	got = collectInitSummary(generic, install.PresetGeneric, result)
	if !reflect.DeepEqual(got.NoCommands, []string{"root"}) || got.Rules != 1 || len(got.Workflows) != 0 || got.SkeletonFiles != 0 || !got.InRepo || got.Integration != "main" {
		t.Errorf("the generic preset: %+v, want a repo without commands, the kit's one rule, no workflow, no skeleton, a repository, one trunk", got)
	}

	if got := collectInitSummary(".", "generic", nil); got.Target != "this directory" {
		t.Errorf("the directory the person is in is called %q", got.Target)
	}
}

// What `awkit init` prints, start to end: the summary, and not a line for
// every file with the full path of the project in it.
func TestCmdInit_SaysWhatItDidOnce(t *testing.T) {
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if code := cmdInit([]string{"--preset", "godot-go"}); code != 0 {
			t.Errorf("cmdInit = %d, want 0", code)
		}
	})
	for _, want := range []string{"AWK is set up in this directory (preset godot-go)", "what agents read: 3 skills, 2 subagents, 4 rule files", "ci-backend.yml, ci-frontend.yml", "awkit doctor"} {
		if !strings.Contains(out, want) {
			t.Errorf("init does not say %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[generate]") || strings.Contains(out, dir) || strings.Contains(out, "Initializing") {
		t.Errorf("init still prints its progress, or the project's full path:\n%s", out)
	}
	if lines := strings.Count(out, "\n"); lines > 25 {
		t.Errorf("init prints %d lines", lines)
	}

	// Again, with the preset named: the configuration is kept, and that is
	// worth a word.
	out = captureStdout(t, func() { cmdInit([]string{"--preset", "godot-go"}) })
	if !strings.Contains(out, "kept as it is") || !strings.Contains(out, "awkit init --preset godot-go --force-config") {
		t.Errorf("a second init with a preset named:\n%s", out)
	}
	out = captureStdout(t, func() { cmdInit(nil) })
	if strings.Contains(out, "--force-config") {
		t.Errorf("a second init with no preset named talks about presets:\n%s", out)
	}
}

// inNewDir runs the test in a new directory and returns it.
func inNewDir(t *testing.T) string {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// A temporary directory may itself be inside a repository.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("AWKIT_SKIP_UPDATE_CHECK", "1")
	return dir
}

// A configuration that is written over keeps its branches: they are the
// project's choice, not the preset's. A new one gets the branches found in
// the repository, and one trunk named main where there is no repository.
func TestBranchesFor(t *testing.T) {
	dir := inNewDir(t)
	if got, want := branchesFor(dir), install.DefaultBranches(); got != want {
		t.Errorf("no configuration and no repository: %+v, want %+v", got, want)
	}

	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "trunk").CombinedOutput(); err != nil {
		t.Skipf("git init: %v\n%s", err, out)
	}
	if got, want := branchesFor(dir), (install.Branches{Integration: "trunk", Release: "trunk"}); got != want {
		t.Errorf("no configuration, a repository on trunk: %+v, want %+v", got, want)
	}

	path := filepath.Join(dir, ".ai", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("git: {integration_branch: next, release_branch: stable}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := branchesFor(dir), (install.Branches{Integration: "next", Release: "stable"}); got != want {
		t.Errorf("a configuration that names its branches: %+v, want %+v", got, want)
	}

	if err := os.WriteFile(path, []byte("git: [not, a, mapping\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := branchesFor(dir), (install.Branches{Integration: "trunk", Release: "trunk"}); got != want {
		t.Errorf("a configuration that cannot be read: %+v, want the repository's %+v", got, want)
	}
}

// `awkit init --force-config` writes the preset's configuration over the one
// that is there, and the branches the project had chosen come along.
func TestCmdInit_ForceConfigKeepsTheBranches(t *testing.T) {
	dir := inNewDir(t)
	captureStdout(t, func() { cmdInit(nil) })
	path := filepath.Join(dir, ".ai", "config.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `integration_branch: "main"`) {
		t.Fatalf("a new project outside a repository does not get one trunk named main:\n%s", content)
	}
	changed := strings.Replace(string(content), `integration_branch: "main"`, `integration_branch: "next"`, 1)
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if code := cmdInit([]string{"--preset", "godot-go", "--force-config"}); code != 0 {
			t.Errorf("cmdInit = %d, want 0", code)
		}
	})
	content, _ = os.ReadFile(path)
	if !strings.Contains(string(content), `integration_branch: "next"`) || !strings.Contains(string(content), `release_branch: "main"`) || !strings.Contains(string(content), "language: csharp") {
		t.Errorf("the configuration written over the project's does not keep its branches, or is not the preset's:\n%s", content)
	}
	if !strings.Contains(out, "pull requests go to next, releases are cut from main") {
		t.Errorf("init does not say where pull requests go:\n%s", out)
	}
}

// The generator is quiet inside an upgrade. What it warned about is shown
// all the same: a rule the configuration lists and the project does not have
// stops every Worker.
func TestCmdUpgrade_ShowsWhatTheGeneratorWarnedAbout(t *testing.T) {
	dir := inNewDir(t)
	captureStdout(t, func() { cmdInit(nil) })
	path := filepath.Join(dir, ".ai", "config.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(content), "  custom: []\n\nagents:", "  custom: [house-rules]\n\nagents:", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	errOut := captureStderr(func() {
		out = captureStdout(t, func() {
			if code := cmdUpgrade([]string{"--no-commit"}); code != 0 {
				t.Errorf("cmdUpgrade = %d, want 0", code)
			}
		})
	})
	if !strings.Contains(errOut, "custom rule 'house-rules' not found: .claude/rules/house-rules.md") {
		t.Errorf("the upgrade does not show the generator's warning:\nstderr:\n%s\nstdout:\n%s", errOut, out)
	}
	if strings.Contains(out, "[generate]") {
		t.Errorf("the upgrade prints the generator's progress:\n%s", out)
	}
}

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

func TestAvailablePresetNames(t *testing.T) {
	names := availablePresetNames()
	if !strings.Contains(names, "generic") {
		t.Error("expected 'generic' in preset names")
	}
	if !strings.Contains(names, "godot-go") {
		t.Error("expected 'godot-go' in preset names")
	}
	if strings.Contains(names, "react-go") {
		t.Error("the removed preset 'react-go' is still listed")
	}
}

func TestColorOutput(t *testing.T) {
	// Test that color functions don't panic
	_ = bold("test")
	_ = cyan("test")

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	info("test message\n")

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "test message") {
		t.Error("info() should output message")
	}
}

func TestBashCompletion(t *testing.T) {
	if !strings.Contains(bashCompletion, "_awkit") {
		t.Error("bash completion should contain _awkit function")
	}
	if !strings.Contains(bashCompletion, "init") {
		t.Error("bash completion should contain init command")
	}
}

func TestZshCompletion(t *testing.T) {
	if !strings.Contains(zshCompletion, "#compdef awkit") {
		t.Error("zsh completion should start with #compdef")
	}
	if !strings.Contains(zshCompletion, "init") {
		t.Error("zsh completion should contain init command")
	}
}

func TestFishCompletion(t *testing.T) {
	if !strings.Contains(fishCompletion, "complete -c awkit") {
		t.Error("fish completion should contain complete -c awkit")
	}
	if !strings.Contains(fishCompletion, "init") {
		t.Error("fish completion should contain init command")
	}
}

func TestPresets(t *testing.T) {
	if len(presets) < 2 {
		t.Error("expected at least 2 presets")
	}

	foundGeneric := false
	foundGodotGo := false
	for _, p := range presets {
		if p.Name == "generic" {
			foundGeneric = true
		}
		if p.Name == "godot-go" {
			foundGodotGo = true
		}
	}

	if !foundGeneric {
		t.Error("missing 'generic' preset")
	}
	if !foundGodotGo {
		t.Error("missing 'godot-go' preset")
	}
	if len(presets) != len(install.Presets) {
		t.Errorf("the CLI lists %d presets, the installer knows %d", len(presets), len(install.Presets))
	}
}

func TestCompletionContainsUpgrade(t *testing.T) {
	// Verify upgrade command is in all completions
	if !strings.Contains(bashCompletion, "upgrade") {
		t.Error("bash completion should contain upgrade command")
	}
	if !strings.Contains(zshCompletion, "upgrade") {
		t.Error("zsh completion should contain upgrade command")
	}
	if !strings.Contains(fishCompletion, "upgrade") {
		t.Error("fish completion should contain upgrade command")
	}
}

func TestUsageContainsUpgrade(t *testing.T) {
	// Capture usage output
	var buf bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	usage()

	w.Close()
	os.Stderr = oldStderr
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "upgrade") {
		t.Error("usage should contain upgrade command")
	}
}

// =============================================================================
// Task 22: Integration Tests for Scaffold
// =============================================================================

func TestInitWithScaffold(t *testing.T) {
	// Test init --scaffold complete flow
	tmpDir, err := os.MkdirTemp("", "awkit-init-scaffold-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// We can't easily test the full CLI, but we can test the install package directly
	// This is effectively what cmdInit does
	result, err := install.Install(awkit.KitFS, tmpDir, install.Options{
		Preset:     install.PresetGodotGo,
		NoGenerate: true,
		Scaffold:   true,
	})
	if err != nil {
		t.Fatalf("Install with scaffold failed: %v", err)
	}

	// Verify AWK kit was installed
	if _, err := os.Stat(layout.Config(tmpDir)); os.IsNotExist(err) {
		t.Error("workflow.yaml should be created")
	}

	// Verify scaffold files were created
	for _, file := range []string{"backend/go.mod", "backend/main.go", "frontend/Frontend.sln", "frontend/game/project.godot"} {
		if _, err := os.Stat(filepath.Join(tmpDir, filepath.FromSlash(file))); os.IsNotExist(err) {
			t.Errorf("%s should be created by scaffold", file)
		}
	}

	// Verify scaffold result
	if result.ScaffoldResult == nil {
		t.Error("ScaffoldResult should not be nil")
	}
	if len(result.ScaffoldResult.Created) == 0 {
		t.Error("ScaffoldResult.Created should not be empty")
	}
}

func TestInitScaffoldDryRun(t *testing.T) {
	// Test init --scaffold --dry-run
	tmpDir, err := os.MkdirTemp("", "awkit-dryrun-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test dry-run scaffold
	result, err := install.Scaffold(tmpDir, install.ScaffoldOptions{
		Preset:      install.PresetGodotGo,
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Scaffold dry-run failed: %v", err)
	}

	// Verify no files were actually created
	if _, err := os.Stat(filepath.Join(tmpDir, "backend", "go.mod")); !os.IsNotExist(err) {
		t.Error("backend/go.mod should NOT be created in dry-run mode")
	}

	// Verify dry-run reports what would be created
	if len(result.Created) == 0 {
		t.Error("dry-run should report files that would be created")
	}

	// Now actually create the files
	actualResult, err := install.Scaffold(tmpDir, install.ScaffoldOptions{
		Preset:      install.PresetGodotGo,
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("Scaffold actual run failed: %v", err)
	}

	// Compare dry-run output with actual result (P5: dry-run should match actual)
	if len(result.Created) != len(actualResult.Created) {
		t.Errorf("dry-run Created count (%d) should match actual Created count (%d)",
			len(result.Created), len(actualResult.Created))
	}
}

func TestUpgradeWithScaffold(t *testing.T) {
	// Test upgrade --scaffold --preset works correctly
	tmpDir, err := os.MkdirTemp("", "awkit-upgrade-scaffold-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// First, do initial install without scaffold
	_, err = install.Install(awkit.KitFS, tmpDir, install.Options{
		Preset:     install.PresetGodotGo,
		NoGenerate: true,
		Scaffold:   false,
	})
	if err != nil {
		t.Fatalf("Initial install failed: %v", err)
	}

	// Verify scaffold files don't exist yet
	if _, err := os.Stat(filepath.Join(tmpDir, "backend", "go.mod")); !os.IsNotExist(err) {
		t.Error("backend/go.mod should not exist before scaffold")
	}

	// Now simulate upgrade --scaffold --preset godot-go
	result, err := install.Scaffold(tmpDir, install.ScaffoldOptions{
		Preset:      install.PresetGodotGo,
		TargetDir:   tmpDir,
		ProjectName: filepath.Base(tmpDir),
		Force:       false,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("Scaffold during upgrade failed: %v", err)
	}

	// Verify scaffold files were created
	if _, err := os.Stat(filepath.Join(tmpDir, "backend", "go.mod")); os.IsNotExist(err) {
		t.Error("backend/go.mod should be created by scaffold")
	}

	if len(result.Created) == 0 {
		t.Error("result.Created should not be empty")
	}
}

func TestUpgradeForceConfigOverwritesWorkflowYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-upgrade-force-config-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Minimal .ai directory so cmdUpgrade considers it installed.
	configDir := filepath.Dir(layout.Config(tmpDir))
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create .ai/config dir: %v", err)
	}

	original := "project:\n  name: original\n"
	if err := os.WriteFile(layout.Config(tmpDir), []byte(original), 0o644); err != nil {
		t.Fatalf("failed to write existing workflow.yaml: %v", err)
	}

	t.Setenv("AWKIT_SKIP_UPDATE_CHECK", "1")
	exitCode := cmdUpgrade([]string{tmpDir, "--no-commit", "--no-generate", "--force-config", "--preset", "generic"})
	if exitCode != 0 {
		t.Fatalf("cmdUpgrade() exit code = %d, want 0", exitCode)
	}

	updated, err := os.ReadFile(layout.Config(tmpDir))
	if err != nil {
		t.Fatalf("failed to read workflow.yaml: %v", err)
	}
	if string(updated) == original {
		t.Fatal("workflow.yaml was not overwritten with upgrade --force-config")
	}
}

// awkit init writes a CI workflow for every repo of the preset. awkit upgrade
// leaves the project's workflows alone: it adds none and rewrites none.
func TestInitWritesCIWorkflows_UpgradeLeavesThemAlone(t *testing.T) {
	t.Setenv("AWKIT_SKIP_UPDATE_CHECK", "1")
	tmpDir := t.TempDir()
	workflows := filepath.Join(tmpDir, ".github", "workflows")
	backendCI := filepath.Join(workflows, "ci-backend.yml")
	frontendCI := filepath.Join(workflows, "ci-frontend.yml")

	if exitCode := cmdInit([]string{"--preset", "godot-go", tmpDir}); exitCode != 0 {
		t.Fatalf("cmdInit() = %d, want 0", exitCode)
	}
	for _, path := range []string{backendCI, frontendCI} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("init should write %s: %v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "CLAUDE.md")); err != nil {
		t.Errorf("init should also write the other generated files: %v", err)
	}

	// The project edits one workflow and drops the other.
	const own = "name: mine\n"
	if err := os.WriteFile(backendCI, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(frontendCI); err != nil {
		t.Fatal(err)
	}

	if exitCode := cmdUpgrade([]string{tmpDir, "--no-commit"}); exitCode != 0 {
		t.Fatalf("cmdUpgrade() = %d, want 0", exitCode)
	}
	if got, _ := os.ReadFile(backendCI); string(got) != own {
		t.Errorf("upgrade rewrote the project's workflow:\n%s", got)
	}
	if _, err := os.Stat(frontendCI); !os.IsNotExist(err) {
		t.Error("upgrade brought back a workflow the project removed")
	}

	// Running init again restores the missing one and keeps the edited one.
	if exitCode := cmdInit([]string{"--preset", "godot-go", tmpDir}); exitCode != 0 {
		t.Fatalf("second cmdInit() = %d, want 0", exitCode)
	}
	if got, _ := os.ReadFile(backendCI); string(got) != own {
		t.Errorf("init without --force rewrote the project's workflow:\n%s", got)
	}
	if _, err := os.Stat(frontendCI); err != nil {
		t.Errorf("init should write the workflow that is missing: %v", err)
	}
}

// The generic preset names no build or test command, so init writes no CI
// workflow: there is nothing it could run.
func TestInitGenericWritesNoCIWorkflow(t *testing.T) {
	t.Setenv("AWKIT_SKIP_UPDATE_CHECK", "1")
	tmpDir := t.TempDir()

	if exitCode := cmdInit([]string{tmpDir}); exitCode != 0 {
		t.Fatalf("cmdInit() = %d, want 0", exitCode)
	}
	if entries, _ := os.ReadDir(filepath.Join(tmpDir, ".github", "workflows")); len(entries) != 0 {
		t.Errorf("init wrote %d workflow files for the generic preset", len(entries))
	}
}

// An upgrade brings the workflows awkit generated in step with the new kit,
// and its commit takes them along: the project is not left with changes it
// never made.
func TestUpgradeRefreshesAwkitsWorkflowsAndCommitsThem(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("AWKIT_SKIP_UPDATE_CHECK", "1")
	tmpDir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test User")
	git("config", "commit.gpgsign", "false")

	if exitCode := cmdInit([]string{"--preset", "godot-go", tmpDir}); exitCode != 0 {
		t.Fatalf("cmdInit() = %d, want 0", exitCode)
	}
	backendCI := filepath.Join(tmpDir, ".github", "workflows", "ci-backend.yml")
	frontendCI := filepath.Join(tmpDir, ".github", "workflows", "ci-frontend.yml")
	current, err := os.ReadFile(backendCI)
	if err != nil {
		t.Fatal(err)
	}

	// The backend workflow as an older kit wrote it (still awkit's: the header
	// is there). The frontend workflow is the project's own.
	older := strings.Replace(string(current), "go test ./...", "go test -short ./...", 1)
	if older == string(current) {
		t.Fatal("could not make the workflow look out of date")
	}
	if err := os.WriteFile(backendCI, []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	const own = "name: mine\n"
	if err := os.WriteFile(frontendCI, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-m", "project as an older kit left it")

	if exitCode := cmdUpgrade([]string{tmpDir}); exitCode != 0 {
		t.Fatalf("cmdUpgrade() = %d, want 0", exitCode)
	}

	if got, _ := os.ReadFile(backendCI); string(got) != string(current) {
		t.Errorf("awkit's workflow was not refreshed:\n%s", got)
	}
	if got, _ := os.ReadFile(frontendCI); string(got) != own {
		t.Errorf("the project's own workflow was changed:\n%s", got)
	}
	if status := git("status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Errorf("the upgrade left uncommitted changes:\n%s", status)
	}
	if subject := strings.TrimSpace(git("log", "-1", "--format=%s")); subject != "[chore] upgrade awkit" {
		t.Errorf("last commit = %q, want the upgrade commit", subject)
	}
	changed := git("show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(changed, ".github/workflows/ci-backend.yml") {
		t.Errorf("the upgrade commit should carry the refreshed workflow, it has:\n%s", changed)
	}
	if strings.Contains(changed, "ci-frontend.yml") {
		t.Errorf("the upgrade commit touched the project's own workflow:\n%s", changed)
	}
}

// awkit generate keeps the CI workflows in step with the config: it writes
// the one that is missing, follows the config in the ones awkit owns, and
// leaves the project's own alone unless --generate-ci.
func TestGenerateKeepsCIWorkflowsInStepWithTheConfig(t *testing.T) {
	t.Setenv("AWKIT_SKIP_UPDATE_CHECK", "1")
	tmpDir := t.TempDir()
	ci := filepath.Join(tmpDir, ".github", "workflows", "ci.yml")
	configPath := layout.Config(tmpDir)
	setTestCommand := func(from, to string) {
		t.Helper()
		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		updated := strings.Replace(string(config), "test: "+from, "test: "+to, 1)
		if updated == string(config) {
			t.Fatalf("the config has no test command %s to replace:\n%s", from, config)
		}
		if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	generate := func(args ...string) {
		t.Helper()
		if exitCode := cmdGenerate(append([]string{"--state-root", tmpDir}, args...)); exitCode != 0 {
			t.Fatalf("cmdGenerate(%v) = %d, want 0", args, exitCode)
		}
	}
	workflow := func() string {
		t.Helper()
		data, err := os.ReadFile(ci)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	// The generic preset names no commands: no workflow yet.
	if exitCode := cmdInit([]string{tmpDir}); exitCode != 0 {
		t.Fatalf("cmdInit() = %d, want 0", exitCode)
	}
	if _, err := os.Stat(ci); !os.IsNotExist(err) {
		t.Fatal("a workflow exists before the project named any command")
	}

	// The project names its test command: generate writes the workflow.
	setTestCommand(`""`, `"make test"`)
	generate()
	if got := workflow(); !strings.Contains(got, "make test") {
		t.Fatalf("the workflow should run the test command:\n%s", got)
	}

	// The command changes: the workflow follows.
	setTestCommand(`"make test"`, `"make check"`)
	generate()
	if got := workflow(); !strings.Contains(got, "make check") || strings.Contains(got, "make test") {
		t.Errorf("the workflow did not follow the config:\n%s", got)
	}

	// A dry run changes nothing.
	setTestCommand(`"make check"`, `"make verify"`)
	generate("--dry-run")
	if got := workflow(); !strings.Contains(got, "make check") {
		t.Errorf("a dry run rewrote the workflow:\n%s", got)
	}

	// The project takes the workflow over: generate leaves it alone,
	// --generate-ci writes it from the config again.
	const own = "name: mine\n"
	if err := os.WriteFile(ci, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	generate()
	if got := workflow(); got != own {
		t.Errorf("generate overwrote the project's own workflow:\n%s", got)
	}
	generate("--generate-ci")
	if got := workflow(); !strings.Contains(got, "make verify") {
		t.Errorf("--generate-ci should write the workflow from the config:\n%s", got)
	}
}

func TestProjectWorkflowPaths(t *testing.T) {
	root := filepath.Join("work", "proj")
	workflows := filepath.Join(root, ".github", "workflows")
	written := []string{
		filepath.Join(workflows, "ci-backend.yml"),
		// A submodule's workflow is in another repository.
		filepath.Join(root, "engine", ".github", "workflows", "ci.yml"),
	}
	removed := []string{filepath.Join(workflows, "ci-frontend.yml")}

	got := projectWorkflowPaths(root, written, removed)

	want := []string{".github/workflows/ci-backend.yml", ".github/workflows/ci-frontend.yml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("projectWorkflowPaths = %v, want %v", got, want)
	}
	if got := projectWorkflowPaths(root); len(got) != 0 {
		t.Errorf("projectWorkflowPaths with nothing written = %v", got)
	}
}

func TestUpgradeScaffoldRequiresPreset(t *testing.T) {
	// Test that upgrade --scaffold without --preset returns error (P6)
	// This is tested at the CLI level - the install package doesn't enforce this
	// The enforcement is in cmdUpgrade() in main.go

	// We verify the error type exists
	if install.ErrMissingPreset == nil {
		t.Error("ErrMissingPreset should be defined")
	}
	if install.ErrMissingPreset.Error() != "--preset required for upgrade --scaffold" {
		t.Errorf("ErrMissingPreset message incorrect: %v", install.ErrMissingPreset)
	}
}

func TestScaffoldVerifyCompatibility(t *testing.T) {
	// Test that scaffold creates files compatible with verify commands (P4)
	testCases := []struct {
		name       string
		preset     string
		verifyFile string // File that must exist for verify to work
	}{
		{"Go backend", install.PresetGodotGo, "backend/go.mod"},
		{"Godot C# frontend", install.PresetGodotGo, "frontend/Frontend.sln"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "awkit-verify-compat-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Scaffold the project
			_, err = install.Scaffold(tmpDir, install.ScaffoldOptions{
				Preset:      tc.preset,
				TargetDir:   tmpDir,
				ProjectName: "test-project",
				Force:       false,
				DryRun:      false,
			})
			if err != nil {
				t.Fatalf("Scaffold failed: %v", err)
			}

			// Verify the critical file exists
			if _, err := os.Stat(filepath.Join(tmpDir, filepath.FromSlash(tc.verifyFile))); os.IsNotExist(err) {
				t.Errorf("%s should exist for verify compatibility", tc.verifyFile)
			}
		})
	}
}

func TestListPresetsOutput(t *testing.T) {
	// Verify list-presets would show correct categories
	singleRepoCount := 0
	monorepoCount := 0

	for _, p := range presets {
		switch p.Category {
		case "single-repo":
			singleRepoCount++
		case "monorepo":
			monorepoCount++
		}
	}

	// One preset of each kind: generic and godot-go
	if singleRepoCount != 1 {
		t.Errorf("expected 1 single-repo preset, got %d", singleRepoCount)
	}
	if monorepoCount != 1 {
		t.Errorf("expected 1 monorepo preset, got %d", monorepoCount)
	}
}

func TestInvalidPresetError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-invalid-preset-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test that invalid preset returns error
	_, err = install.Install(awkit.KitFS, tmpDir, install.Options{
		Preset:     "invalid-preset-name",
		NoGenerate: true,
	})

	if err == nil {
		t.Error("expected error for invalid preset")
	}

	// Verify error message contains "unknown preset"
	if !strings.Contains(err.Error(), "unknown preset") {
		t.Errorf("error should mention 'unknown preset', got: %v", err)
	}
}

func TestAllPresetsExist(t *testing.T) {
	// Verify all expected presets exist (Req 7.5)
	expectedPresets := install.Presets

	for _, expected := range expectedPresets {
		found := false
		for _, p := range presets {
			if p.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected preset: %s", expected)
		}
	}
}

func TestShellCompletionContainsAllPresets(t *testing.T) {
	// Verify shell completions include all presets (Req 5.4)
	expectedPresets := install.Presets

	for _, preset := range expectedPresets {
		if !strings.Contains(bashCompletion, preset) {
			t.Errorf("bash completion missing preset: %s", preset)
		}
		if !strings.Contains(zshCompletion, preset) {
			t.Errorf("zsh completion missing preset: %s", preset)
		}
		if !strings.Contains(fishCompletion, preset) {
			t.Errorf("fish completion missing preset: %s", preset)
		}
	}

	// Presets that were removed must not be offered any more.
	for _, removed := range []string{"react-go", "react-python", "unity-go", "unreal-go", "python", "rust", "dotnet"} {
		for shell, script := range map[string]string{"bash": bashCompletion, "zsh": zshCompletion, "fish": fishCompletion} {
			if strings.Contains(script, removed) {
				t.Errorf("%s completion still offers the removed preset %s", shell, removed)
			}
		}
	}
}

func TestShellCompletionContainsNewCommands(t *testing.T) {
	newCommands := []string{"feedback-stats", "context-snapshot"}
	for _, cmd := range newCommands {
		if !strings.Contains(bashCompletion, cmd) {
			t.Errorf("bash completion should contain %s", cmd)
		}
		if !strings.Contains(zshCompletion, cmd) {
			t.Errorf("zsh completion should contain %s", cmd)
		}
		if !strings.Contains(fishCompletion, cmd) {
			t.Errorf("fish completion should contain %s", cmd)
		}
	}
}

func TestUsageContainsNewCommands(t *testing.T) {
	var buf bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	usage()

	w.Close()
	os.Stderr = oldStderr
	buf.ReadFrom(r)

	output := buf.String()
	newCommands := []string{"feedback-stats", "context-snapshot"}
	for _, cmd := range newCommands {
		if !strings.Contains(output, cmd) {
			t.Errorf("usage should contain %s command", cmd)
		}
	}
}

func TestShellCompletionContainsScaffoldFlag(t *testing.T) {
	// Verify shell completions include --scaffold flag
	if !strings.Contains(bashCompletion, "--scaffold") {
		t.Error("bash completion should contain --scaffold flag")
	}
	if !strings.Contains(zshCompletion, "--scaffold") {
		t.Error("zsh completion should contain --scaffold flag")
	}
	// Fish uses -l for long options
	if !strings.Contains(fishCompletion, "-l scaffold") {
		t.Error("fish completion should contain -l scaffold flag")
	}
}

// TestInitFlagOrderIndependence verifies that flags work regardless of position
// relative to the path argument (fixes issue where flags after path were ignored)
func TestInitFlagOrderIndependence(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{
			name:     "flags before path",
			args:     []string{"--preset", "godot-go", "--dry-run", "."},
			wantExit: 0,
		},
		{
			name:     "flags after path",
			args:     []string{".", "--preset", "godot-go", "--dry-run"},
			wantExit: 0,
		},
		{
			name:     "flags mixed with path",
			args:     []string{"--preset", "godot-go", ".", "--dry-run"},
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp dir for test
			tmpDir, err := os.MkdirTemp("", "awkit-flag-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Replace "." with actual temp dir
			args := make([]string, len(tt.args))
			for i, arg := range tt.args {
				if arg == "." {
					args[i] = tmpDir
				} else {
					args[i] = arg
				}
			}

			exitCode := cmdInit(args)
			if exitCode != tt.wantExit {
				t.Errorf("cmdInit() exit code = %d, want %d", exitCode, tt.wantExit)
			}
		})
	}
}

// TestUpgradeFlagOrderIndependence verifies that upgrade flags work regardless of position
func TestUpgradeFlagOrderIndependence(t *testing.T) {
	// Create a temp dir with AWK already installed
	tmpDir, err := os.MkdirTemp("", "awkit-upgrade-flag-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal .ai directory structure
	aiDir := filepath.Join(tmpDir, ".ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		t.Fatalf("failed to create .ai dir: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{
			name:     "flags before path",
			args:     []string{"--dry-run", tmpDir},
			wantExit: 0,
		},
		{
			name:     "flags after path",
			args:     []string{tmpDir, "--dry-run"},
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := cmdUpgrade(tt.args)
			if exitCode != tt.wantExit {
				t.Errorf("cmdUpgrade() exit code = %d, want %d", exitCode, tt.wantExit)
			}
		})
	}
}

// TestUpgradeAutoCommit verifies that upgrade auto-commits changes by default
func TestUpgradeAutoCommit(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping auto-commit test")
	}

	// Create a temp dir with git repo
	tmpDir, err := os.MkdirTemp("", "awkit-autocommit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	exec.Command("git", "init").Dir = tmpDir
	exec.Command("git", "config", "user.email", "test@example.com").Dir = tmpDir
	exec.Command("git", "config", "user.name", "Test User").Dir = tmpDir

	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	configCmd := exec.Command("git", "config", "user.email", "test@example.com")
	configCmd.Dir = tmpDir
	configCmd.Run()

	configCmd2 := exec.Command("git", "config", "user.name", "Test User")
	configCmd2.Dir = tmpDir
	configCmd2.Run()

	// Create minimal .ai directory
	aiDir := filepath.Join(tmpDir, ".ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		t.Fatalf("failed to create .ai dir: %v", err)
	}

	// Initial commit
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = tmpDir
	addCmd.Run()

	commitCmd := exec.Command("git", "commit", "-m", "initial commit", "--allow-empty")
	commitCmd.Dir = tmpDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("initial commit failed: %v", err)
	}

	// Run upgrade with --dry-run to avoid actual upgrade
	exitCode := cmdUpgrade([]string{tmpDir, "--dry-run"})
	if exitCode != 0 {
		t.Errorf("cmdUpgrade() exit code = %d, want 0", exitCode)
	}
}

// TestUpgradeNoCommitFlag verifies that --no-commit skips auto-commit
func TestUpgradeNoCommitFlag(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-nocommit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal .ai directory
	aiDir := filepath.Join(tmpDir, ".ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		t.Fatalf("failed to create .ai dir: %v", err)
	}

	// Run upgrade with --no-commit and --dry-run
	exitCode := cmdUpgrade([]string{tmpDir, "--no-commit", "--dry-run"})
	if exitCode != 0 {
		t.Errorf("cmdUpgrade() exit code = %d, want 0", exitCode)
	}
}

// TestUpgradeCIMigration verifies that upgrade automatically migrates CI workflow
func TestUpgradeCIMigration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-upgrade-ci-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal .ai directory
	aiDir := filepath.Join(tmpDir, ".ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		t.Fatalf("failed to create .ai dir: %v", err)
	}

	// Create old CI file with awk job (simulating old version)
	ciDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(ciDir, 0755); err != nil {
		t.Fatalf("failed to create .github/workflows dir: %v", err)
	}
	oldCI := `name: ci
on: [push]
jobs:
  awk:
    runs-on: ubuntu-latest
    steps:
      - run: echo "old awk job"
  backend:
    runs-on: ubuntu-latest
    steps:
      - run: echo "backend"
  custom-job:
    runs-on: ubuntu-latest
    steps:
      - run: echo "user custom job"
`
	ciPath := filepath.Join(ciDir, "ci.yml")
	if err := os.WriteFile(ciPath, []byte(oldCI), 0644); err != nil {
		t.Fatalf("failed to write old CI file: %v", err)
	}

	// Run upgrade (should auto-migrate awk job but preserve other jobs)
	exitCode := cmdUpgrade([]string{tmpDir, "--no-commit"})
	if exitCode != 0 {
		t.Errorf("cmdUpgrade() exit code = %d, want 0", exitCode)
	}

	// Verify awk job is removed but other jobs preserved
	content, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI file: %v", err)
	}
	contentStr := string(content)

	// awk job should be removed (auto-migration)
	if strings.Contains(contentStr, "awk:") {
		t.Error("CI file should NOT contain 'awk:' job after auto-migration")
	}
	if strings.Contains(contentStr, "old awk job") {
		t.Error("CI file should NOT contain 'old awk job' after auto-migration")
	}

	// User's custom jobs should be preserved
	if !strings.Contains(contentStr, "backend:") {
		t.Error("CI file should preserve 'backend:' job")
	}
	if !strings.Contains(contentStr, "custom-job:") {
		t.Error("CI file should preserve 'custom-job:' (user's custom job)")
	}
	if !strings.Contains(contentStr, "user custom job") {
		t.Error("CI file should preserve user's custom job content")
	}
}

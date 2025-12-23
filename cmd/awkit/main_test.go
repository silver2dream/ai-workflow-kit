package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awkit "github.com/silver2dream/ai-workflow-kit"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
)

func TestAvailablePresetNames(t *testing.T) {
	names := availablePresetNames()
	if !strings.Contains(names, "generic") {
		t.Error("expected 'generic' in preset names")
	}
	if !strings.Contains(names, "react-go") {
		t.Error("expected 'react-go' in preset names")
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
	foundReactGo := false
	for _, p := range presets {
		if p.Name == "generic" {
			foundGeneric = true
		}
		if p.Name == "react-go" {
			foundReactGo = true
		}
	}

	if !foundGeneric {
		t.Error("missing 'generic' preset")
	}
	if !foundReactGo {
		t.Error("missing 'react-go' preset")
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
		Preset:     "go",
		NoGenerate: true,
		Scaffold:   true,
	})
	if err != nil {
		t.Fatalf("Install with scaffold failed: %v", err)
	}

	// Verify AWK kit was installed
	if _, err := os.Stat(filepath.Join(tmpDir, ".ai", "config", "workflow.yaml")); os.IsNotExist(err) {
		t.Error("workflow.yaml should be created")
	}

	// Verify scaffold files were created
	if _, err := os.Stat(filepath.Join(tmpDir, "go.mod")); os.IsNotExist(err) {
		t.Error("go.mod should be created by scaffold")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "main.go")); os.IsNotExist(err) {
		t.Error("main.go should be created by scaffold")
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
		Preset:      "go",
		TargetDir:   tmpDir,
		ProjectName: "test-project",
		Force:       false,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Scaffold dry-run failed: %v", err)
	}

	// Verify no files were actually created
	if _, err := os.Stat(filepath.Join(tmpDir, "go.mod")); !os.IsNotExist(err) {
		t.Error("go.mod should NOT be created in dry-run mode")
	}

	// Verify dry-run reports what would be created
	if len(result.Created) == 0 {
		t.Error("dry-run should report files that would be created")
	}

	// Now actually create the files
	actualResult, err := install.Scaffold(tmpDir, install.ScaffoldOptions{
		Preset:      "go",
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
		Preset:     "go",
		NoGenerate: true,
		Scaffold:   false,
	})
	if err != nil {
		t.Fatalf("Initial install failed: %v", err)
	}

	// Verify scaffold files don't exist yet
	if _, err := os.Stat(filepath.Join(tmpDir, "go.mod")); !os.IsNotExist(err) {
		t.Error("go.mod should not exist before scaffold")
	}

	// Now simulate upgrade --scaffold --preset go
	result, err := install.Scaffold(tmpDir, install.ScaffoldOptions{
		Preset:      "go",
		TargetDir:   tmpDir,
		ProjectName: filepath.Base(tmpDir),
		Force:       false,
		DryRun:      false,
	})
	if err != nil {
		t.Fatalf("Scaffold during upgrade failed: %v", err)
	}

	// Verify scaffold files were created
	if _, err := os.Stat(filepath.Join(tmpDir, "go.mod")); os.IsNotExist(err) {
		t.Error("go.mod should be created by scaffold")
	}

	if len(result.Created) == 0 {
		t.Error("result.Created should not be empty")
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
		{"Go", "go", "go.mod"},
		{"Python", "python", "pyproject.toml"},
		{"Rust", "rust", "Cargo.toml"},
		{"Dotnet", "dotnet", "Program.cs"},
		{"Node", "node", "package.json"},
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
			if _, err := os.Stat(filepath.Join(tmpDir, tc.verifyFile)); os.IsNotExist(err) {
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

	// Should have 6 single-repo presets
	if singleRepoCount != 6 {
		t.Errorf("expected 6 single-repo presets, got %d", singleRepoCount)
	}

	// Should have 5 monorepo presets
	if monorepoCount != 5 {
		t.Errorf("expected 5 monorepo presets, got %d", monorepoCount)
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
	expectedPresets := []string{
		"generic", "go", "python", "rust", "dotnet", "node",
		"react-go", "react-python", "unity-go", "godot-go", "unreal-go",
	}

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
	expectedPresets := []string{
		"generic", "go", "python", "rust", "dotnet", "node",
		"react-go", "react-python", "unity-go", "godot-go", "unreal-go",
	}

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

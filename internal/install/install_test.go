package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// createMinimalMockFS creates a mock FS with minimal required files
func createMinimalMockFS() fstest.MapFS {
	return fstest.MapFS{
		".ai/config/workflow.schema.json": &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":         &fstest.MapFile{Data: []byte("#!/bin/bash\necho test")},
		".ai/rules/.gitkeep":              &fstest.MapFile{Data: []byte("")},
		".ai/commands/.gitkeep":           &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":          &fstest.MapFile{Data: []byte("")},
	}
}

func TestInstall_CurrentDirectory(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	// Test install with absolute path
	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
		WithCI:     false,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Verify .ai directory was created
	aiDir := filepath.Join(tmpDir, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		t.Error(".ai directory was not created")
	}

	// Verify config directory was created
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error(".ai/config directory was not created")
	}
}

func TestInstall_DotPath(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	mockFS := createMinimalMockFS()

	// Test install with "."
	result, err := Install(mockFS, ".", Options{
		Preset:     "generic",
		NoGenerate: true,
		WithCI:     false,
	})
	if err != nil {
		t.Fatalf("Install with '.' failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Verify .ai directory was created
	aiDir := filepath.Join(tmpDir, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		t.Error(".ai directory was not created")
	}
}

func TestInstall_InvalidPreset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "invalid-preset",
		NoGenerate: true,
	})
	if err == nil {
		t.Error("expected error for invalid preset, got nil")
	}
}

func TestInstall_NonExistentDirectory(t *testing.T) {
	mockFS := createMinimalMockFS()

	_, err := Install(mockFS, "/nonexistent/path/that/does/not/exist", Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

func TestInstall_ConfigSkipped(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml
	configDir := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("existing: true"), 0o644)

	mockFS := createMinimalMockFS()

	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
		WithCI:     false,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}
	if !result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be true")
	}

	// Verify original content preserved
	content, _ := os.ReadFile(filepath.Join(configDir, "workflow.yaml"))
	if string(content) != "existing: true" {
		t.Error("existing workflow.yaml was overwritten")
	}
}

func TestInstall_ForceConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml
	configDir := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte("existing: true"), 0o644)

	mockFS := createMinimalMockFS()

	result, err := Install(mockFS, tmpDir, Options{
		Preset:      "generic",
		ForceConfig: true,
		NoGenerate:  true,
		WithCI:      false,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}
	if result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be false with --force-config")
	}

	// Verify content was overwritten
	content, _ := os.ReadFile(filepath.Join(configDir, "workflow.yaml"))
	if string(content) == "existing: true" {
		t.Error("workflow.yaml was not overwritten with --force-config")
	}
}

func TestInstall_PresetConfigNotOverwrittenByCopyDir(t *testing.T) {
	// This test ensures that when using a preset (e.g., react-go),
	// the preset's workflow.yaml is NOT overwritten by copyDir
	// which copies the embedded default workflow.yaml.
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock FS that includes a default workflow.yaml
	// (simulating the embedded kit files with a different config)
	mockFS := fstest.MapFS{
		".ai/config/workflow.yaml":              &fstest.MapFile{Data: []byte("language: unity\n")},
		".ai/config/workflow.schema.json":       &fstest.MapFile{Data: []byte(`{}`)},
		".ai/scripts/generate.sh":               &fstest.MapFile{Data: []byte("#!/bin/bash\necho test")},
		".ai/rules/.gitkeep":                    &fstest.MapFile{Data: []byte("")},
		".ai/rules/_examples/backend-go.md":     &fstest.MapFile{Data: []byte("# Go rules")},
		".ai/rules/_examples/frontend-react.md": &fstest.MapFile{Data: []byte("# React rules")},
		".ai/commands/.gitkeep":                 &fstest.MapFile{Data: []byte("")},
		".ai/templates/.gitkeep":                &fstest.MapFile{Data: []byte("")},
	}

	// Install with react-go preset and force mode
	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "react-go",
		Force:      true,
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Read the resulting workflow.yaml
	content, err := os.ReadFile(filepath.Join(tmpDir, ".ai", "config", "workflow.yaml"))
	if err != nil {
		t.Fatalf("failed to read workflow.yaml: %v", err)
	}

	// It should contain "typescript" (from react-go preset), NOT "unity" (from embedded default)
	if strings.Contains(string(content), "language: unity") {
		t.Error("workflow.yaml was overwritten by copyDir - should preserve preset config")
	}
	if !strings.Contains(string(content), "language: typescript") && !strings.Contains(string(content), "language: go") {
		t.Error("workflow.yaml should contain react-go preset languages (typescript/go)")
	}
}

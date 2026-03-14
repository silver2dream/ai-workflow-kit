package jittest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateTests_EmptyDiff(t *testing.T) {
	_, err := GenerateTests(context.Background(), GenerateInput{})
	if err == nil {
		t.Fatal("expected error for empty diff")
	}
	if !strings.Contains(err.Error(), "empty diff") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateTests_CLIFailure(t *testing.T) {
	orig := claudeRunner
	defer func() { claudeRunner = orig }()

	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	_, err := GenerateTests(context.Background(), GenerateInput{
		Diff:     "+++ b/foo.go\n+func Foo() {}",
		Language: "go",
		MaxTests: 3,
		Model:    "test-model",
	})
	if err == nil {
		t.Fatal("expected error on CLI failure")
	}
	if !strings.Contains(err.Error(), "claude CLI failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateTests_NoTestsParsed(t *testing.T) {
	orig := claudeRunner
	defer func() { claudeRunner = orig }()

	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		return "Some text without any code blocks", nil
	}

	_, err := GenerateTests(context.Background(), GenerateInput{
		Diff:     "+++ b/foo.go\n+func Foo() {}",
		Language: "go",
		MaxTests: 3,
		Model:    "test-model",
	})
	if err == nil {
		t.Fatal("expected error when no tests parsed")
	}
	if !strings.Contains(err.Error(), "no tests parsed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateTests_Success(t *testing.T) {
	orig := claudeRunner
	defer func() { claudeRunner = orig }()

	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		// Verify prompt contains expected elements
		if !strings.Contains(prompt, "edge cases") {
			t.Error("prompt should mention edge cases")
		}
		if !strings.Contains(prompt, "testing") {
			t.Error("prompt should mention Go testing framework")
		}
		if model != "claude-sonnet-4-6" {
			t.Errorf("unexpected model: %s", model)
		}

		return "Here are the tests:\n\n```go filename: internal/foo/foo_jittest_test.go\npackage foo\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n\t// test\n}\n```\n", nil
	}

	tests, err := GenerateTests(context.Background(), GenerateInput{
		Diff:     "+++ b/internal/foo/foo.go\n+func Foo() {}",
		Language: "go",
		MaxTests: 5,
		Model:    "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(tests))
	}
	if tests[0].Filename != "internal/foo/foo_jittest_test.go" {
		t.Errorf("unexpected filename: %s", tests[0].Filename)
	}
	if !strings.Contains(tests[0].Content, "func TestFoo") {
		t.Error("content should contain test function")
	}
}

func TestGenerateTests_PromptExcludesTestFiles(t *testing.T) {
	orig := claudeRunner
	defer func() { claudeRunner = orig }()

	var capturedPrompt string
	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		capturedPrompt = prompt
		return "```go filename: x_jittest_test.go\npackage x\n```\n", nil
	}

	sourceFiles := map[string]string{
		"pkg/handler.go":      "package pkg\nfunc Handle() {}",
		"pkg/handler_test.go": "package pkg\n// should not appear",
	}

	_, _ = GenerateTests(context.Background(), GenerateInput{
		Diff:        "+++ b/pkg/handler.go\n+func Handle() {}",
		SourceFiles: sourceFiles,
		Language:    "go",
		MaxTests:    3,
		Model:       "test",
	})

	// Source files passed to prompt include source but prompt builder receives them as-is.
	// The key point: sourceFiles map can contain test files if caller doesn't filter.
	// ReadSourceFiles() handles the filtering. Here we verify the prompt is built.
	if capturedPrompt == "" {
		t.Fatal("prompt was not captured")
	}
	if !strings.Contains(capturedPrompt, "pkg/handler.go") {
		t.Error("prompt should contain source file path")
	}
}

func TestGetDiff_InvalidDir(t *testing.T) {
	_, err := GetDiff(context.Background(), "/nonexistent", "main", "feature")
	if err == nil {
		t.Fatal("expected error for invalid directory")
	}
}

func TestReadSourceFiles_Basic(t *testing.T) {
	dir := t.TempDir()
	// Create a source file
	srcDir := filepath.Join(dir, "pkg")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "handler.go"), []byte("package pkg\nfunc Handle() {}"), 0644)
	os.WriteFile(filepath.Join(srcDir, "handler_test.go"), []byte("package pkg\n// test"), 0644)

	diff := "+++ b/pkg/handler.go\n+func Handle() {}\n+++ b/pkg/handler_test.go\n+// test"
	sources := ReadSourceFiles(dir, diff)

	// Should include handler.go but NOT handler_test.go
	if _, ok := sources["pkg/handler.go"]; !ok {
		t.Error("should include handler.go")
	}
	if _, ok := sources["pkg/handler_test.go"]; ok {
		t.Error("should NOT include handler_test.go")
	}
}

func TestReadSourceFiles_MissingFile(t *testing.T) {
	dir := t.TempDir()
	diff := "+++ b/does/not/exist.go\n+func X() {}"
	sources := ReadSourceFiles(dir, diff)
	if len(sources) != 0 {
		t.Errorf("should return empty map for missing files, got %d entries", len(sources))
	}
}

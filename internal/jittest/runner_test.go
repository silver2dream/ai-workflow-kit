package jittest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTests_EmptyTests(t *testing.T) {
	result := RunTests(context.Background(), t.TempDir(), nil, "echo ok")
	if result.Generated != 0 {
		t.Errorf("expected 0 generated, got %d", result.Generated)
	}
}

func TestRunTests_WriteAndCleanup(t *testing.T) {
	dir := t.TempDir()

	// Stub executeTests to just succeed
	origExec := executeTests
	defer func() { executeTests = origExec }()
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		return testExecOutcome{exitCode: 0}
	}

	tests := []GeneratedTest{
		{Filename: "pkg/foo_jittest_test.go", Content: "package pkg\n"},
	}

	result := RunTests(context.Background(), dir, tests, "echo ok")

	// Verify test file was cleaned up
	jitFile := filepath.Join(dir, "pkg/foo_jittest_test.go")
	if _, err := os.Stat(jitFile); !os.IsNotExist(err) {
		t.Error("JiT test file should have been cleaned up")
	}

	if result.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Passed)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}
}

func TestRunTests_TestFailure(t *testing.T) {
	dir := t.TempDir()

	origExec := executeTests
	defer func() { executeTests = origExec }()
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		return testExecOutcome{exitCode: 1, failed: true, stdout: "FAIL pkg/foo_jittest_test.go"}
	}

	tests := []GeneratedTest{
		{Filename: "pkg/foo_jittest_test.go", Content: "package pkg\n"},
	}

	result := RunTests(context.Background(), dir, tests, "go test")
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.Passed != 0 {
		t.Errorf("expected 0 passed, got %d", result.Passed)
	}
}

func TestRunTests_CompileError(t *testing.T) {
	dir := t.TempDir()

	origExec := executeTests
	defer func() { executeTests = origExec }()
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		return testExecOutcome{
			exitCode:     2,
			failed:       true,
			compileError: true,
			stderr:       "syntax error: unexpected token",
		}
	}

	tests := []GeneratedTest{
		{Filename: "pkg/foo_jittest_test.go", Content: "invalid go code"},
	}

	result := RunTests(context.Background(), dir, tests, "go test")
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped on compile error, got %d", result.Skipped)
	}
	if !strings.Contains(result.Error, "compilation failed") {
		t.Errorf("expected compilation error message, got: %s", result.Error)
	}
}

func TestRunTests_WriteFailure(t *testing.T) {
	// Use a read-only directory to force write failure
	dir := t.TempDir()
	readOnlyDir := filepath.Join(dir, "readonly")
	os.MkdirAll(readOnlyDir, 0555)

	origExec := executeTests
	defer func() { executeTests = origExec }()
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		return testExecOutcome{exitCode: 0}
	}

	tests := []GeneratedTest{
		{Filename: "readonly/subdir/test_jittest.go", Content: "package x\n"},
	}

	result := RunTests(context.Background(), dir, tests, "echo ok")
	// Should either write successfully (if OS allows) or skip
	if result.Skipped+result.Passed != 1 {
		t.Errorf("expected 1 total (skipped or passed), got skipped=%d passed=%d", result.Skipped, result.Passed)
	}
}

func TestCleanupJiTFiles_RemovesAllJitTestFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some jittest files
	pkg := filepath.Join(dir, "pkg")
	os.MkdirAll(pkg, 0755)
	os.WriteFile(filepath.Join(pkg, "foo_jittest_test.go"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(pkg, "bar_jittest_test.go"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(pkg, "real_test.go"), []byte("test"), 0644)

	cleanupJiTFiles(dir, nil)

	// jittest files should be gone
	if _, err := os.Stat(filepath.Join(pkg, "foo_jittest_test.go")); !os.IsNotExist(err) {
		t.Error("foo_jittest_test.go should have been removed")
	}
	if _, err := os.Stat(filepath.Join(pkg, "bar_jittest_test.go")); !os.IsNotExist(err) {
		t.Error("bar_jittest_test.go should have been removed")
	}
	// Non-jittest files should remain
	if _, err := os.Stat(filepath.Join(pkg, "real_test.go")); os.IsNotExist(err) {
		t.Error("real_test.go should NOT have been removed")
	}
}

func TestIsCompileError(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{"Build failed: syntax error", true},
		{"cannot find package foo", true},
		{"SyntaxError: unexpected token", true},
		{"TS2304: Cannot find name 'foo'", true},
		{"FAIL: TestFoo", false},
		{"ok  pkg/foo  0.5s", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			if got := isCompileError(tt.output); got != tt.want {
				t.Errorf("isCompileError(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestExtractTestError_WithRelevantLine(t *testing.T) {
	output := "=== RUN TestFoo\n--- FAIL: TestFoo (0.00s)\n    foo_jittest_test.go:15: Error expected 1, got 2\nFAIL\n"
	err := extractTestError(output, "foo_jittest_test.go")
	if err == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestExtractTestError_NoRelevantLine(t *testing.T) {
	output := "some output\nanother line\n"
	err := extractTestError(output, "missing_file.go")
	if err == "" {
		t.Fatal("expected fallback error message")
	}
}

func TestExtractTestError_EmptyOutput(t *testing.T) {
	err := extractTestError("", "test.go")
	if err != "test failed (no details)" {
		t.Errorf("expected fallback message, got: %s", err)
	}
}

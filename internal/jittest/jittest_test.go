package jittest

import (
	"context"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func boolPtr(v bool) *bool { return &v }

func TestRun_DisabledConfig(t *testing.T) {
	cfg := analyzer.JiTTestConfig{Enabled: boolPtr(false)}
	_, err := Run(context.Background(), Input{}, cfg)
	if err == nil {
		t.Fatal("expected error when jittest is disabled")
	}
	if err.Error() != "jittest is not enabled" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_NilEnabled(t *testing.T) {
	cfg := analyzer.JiTTestConfig{} // Enabled is nil = false
	_, err := Run(context.Background(), Input{}, cfg)
	if err == nil {
		t.Fatal("expected error when jittest Enabled is nil (default disabled)")
	}
}

func TestRun_EnabledNoWorkDir(t *testing.T) {
	cfg := analyzer.JiTTestConfig{
		Enabled:        boolPtr(true),
		MaxTests:       5,
		TimeoutSeconds: 120,
		FailurePolicy:  "warn",
		Model:          "claude-sonnet-4-6",
	}
	// No WorkDir → git diff will fail → result with error
	result, err := Run(context.Background(), Input{WorkDir: "/nonexistent", BaseBranch: "main", HeadBranch: "HEAD"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error (Run should return Result, not error): %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error == "" {
		t.Error("expected error in result for invalid workdir")
	}
}

func TestRun_EnabledFullPipeline(t *testing.T) {
	origClaude := claudeRunner
	origExec := executeTests
	defer func() {
		claudeRunner = origClaude
		executeTests = origExec
	}()

	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		return "```go filename: pkg/foo_jittest_test.go\npackage pkg\nfunc TestFoo(t *testing.T) {}\n```\n", nil
	}
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		return testExecOutcome{exitCode: 0}
	}

	dir := t.TempDir()

	cfg := analyzer.JiTTestConfig{
		Enabled:        boolPtr(true),
		MaxTests:       3,
		TimeoutSeconds: 60,
		FailurePolicy:  "warn",
		Model:          "test",
	}

	// Create a fake diff scenario
	origGetDiff := getDiffFunc
	defer func() { getDiffFunc = origGetDiff }()
	getDiffFunc = func(ctx context.Context, workDir, base, head string) (string, error) {
		return "+++ b/pkg/foo.go\n+func Foo() {}", nil
	}

	result, err := Run(context.Background(), Input{
		WorkDir:     dir,
		BaseBranch:  "main",
		HeadBranch:  "HEAD",
		Language:    "go",
		TestCommand: "echo ok",
	}, cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Generated != 1 {
		t.Errorf("expected 1 generated, got %d", result.Generated)
	}
	if result.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Passed)
	}
}

func TestJiTTestConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{"nil = false", nil, false},
		{"true", boolPtr(true), true},
		{"false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := analyzer.JiTTestConfig{Enabled: tt.enabled}
			if got := cfg.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJiTTestConfig_Defaults(t *testing.T) {
	// Simulate LoadConfig defaults
	cfg := analyzer.JiTTestConfig{}

	// Before LoadConfig applies defaults, all zero values
	if cfg.MaxTests != 0 {
		t.Errorf("MaxTests should be zero before defaults, got %d", cfg.MaxTests)
	}
	if cfg.FailurePolicy != "" {
		t.Errorf("FailurePolicy should be empty before defaults, got %s", cfg.FailurePolicy)
	}
}

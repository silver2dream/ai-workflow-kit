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

func TestRun_EnabledReturnsStub(t *testing.T) {
	cfg := analyzer.JiTTestConfig{
		Enabled:        boolPtr(true),
		MaxTests:       5,
		TimeoutSeconds: 120,
		FailurePolicy:  "warn",
		Model:          "claude-sonnet-4-6",
	}
	result, err := Run(context.Background(), Input{}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Skipped != 5 {
		t.Errorf("expected 5 skipped, got %d", result.Skipped)
	}
	if result.Error != "jittest not yet implemented" {
		t.Errorf("expected stub error message, got: %s", result.Error)
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

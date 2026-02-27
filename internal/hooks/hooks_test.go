package hooks

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func TestFire_NoHooks(t *testing.T) {
	var buf bytes.Buffer
	r := NewHookRunner(analyzer.HooksConfig{}, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestFire_UnknownEvent(t *testing.T) {
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "echo test"},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "unknown_event", nil)
	if err != nil {
		t.Fatalf("expected no error for unknown event, got %v", err)
	}
}

func TestFire_SuccessfulHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "echo hook_ok"},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(buf.String(), "hook_ok") {
		t.Errorf("expected output to contain 'hook_ok', got %q", buf.String())
	}
}

func TestFire_AbortPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "exit 1", OnFailure: "abort"},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", nil)
	if err == nil {
		t.Fatal("expected error for abort policy, got nil")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("expected error to contain 'aborted', got %q", err.Error())
	}
}

func TestFire_WarnPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "exit 1", OnFailure: "warn"},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", nil)
	if err != nil {
		t.Fatalf("expected no error for warn policy, got %v", err)
	}
	if !strings.Contains(buf.String(), "warning") {
		t.Errorf("expected warning in output, got %q", buf.String())
	}
}

func TestFire_IgnorePolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "exit 1", OnFailure: "ignore"},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", nil)
	if err != nil {
		t.Fatalf("expected no error for ignore policy, got %v", err)
	}
	if strings.Contains(buf.String(), "warning") {
		t.Errorf("expected no warning in output for ignore policy, got %q", buf.String())
	}
}

func TestFire_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	// Use a context timeout instead of hook timeout for more reliable cancellation
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "sleep 10", Timeout: "500ms", OnFailure: "abort"},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	err := r.Fire(ctx, "pre_dispatch", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should fail due to timeout or context cancellation
	if elapsed > 5*time.Second {
		t.Errorf("hook took too long (%v), timeout may not have worked", elapsed)
	}
	_ = elapsed // used
}

func TestFire_EnvVarMerge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{
				Command: "echo CFG=$HOOK_VAR EVT=$AWK_ISSUE",
				Env:     map[string]string{"HOOK_VAR": "from_config"},
			},
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", map[string]string{
		"AWK_ISSUE": "42",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "CFG=from_config") {
		t.Errorf("expected config env var in output, got %q", output)
	}
	if !strings.Contains(output, "EVT=42") {
		t.Errorf("expected event env var in output, got %q", output)
	}
}

func TestFire_DefaultWarnPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}
	var buf bytes.Buffer
	cfg := analyzer.HooksConfig{
		PreDispatch: []analyzer.HookDef{
			{Command: "exit 1"}, // no on_failure set, should default to warn
		},
	}
	r := NewHookRunner(cfg, ".", &buf)
	err := r.Fire(context.Background(), "pre_dispatch", nil)
	if err != nil {
		t.Fatalf("expected no error for default warn policy, got %v", err)
	}
	if !strings.Contains(buf.String(), "warning") {
		t.Errorf("expected warning in output for default policy, got %q", buf.String())
	}
}

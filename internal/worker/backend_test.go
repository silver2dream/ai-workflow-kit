package worker

import (
	"reflect"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

func TestCodexBackend_Name(t *testing.T) {
	b := NewCodexBackend()
	if b.Name() != "codex" {
		t.Errorf("expected 'codex', got %q", b.Name())
	}
}

func TestClaudeCodeBackend_Name(t *testing.T) {
	b := NewClaudeCodeBackend("sonnet", 50, false)
	if b.Name() != "claude-code" {
		t.Errorf("expected 'claude-code', got %q", b.Name())
	}
}

func TestClaudeCodeBackend_Defaults(t *testing.T) {
	b := NewClaudeCodeBackend("", 0, false)
	if b.Model != "sonnet" {
		t.Errorf("expected default model 'sonnet', got %q", b.Model)
	}
	if b.MaxTurns != 50 {
		t.Errorf("expected default max_turns 50, got %d", b.MaxTurns)
	}
}

// The Worker's claude is given the effort the configuration names, and no
// --effort at all when it names none.
func TestClaudeCodeBackend_Args(t *testing.T) {
	orig := effortArgs
	t.Cleanup(func() { effortArgs = orig })
	effortArgs = func(level string) []string {
		if level == "" {
			return nil
		}
		return []string{"--effort", level}
	}

	b := NewClaudeCodeBackend("opus", 80, true)
	b.Effort = "xhigh"
	want := []string{"--print", "--model", "opus", "--max-turns", "80", "--effort", "xhigh", "--dangerously-skip-permissions"}
	if got := b.args(); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}

	plain := NewClaudeCodeBackend("sonnet", 50, false)
	want = []string{"--print", "--model", "sonnet", "--max-turns", "50"}
	if got := plain.args(); !reflect.DeepEqual(got, want) {
		t.Errorf("args with no effort = %q, want %q", got, want)
	}

	// The configuration's effort reaches the backend a Worker is run with.
	cfg := &workflowConfig{}
	cfg.Worker.ClaudeCode = config.ClaudeCode{Model: "opus", Effort: "max", MaxTurns: 30}
	got, err := registryFor(cfg).Get("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if claude := got.(*ClaudeCodeBackend); claude.Effort != "max" || claude.Model != "opus" || claude.MaxTurns != 30 {
		t.Errorf("the Worker's claude-code backend = %+v, want opus with effort max and 30 turns", claude)
	}
}

func TestBackendRegistry_Get(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(NewCodexBackend())

	b, err := reg.Get("codex")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if b.Name() != "codex" {
		t.Errorf("expected 'codex', got %q", b.Name())
	}
}

func TestBackendRegistry_GetUnknown(t *testing.T) {
	reg := NewBackendRegistry()
	_, err := reg.Get("unknown")
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
}

func TestDefaultRegistry(t *testing.T) {
	reg := DefaultRegistry("sonnet", "", 50, false)
	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 backends, got %d: %v", len(names), names)
	}

	// Check both backends are registered
	codex, err := reg.Get("codex")
	if err != nil {
		t.Fatalf("expected codex backend, got error: %v", err)
	}
	if codex.Name() != "codex" {
		t.Errorf("expected 'codex', got %q", codex.Name())
	}

	claude, err := reg.Get("claude-code")
	if err != nil {
		t.Fatalf("expected claude-code backend, got error: %v", err)
	}
	if claude.Name() != "claude-code" {
		t.Errorf("expected 'claude-code', got %q", claude.Name())
	}
}

func TestBackendRegistry_Names(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(NewCodexBackend())
	reg.Register(NewClaudeCodeBackend("sonnet", 50, false))

	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	// Names should be sorted
	if names[0] != "claude-code" || names[1] != "codex" {
		t.Errorf("expected sorted names [claude-code, codex], got %v", names)
	}
}

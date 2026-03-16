package worker

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// mockBackend for registry tests
// ---------------------------------------------------------------------------

type mockBackend struct {
	name string
}

func (m *mockBackend) Name() string { return m.name }
func (m *mockBackend) Execute(ctx context.Context, opts BackendOptions) BackendResult {
	return BackendResult{ExitCode: 0}
}
func (m *mockBackend) Available() error { return nil }

// ---------------------------------------------------------------------------
// BackendRegistry
// ---------------------------------------------------------------------------

func TestCov_BackendRegistry_RegisterAndGet(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(&mockBackend{name: "test-backend"})

	b, err := reg.Get("test-backend")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if b.Name() != "test-backend" {
		t.Errorf("expected 'test-backend', got %q", b.Name())
	}
}

func TestCov_BackendRegistry_GetNotFound(t *testing.T) {
	reg := NewBackendRegistry()

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent backend")
	}
}

func TestCov_BackendRegistry_Names_Extended(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(&mockBackend{name: "beta"})
	reg.Register(&mockBackend{name: "alpha"})

	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	// Should be sorted
	if names[0] != "alpha" {
		t.Errorf("expected first name 'alpha', got %q", names[0])
	}
	if names[1] != "beta" {
		t.Errorf("expected second name 'beta', got %q", names[1])
	}
}

func TestCov_BackendRegistry_OverwriteRegistration(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(&mockBackend{name: "test"})
	reg.Register(&mockBackend{name: "test"}) // overwrite

	names := reg.Names()
	if len(names) != 1 {
		t.Errorf("expected 1 name after overwrite, got %d", len(names))
	}
}

func TestCov_DefaultRegistry_Extended(t *testing.T) {
	reg := DefaultRegistry("", 0, false)
	names := reg.Names()

	foundCodex := false
	foundClaude := false
	for _, name := range names {
		if name == "codex" {
			foundCodex = true
		}
		if name == "claude-code" {
			foundClaude = true
		}
	}

	if !foundCodex {
		t.Error("expected 'codex' in default registry")
	}
	if !foundClaude {
		t.Error("expected 'claude-code' in default registry")
	}
}

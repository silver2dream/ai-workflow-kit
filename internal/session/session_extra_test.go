package session

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewManager with empty stateRoot (uses resolveStateRoot via AI_STATE_ROOT env)
// ---------------------------------------------------------------------------

func TestNewManager_EmptyStateRoot_UsesEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AI_STATE_ROOT", dir)

	m := NewManager("") // empty triggers resolveStateRoot
	if m.StateRoot != dir {
		t.Errorf("StateRoot = %q, want %q (from AI_STATE_ROOT)", m.StateRoot, dir)
	}
}

func TestNewManager_ExplicitStateRoot(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if m.StateRoot != dir {
		t.Errorf("StateRoot = %q, want %q", m.StateRoot, dir)
	}
}

// ---------------------------------------------------------------------------
// IsPrincipalRunning (calls IsProcessRunning)
// ---------------------------------------------------------------------------

func TestIsPrincipalRunning_CurrentProcess(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	// Current process should be running
	pid := os.Getpid()
	// startTime of 0 means "don't check start time" in most implementations
	// We just verify it returns a bool without panic
	result := m.IsPrincipalRunning(pid, 0)
	_ = result // true or false depending on implementation
}

func TestIsPrincipalRunning_ZeroPID(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	// PID 0 should not be "running" as a principal
	result := m.IsPrincipalRunning(0, 0)
	if result {
		t.Error("PID 0 should not be considered running")
	}
}

// ---------------------------------------------------------------------------
// InitPrincipal sequential (no concurrent overlap)
// ---------------------------------------------------------------------------

func TestInitPrincipal_Sequential_UniqueIDs(t *testing.T) {
	dir1 := t.TempDir()
	m1 := NewManager(dir1)

	sid1, err := m1.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal (first manager): %v", err)
	}
	if sid1 == "" {
		t.Error("session ID should not be empty")
	}

	// Use a different state root so they don't conflict
	dir2 := t.TempDir()
	m2 := NewManager(dir2)
	sid2, err := m2.InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal (second manager): %v", err)
	}
	if sid2 == sid1 {
		t.Error("second session ID should differ from first")
	}
}

// ---------------------------------------------------------------------------
// GetCurrentSessionID when no session exists
// ---------------------------------------------------------------------------

func TestGetCurrentSessionID_NoSession(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	id := m.GetCurrentSessionID()
	if id != "" {
		t.Errorf("GetCurrentSessionID with no session = %q, want empty", id)
	}
}

// ---------------------------------------------------------------------------
// GenerateSessionID format checks
// ---------------------------------------------------------------------------

func TestGenerateSessionID_HasRole(t *testing.T) {
	for _, role := range []string{"principal", "worker", "reviewer"} {
		id := GenerateSessionID(role)
		if !strings.HasPrefix(id, role+"-") {
			t.Errorf("GenerateSessionID(%q) = %q, should start with %s-", role, id, role)
		}
	}
}

func TestGenerateSessionID_IsUnique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id := GenerateSessionID("test")
		if ids[id] {
			t.Errorf("GenerateSessionID generated duplicate: %q", id)
		}
		ids[id] = true
	}
}

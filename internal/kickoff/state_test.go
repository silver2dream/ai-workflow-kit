package kickoff

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewStateManager tests state manager creation
func TestNewStateManager(t *testing.T) {
	mgr := NewStateManager("/path/to/state.json")

	if mgr == nil {
		t.Fatal("NewStateManager returned nil")
	}

	if mgr.stateFile != "/path/to/state.json" {
		t.Errorf("stateFile = %q, want %q", mgr.stateFile, "/path/to/state.json")
	}
}

// TestStateManager_SaveLoadState tests save and load cycle
func TestStateManager_SaveLoadState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "last_run.json")
	mgr := NewStateManager(stateFile)

	// Save state
	state := &RunState{
		Phase:            "STEP-3",
		CompletedTasks:   []string{"1.1", "1.2", "2.1"},
		PendingTasks:     []string{"2.2", "3.1"},
		IssuesInProgress: []int{42, 43},
	}

	if err := mgr.SaveState(state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Load state
	loaded, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	// Verify fields
	if loaded.Phase != state.Phase {
		t.Errorf("Phase = %q, want %q", loaded.Phase, state.Phase)
	}

	if len(loaded.CompletedTasks) != len(state.CompletedTasks) {
		t.Errorf("CompletedTasks count = %d, want %d", len(loaded.CompletedTasks), len(state.CompletedTasks))
	}

	if len(loaded.PendingTasks) != len(state.PendingTasks) {
		t.Errorf("PendingTasks count = %d, want %d", len(loaded.PendingTasks), len(state.PendingTasks))
	}

	if len(loaded.IssuesInProgress) != 2 {
		t.Errorf("IssuesInProgress count = %d, want 2", len(loaded.IssuesInProgress))
	}

	// SavedAt should be set
	if loaded.SavedAt.IsZero() {
		t.Error("SavedAt should be set")
	}
}

// TestStateManager_HasState tests state existence check
func TestStateManager_HasState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-has-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "last_run.json")
	mgr := NewStateManager(stateFile)

	// Initially no state
	if mgr.HasState() {
		t.Error("HasState should return false when no state exists")
	}

	// Save state
	mgr.SaveState(&RunState{Phase: "test"})

	// Now has state
	if !mgr.HasState() {
		t.Error("HasState should return true after saving state")
	}
}

// TestStateManager_IsStale tests stale state detection
func TestStateManager_IsStale(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-stale-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "last_run.json")
	mgr := NewStateManager(stateFile)

	// Save fresh state
	mgr.SaveState(&RunState{Phase: "test"})

	// Fresh state should not be stale
	if mgr.IsStale() {
		t.Error("Fresh state should not be stale")
	}

	// Manually create stale state (> 24 hours old)
	staleState := &RunState{
		Phase:   "old",
		SavedAt: time.Now().Add(-25 * time.Hour),
	}

	// Write directly to bypass SaveState's timestamp update
	data := `{"phase":"old","saved_at":"` + staleState.SavedAt.Format(time.RFC3339) + `"}`
	os.WriteFile(stateFile, []byte(data), 0644)

	// Should be stale now
	if !mgr.IsStale() {
		t.Error("State older than 24 hours should be stale")
	}
}

// TestStateManager_ClearState tests state clearing
func TestStateManager_ClearState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-clear-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "last_run.json")
	mgr := NewStateManager(stateFile)

	// Save state
	mgr.SaveState(&RunState{Phase: "test"})

	if !mgr.HasState() {
		t.Fatal("State should exist after save")
	}

	// Clear state
	if err := mgr.ClearState(); err != nil {
		t.Fatalf("ClearState failed: %v", err)
	}

	// State should be gone
	if mgr.HasState() {
		t.Error("State should not exist after clear")
	}
}

// TestStateManager_ClearState_NoFile tests clearing non-existent state
func TestStateManager_ClearState_NoFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-clear-nofile-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "nonexistent.json")
	mgr := NewStateManager(stateFile)

	// Should not error when clearing non-existent file
	if err := mgr.ClearState(); err != nil {
		t.Errorf("ClearState should not error for non-existent file: %v", err)
	}
}

// TestStateManager_LoadState_FileNotFound tests loading non-existent state
func TestStateManager_LoadState_FileNotFound(t *testing.T) {
	mgr := NewStateManager("/nonexistent/path/state.json")

	_, err := mgr.LoadState()
	if err == nil {
		t.Error("LoadState should error for non-existent file")
	}
}

// TestStateManager_LoadState_InvalidJSON tests loading invalid JSON
func TestStateManager_LoadState_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "invalid.json")
	os.WriteFile(stateFile, []byte("not valid json"), 0644)

	mgr := NewStateManager(stateFile)

	_, err = mgr.LoadState()
	if err == nil {
		t.Error("LoadState should error for invalid JSON")
	}
}

// TestStateManager_SaveState_CreatesDirectory tests directory creation
func TestStateManager_SaveState_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "state-mkdir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use nested path that doesn't exist
	stateFile := filepath.Join(tmpDir, "nested", "dir", "state.json")
	mgr := NewStateManager(stateFile)

	// Save should create directories
	if err := mgr.SaveState(&RunState{Phase: "test"}); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("State file should exist after save")
	}
}

// TestStateStaleThreshold tests the stale threshold constant
func TestStateStaleThreshold(t *testing.T) {
	// Should be 24 hours
	expected := 24 * time.Hour
	if StateStaleThreshold != expected {
		t.Errorf("StateStaleThreshold = %v, want %v", StateStaleThreshold, expected)
	}
}

// TestRunState_Fields tests RunState struct fields
func TestRunState_Fields(t *testing.T) {
	state := RunState{
		Phase:            "STEP-4",
		CompletedTasks:   []string{"1.1"},
		PendingTasks:     []string{"2.1"},
		IssuesInProgress: []int{42},
		SavedAt:          time.Now(),
	}

	if state.Phase != "STEP-4" {
		t.Errorf("Phase = %q, want %q", state.Phase, "STEP-4")
	}

	if len(state.CompletedTasks) != 1 {
		t.Errorf("CompletedTasks count = %d, want 1", len(state.CompletedTasks))
	}

	if len(state.PendingTasks) != 1 {
		t.Errorf("PendingTasks count = %d, want 1", len(state.PendingTasks))
	}

	if len(state.IssuesInProgress) != 1 {
		t.Errorf("IssuesInProgress count = %d, want 1", len(state.IssuesInProgress))
	}
}

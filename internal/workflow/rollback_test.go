package workflow

import (
	"strings"
	"testing"
)

// TestSubmoduleRollbackConsistency tests submodule rollback consistency.
// Property 12: Submodule Rollback Consistency

func TestRollbackOrderSubmoduleFirst(t *testing.T) {
	// Test submodule is reverted before parent (Req 18.1, 18.2).
	state := NewRollbackState()

	success, errMsg := RollbackSubmodule("abc123", "def456", state)

	if !success {
		t.Errorf("RollbackSubmodule() success = %v, want true", success)
	}

	if errMsg != "" {
		t.Errorf("RollbackSubmodule() error = %q, want empty", errMsg)
	}

	if !VerifyRollbackOrder(state.OperationsLog) {
		t.Error("VerifyRollbackOrder() = false, want true")
	}

	// Verify order explicitly
	submoduleIdx := -1
	parentIdx := -1
	for i, op := range state.OperationsLog {
		if op == "revert_submodule" {
			submoduleIdx = i
		}
		if op == "revert_parent" {
			parentIdx = i
		}
	}

	if submoduleIdx == -1 {
		t.Error("revert_submodule not found in operations log")
	}

	if parentIdx == -1 {
		t.Error("revert_parent not found in operations log")
	}

	if submoduleIdx >= parentIdx {
		t.Errorf("revert_submodule index (%d) should be less than revert_parent index (%d)", submoduleIdx, parentIdx)
	}
}

func TestBothCommitsReverted(t *testing.T) {
	// Test both submodule and parent are reverted (Req 18.3).
	state := NewRollbackState()

	success, _ := RollbackSubmodule("abc123", "def456", state)

	if !success {
		t.Error("RollbackSubmodule() success = false, want true")
	}

	if !state.SubmoduleReverted {
		t.Error("SubmoduleReverted = false, want true")
	}

	if !state.ParentReverted {
		t.Error("ParentReverted = false, want true")
	}
}

func TestMissingSubmoduleSHAFails(t *testing.T) {
	// Test rollback fails without submodule SHA (Req 18.4).
	state := NewRollbackState()

	success, errMsg := RollbackSubmodule("", "def456", state)

	if success {
		t.Error("RollbackSubmodule() success = true, want false")
	}

	if !strings.Contains(errMsg, "No submodule SHA") {
		t.Errorf("error = %q, should contain 'No submodule SHA'", errMsg)
	}
}

func TestMissingParentSHAFails(t *testing.T) {
	// Test rollback fails without parent SHA (Req 18.4).
	state := NewRollbackState()

	success, errMsg := RollbackSubmodule("abc123", "", state)

	if success {
		t.Error("RollbackSubmodule() success = true, want false")
	}

	if !strings.Contains(errMsg, "No parent SHA") {
		t.Errorf("error = %q, should contain 'No parent SHA'", errMsg)
	}
}

// TestRollbackByRepoType tests rollback for different repo types.

func TestRootTypeRollback(t *testing.T) {
	// Test root type rollback only reverts parent.
	state := NewRollbackState()

	success, _ := RollbackRoot("abc123", state)

	if !success {
		t.Error("RollbackRoot() success = false, want true")
	}

	if !state.ParentReverted {
		t.Error("ParentReverted = false, want true")
	}

	if state.SubmoduleReverted {
		t.Error("SubmoduleReverted = true, want false")
	}
}

func TestDirectoryTypeRollback(t *testing.T) {
	// Test directory type rollback only reverts parent.
	state := NewRollbackState()

	success, _ := RollbackDirectory("abc123", state)

	if !success {
		t.Error("RollbackDirectory() success = false, want true")
	}

	if !state.ParentReverted {
		t.Error("ParentReverted = false, want true")
	}

	if state.SubmoduleReverted {
		t.Error("SubmoduleReverted = true, want false")
	}
}

func TestSubmoduleTypeRollback(t *testing.T) {
	// Test submodule type rollback reverts both.
	state := NewRollbackState()

	success, _ := RollbackSubmodule("abc123", "def456", state)

	if !success {
		t.Error("RollbackSubmodule() success = false, want true")
	}

	if !state.ParentReverted {
		t.Error("ParentReverted = false, want true")
	}

	if !state.SubmoduleReverted {
		t.Error("SubmoduleReverted = false, want true")
	}
}

// TestVerifyRollbackOrder tests rollback order verification function.

func TestCorrectOrder(t *testing.T) {
	// Test correct order is verified.
	log := []string{"revert_submodule", "revert_parent"}

	if !VerifyRollbackOrder(log) {
		t.Error("VerifyRollbackOrder() = false, want true")
	}
}

func TestIncorrectOrder(t *testing.T) {
	// Test incorrect order is detected.
	log := []string{"revert_parent", "revert_submodule"}

	if VerifyRollbackOrder(log) {
		t.Error("VerifyRollbackOrder() = true, want false")
	}
}

func TestEmptyLog(t *testing.T) {
	// Test empty log returns true.
	if !VerifyRollbackOrder([]string{}) {
		t.Error("VerifyRollbackOrder([]) = false, want true")
	}
}

func TestPartialLog(t *testing.T) {
	// Test partial log returns true.
	log := []string{"revert_submodule"}

	if !VerifyRollbackOrder(log) {
		t.Error("VerifyRollbackOrder() = false, want true")
	}
}

// Table-driven test for rollback scenarios.
func TestRollbackSubmodule_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		submoduleSHA string
		parentSHA    string
		wantSuccess  bool
		wantError    string
	}{
		{
			name:         "both SHAs provided succeeds",
			submoduleSHA: "abc123",
			parentSHA:    "def456",
			wantSuccess:  true,
			wantError:    "",
		},
		{
			name:         "empty submodule SHA fails",
			submoduleSHA: "",
			parentSHA:    "def456",
			wantSuccess:  false,
			wantError:    "No submodule SHA",
		},
		{
			name:         "empty parent SHA fails",
			submoduleSHA: "abc123",
			parentSHA:    "",
			wantSuccess:  false,
			wantError:    "No parent SHA",
		},
		{
			name:         "both empty fails on submodule first",
			submoduleSHA: "",
			parentSHA:    "",
			wantSuccess:  false,
			wantError:    "No submodule SHA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewRollbackState()
			success, errMsg := RollbackSubmodule(tt.submoduleSHA, tt.parentSHA, state)

			if success != tt.wantSuccess {
				t.Errorf("success = %v, want %v", success, tt.wantSuccess)
			}

			if tt.wantError != "" && !strings.Contains(errMsg, tt.wantError) {
				t.Errorf("error = %q, should contain %q", errMsg, tt.wantError)
			}

			if tt.wantError == "" && errMsg != "" {
				t.Errorf("error = %q, want empty", errMsg)
			}
		})
	}
}

func TestVerifyRollbackOrder_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		log  []string
		want bool
	}{
		{
			name: "correct order",
			log:  []string{"revert_submodule", "revert_parent"},
			want: true,
		},
		{
			name: "incorrect order",
			log:  []string{"revert_parent", "revert_submodule"},
			want: false,
		},
		{
			name: "empty log",
			log:  []string{},
			want: true,
		},
		{
			name: "only submodule",
			log:  []string{"revert_submodule"},
			want: true,
		},
		{
			name: "only parent",
			log:  []string{"revert_parent"},
			want: true,
		},
		{
			name: "correct order with extra operations",
			log:  []string{"init", "revert_submodule", "cleanup", "revert_parent", "done"},
			want: true,
		},
		{
			name: "incorrect order with extra operations",
			log:  []string{"init", "revert_parent", "cleanup", "revert_submodule", "done"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyRollbackOrder(tt.log)
			if got != tt.want {
				t.Errorf("VerifyRollbackOrder(%v) = %v, want %v", tt.log, got, tt.want)
			}
		})
	}
}

func TestRollbackRoot_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		parentSHA   string
		wantSuccess bool
		wantError   string
	}{
		{
			name:        "valid SHA succeeds",
			parentSHA:   "abc123",
			wantSuccess: true,
			wantError:   "",
		},
		{
			name:        "empty SHA fails",
			parentSHA:   "",
			wantSuccess: false,
			wantError:   "No parent SHA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewRollbackState()
			success, errMsg := RollbackRoot(tt.parentSHA, state)

			if success != tt.wantSuccess {
				t.Errorf("success = %v, want %v", success, tt.wantSuccess)
			}

			if tt.wantError != "" && !strings.Contains(errMsg, tt.wantError) {
				t.Errorf("error = %q, should contain %q", errMsg, tt.wantError)
			}
		})
	}
}

func TestRollbackDirectory_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		parentSHA   string
		wantSuccess bool
		wantError   string
	}{
		{
			name:        "valid SHA succeeds",
			parentSHA:   "abc123",
			wantSuccess: true,
			wantError:   "",
		},
		{
			name:        "empty SHA fails",
			parentSHA:   "",
			wantSuccess: false,
			wantError:   "No parent SHA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewRollbackState()
			success, errMsg := RollbackDirectory(tt.parentSHA, state)

			if success != tt.wantSuccess {
				t.Errorf("success = %v, want %v", success, tt.wantSuccess)
			}

			if tt.wantError != "" && !strings.Contains(errMsg, tt.wantError) {
				t.Errorf("error = %q, should contain %q", errMsg, tt.wantError)
			}
		})
	}
}

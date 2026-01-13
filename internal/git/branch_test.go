package git

import "testing"

// TestSubmoduleBranchManagement tests submodule branch management.
// Property 24: Submodule Branch Management

func TestCreateNewBranch(t *testing.T) {
	// Test creating new branch in submodule (Req 16.1)
	state := NewBranchState()

	result := SetupSubmoduleBranch("feat/ai-issue-1", state)

	if result != "feat/ai-issue-1" {
		t.Errorf("expected branch name 'feat/ai-issue-1', got %q", result)
	}
	if !state.HasLocalBranch("feat/ai-issue-1") {
		t.Error("expected branch to be in local branches")
	}
	if state.CurrentBranch != "feat/ai-issue-1" {
		t.Errorf("expected current branch to be 'feat/ai-issue-1', got %q", state.CurrentBranch)
	}
}

func TestReuseExistingLocalBranch(t *testing.T) {
	// Test reusing existing local branch (Req 16.4)
	state := NewBranchState()
	state.AddLocalBranch("feat/ai-issue-1")

	result := SetupSubmoduleBranch("feat/ai-issue-1", state)

	if result != "feat/ai-issue-1" {
		t.Errorf("expected branch name 'feat/ai-issue-1', got %q", result)
	}
	if state.CurrentBranch != "feat/ai-issue-1" {
		t.Errorf("expected current branch to be 'feat/ai-issue-1', got %q", state.CurrentBranch)
	}
}

func TestCheckoutRemoteBranch(t *testing.T) {
	// Test checking out remote branch (Req 16.4)
	state := NewBranchState()
	state.AddRemoteBranch("feat/ai-issue-1")

	result := SetupSubmoduleBranch("feat/ai-issue-1", state)

	if result != "feat/ai-issue-1" {
		t.Errorf("expected branch name 'feat/ai-issue-1', got %q", result)
	}
	if !state.HasLocalBranch("feat/ai-issue-1") {
		t.Error("expected branch to be added to local branches")
	}
	if state.CurrentBranch != "feat/ai-issue-1" {
		t.Errorf("expected current branch to be 'feat/ai-issue-1', got %q", state.CurrentBranch)
	}
}

func TestDefaultBranchUsedAsBase(t *testing.T) {
	// Test default branch is used as base (Req 16.2)
	state := NewBranchState()
	state.DefaultBranch = "develop"

	defaultBranch := GetSubmoduleDefaultBranch(state)

	if defaultBranch != "develop" {
		t.Errorf("expected default branch 'develop', got %q", defaultBranch)
	}
}

// TestBranchState tests branch state tracking

func TestInitialState(t *testing.T) {
	// Test initial branch state
	state := NewBranchState()

	if state.LocalBranchCount() != 0 {
		t.Errorf("expected 0 local branches, got %d", state.LocalBranchCount())
	}
	if state.RemoteBranchCount() != 0 {
		t.Errorf("expected 0 remote branches, got %d", state.RemoteBranchCount())
	}
	if state.CurrentBranch != "" {
		t.Errorf("expected empty current branch, got %q", state.CurrentBranch)
	}
	if state.DefaultBranch != "main" {
		t.Errorf("expected default branch 'main', got %q", state.DefaultBranch)
	}
}

func TestMultipleBranches(t *testing.T) {
	// Test multiple branches can be tracked
	state := NewBranchState()

	SetupSubmoduleBranch("feat/ai-issue-1", state)
	SetupSubmoduleBranch("feat/ai-issue-2", state)

	if !state.HasLocalBranch("feat/ai-issue-1") {
		t.Error("expected feat/ai-issue-1 in local branches")
	}
	if !state.HasLocalBranch("feat/ai-issue-2") {
		t.Error("expected feat/ai-issue-2 in local branches")
	}
}

func TestCurrentBranchUpdated(t *testing.T) {
	// Test current branch is updated
	state := NewBranchState()

	SetupSubmoduleBranch("feat/ai-issue-1", state)
	if state.CurrentBranch != "feat/ai-issue-1" {
		t.Errorf("expected current branch 'feat/ai-issue-1', got %q", state.CurrentBranch)
	}

	SetupSubmoduleBranch("feat/ai-issue-2", state)
	if state.CurrentBranch != "feat/ai-issue-2" {
		t.Errorf("expected current branch 'feat/ai-issue-2', got %q", state.CurrentBranch)
	}
}

// TestBranchNaming tests branch naming conventions

func TestValidBranchNames(t *testing.T) {
	// Test valid branch names are accepted (table-driven test)
	tests := []struct {
		name       string
		branchName string
	}{
		{"feat issue 1", "feat/ai-issue-1"},
		{"feat issue 123", "feat/ai-issue-123"},
		{"fix issue 456", "fix/ai-issue-456"},
		{"chore issue 789", "chore/ai-issue-789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewBranchState()

			result := SetupSubmoduleBranch(tt.branchName, state)

			if result != tt.branchName {
				t.Errorf("expected branch name %q, got %q", tt.branchName, result)
			}
			if !state.HasLocalBranch(tt.branchName) {
				t.Errorf("expected %q in local branches", tt.branchName)
			}
		})
	}
}

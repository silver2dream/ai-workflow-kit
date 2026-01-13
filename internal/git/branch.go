// Package git provides git operations for branch management, worktrees, and submodules.
package git

// BranchState tracks branch state for a repository.
type BranchState struct {
	LocalBranches  map[string]bool
	RemoteBranches map[string]bool
	CurrentBranch  string
	DefaultBranch  string
}

// NewBranchState creates a new BranchState with default values.
func NewBranchState() *BranchState {
	return &BranchState{
		LocalBranches:  make(map[string]bool),
		RemoteBranches: make(map[string]bool),
		CurrentBranch:  "",
		DefaultBranch:  "main",
	}
}

// SetupSubmoduleBranch creates or reuses a branch in a submodule.
//
// Property 24: Submodule Branch Management
// For any submodule-type repo, the system SHALL:
//   - Create branches in the submodule first (Req 16.1)
//   - Use the submodule's default branch as base (Req 16.2)
//   - Reuse existing branches when available (Req 16.4)
//
// Returns the branch name that was set up.
func SetupSubmoduleBranch(branchName string, state *BranchState) string {
	// Check if branch already exists locally (Req 16.4)
	if state.LocalBranches[branchName] {
		state.CurrentBranch = branchName
		return branchName
	}

	// Check if branch exists on remote (Req 16.4)
	if state.RemoteBranches[branchName] {
		state.LocalBranches[branchName] = true
		state.CurrentBranch = branchName
		return branchName
	}

	// Create new branch from default branch (Req 16.1, 16.2)
	state.LocalBranches[branchName] = true
	state.CurrentBranch = branchName
	return branchName
}

// GetSubmoduleDefaultBranch returns the submodule's default branch.
func GetSubmoduleDefaultBranch(state *BranchState) string {
	return state.DefaultBranch
}

// LocalBranchCount returns the number of local branches.
func (s *BranchState) LocalBranchCount() int {
	return len(s.LocalBranches)
}

// RemoteBranchCount returns the number of remote branches.
func (s *BranchState) RemoteBranchCount() int {
	return len(s.RemoteBranches)
}

// HasLocalBranch checks if a local branch exists.
func (s *BranchState) HasLocalBranch(branch string) bool {
	return s.LocalBranches[branch]
}

// HasRemoteBranch checks if a remote branch exists.
func (s *BranchState) HasRemoteBranch(branch string) bool {
	return s.RemoteBranches[branch]
}

// AddLocalBranch adds a branch to local branches.
func (s *BranchState) AddLocalBranch(branch string) {
	s.LocalBranches[branch] = true
}

// AddRemoteBranch adds a branch to remote branches.
func (s *BranchState) AddRemoteBranch(branch string) {
	s.RemoteBranches[branch] = true
}

package workflow

// RollbackState tracks rollback operation state.
type RollbackState struct {
	OperationsLog     []string
	SubmoduleReverted bool
	ParentReverted    bool
	Error             string
}

// NewRollbackState creates a new RollbackState.
func NewRollbackState() *RollbackState {
	return &RollbackState{
		OperationsLog: []string{},
	}
}

// RollbackSubmodule rollbacks submodule and parent commits.
//
// Property 12: Submodule Rollback Consistency
// *For any* rollback of a PR that modified a submodule, the system SHALL
// revert both the submodule commit and the parent reference in the correct
// order (submodule first, then parent).
//
// Returns: (success, error_message)
func RollbackSubmodule(submoduleSHA, parentSHA string, state *RollbackState) (bool, string) {
	// Step 1: Revert submodule commit first (Req 18.1)
	state.OperationsLog = append(state.OperationsLog, "revert_submodule")
	if submoduleSHA == "" {
		state.Error = "No submodule SHA to revert"
		return false, state.Error
	}
	state.SubmoduleReverted = true

	// Step 2: Revert parent reference (Req 18.2)
	state.OperationsLog = append(state.OperationsLog, "revert_parent")
	if parentSHA == "" {
		state.Error = "No parent SHA to revert"
		return false, state.Error
	}
	state.ParentReverted = true

	return true, ""
}

// VerifyRollbackOrder verifies rollback operations happened in correct order.
//
// Property 12: Submodule Rollback Consistency
// Expected order: submodule revert before parent revert
func VerifyRollbackOrder(operationsLog []string) bool {
	submoduleIdx := -1
	parentIdx := -1

	for i, op := range operationsLog {
		if op == "revert_submodule" && submoduleIdx == -1 {
			submoduleIdx = i
		}
		if op == "revert_parent" && parentIdx == -1 {
			parentIdx = i
		}
	}

	// Not enough operations to verify
	if submoduleIdx == -1 || parentIdx == -1 {
		return true
	}

	return submoduleIdx < parentIdx
}

// RollbackDirectory rollbacks directory type commit.
//
// Returns: (success, error_message)
func RollbackDirectory(parentSHA string, state *RollbackState) (bool, string) {
	state.OperationsLog = append(state.OperationsLog, "revert_parent")
	if parentSHA == "" {
		state.Error = "No parent SHA to revert"
		return false, state.Error
	}
	state.ParentReverted = true
	return true, ""
}

// RollbackRoot rollbacks root type commit.
//
// Returns: (success, error_message)
func RollbackRoot(parentSHA string, state *RollbackState) (bool, string) {
	state.OperationsLog = append(state.OperationsLog, "revert_parent")
	if parentSHA == "" {
		state.Error = "No parent SHA to revert"
		return false, state.Error
	}
	state.ParentReverted = true
	return true, ""
}

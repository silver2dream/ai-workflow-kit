package git

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// SubmoduleState tracks submodule operation state.
type SubmoduleState struct {
	SubmoduleSHA      string
	ParentSHA         string
	ConsistencyStatus string
	OperationsLog     []string
}

// NewSubmoduleState creates a new SubmoduleState with default values.
func NewSubmoduleState() *SubmoduleState {
	return &SubmoduleState{
		SubmoduleSHA:      "",
		ParentSHA:         "",
		ConsistencyStatus: "consistent",
		OperationsLog:     []string{},
	}
}

// CheckSubmoduleBoundary checks if all staged changes are within submodule boundary.
//
// Property 14: Submodule File Boundary
// For any commit in a submodule-type repo, all changed files SHALL be
// within the submodule path (unless explicitly overridden).
//
// Returns (is_valid, list_of_outside_files).
func CheckSubmoduleBoundary(wtDir, submodulePath string, allowParent bool) (bool, []string) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = wtDir
	output, err := cmd.Output()
	if err != nil {
		return true, nil
	}

	changedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	var outsideFiles []string

	// Normalize submodule path
	submodulePath = strings.TrimSuffix(submodulePath, "/")

	for _, file := range changedFiles {
		if file == "" {
			continue
		}
		// Check if file is within submodule path
		if !strings.HasPrefix(file, submodulePath+"/") && file != submodulePath {
			outsideFiles = append(outsideFiles, file)
		}
	}

	if len(outsideFiles) > 0 && !allowParent {
		return false, outsideFiles
	}

	return true, outsideFiles
}

// GitCommitSubmodule commits changes in submodule first, then updates parent reference.
//
// Property 5: Submodule Git Operations Order
// For any submodule-type repo, git operations SHALL follow this order:
//  1. Commit in submodule first
//  2. Update parent's submodule reference
//
// Returns (success, error_message).
func GitCommitSubmodule(wtDir, submodulePath, commitMsg string, state *SubmoduleState) (bool, string) {
	submoduleDir := filepath.Join(wtDir, submodulePath)

	// Step 1: Stage and commit in submodule first
	state.OperationsLog = append(state.OperationsLog, "submodule_stage")
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = submoduleDir
	if err := cmd.Run(); err != nil {
		return false, "failed to stage in submodule: " + err.Error()
	}

	// Check if there are changes
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = submoduleDir
	if err := cmd.Run(); err == nil {
		// returncode == 0 means no changes
		return false, "no changes in submodule"
	}

	state.OperationsLog = append(state.OperationsLog, "submodule_commit")
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = submoduleDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		state.ConsistencyStatus = "submodule_commit_failed"
		return false, "submodule commit failed: " + string(output)
	}

	// Record submodule SHA
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = submoduleDir
	shaOutput, err := cmd.Output()
	if err != nil {
		return false, "failed to get submodule SHA: " + err.Error()
	}
	state.SubmoduleSHA = strings.TrimSpace(string(shaOutput))

	// Step 2: Update parent's submodule reference
	state.OperationsLog = append(state.OperationsLog, "parent_stage_submodule_ref")
	cmd = exec.Command("git", "add", submodulePath)
	cmd.Dir = wtDir
	if err := cmd.Run(); err != nil {
		return false, "failed to stage submodule ref: " + err.Error()
	}

	state.OperationsLog = append(state.OperationsLog, "parent_commit")
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = wtDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		state.ConsistencyStatus = "submodule_committed_parent_failed"
		return false, "parent commit failed: " + string(output)
	}

	// Record parent SHA
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = wtDir
	shaOutput, err = cmd.Output()
	if err != nil {
		return false, "failed to get parent SHA: " + err.Error()
	}
	state.ParentSHA = strings.TrimSpace(string(shaOutput))

	return true, ""
}

// VerifyCommitOrder verifies that operations happened in correct order.
//
// Property 5: Submodule Git Operations Order
// Expected order: submodule operations before parent operations.
func VerifyCommitOrder(operationsLog []string) bool {
	submoduleOps := []string{"submodule_stage", "submodule_commit"}
	parentOps := []string{"parent_stage_submodule_ref", "parent_commit"}

	// Find indices
	var submoduleIndices, parentIndices []int

	for _, op := range submoduleOps {
		for i, logOp := range operationsLog {
			if logOp == op {
				submoduleIndices = append(submoduleIndices, i)
				break
			}
		}
	}

	for _, op := range parentOps {
		for i, logOp := range operationsLog {
			if logOp == op {
				parentIndices = append(parentIndices, i)
				break
			}
		}
	}

	if len(submoduleIndices) == 0 || len(parentIndices) == 0 {
		return true // Not enough operations to verify
	}

	// All submodule ops should come before all parent ops
	maxSubmodule := submoduleIndices[0]
	for _, idx := range submoduleIndices {
		if idx > maxSubmodule {
			maxSubmodule = idx
		}
	}

	minParent := parentIndices[0]
	for _, idx := range parentIndices {
		if idx < minParent {
			minParent = idx
		}
	}

	return maxSubmodule < minParent
}

// GitCommitDirectory commits changes for directory type repo.
//
// Property 22: Directory Type Git Operations
// For any directory-type repo, git operations SHALL execute from the
// worktree root and include all changes in the commit.
//
// Returns (success, error_message).
func GitCommitDirectory(wtDir, repoPath, commitMsg string) (bool, string) {
	// Stage all changes from worktree root
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = wtDir
	if err := cmd.Run(); err != nil {
		return false, "failed to stage: " + err.Error()
	}

	// Check if there are changes
	cmd = exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = wtDir
	if err := cmd.Run(); err == nil {
		// returncode == 0 means no changes
		return false, "no changes"
	}

	// Commit from worktree root
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = wtDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "commit failed: " + string(output)
	}

	return true, ""
}

// GetCommittedFiles returns the list of files in the last commit.
func GetCommittedFiles(wtDir string) ([]string, error) {
	cmd := exec.Command("git", "show", "--name-only", "--format=")
	cmd.Dir = wtDir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

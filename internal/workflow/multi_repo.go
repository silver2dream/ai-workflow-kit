package workflow

// RepoConfig represents repository configuration.
// Used for multi-repo coordination in workflow processing.
type RepoConfig struct {
	Name     string
	RepoType string
	Path     string
}

// ExecutionResult represents the result of repo execution.
type ExecutionResult struct {
	RepoName string
	Success  bool
	Error    string
}

// ExecuteFunc is a function type for executing repo operations.
type ExecuteFunc func(repo RepoConfig) (success bool, err string)

// ProcessReposSequential processes multiple repos sequentially.
//
// Property 25: Multi-Repo Coordination
// *For any* ticket specifying multiple repos, the system SHALL:
// - Process them in the specified order (sequential) (Req 17.1)
// - Stop on first failure (Req 17.2)
// - Handle submodule repos with submodule-specific logic (Req 17.3, 17.4)
//
// Returns: (overall_success, list_of_results)
func ProcessReposSequential(repos []RepoConfig, executeFn ExecuteFunc) (bool, []ExecutionResult) {
	var results []ExecutionResult

	for _, repo := range repos {
		var success bool
		var errMsg string

		// Execute repo (Req 17.1 - sequential)
		if executeFn != nil {
			success, errMsg = executeFn(repo)
		} else {
			success, errMsg = true, ""
		}

		result := ExecutionResult{
			RepoName: repo.Name,
			Success:  success,
			Error:    errMsg,
		}
		results = append(results, result)

		// Stop on first failure (Req 17.2)
		if !success {
			return false, results
		}
	}

	return true, results
}

// GetRepoExecutionOrder returns the order repos will be executed.
//
// Returns: List of repo names in execution order
func GetRepoExecutionOrder(repos []RepoConfig) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}

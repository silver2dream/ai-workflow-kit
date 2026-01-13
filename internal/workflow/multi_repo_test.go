package workflow

import (
	"testing"
)

// TestMultiRepoCoordination tests multi-repo coordination.
// Property 25: Multi-Repo Coordination

func TestSequentialProcessing(t *testing.T) {
	// Test repos are processed sequentially (Req 17.1).
	repos := []RepoConfig{
		{Name: "backend", RepoType: "directory", Path: "backend"},
		{Name: "frontend", RepoType: "directory", Path: "frontend"},
	}
	executionOrder := []string{}

	trackExecution := func(repo RepoConfig) (bool, string) {
		executionOrder = append(executionOrder, repo.Name)
		return true, ""
	}

	success, results := ProcessReposSequential(repos, trackExecution)

	if !success {
		t.Errorf("ProcessReposSequential() success = %v, want true", success)
	}

	if len(executionOrder) != 2 {
		t.Errorf("execution order length = %d, want 2", len(executionOrder))
	}

	expected := []string{"backend", "frontend"}
	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("executionOrder[%d] = %q, want %q", i, executionOrder[i], name)
		}
	}

	if len(results) != 2 {
		t.Errorf("results length = %d, want 2", len(results))
	}
}

func TestStopOnFirstFailure(t *testing.T) {
	// Test processing stops on first failure (Req 17.2).
	repos := []RepoConfig{
		{Name: "backend", RepoType: "directory", Path: "backend"},
		{Name: "frontend", RepoType: "directory", Path: "frontend"},
		{Name: "shared", RepoType: "directory", Path: "shared"},
	}

	failOnFrontend := func(repo RepoConfig) (bool, string) {
		if repo.Name == "frontend" {
			return false, "Frontend failed"
		}
		return true, ""
	}

	success, results := ProcessReposSequential(repos, failOnFrontend)

	if success {
		t.Error("ProcessReposSequential() success = true, want false")
	}

	// Only backend and frontend processed
	if len(results) != 2 {
		t.Errorf("results length = %d, want 2", len(results))
	}

	if !results[0].Success {
		t.Error("results[0].Success = false, want true")
	}

	if results[1].Success {
		t.Error("results[1].Success = true, want false")
	}
}

func TestSubmoduleReposIncluded(t *testing.T) {
	// Test submodule repos are processed (Req 17.3).
	repos := []RepoConfig{
		{Name: "backend", RepoType: "submodule", Path: "backend"},
		{Name: "frontend", RepoType: "directory", Path: "frontend"},
	}
	type repoInfo struct {
		name     string
		repoType string
	}
	processed := []repoInfo{}

	trackType := func(repo RepoConfig) (bool, string) {
		processed = append(processed, repoInfo{repo.Name, repo.RepoType})
		return true, ""
	}

	success, _ := ProcessReposSequential(repos, trackType)

	if !success {
		t.Error("ProcessReposSequential() success = false, want true")
	}

	foundSubmodule := false
	for _, info := range processed {
		if info.name == "backend" && info.repoType == "submodule" {
			foundSubmodule = true
			break
		}
	}

	if !foundSubmodule {
		t.Error("submodule repo ('backend', 'submodule') not found in processed list")
	}
}

func TestAllReposProcessedOnSuccess(t *testing.T) {
	// Test all repos are processed when all succeed (Req 17.4).
	repos := []RepoConfig{
		{Name: "backend", RepoType: "directory", Path: "backend"},
		{Name: "frontend", RepoType: "directory", Path: "frontend"},
		{Name: "shared", RepoType: "submodule", Path: "shared"},
	}

	success, results := ProcessReposSequential(repos, nil)

	if !success {
		t.Error("ProcessReposSequential() success = false, want true")
	}

	if len(results) != 3 {
		t.Errorf("results length = %d, want 3", len(results))
	}

	for i, r := range results {
		if !r.Success {
			t.Errorf("results[%d].Success = false, want true", i)
		}
	}
}

// TestExecutionOrder tests execution order.

func TestOrderPreserved(t *testing.T) {
	// Test execution order is preserved.
	repos := []RepoConfig{
		{Name: "first", RepoType: "directory", Path: "first"},
		{Name: "second", RepoType: "directory", Path: "second"},
		{Name: "third", RepoType: "directory", Path: "third"},
	}

	order := GetRepoExecutionOrder(repos)

	expected := []string{"first", "second", "third"}
	if len(order) != len(expected) {
		t.Errorf("order length = %d, want %d", len(order), len(expected))
	}

	for i, name := range expected {
		if order[i] != name {
			t.Errorf("order[%d] = %q, want %q", i, order[i], name)
		}
	}
}

func TestEmptyRepos(t *testing.T) {
	// Test empty repos list.
	repos := []RepoConfig{}

	success, results := ProcessReposSequential(repos, nil)

	if !success {
		t.Error("ProcessReposSequential() success = false, want true")
	}

	if len(results) != 0 {
		t.Errorf("results length = %d, want 0", len(results))
	}
}

// TestExecutionResult tests execution result tracking.

func TestResultIncludesRepoName(t *testing.T) {
	// Test result includes repo name.
	result := ExecutionResult{RepoName: "backend", Success: true}

	if result.RepoName != "backend" {
		t.Errorf("RepoName = %q, want %q", result.RepoName, "backend")
	}
}

func TestResultIncludesSuccess(t *testing.T) {
	// Test result includes success status.
	result := ExecutionResult{RepoName: "backend", Success: true}

	if !result.Success {
		t.Error("Success = false, want true")
	}
}

func TestResultIncludesError(t *testing.T) {
	// Test result includes error message.
	result := ExecutionResult{RepoName: "backend", Success: false, Error: "Something failed"}

	if result.Error != "Something failed" {
		t.Errorf("Error = %q, want %q", result.Error, "Something failed")
	}
}

// TestRepoConfig tests repo configuration.

func TestConfigHasName(t *testing.T) {
	// Test config has name.
	config := RepoConfig{Name: "backend", RepoType: "directory", Path: "backend"}

	if config.Name != "backend" {
		t.Errorf("Name = %q, want %q", config.Name, "backend")
	}
}

func TestConfigHasType(t *testing.T) {
	// Test config has type.
	config := RepoConfig{Name: "backend", RepoType: "submodule", Path: "backend"}

	if config.RepoType != "submodule" {
		t.Errorf("RepoType = %q, want %q", config.RepoType, "submodule")
	}
}

func TestConfigHasPath(t *testing.T) {
	// Test config has path.
	config := RepoConfig{Name: "backend", RepoType: "directory", Path: "backend/src"}

	if config.Path != "backend/src" {
		t.Errorf("Path = %q, want %q", config.Path, "backend/src")
	}
}

// Table-driven test for multi-repo processing scenarios.
func TestProcessReposSequential_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		repos          []RepoConfig
		executeFn      ExecuteFunc
		wantSuccess    bool
		wantResultsLen int
	}{
		{
			name:           "empty repos succeeds",
			repos:          []RepoConfig{},
			executeFn:      nil,
			wantSuccess:    true,
			wantResultsLen: 0,
		},
		{
			name: "all succeed",
			repos: []RepoConfig{
				{Name: "a", RepoType: "directory", Path: "a"},
				{Name: "b", RepoType: "directory", Path: "b"},
			},
			executeFn:      func(r RepoConfig) (bool, string) { return true, "" },
			wantSuccess:    true,
			wantResultsLen: 2,
		},
		{
			name: "first fails",
			repos: []RepoConfig{
				{Name: "a", RepoType: "directory", Path: "a"},
				{Name: "b", RepoType: "directory", Path: "b"},
			},
			executeFn:      func(r RepoConfig) (bool, string) { return false, "error" },
			wantSuccess:    false,
			wantResultsLen: 1,
		},
		{
			name: "second fails",
			repos: []RepoConfig{
				{Name: "a", RepoType: "directory", Path: "a"},
				{Name: "b", RepoType: "directory", Path: "b"},
				{Name: "c", RepoType: "directory", Path: "c"},
			},
			executeFn: func(r RepoConfig) (bool, string) {
				if r.Name == "b" {
					return false, "b failed"
				}
				return true, ""
			},
			wantSuccess:    false,
			wantResultsLen: 2,
		},
		{
			name: "nil executeFn succeeds for all",
			repos: []RepoConfig{
				{Name: "a", RepoType: "submodule", Path: "a"},
				{Name: "b", RepoType: "directory", Path: "b"},
			},
			executeFn:      nil,
			wantSuccess:    true,
			wantResultsLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			success, results := ProcessReposSequential(tt.repos, tt.executeFn)
			if success != tt.wantSuccess {
				t.Errorf("success = %v, want %v", success, tt.wantSuccess)
			}
			if len(results) != tt.wantResultsLen {
				t.Errorf("results length = %d, want %d", len(results), tt.wantResultsLen)
			}
		})
	}
}

package worker

import (
	"strings"
	"testing"
	"time"
)

// BuildResultOptions contains options for building a result record.
// Property 10: Result Recording Completeness
type BuildResultOptions struct {
	IssueID           string
	Status            string
	Repo              string
	RepoType          string
	Branch            string
	BaseBranch        string
	HeadSHA           string
	WorkDir           string
	PRURL             string
	SubmoduleSHA      string
	ConsistencyStatus string
	FailureStage      string
	RecoveryCommand   string
	DurationSeconds   int
	RetryCount        int
}

// BuildResult builds a result record with all required fields.
// Property 10: Result Recording Completeness
// - repo_type field (Req 11.1)
// - work_dir field (Req 11.2)
// - For submodule: submodule_sha and consistency_status (Req 11.3, 11.4)
// - On failure: failure_stage (Req 11.5, 11.6)
// - consistency_status (Req 24.3)
// - recovery_command when inconsistent (Req 24.4, 24.5)
func BuildResult(opts BuildResultOptions) *IssueResult {
	result := &IssueResult{
		IssueID:      opts.IssueID,
		Status:       opts.Status,
		Repo:         opts.Repo,
		RepoType:     opts.RepoType, // Req 11.1
		Branch:       opts.Branch,
		BaseBranch:   opts.BaseBranch,
		HeadSHA:      opts.HeadSHA,
		WorkDir:      opts.WorkDir, // Req 11.2
		TimestampUTC: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		PRURL:        opts.PRURL,
		Metrics: ResultMetrics{
			DurationSeconds: opts.DurationSeconds,
			RetryCount:      opts.RetryCount,
		},
	}

	// Submodule-specific fields (Req 11.3, 11.4)
	if opts.RepoType == "submodule" {
		result.SubmoduleSHA = opts.SubmoduleSHA
		result.ConsistencyStatus = opts.ConsistencyStatus // Req 24.3
		if result.ConsistencyStatus == "" {
			result.ConsistencyStatus = "consistent"
		}
	}

	// Failure-specific fields (Req 11.5, 11.6)
	if opts.Status == "failed" {
		result.FailureStage = opts.FailureStage
	}

	// Recovery command for inconsistent state (Req 24.4, 24.5)
	if opts.ConsistencyStatus != "" && opts.ConsistencyStatus != "consistent" && opts.RecoveryCommand != "" {
		result.RecoveryCommand = opts.RecoveryCommand
	}

	return result
}

// ValidateResult validates result record has all required fields.
// Returns (is_valid, list_of_missing_fields)
func ValidateResult(result *IssueResult) (bool, []string) {
	var missing []string

	if result.IssueID == "" {
		missing = append(missing, "issue_id")
	}
	if result.Status == "" {
		missing = append(missing, "status")
	}
	if result.Repo == "" {
		missing = append(missing, "repo")
	}
	if result.RepoType == "" {
		missing = append(missing, "repo_type")
	}
	if result.Branch == "" {
		missing = append(missing, "branch")
	}
	if result.BaseBranch == "" {
		missing = append(missing, "base_branch")
	}
	if result.HeadSHA == "" {
		missing = append(missing, "head_sha")
	}
	if result.WorkDir == "" {
		missing = append(missing, "work_dir")
	}
	if result.TimestampUTC == "" {
		missing = append(missing, "timestamp_utc")
	}

	// Check submodule-specific fields
	if result.RepoType == "submodule" {
		if result.SubmoduleSHA == "" {
			missing = append(missing, "submodule_sha")
		}
		if result.ConsistencyStatus == "" {
			missing = append(missing, "consistency_status")
		}
	}

	// Check failure-specific fields
	if result.Status == "failed" {
		if result.FailureStage == "" {
			missing = append(missing, "failure_stage")
		}
	}

	return len(missing) == 0, missing
}

// TestResultRecordingCompleteness tests result recording completeness.
// Property 10: Result Recording Completeness
func TestResultRecordingCompleteness(t *testing.T) {
	t.Run("result_includes_repo_type", func(t *testing.T) {
		// Test result.json includes repo_type field (Req 11.1)
		result := BuildResult(BuildResultOptions{
			IssueID:    "1",
			Status:     "success",
			Repo:       "backend",
			RepoType:   "directory",
			Branch:     "feat/ai-issue-1",
			BaseBranch: "develop",
			HeadSHA:    "abc123",
			WorkDir:    "/worktree/backend",
		})

		if result.RepoType == "" {
			t.Error("Expected result to include repo_type")
		}
		if result.RepoType != "directory" {
			t.Errorf("Expected repo_type 'directory', got '%s'", result.RepoType)
		}
	})

	t.Run("result_includes_work_dir", func(t *testing.T) {
		// Test result.json includes work_dir field (Req 11.2)
		result := BuildResult(BuildResultOptions{
			IssueID:    "1",
			Status:     "success",
			Repo:       "backend",
			RepoType:   "directory",
			Branch:     "feat/ai-issue-1",
			BaseBranch: "develop",
			HeadSHA:    "abc123",
			WorkDir:    "/worktree/backend",
		})

		if result.WorkDir == "" {
			t.Error("Expected result to include work_dir")
		}
		if result.WorkDir != "/worktree/backend" {
			t.Errorf("Expected work_dir '/worktree/backend', got '%s'", result.WorkDir)
		}
	})

	t.Run("result_includes_submodule_sha", func(t *testing.T) {
		// Test result.json includes submodule_sha for submodule type (Req 11.3, 11.4)
		result := BuildResult(BuildResultOptions{
			IssueID:      "1",
			Status:       "success",
			Repo:         "backend",
			RepoType:     "submodule",
			Branch:       "feat/ai-issue-1",
			BaseBranch:   "develop",
			HeadSHA:      "abc123",
			WorkDir:      "/worktree/backend",
			SubmoduleSHA: "def456",
		})

		if result.SubmoduleSHA == "" {
			t.Error("Expected result to include submodule_sha")
		}
		if result.SubmoduleSHA != "def456" {
			t.Errorf("Expected submodule_sha 'def456', got '%s'", result.SubmoduleSHA)
		}
	})

	t.Run("result_includes_failure_stage", func(t *testing.T) {
		// Test result.json includes failure_stage on failure (Req 11.5, 11.6)
		result := BuildResult(BuildResultOptions{
			IssueID:      "1",
			Status:       "failed",
			Repo:         "backend",
			RepoType:     "directory",
			Branch:       "feat/ai-issue-1",
			BaseBranch:   "develop",
			HeadSHA:      "abc123",
			WorkDir:      "/worktree/backend",
			FailureStage: "git_commit",
		})

		if result.FailureStage == "" {
			t.Error("Expected result to include failure_stage")
		}
		if result.FailureStage != "git_commit" {
			t.Errorf("Expected failure_stage 'git_commit', got '%s'", result.FailureStage)
		}
	})

	t.Run("result_includes_consistency_status", func(t *testing.T) {
		// Test result.json includes consistency_status for submodule (Req 24.3)
		result := BuildResult(BuildResultOptions{
			IssueID:           "1",
			Status:            "success",
			Repo:              "backend",
			RepoType:          "submodule",
			Branch:            "feat/ai-issue-1",
			BaseBranch:        "develop",
			HeadSHA:           "abc123",
			WorkDir:           "/worktree/backend",
			SubmoduleSHA:      "def456",
			ConsistencyStatus: "consistent",
		})

		if result.ConsistencyStatus == "" {
			t.Error("Expected result to include consistency_status")
		}
		if result.ConsistencyStatus != "consistent" {
			t.Errorf("Expected consistency_status 'consistent', got '%s'", result.ConsistencyStatus)
		}
	})

	t.Run("result_includes_recovery_command", func(t *testing.T) {
		// Test result.json includes recovery_command when inconsistent (Req 24.4, 24.5)
		result := BuildResult(BuildResultOptions{
			IssueID:           "1",
			Status:            "failed",
			Repo:              "backend",
			RepoType:          "submodule",
			Branch:            "feat/ai-issue-1",
			BaseBranch:        "develop",
			HeadSHA:           "abc123",
			WorkDir:           "/worktree/backend",
			SubmoduleSHA:      "def456",
			ConsistencyStatus: "parent_push_failed_submodule_pushed",
			FailureStage:      "git_push",
			RecoveryCommand:   "git -C backend reset --hard HEAD~1 && git push -f origin feat/ai-issue-1",
		})

		if result.RecoveryCommand == "" {
			t.Error("Expected result to include recovery_command")
		}
		if !strings.Contains(result.RecoveryCommand, "reset --hard") {
			t.Errorf("Expected recovery_command to contain 'reset --hard', got '%s'", result.RecoveryCommand)
		}
	})
}

// TestResultValidation tests result validation function.
func TestResultValidation(t *testing.T) {
	t.Run("valid_root_result", func(t *testing.T) {
		// Test valid root type result passes validation
		result := BuildResult(BuildResultOptions{
			IssueID:    "1",
			Status:     "success",
			Repo:       "root",
			RepoType:   "root",
			Branch:     "feat/ai-issue-1",
			BaseBranch: "develop",
			HeadSHA:    "abc123",
			WorkDir:    "/worktree",
		})

		isValid, missing := ValidateResult(result)
		if !isValid {
			t.Errorf("Expected valid root result, missing fields: %v", missing)
		}
		if len(missing) != 0 {
			t.Errorf("Expected no missing fields, got %v", missing)
		}
	})

	t.Run("valid_directory_result", func(t *testing.T) {
		// Test valid directory type result passes validation
		result := BuildResult(BuildResultOptions{
			IssueID:    "1",
			Status:     "success",
			Repo:       "backend",
			RepoType:   "directory",
			Branch:     "feat/ai-issue-1",
			BaseBranch: "develop",
			HeadSHA:    "abc123",
			WorkDir:    "/worktree/backend",
		})

		isValid, missing := ValidateResult(result)
		if !isValid {
			t.Errorf("Expected valid directory result, missing fields: %v", missing)
		}
		if len(missing) != 0 {
			t.Errorf("Expected no missing fields, got %v", missing)
		}
	})

	t.Run("valid_submodule_result", func(t *testing.T) {
		// Test valid submodule type result passes validation
		result := BuildResult(BuildResultOptions{
			IssueID:           "1",
			Status:            "success",
			Repo:              "backend",
			RepoType:          "submodule",
			Branch:            "feat/ai-issue-1",
			BaseBranch:        "develop",
			HeadSHA:           "abc123",
			WorkDir:           "/worktree/backend",
			SubmoduleSHA:      "def456",
			ConsistencyStatus: "consistent",
		})

		isValid, missing := ValidateResult(result)
		if !isValid {
			t.Errorf("Expected valid submodule result, missing fields: %v", missing)
		}
		if len(missing) != 0 {
			t.Errorf("Expected no missing fields, got %v", missing)
		}
	})

	t.Run("invalid_submodule_missing_sha", func(t *testing.T) {
		// Test submodule result without submodule_sha fails validation
		result := &IssueResult{
			IssueID:           "1",
			Status:            "success",
			Repo:              "backend",
			RepoType:          "submodule",
			Branch:            "feat/ai-issue-1",
			BaseBranch:        "develop",
			HeadSHA:           "abc123",
			WorkDir:           "/worktree/backend",
			TimestampUTC:      "2025-01-01T00:00:00Z",
			ConsistencyStatus: "consistent",
		}

		isValid, missing := ValidateResult(result)
		if isValid {
			t.Error("Expected invalid result for submodule without submodule_sha")
		}
		found := false
		for _, field := range missing {
			if field == "submodule_sha" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected 'submodule_sha' in missing fields, got %v", missing)
		}
	})

	t.Run("invalid_failed_missing_stage", func(t *testing.T) {
		// Test failed result without failure_stage fails validation
		result := &IssueResult{
			IssueID:      "1",
			Status:       "failed",
			Repo:         "backend",
			RepoType:     "directory",
			Branch:       "feat/ai-issue-1",
			BaseBranch:   "develop",
			HeadSHA:      "abc123",
			WorkDir:      "/worktree/backend",
			TimestampUTC: "2025-01-01T00:00:00Z",
		}

		isValid, missing := ValidateResult(result)
		if isValid {
			t.Error("Expected invalid result for failed without failure_stage")
		}
		found := false
		for _, field := range missing {
			if field == "failure_stage" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected 'failure_stage' in missing fields, got %v", missing)
		}
	})
}

// TestResultMetrics tests result metrics recording.
func TestResultMetrics(t *testing.T) {
	t.Run("result_includes_duration", func(t *testing.T) {
		// Test result includes duration_seconds in metrics
		result := BuildResult(BuildResultOptions{
			IssueID:         "1",
			Status:          "success",
			Repo:            "backend",
			RepoType:        "directory",
			Branch:          "feat/ai-issue-1",
			BaseBranch:      "develop",
			HeadSHA:         "abc123",
			WorkDir:         "/worktree/backend",
			DurationSeconds: 120,
		})

		if result.Metrics.DurationSeconds != 120 {
			t.Errorf("Expected duration_seconds 120, got %d", result.Metrics.DurationSeconds)
		}
	})

	t.Run("result_includes_retry_count", func(t *testing.T) {
		// Test result includes retry_count in metrics
		result := BuildResult(BuildResultOptions{
			IssueID:    "1",
			Status:     "success",
			Repo:       "backend",
			RepoType:   "directory",
			Branch:     "feat/ai-issue-1",
			BaseBranch: "develop",
			HeadSHA:    "abc123",
			WorkDir:    "/worktree/backend",
			RetryCount: 2,
		})

		if result.Metrics.RetryCount != 2 {
			t.Errorf("Expected retry_count 2, got %d", result.Metrics.RetryCount)
		}
	})
}

// TestResultAllRepoTypes tests result recording for all repo types.
func TestResultAllRepoTypes(t *testing.T) {
	repoTypes := []string{"root", "directory", "submodule"}

	t.Run("result_has_repo_type_field", func(t *testing.T) {
		// Test all repo types have repo_type field
		for _, repoType := range repoTypes {
			t.Run(repoType, func(t *testing.T) {
				submoduleSHA := ""
				if repoType == "submodule" {
					submoduleSHA = "def456"
				}

				result := BuildResult(BuildResultOptions{
					IssueID:      "1",
					Status:       "success",
					Repo:         "test",
					RepoType:     repoType,
					Branch:       "feat/ai-issue-1",
					BaseBranch:   "develop",
					HeadSHA:      "abc123",
					WorkDir:      "/worktree",
					SubmoduleSHA: submoduleSHA,
				})

				if result.RepoType != repoType {
					t.Errorf("Expected repo_type '%s', got '%s'", repoType, result.RepoType)
				}
			})
		}
	})

	t.Run("result_has_work_dir_field", func(t *testing.T) {
		// Test all repo types have work_dir field
		for _, repoType := range repoTypes {
			t.Run(repoType, func(t *testing.T) {
				submoduleSHA := ""
				if repoType == "submodule" {
					submoduleSHA = "def456"
				}

				result := BuildResult(BuildResultOptions{
					IssueID:      "1",
					Status:       "success",
					Repo:         "test",
					RepoType:     repoType,
					Branch:       "feat/ai-issue-1",
					BaseBranch:   "develop",
					HeadSHA:      "abc123",
					WorkDir:      "/worktree/test",
					SubmoduleSHA: submoduleSHA,
				})

				if result.WorkDir == "" {
					t.Errorf("Expected work_dir to be set for repo type '%s'", repoType)
				}
			})
		}
	})
}

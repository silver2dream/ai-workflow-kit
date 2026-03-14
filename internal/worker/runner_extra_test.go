package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// isValidGitRef
// ---------------------------------------------------------------------------

func TestIsValidGitRef_ValidRefs(t *testing.T) {
	valid := []string{
		"main",
		"develop",
		"feat/my-feature",
		"fix/bug-123",
		"v1.0.0",
		"release-1.2.3",
		"feat/ai-issue-42",
	}
	for _, ref := range valid {
		t.Run(ref, func(t *testing.T) {
			if !isValidGitRef(ref) {
				t.Errorf("isValidGitRef(%q) = false, want true", ref)
			}
		})
	}
}

func TestIsValidGitRef_InvalidRefs(t *testing.T) {
	invalid := []string{
		"",
		"feat branch",      // space
		"ref~1",            // tilde
		"HEAD^1",           // caret
		"ref:other",        // colon
		"ref?wildcard",     // question mark
		"ref*glob",         // asterisk
		"ref[bracket",      // bracket
		"ref\\backslash",   // backslash
		".hidden",          // starts with dot
		"ends-with.",       // ends with dot
		"/starts-slash",    // starts with slash
		"ends-slash/",      // ends with slash
		"two..dots",        // consecutive dots
		"double//slash",    // double slash
		"ref.lock",         // ends with .lock
		"ref@{reflog}",     // @{ sequence
	}
	for _, ref := range invalid {
		t.Run(fmt.Sprintf("%q", ref), func(t *testing.T) {
			if isValidGitRef(ref) {
				t.Errorf("isValidGitRef(%q) = true, want false", ref)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isValidRepoPath
// ---------------------------------------------------------------------------

func TestIsValidRepoPath_Valid(t *testing.T) {
	valid := []string{
		"backend",
		"frontend",
		"my-service",
		"subdir/nested",
		"some-repo",
	}
	for _, p := range valid {
		t.Run(p, func(t *testing.T) {
			if !isValidRepoPath(p) {
				t.Errorf("isValidRepoPath(%q) = false, want true", p)
			}
		})
	}
}

func TestIsValidRepoPath_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"../escape",
		"a/../../b",
		"with..dots",
	}
	for _, p := range invalid {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			if isValidRepoPath(p) {
				t.Errorf("isValidRepoPath(%q) = true, want false", p)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildWorkerStartExtra
// ---------------------------------------------------------------------------

func TestBuildWorkerStartExtra_FirstAttempt(t *testing.T) {
	result := buildWorkerStartExtra(1)
	if result != "" {
		t.Errorf("buildWorkerStartExtra(1) = %q, want empty string", result)
	}
}

func TestBuildWorkerStartExtra_Retry(t *testing.T) {
	result := buildWorkerStartExtra(2)
	if !strings.Contains(result, "2") {
		t.Errorf("buildWorkerStartExtra(2) should mention attempt number, got %q", result)
	}
}

func TestBuildWorkerStartExtra_HighRetry(t *testing.T) {
	result := buildWorkerStartExtra(5)
	if !strings.Contains(result, "5") {
		t.Errorf("buildWorkerStartExtra(5) should mention attempt 5, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// buildWorkerCompleteExtra
// ---------------------------------------------------------------------------

func TestBuildWorkerCompleteExtra_AllFields(t *testing.T) {
	result := buildWorkerCompleteExtra("https://github.com/o/r/pull/1", 90*time.Second, "success", 0)
	if !strings.Contains(result, "success") {
		t.Errorf("expected 'success' in output, got %q", result)
	}
	if !strings.Contains(result, "https://github.com/o/r/pull/1") {
		t.Errorf("expected PR URL in output, got %q", result)
	}
	if !strings.Contains(result, "1m 30s") {
		t.Errorf("expected formatted duration in output, got %q", result)
	}
}

func TestBuildWorkerCompleteExtra_NoExitCodeWhenZero(t *testing.T) {
	result := buildWorkerCompleteExtra("", 0, "success", 0)
	// Exit code 0 should not appear
	if strings.Contains(result, "Exit Code") {
		t.Errorf("exit code 0 should not appear in output, got %q", result)
	}
}

func TestBuildWorkerCompleteExtra_NonZeroExitCode(t *testing.T) {
	result := buildWorkerCompleteExtra("", 0, "failed", 1)
	if !strings.Contains(result, "1") {
		t.Errorf("non-zero exit code should appear in output, got %q", result)
	}
}

func TestBuildWorkerCompleteExtra_Empty(t *testing.T) {
	result := buildWorkerCompleteExtra("", 0, "", 0)
	if result != "" {
		t.Errorf("all-empty buildWorkerCompleteExtra should return empty, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// resolveSpecName / resolveTaskLine
// ---------------------------------------------------------------------------

func TestResolveSpecName_FromEnv(t *testing.T) {
	t.Setenv("AI_SPEC_NAME", "my-spec")
	meta := &TicketMetadata{SpecName: "other-spec"}
	result := resolveSpecName(meta)
	if result != "my-spec" {
		t.Errorf("resolveSpecName from env = %q, want %q", result, "my-spec")
	}
}

func TestResolveSpecName_FromMeta(t *testing.T) {
	t.Setenv("AI_SPEC_NAME", "")
	meta := &TicketMetadata{SpecName: "meta-spec"}
	result := resolveSpecName(meta)
	if result != "meta-spec" {
		t.Errorf("resolveSpecName from meta = %q, want %q", result, "meta-spec")
	}
}

func TestResolveTaskLine_FromEnv(t *testing.T) {
	t.Setenv("AI_TASK_LINE", "42")
	meta := &TicketMetadata{TaskLine: 10}
	result := resolveTaskLine(meta)
	if result != "42" {
		t.Errorf("resolveTaskLine from env = %q, want %q", result, "42")
	}
}

func TestResolveTaskLine_FromMeta(t *testing.T) {
	t.Setenv("AI_TASK_LINE", "")
	meta := &TicketMetadata{TaskLine: 7}
	result := resolveTaskLine(meta)
	if result != "7" {
		t.Errorf("resolveTaskLine from meta = %q, want %q", result, "7")
	}
}

func TestResolveTaskLine_Empty(t *testing.T) {
	t.Setenv("AI_TASK_LINE", "")
	meta := &TicketMetadata{TaskLine: 0}
	result := resolveTaskLine(meta)
	if result != "" {
		t.Errorf("resolveTaskLine with no data = %q, want empty", result)
	}
}

// ---------------------------------------------------------------------------
// resolveTimeout
// ---------------------------------------------------------------------------

func TestResolveTimeout_FromEnv(t *testing.T) {
	t.Setenv("TEST_TIMEOUT_VAR", "120")
	result := resolveTimeout("TEST_TIMEOUT_VAR", 30*time.Second)
	if result != 120*time.Second {
		t.Errorf("resolveTimeout from env = %v, want 120s", result)
	}
}

func TestResolveTimeout_Fallback(t *testing.T) {
	t.Setenv("TEST_TIMEOUT_VAR2", "")
	result := resolveTimeout("TEST_TIMEOUT_VAR2", 45*time.Second)
	if result != 45*time.Second {
		t.Errorf("resolveTimeout fallback = %v, want 45s", result)
	}
}

func TestResolveTimeout_InvalidEnv(t *testing.T) {
	t.Setenv("TEST_TIMEOUT_VAR3", "not-a-number")
	result := resolveTimeout("TEST_TIMEOUT_VAR3", 10*time.Second)
	if result != 10*time.Second {
		t.Errorf("resolveTimeout with invalid env = %v, want 10s", result)
	}
}

func TestResolveTimeout_ZeroEnv(t *testing.T) {
	t.Setenv("TEST_TIMEOUT_VAR4", "0")
	result := resolveTimeout("TEST_TIMEOUT_VAR4", 5*time.Second)
	// 0 is not > 0, so should fall back
	if result != 5*time.Second {
		t.Errorf("resolveTimeout with zero env = %v, want 5s", result)
	}
}

// ---------------------------------------------------------------------------
// resolveRepoConfig
// ---------------------------------------------------------------------------

func TestResolveRepoConfig_NilConfig(t *testing.T) {
	repoType, repoPath := resolveRepoConfig(nil, "backend")
	if repoType != "directory" {
		t.Errorf("repoType = %q, want %q", repoType, "directory")
	}
	if repoPath != "./" {
		t.Errorf("repoPath = %q, want %q", repoPath, "./")
	}
}

func TestResolveRepoConfig_FoundRepo(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "backend", Type: "submodule", Path: "services/backend"},
		},
	}
	repoType, repoPath := resolveRepoConfig(cfg, "backend")
	if repoType != "submodule" {
		t.Errorf("repoType = %q, want %q", repoType, "submodule")
	}
	if repoPath != "services/backend" {
		t.Errorf("repoPath = %q, want %q", repoPath, "services/backend")
	}
}

func TestResolveRepoConfig_NotFound(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "frontend", Type: "directory", Path: "web"},
		},
	}
	repoType, repoPath := resolveRepoConfig(cfg, "backend")
	if repoType != "directory" {
		t.Errorf("repoType = %q, want %q", repoType, "directory")
	}
	if repoPath != "./" {
		t.Errorf("repoPath = %q, want %q", repoPath, "./")
	}
}

func TestResolveRepoConfig_DefaultsWhenEmpty(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "api"}, // no Type or Path
		},
	}
	repoType, repoPath := resolveRepoConfig(cfg, "api")
	if repoType != "directory" {
		t.Errorf("default repoType = %q, want %q", repoType, "directory")
	}
	if repoPath != "./" {
		t.Errorf("default repoPath = %q, want %q", repoPath, "./")
	}
}

// ---------------------------------------------------------------------------
// loadWorkflowConfig
// ---------------------------------------------------------------------------

func TestLoadWorkflowConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `repos:
  - name: backend
    type: directory
    path: backend
    language: go
git:
  integration_branch: develop
  release_branch: main
`
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadWorkflowConfig(configPath)
	if err != nil {
		t.Fatalf("loadWorkflowConfig: %v", err)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Name != "backend" {
		t.Errorf("repos = %v, want backend", cfg.Repos)
	}
	if cfg.Git.IntegrationBranch != "develop" {
		t.Errorf("integration_branch = %q, want %q", cfg.Git.IntegrationBranch, "develop")
	}
}

func TestLoadWorkflowConfig_MissingFile(t *testing.T) {
	_, err := loadWorkflowConfig("/nonexistent/path/workflow.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadWorkflowConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte("not: valid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := loadWorkflowConfig(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// ---------------------------------------------------------------------------
// cleanupIndexLocks
// ---------------------------------------------------------------------------

func TestCleanupIndexLocks_RemovesLock(t *testing.T) {
	dir := t.TempDir()
	// Create a fake .git dir with an index.lock
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(lockPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cleanupIndexLocks(dir, nil)

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("index.lock should have been removed")
	}
}

func TestCleanupIndexLocks_NoLock_NoPanic(t *testing.T) {
	dir := t.TempDir()
	// Should not panic even if .git doesn't exist
	cleanupIndexLocks(dir, nil)
}

// ---------------------------------------------------------------------------
// resetFailCount (runner-local version)
// ---------------------------------------------------------------------------

func TestRunnerResetFailCount_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	failFile := filepath.Join(dir, "fail_count.txt")
	if err := os.WriteFile(failFile, []byte("3"), 0644); err != nil {
		t.Fatal(err)
	}

	resetFailCount(dir, nil)

	if _, err := os.Stat(failFile); !os.IsNotExist(err) {
		t.Error("fail_count.txt should have been removed")
	}
}

func TestRunnerResetFailCount_NonExistentFile_NoPanic(t *testing.T) {
	dir := t.TempDir()
	// Should not panic if file doesn't exist
	resetFailCount(dir, nil)
}

// ---------------------------------------------------------------------------
// loadAttemptInfo
// ---------------------------------------------------------------------------

func TestLoadAttemptInfo_NoResult_ReturnsAttempt1(t *testing.T) {
	dir := t.TempDir()
	info := loadAttemptInfo(dir, 1)
	if info.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", info.AttemptNumber)
	}
}

func TestLoadAttemptInfo_WithFailCount(t *testing.T) {
	dir := t.TempDir()
	// Write fail count of 2
	runsDir := filepath.Join(dir, ".ai", "runs", "issue-5")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "fail_count.txt"), []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	info := loadAttemptInfo(dir, 5)
	if info.AttemptNumber != 3 { // 2 fails → attempt 3
		t.Errorf("AttemptNumber = %d, want 3", info.AttemptNumber)
	}
}

// ---------------------------------------------------------------------------
// AttemptGuard
// ---------------------------------------------------------------------------

func TestAttemptGuard_FirstAttempt_CanProceed(t *testing.T) {
	dir := t.TempDir()
	guard := &AttemptGuard{StateRoot: dir, IssueID: 1, MaxAttempts: 3}

	result, err := guard.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.CanProceed {
		t.Error("first attempt should be allowed")
	}
	if result.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want 1", result.AttemptNumber)
	}
}

func TestAttemptGuard_MaxAttempts_BlocksProceed(t *testing.T) {
	dir := t.TempDir()
	guard := &AttemptGuard{StateRoot: dir, IssueID: 2, MaxAttempts: 2}

	// Fill up fail count
	runsDir := filepath.Join(dir, ".ai", "runs", "issue-2")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "fail_count.txt"), []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := guard.Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.CanProceed {
		t.Error("should be blocked when at max attempts")
	}
	if result.Reason != "max_attempts" {
		t.Errorf("Reason = %q, want %q", result.Reason, "max_attempts")
	}
}

func TestAttemptGuard_Reset_SetsZero(t *testing.T) {
	dir := t.TempDir()
	guard := &AttemptGuard{StateRoot: dir, IssueID: 3, MaxAttempts: 3}

	// Run a check to create the file
	if _, err := guard.Check(); err != nil {
		t.Fatal(err)
	}

	// Reset
	if err := guard.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// After reset, the file should contain 0
	countFile := filepath.Join(dir, ".ai", "runs", "issue-3", "fail_count.txt")
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.TrimSpace(string(data)) != "0" {
		t.Errorf("fail_count.txt = %q, want %q", string(data), "0")
	}
}

func TestAttemptGuard_RecordFailure_CreatesEntry(t *testing.T) {
	dir := t.TempDir()
	// Create state dir
	if err := os.MkdirAll(filepath.Join(dir, ".ai", "state"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ai", "runs", "issue-4"), 0755); err != nil {
		t.Fatal(err)
	}

	guard := &AttemptGuard{StateRoot: dir, IssueID: 4, MaxAttempts: 3}
	if err := guard.RecordFailure("test_error", true); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	historyPath := filepath.Join(dir, ".ai", "state", "failure_history.jsonl")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "test_error") {
		t.Errorf("failure_history.jsonl should contain error type, got %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// TraceRecorder
// ---------------------------------------------------------------------------

func TestTraceRecorder_BasicLifecycle(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	rec, err := NewTraceRecorder(dir, 1, "backend", "feat/test", "main", 12345, now)
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	rec.StepStart("setup")
	if err := rec.StepEnd("success", "", nil); err != nil {
		t.Fatalf("StepEnd: %v", err)
	}

	if err := rec.Finalize(nil); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Load the trace file
	trace, err := LoadTrace(dir, 1)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if trace.Status != "success" {
		t.Errorf("trace.Status = %q, want %q", trace.Status, "success")
	}
	if len(trace.Steps) != 1 {
		t.Errorf("len(steps) = %d, want 1", len(trace.Steps))
	}
	if trace.Steps[0].Name != "setup" {
		t.Errorf("step name = %q, want %q", trace.Steps[0].Name, "setup")
	}
}

func TestTraceRecorder_FinalizeWithError(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewTraceRecorder(dir, 2, "frontend", "feat/ui", "develop", 999, time.Now())
	if err != nil {
		t.Fatalf("NewTraceRecorder: %v", err)
	}

	if err := rec.Finalize(fmt.Errorf("something went wrong")); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	trace, err := LoadTrace(dir, 2)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if trace.Status != "failed" {
		t.Errorf("trace.Status = %q, want %q", trace.Status, "failed")
	}
	if !strings.Contains(trace.Error, "something went wrong") {
		t.Errorf("trace.Error = %q, should contain error message", trace.Error)
	}
}

func TestTraceRecorder_StepEndWithoutStart_NoError(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewTraceRecorder(dir, 3, "root", "main", "main", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// StepEnd without StepStart should be a no-op, not panic
	if err := rec.StepEnd("success", "", nil); err != nil {
		t.Errorf("StepEnd without StepStart should return nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DispatchOutput.FormatBashOutput
// ---------------------------------------------------------------------------

func TestDispatchOutput_FormatBashOutput_Success(t *testing.T) {
	o := &DispatchOutput{
		Status: "success",
		PRURL:  "https://github.com/o/r/pull/1",
	}
	out := o.FormatBashOutput()
	if !strings.Contains(out, "WORKER_STATUS=success") {
		t.Errorf("expected WORKER_STATUS=success, got %q", out)
	}
	if !strings.Contains(out, "PR_URL=") {
		t.Errorf("expected PR_URL, got %q", out)
	}
}

func TestDispatchOutput_FormatBashOutput_Failed(t *testing.T) {
	o := &DispatchOutput{
		Status: "failed",
		Error:  "something broke",
	}
	out := o.FormatBashOutput()
	if !strings.Contains(out, "WORKER_STATUS=failed") {
		t.Errorf("expected WORKER_STATUS=failed, got %q", out)
	}
	if !strings.Contains(out, "WORKER_ERROR=") {
		t.Errorf("expected WORKER_ERROR, got %q", out)
	}
}

func TestDispatchOutput_FormatBashOutput_NeedsConflictResolution(t *testing.T) {
	o := &DispatchOutput{
		Status:       "needs_conflict_resolution",
		WorktreePath: "/tmp/worktree",
		IssueNumber:  42,
		PRNumber:     7,
	}
	out := o.FormatBashOutput()
	if !strings.Contains(out, "WORKTREE_PATH=") {
		t.Errorf("expected WORKTREE_PATH, got %q", out)
	}
	if !strings.Contains(out, "ISSUE_NUMBER=42") {
		t.Errorf("expected ISSUE_NUMBER=42, got %q", out)
	}
	if !strings.Contains(out, "PR_NUMBER=7") {
		t.Errorf("expected PR_NUMBER=7, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// DispatchLogger
// ---------------------------------------------------------------------------

func TestDispatchLogger_LogAndClose(t *testing.T) {
	dir := t.TempDir()
	logger := NewDispatchLogger(dir, 99)

	logger.Log("test message %d", 42)
	if err := logger.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	logPath := filepath.Join(dir, ".ai", "exe-logs", "principal.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "test message 42") {
		t.Errorf("log file should contain the message, got %q", string(data))
	}
}

func TestDispatchLogger_NilFile_DoesNotPanic(t *testing.T) {
	logger := &DispatchLogger{} // nil file
	logger.Log("this should not panic")
	if err := logger.Close(); err != nil {
		t.Errorf("Close on nil file should not error: %v", err)
	}
}

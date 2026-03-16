package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// ===========================================================================
// ternary (status.go)
// ===========================================================================

func TestTernary_True(t *testing.T) {
	if ternary(true, "yes", "no") != "yes" {
		t.Error("ternary(true) should return first value")
	}
}

func TestTernary_False(t *testing.T) {
	if ternary(false, "yes", "no") != "no" {
		t.Error("ternary(false) should return second value")
	}
}

// ===========================================================================
// cmdHelp (main.go)
// ===========================================================================

func TestCmdHelp_KnownCommands(t *testing.T) {
	commands := []string{
		"init", "install", "upgrade", "uninstall", "kickoff",
		"validate", "status", "next",
		"check-result", "dispatch-worker", "run-issue",
		"session", "analyze-next", "stop-workflow",
		"prepare-review", "submit-review",
		"create-task", "create-epic", "audit-epic",
		"doctor", "reset", "generate", "list-presets",
		"check-update", "completion", "hooks",
		"events", "feedback-stats", "context-snapshot",
		"version",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			exitCode := cmdHelp(cmd)
			if exitCode != 0 {
				t.Errorf("cmdHelp(%q) = %d, want 0", cmd, exitCode)
			}
		})
	}
}

func TestCmdHelp_UnknownCommand(t *testing.T) {
	exitCode := cmdHelp("nonexistent-command")
	if exitCode != 2 {
		t.Errorf("cmdHelp(unknown) = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdCompletion (main.go)
// ===========================================================================

func TestCmdCompletion_Bash(t *testing.T) {
	exitCode := cmdCompletion([]string{"bash"})
	if exitCode != 0 {
		t.Errorf("cmdCompletion(bash) = %d, want 0", exitCode)
	}
}

func TestCmdCompletion_Zsh(t *testing.T) {
	exitCode := cmdCompletion([]string{"zsh"})
	if exitCode != 0 {
		t.Errorf("cmdCompletion(zsh) = %d, want 0", exitCode)
	}
}

func TestCmdCompletion_Fish(t *testing.T) {
	exitCode := cmdCompletion([]string{"fish"})
	if exitCode != 0 {
		t.Errorf("cmdCompletion(fish) = %d, want 0", exitCode)
	}
}

func TestCmdCompletion_UnsupportedShell(t *testing.T) {
	exitCode := cmdCompletion([]string{"powershell"})
	if exitCode != 2 {
		t.Errorf("cmdCompletion(powershell) = %d, want 2", exitCode)
	}
}

func TestCmdCompletion_NoArgs(t *testing.T) {
	exitCode := cmdCompletion([]string{})
	if exitCode != 2 {
		t.Errorf("cmdCompletion() = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdListPresets (main.go)
// ===========================================================================

func TestCmdListPresets(t *testing.T) {
	exitCode := cmdListPresets()
	if exitCode != 0 {
		t.Errorf("cmdListPresets() = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdHooks subcommand routing (hooks.go)
// ===========================================================================

func TestCmdHooks_NoArgs(t *testing.T) {
	exitCode := cmdHooks([]string{})
	if exitCode != 2 {
		t.Errorf("cmdHooks() = %d, want 2", exitCode)
	}
}

func TestCmdHooks_Help(t *testing.T) {
	exitCode := cmdHooks([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdHooks(--help) = %d, want 0", exitCode)
	}
}

func TestCmdHooks_HelpShort(t *testing.T) {
	exitCode := cmdHooks([]string{"-h"})
	if exitCode != 0 {
		t.Errorf("cmdHooks(-h) = %d, want 0", exitCode)
	}
}

func TestCmdHooks_UnknownSubcommand(t *testing.T) {
	exitCode := cmdHooks([]string{"unknown"})
	if exitCode != 2 {
		t.Errorf("cmdHooks(unknown) = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdCheckResult flag validation (check_result.go)
// ===========================================================================

func TestCmdCheckResult_Help(t *testing.T) {
	exitCode := cmdCheckResult([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdCheckResult(--help) = %d, want 0", exitCode)
	}
}

func TestCmdCheckResult_HelpShort(t *testing.T) {
	exitCode := cmdCheckResult([]string{"-h"})
	if exitCode != 0 {
		t.Errorf("cmdCheckResult(-h) = %d, want 0", exitCode)
	}
}

func TestCmdCheckResult_MissingIssue(t *testing.T) {
	exitCode := cmdCheckResult([]string{})
	if exitCode != 2 {
		t.Errorf("cmdCheckResult() = %d, want 2 (missing --issue)", exitCode)
	}
}

// ===========================================================================
// cmdDispatchWorker flag validation (dispatch_worker.go)
// ===========================================================================

func TestCmdDispatchWorker_Help(t *testing.T) {
	exitCode := cmdDispatchWorker([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdDispatchWorker(--help) = %d, want 0", exitCode)
	}
}

func TestCmdDispatchWorker_MissingIssue(t *testing.T) {
	exitCode := cmdDispatchWorker([]string{})
	if exitCode != 2 {
		t.Errorf("cmdDispatchWorker() = %d, want 2 (missing --issue)", exitCode)
	}
}

// ===========================================================================
// cmdCreateEpic flag validation (create_epic.go)
// ===========================================================================

func TestCmdCreateEpic_Help(t *testing.T) {
	exitCode := cmdCreateEpic([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdCreateEpic(--help) = %d, want 0", exitCode)
	}
}

func TestCmdCreateEpic_MissingSpec(t *testing.T) {
	exitCode := cmdCreateEpic([]string{})
	if exitCode != 2 {
		t.Errorf("cmdCreateEpic() = %d, want 2 (missing --spec)", exitCode)
	}
}

func TestCmdCreateEpic_MissingBodyFile(t *testing.T) {
	exitCode := cmdCreateEpic([]string{"--spec", "test"})
	if exitCode != 2 {
		t.Errorf("cmdCreateEpic(--spec test) = %d, want 2 (missing --body-file)", exitCode)
	}
}

// ===========================================================================
// cmdCreateTask flag validation (create_task.go)
// ===========================================================================

func TestCmdCreateTask_Help(t *testing.T) {
	exitCode := cmdCreateTask([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdCreateTask(--help) = %d, want 0", exitCode)
	}
}

func TestCmdCreateTask_MissingSpec(t *testing.T) {
	exitCode := cmdCreateTask([]string{})
	if exitCode != 2 {
		t.Errorf("cmdCreateTask() = %d, want 2 (missing --spec)", exitCode)
	}
}

func TestCmdCreateTask_MissingTaskLine(t *testing.T) {
	exitCode := cmdCreateTask([]string{"--spec", "test"})
	if exitCode != 2 {
		t.Errorf("cmdCreateTask(--spec only) = %d, want 2 (missing --task-line)", exitCode)
	}
}

func TestCmdCreateTask_MissingBodyFile(t *testing.T) {
	exitCode := cmdCreateTask([]string{"--spec", "test", "--task-line", "1"})
	if exitCode != 2 {
		t.Errorf("cmdCreateTask(--spec --task-line) = %d, want 2 (missing --body-file)", exitCode)
	}
}

func TestCmdCreateTask_InvalidTaskLine(t *testing.T) {
	exitCode := cmdCreateTask([]string{"--spec", "test", "--task-line", "0", "--body-file", "body.md"})
	if exitCode != 2 {
		t.Errorf("cmdCreateTask(task-line=0) = %d, want 2 (task-line must be positive)", exitCode)
	}
}

// ===========================================================================
// cmdAnalyzeNext flag validation (analyze_next.go)
// ===========================================================================

func TestCmdAnalyzeNext_Help(t *testing.T) {
	exitCode := cmdAnalyzeNext([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdAnalyzeNext(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdAuditEpic flag validation (audit_epic.go)
// ===========================================================================

func TestCmdAuditEpic_Help(t *testing.T) {
	exitCode := cmdAuditEpic([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdAuditEpic(--help) = %d, want 0", exitCode)
	}
}

func TestCmdAuditEpic_MissingSpec(t *testing.T) {
	exitCode := cmdAuditEpic([]string{})
	if exitCode != 2 {
		t.Errorf("cmdAuditEpic() = %d, want 2 (missing --spec)", exitCode)
	}
}

// ===========================================================================
// cmdStopWorkflow flag validation (stop_workflow.go)
// ===========================================================================

func TestCmdStopWorkflow_Help(t *testing.T) {
	exitCode := cmdStopWorkflow([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdStopWorkflow(--help) = %d, want 0", exitCode)
	}
}

func TestCmdStopWorkflow_MissingReason(t *testing.T) {
	exitCode := cmdStopWorkflow([]string{})
	if exitCode != 2 {
		t.Errorf("cmdStopWorkflow() = %d, want 2 (missing reason)", exitCode)
	}
}

func TestCmdStopWorkflow_InvalidReason(t *testing.T) {
	exitCode := cmdStopWorkflow([]string{"invalid_reason_xyz"})
	if exitCode != 2 {
		t.Errorf("cmdStopWorkflow(invalid) = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdRunIssue flag validation (run_issue.go)
// ===========================================================================

func TestCmdRunIssue_Help(t *testing.T) {
	exitCode := cmdRunIssue([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdRunIssue(--help) = %d, want 0", exitCode)
	}
}

func TestCmdRunIssue_MissingArgs(t *testing.T) {
	exitCode := cmdRunIssue([]string{})
	if exitCode != 2 {
		t.Errorf("cmdRunIssue() = %d, want 2 (missing --issue and --ticket)", exitCode)
	}
}

func TestCmdRunIssue_MissingTicket(t *testing.T) {
	exitCode := cmdRunIssue([]string{"--issue", "25"})
	if exitCode != 2 {
		t.Errorf("cmdRunIssue(--issue only) = %d, want 2 (missing --ticket)", exitCode)
	}
}

// ===========================================================================
// cmdPrepareReview flag validation (prepare_review.go)
// ===========================================================================

func TestCmdPrepareReview_Help(t *testing.T) {
	exitCode := cmdPrepareReview([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdPrepareReview(--help) = %d, want 0", exitCode)
	}
}

func TestCmdPrepareReview_MissingPR(t *testing.T) {
	exitCode := cmdPrepareReview([]string{})
	if exitCode != 2 {
		t.Errorf("cmdPrepareReview() = %d, want 2 (missing --pr)", exitCode)
	}
}

func TestCmdPrepareReview_MissingIssue(t *testing.T) {
	exitCode := cmdPrepareReview([]string{"--pr", "42"})
	if exitCode != 2 {
		t.Errorf("cmdPrepareReview(--pr only) = %d, want 2 (missing --issue)", exitCode)
	}
}

// ===========================================================================
// cmdSubmitReview flag validation (submit_review.go)
// ===========================================================================

func TestCmdSubmitReview_Help(t *testing.T) {
	exitCode := cmdSubmitReview([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdSubmitReview(--help) = %d, want 0", exitCode)
	}
}

func TestCmdSubmitReview_MissingPR(t *testing.T) {
	exitCode := cmdSubmitReview([]string{})
	if exitCode != 2 {
		t.Errorf("cmdSubmitReview() = %d, want 2 (missing --pr)", exitCode)
	}
}

func TestCmdSubmitReview_MissingIssue(t *testing.T) {
	exitCode := cmdSubmitReview([]string{"--pr", "42"})
	if exitCode != 2 {
		t.Errorf("cmdSubmitReview(--pr only) = %d, want 2 (missing --issue)", exitCode)
	}
}

func TestCmdSubmitReview_InvalidScore(t *testing.T) {
	exitCode := cmdSubmitReview([]string{"--pr", "42", "--issue", "25", "--score", "0"})
	if exitCode != 2 {
		t.Errorf("cmdSubmitReview(score=0) = %d, want 2 (score out of range)", exitCode)
	}
}

func TestCmdSubmitReview_ScoreTooHigh(t *testing.T) {
	exitCode := cmdSubmitReview([]string{"--pr", "42", "--issue", "25", "--score", "11"})
	if exitCode != 2 {
		t.Errorf("cmdSubmitReview(score=11) = %d, want 2 (score out of range)", exitCode)
	}
}

func TestCmdSubmitReview_InvalidCIStatus(t *testing.T) {
	exitCode := cmdSubmitReview([]string{
		"--pr", "42", "--issue", "25", "--score", "8",
		"--ci-status", "invalid",
	})
	if exitCode != 2 {
		t.Errorf("cmdSubmitReview(ci-status=invalid) = %d, want 2", exitCode)
	}
}

func TestCmdSubmitReview_MissingBody(t *testing.T) {
	exitCode := cmdSubmitReview([]string{
		"--pr", "42", "--issue", "25", "--score", "8",
		"--ci-status", "passed",
	})
	if exitCode != 2 {
		t.Errorf("cmdSubmitReview(missing body) = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdGenerate flag validation (generate.go)
// ===========================================================================

func TestCmdGenerate_Help(t *testing.T) {
	exitCode := cmdGenerate([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdGenerate(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdValidate flag validation (validate.go)
// ===========================================================================

func TestCmdValidate_Help(t *testing.T) {
	exitCode := cmdValidate([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdValidate(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdStatus flag validation (status.go)
// ===========================================================================

func TestCmdStatus_Help(t *testing.T) {
	exitCode := cmdStatus([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdStatus(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdNext flag validation (next.go)
// ===========================================================================

func TestCmdNext_Help(t *testing.T) {
	exitCode := cmdNext([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdNext(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdEvents flag validation (events.go)
// ===========================================================================

func TestCmdEvents_Help(t *testing.T) {
	exitCode := cmdEvents([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdEvents(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdKickoff flag validation (kickoff.go)
// ===========================================================================

func TestCmdKickoff_Help(t *testing.T) {
	exitCode := cmdKickoff([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdKickoff(--help) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdSession subcommand routing (session.go)
// ===========================================================================

func TestCmdSession_NoArgs(t *testing.T) {
	exitCode := cmdSession([]string{})
	if exitCode != 2 {
		t.Errorf("cmdSession() = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdInit flag validation (main.go)
// ===========================================================================

func TestCmdInit_Help(t *testing.T) {
	exitCode := cmdInit([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdInit(--help) = %d, want 0", exitCode)
	}
}

func TestCmdInit_InvalidPreset(t *testing.T) {
	tmpDir := t.TempDir()
	exitCode := cmdInit([]string{"--preset", "nonexistent-preset", tmpDir})
	if exitCode != 2 {
		t.Errorf("cmdInit(invalid preset) = %d, want 2", exitCode)
	}
}

// ===========================================================================
// cmdUpgrade flag validation (main.go)
// ===========================================================================

func TestCmdUpgrade_Help(t *testing.T) {
	exitCode := cmdUpgrade([]string{"--help"})
	if exitCode != 0 {
		t.Errorf("cmdUpgrade(--help) = %d, want 0", exitCode)
	}
}

func TestCmdUpgrade_ScaffoldWithoutPreset(t *testing.T) {
	exitCode := cmdUpgrade([]string{"--scaffold"})
	if exitCode != 2 {
		t.Errorf("cmdUpgrade(--scaffold without --preset) = %d, want 2", exitCode)
	}
}

func TestCmdUpgrade_ForceConfigWithoutPreset(t *testing.T) {
	exitCode := cmdUpgrade([]string{"--force-config"})
	if exitCode != 2 {
		t.Errorf("cmdUpgrade(--force-config without --preset) = %d, want 2", exitCode)
	}
}

func TestCmdUpgrade_InvalidPreset(t *testing.T) {
	exitCode := cmdUpgrade([]string{"--preset", "nonexistent"})
	if exitCode != 2 {
		t.Errorf("cmdUpgrade(invalid preset) = %d, want 2", exitCode)
	}
}

func TestCmdUpgrade_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	exitCode := cmdUpgrade([]string{tmpDir})
	if exitCode != 1 {
		t.Errorf("cmdUpgrade(not installed) = %d, want 1", exitCode)
	}
}

// ===========================================================================
// cmdDoctor (doctor.go) - runs in temp dir
// ===========================================================================

func TestCmdDoctor_InTempDir(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// doctor should complete without crashing even in empty dir
	exitCode := cmdDoctor([]string{})
	// May return 0 or 1 depending on checks; just verify it doesn't crash
	if exitCode < 0 || exitCode > 1 {
		t.Errorf("cmdDoctor() = %d, want 0 or 1", exitCode)
	}
}

// ===========================================================================
// cmdReset (doctor.go) - runs in temp dir with dry-run
// ===========================================================================

func TestCmdReset_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	// Create minimal .ai/state dir
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0o755)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	exitCode := cmdReset([]string{"--dry-run"})
	if exitCode != 0 {
		t.Errorf("cmdReset(--dry-run) = %d, want 0", exitCode)
	}
}

func TestCmdReset_DefaultInEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	exitCode := cmdReset([]string{})
	if exitCode != 0 {
		t.Errorf("cmdReset() in empty dir = %d, want 0", exitCode)
	}
}

func TestCmdReset_SpecificFlags(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".ai", "state"), 0o755)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Test individual flags that don't require GitHub
	flags := []string{
		"--state", "--attempts", "--stop", "--lock",
		"--deprecated", "--results", "--traces", "--events",
		"--temp", "--sessions", "--reports", "--orphans",
	}
	for _, f := range flags {
		t.Run(f, func(t *testing.T) {
			exitCode := cmdReset([]string{f, "--dry-run"})
			if exitCode != 0 {
				t.Errorf("cmdReset(%s --dry-run) = %d, want 0", f, exitCode)
			}
		})
	}
}

// ===========================================================================
// cmdContextSnapshot (context_snapshot.go) - safe with temp dir
// ===========================================================================

func TestCmdContextSnapshot_TempDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Create minimal .ai/state dir
	os.MkdirAll(filepath.Join(tmpDir, ".ai", "state"), 0o755)

	exitCode := cmdContextSnapshot([]string{tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdContextSnapshot(tmpDir) = %d, want 0", exitCode)
	}
}

func TestCmdContextSnapshot_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	exitCode := cmdContextSnapshot([]string{tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdContextSnapshot(empty) = %d, want 0", exitCode)
	}
}

func TestCmdContextSnapshot_WithSTOPFile(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0o755)
	os.WriteFile(filepath.Join(stateDir, "STOP"), []byte(""), 0o644)

	exitCode := cmdContextSnapshot([]string{tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdContextSnapshot(with STOP) = %d, want 0", exitCode)
	}
}

func TestCmdContextSnapshot_WithLoopCount(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0o755)
	os.WriteFile(filepath.Join(stateDir, "loop_count.txt"), []byte("5"), 0o644)

	exitCode := cmdContextSnapshot([]string{tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdContextSnapshot(with loop_count) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdFeedbackStats (feedback_stats.go) - safe with temp dir
// ===========================================================================

func TestCmdFeedbackStats_NoFeedback(t *testing.T) {
	tmpDir := t.TempDir()
	exitCode := cmdFeedbackStats([]string{tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdFeedbackStats(empty) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// usage* functions - verify they contain expected content (coverage)
// ===========================================================================

func captureStderr(fn func()) string {
	var buf bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = oldStderr
	buf.ReadFrom(r)
	return buf.String()
}

func TestUsageDoctor_ContainsContent(t *testing.T) {
	output := captureStderr(usageDoctor)
	if !strings.Contains(output, "doctor") {
		t.Error("usageDoctor should mention 'doctor'")
	}
}

func TestUsageReset_ContainsContent(t *testing.T) {
	output := captureStderr(usageReset)
	if !strings.Contains(output, "reset") {
		t.Error("usageReset should mention 'reset'")
	}
	if !strings.Contains(output, "--dry-run") {
		t.Error("usageReset should mention --dry-run")
	}
	if !strings.Contains(output, "--all") {
		t.Error("usageReset should mention --all")
	}
}

func TestUsageCheckResult_ContainsContent(t *testing.T) {
	output := captureStderr(usageCheckResult)
	if !strings.Contains(output, "--issue") {
		t.Error("usageCheckResult should mention --issue")
	}
}

func TestUsageDispatchWorker_ContainsContent(t *testing.T) {
	output := captureStderr(usageDispatchWorker)
	if !strings.Contains(output, "--issue") {
		t.Error("usageDispatchWorker should mention --issue")
	}
	if !strings.Contains(output, "--merge-issue") {
		t.Error("usageDispatchWorker should mention --merge-issue")
	}
}

func TestUsageCreateEpic_ContainsContent(t *testing.T) {
	output := captureStderr(usageCreateEpic)
	if !strings.Contains(output, "--spec") {
		t.Error("usageCreateEpic should mention --spec")
	}
	if !strings.Contains(output, "--body-file") {
		t.Error("usageCreateEpic should mention --body-file")
	}
}

func TestUsageCreateTask_ContainsContent(t *testing.T) {
	output := captureStderr(usageCreateTask)
	if !strings.Contains(output, "--spec") {
		t.Error("usageCreateTask should mention --spec")
	}
	if !strings.Contains(output, "--task-line") {
		t.Error("usageCreateTask should mention --task-line")
	}
}

func TestUsageAnalyzeNext_ContainsContent(t *testing.T) {
	output := captureStderr(usageAnalyzeNext)
	if !strings.Contains(output, "analyze") {
		t.Error("usageAnalyzeNext should mention 'analyze'")
	}
}

func TestUsageAuditEpic_ContainsContent(t *testing.T) {
	output := captureStderr(usageAuditEpic)
	if !strings.Contains(output, "--spec") {
		t.Error("usageAuditEpic should mention --spec")
	}
}

func TestUsageStopWorkflow_ContainsContent(t *testing.T) {
	output := captureStderr(usageStopWorkflow)
	if !strings.Contains(output, "stop") {
		t.Error("usageStopWorkflow should mention 'stop'")
	}
	if !strings.Contains(output, "all_tasks_complete") {
		t.Error("usageStopWorkflow should mention 'all_tasks_complete'")
	}
}

func TestUsageRunIssue_ContainsContent(t *testing.T) {
	output := captureStderr(usageRunIssue)
	if !strings.Contains(output, "--issue") {
		t.Error("usageRunIssue should mention --issue")
	}
	if !strings.Contains(output, "--ticket") {
		t.Error("usageRunIssue should mention --ticket")
	}
}

func TestUsagePrepareReview_ContainsContent(t *testing.T) {
	output := captureStderr(usagePrepareReview)
	if !strings.Contains(output, "--pr") {
		t.Error("usagePrepareReview should mention --pr")
	}
}

func TestUsageSubmitReview_ContainsContent(t *testing.T) {
	output := captureStderr(usageSubmitReview)
	if !strings.Contains(output, "--score") {
		t.Error("usageSubmitReview should mention --score")
	}
	if !strings.Contains(output, "--ci-status") {
		t.Error("usageSubmitReview should mention --ci-status")
	}
}

func TestUsageSession_ContainsContent(t *testing.T) {
	output := captureStderr(usageSession)
	if !strings.Contains(output, "session") {
		t.Error("usageSession should mention 'session'")
	}
}

func TestUsageGenerate_ContainsContent(t *testing.T) {
	output := captureStderr(usageGenerate)
	if !strings.Contains(output, "generate") {
		t.Error("usageGenerate should mention 'generate'")
	}
}

func TestUsageValidate_ContainsContent(t *testing.T) {
	output := captureStderr(usageValidate)
	if !strings.Contains(output, "validate") {
		t.Error("usageValidate should mention 'validate'")
	}
}

func TestUsageStatus_ContainsContent(t *testing.T) {
	output := captureStderr(usageStatus)
	if !strings.Contains(output, "status") {
		t.Error("usageStatus should mention 'status'")
	}
}

func TestUsageNext_ContainsContent(t *testing.T) {
	output := captureStderr(usageNext)
	if !strings.Contains(output, "next") {
		t.Error("usageNext should mention 'next'")
	}
}

func TestUsageEvents_ContainsContent(t *testing.T) {
	output := captureStderr(usageEvents)
	if !strings.Contains(output, "events") {
		t.Error("usageEvents should mention 'events'")
	}
}

func TestUsageHooks_ContainsContent(t *testing.T) {
	output := captureStderr(usageHooks)
	if !strings.Contains(output, "hooks") {
		t.Error("usageHooks should mention 'hooks'")
	}
}

func TestUsageKickoff_ContainsContent(t *testing.T) {
	output := captureStderr(usageKickoff)
	if !strings.Contains(output, "kickoff") {
		t.Error("usageKickoff should mention 'kickoff'")
	}
}

func TestUsageInit_ContainsContent(t *testing.T) {
	output := captureStderr(usageInit)
	if !strings.Contains(output, "init") {
		t.Error("usageInit should mention 'init'")
	}
	if !strings.Contains(output, "--preset") {
		t.Error("usageInit should mention --preset")
	}
}

func TestUsageUpgrade_ContainsContent(t *testing.T) {
	output := captureStderr(usageUpgrade)
	if !strings.Contains(output, "upgrade") {
		t.Error("usageUpgrade should mention 'upgrade'")
	}
}

func TestUsageUninstall_ContainsContent(t *testing.T) {
	output := captureStderr(usageUninstall)
	if !strings.Contains(output, "uninstall") {
		t.Error("usageUninstall should mention 'uninstall'")
	}
}

func TestUsageCheckUpdate_ContainsContent(t *testing.T) {
	output := captureStderr(usageCheckUpdate)
	if !strings.Contains(output, "check") {
		t.Error("usageCheckUpdate should mention 'check'")
	}
}

func TestUsageCompletion_ContainsContent(t *testing.T) {
	output := captureStderr(usageCompletion)
	if !strings.Contains(output, "completion") {
		t.Error("usageCompletion should mention 'completion'")
	}
	if !strings.Contains(output, "bash") {
		t.Error("usageCompletion should mention 'bash'")
	}
}

func TestUsageFeedbackStats_ContainsContent(t *testing.T) {
	output := captureStderr(usageFeedbackStats)
	if !strings.Contains(output, "feedback") {
		t.Error("usageFeedbackStats should mention 'feedback'")
	}
}

func TestUsageContextSnapshot_ContainsContent(t *testing.T) {
	output := captureStderr(usageContextSnapshot)
	if !strings.Contains(output, "context") {
		t.Error("usageContextSnapshot should mention 'context'")
	}
}

// ===========================================================================
// Color/output helper functions (main.go)
// ===========================================================================

func TestSuccess_DoesNotPanic(t *testing.T) {
	// Redirect stdout to avoid polluting test output
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		w.Close()
		os.Stdout = oldStdout
	}()

	success("test %s\n", "message")
}

func TestWarn_DoesNotPanic(t *testing.T) {
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		w.Close()
		os.Stderr = oldStderr
	}()

	warn("test %s\n", "warning")
}

func TestErrorf_DoesNotPanic(t *testing.T) {
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		w.Close()
		os.Stderr = oldStderr
	}()

	errorf("test %s\n", "error")
}

// ===========================================================================
// cmdValidate with bad config path (validate.go)
// ===========================================================================

func TestCmdValidate_BadConfigPath(t *testing.T) {
	exitCode := cmdValidate([]string{"--config", "/nonexistent/path/workflow.yaml"})
	if exitCode != 1 {
		t.Errorf("cmdValidate(bad path) = %d, want 1", exitCode)
	}
}

// ===========================================================================
// cmdInit dry-run (main.go) - tests dry-run path
// ===========================================================================

func TestCmdInit_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	exitCode := cmdInit([]string{"--dry-run", "--preset", "go", tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdInit(--dry-run) = %d, want 0", exitCode)
	}
}

func TestCmdInit_DryRunWithScaffold(t *testing.T) {
	tmpDir := t.TempDir()
	exitCode := cmdInit([]string{"--dry-run", "--preset", "go", "--scaffold", tmpDir})
	if exitCode != 0 {
		t.Errorf("cmdInit(--dry-run --scaffold) = %d, want 0", exitCode)
	}
}

// ===========================================================================
// cmdUninstall flag validation (main.go)
// ===========================================================================

func TestCmdUninstall_DryRun_NoAIDir(t *testing.T) {
	tmpDir := t.TempDir()
	// uninstall with dry-run on a dir without .ai should return non-zero
	exitCode := cmdUninstall([]string{"--dry-run", tmpDir})
	// Should fail because nothing to uninstall
	if exitCode != 1 {
		t.Errorf("cmdUninstall(--dry-run, no .ai) = %d, want 1", exitCode)
	}
}

// ===========================================================================
// printEvent (events.go) - verify it doesn't panic
// ===========================================================================

func TestPrintEvent_BasicEvent(t *testing.T) {
	// Just verify printEvent doesn't panic with various event types
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		w.Close()
		os.Stdout = oldStdout
	}()

	// Import trace is already used in events_test.go
	// Basic event
	printEvent(trace.Event{
		Seq:       1,
		Level:     "info",
		Component: "principal",
		Type:      "test",
	})

	// Event with issue and PR
	printEvent(trace.Event{
		Seq:       2,
		Level:     "warn",
		Component: "worker",
		Type:      "dispatch",
		IssueID:   25,
		PRNumber:  30,
	})

	// Event with error
	printEvent(trace.Event{
		Seq:       3,
		Level:     "error",
		Component: "github",
		Type:      "api_call",
		Error:     "rate limited",
	})

	// Event with decision
	printEvent(trace.Event{
		Seq:       4,
		Level:     "decision",
		Component: "principal",
		Type:      "analyze",
		Decision: &trace.Decision{
			Rule:       "test_rule",
			Conditions: map[string]any{"key": "value"},
			Result:     "CONTINUE",
		},
	})

	// Decision with error
	printEvent(trace.Event{
		Seq:       5,
		Level:     "decision",
		Component: "principal",
		Type:      "analyze",
		Error:     "some error",
		Decision: &trace.Decision{
			Rule:       "error_rule",
			Conditions: map[string]any{},
			Result:     "FAIL_FINAL",
		},
	})

	// Event with data
	printEvent(trace.Event{
		Seq:       6,
		Level:     "info",
		Component: "reviewer",
		Type:      "review",
		Data:      map[string]any{"score": 8, "result": "approved"},
	})
}

// ===========================================================================
// Table-driven: cmd*() help flag returns 0
// ===========================================================================

func TestAllCommands_HelpFlag(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]string) int
	}{
		{"check-result", cmdCheckResult},
		{"dispatch-worker", cmdDispatchWorker},
		{"create-epic", cmdCreateEpic},
		{"create-task", cmdCreateTask},
		{"analyze-next", cmdAnalyzeNext},
		{"audit-epic", cmdAuditEpic},
		{"stop-workflow", cmdStopWorkflow},
		{"run-issue", cmdRunIssue},
		{"prepare-review", cmdPrepareReview},
		{"submit-review", cmdSubmitReview},
		{"generate", cmdGenerate},
		{"validate", cmdValidate},
		{"status", cmdStatus},
		{"next", cmdNext},
		{"events", cmdEvents},
		{"kickoff", cmdKickoff},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/-h", func(t *testing.T) {
			exitCode := tt.fn([]string{"-h"})
			if exitCode != 0 {
				t.Errorf("cmd %s -h returned %d, want 0", tt.name, exitCode)
			}
		})
	}
}

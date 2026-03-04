package kickoff

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// LockManager – additional cases
// =============================================================================

// TestLockManager_LockInfoContainsPIDAndHostname verifies the JSON written
// by Acquire includes the current PID and a non-empty hostname.
func TestLockManager_LockInfoContainsPIDAndHostname(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "pid-host.lock")

	lock := NewLockManager(lockFile)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	defer lock.Release()

	data, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("unmarshal lock info: %v", err)
	}

	if info.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}
	if info.Hostname == "" {
		t.Error("Hostname should not be empty")
	}
	if info.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

// TestLockManager_IsStale_CurrentPID verifies that a lock held by the
// current process is NOT reported as stale.
func TestLockManager_IsStale_CurrentPID(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "current.lock")

	lock := NewLockManager(lockFile)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lock.Release()

	if lock.IsStale() {
		t.Error("Lock held by current process must not be stale")
	}
}

// TestLockManager_IsStale_NoFile verifies IsStale returns false when the
// lock file does not exist (can't read → not stale).
func TestLockManager_IsStale_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "absent.lock")

	lock := NewLockManager(lockFile)
	if lock.IsStale() {
		t.Error("IsStale should return false when lock file does not exist")
	}
}

// TestLockManager_IsStale_UnreadableFile verifies IsStale returns false when
// the lock file exists but contains invalid JSON (unreadable content).
func TestLockManager_IsStale_UnreadableFile(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "corrupt.lock")

	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(lockFile, []byte("not-json"), 0600); err != nil {
		t.Fatalf("write corrupt lock: %v", err)
	}

	lock := NewLockManager(lockFile)
	// readLockInfo will fail → IsStale returns false
	if lock.IsStale() {
		t.Error("IsStale should return false when lock file is unreadable/corrupt")
	}
}

// TestLockManager_AcquireAfterCorrupt verifies that Acquire succeeds when
// the existing lock file contains corrupt JSON (cannot determine PID).
func TestLockManager_AcquireAfterCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	lockFile := filepath.Join(tmpDir, "corrupt2.lock")

	if err := os.WriteFile(lockFile, []byte("{bad json}"), 0600); err != nil {
		t.Fatalf("write corrupt lock: %v", err)
	}

	lock := NewLockManager(lockFile)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("Acquire after corrupt lock should succeed: %v", err)
	}
	defer lock.Release()

	// Verify the new lock has valid content
	info, err := lock.readLockInfo()
	if err != nil {
		t.Fatalf("readLockInfo after re-acquire: %v", err)
	}
	if info.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}
}

// =============================================================================
// Config.Validate – additional cases
// =============================================================================

// TestConfig_Validate_SingleRepoType confirms "single-repo" is a valid type.
func TestConfig_Validate_SingleRepoType(t *testing.T) {
	cfg := &Config{
		Project: ProjectConfig{Name: "proj", Type: "single-repo"},
		Git:     GitConfig{IntegrationBranch: "main"},
	}
	errs := cfg.Validate()
	if len(errs) != 0 {
		t.Errorf("single-repo should be valid, got errors: %v", errs)
	}
}

// TestConfig_Validate_MultipleErrors verifies that all errors are collected
// rather than stopping at the first one.
func TestConfig_Validate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Project: ProjectConfig{}, // missing name and type
		// missing integration_branch
	}
	errs := cfg.Validate()
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors (name, type, branch), got %d: %v", len(errs), errs)
	}
}

// TestConfig_Validate_EmptySpecsPath confirms an empty specs.base_path
// is not itself a validation error (it is optional).
func TestConfig_Validate_EmptySpecsPath(t *testing.T) {
	cfg := &Config{
		Project: ProjectConfig{Name: "proj", Type: "monorepo"},
		Git:     GitConfig{IntegrationBranch: "develop"},
		Specs:   SpecsConfig{BasePath: ""},
	}
	errs := cfg.Validate()
	if len(errs) != 0 {
		t.Errorf("empty specs.base_path should not cause validation error, got: %v", errs)
	}
}

// TestConfig_ValidatePaths_EmptyRepos confirms ValidatePaths with no repos
// and no specs path returns zero errors.
func TestConfig_ValidatePaths_EmptyRepos(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{Repos: nil, Specs: SpecsConfig{BasePath: ""}}
	errs := cfg.ValidatePaths(tmpDir)
	if len(errs) != 0 {
		t.Errorf("empty repos should produce no path errors, got: %v", errs)
	}
}

// TestConfig_ValidatePaths_MissingRepoPath confirms a missing repo path is
// flagged with the correct field name.
func TestConfig_ValidatePaths_MissingRepoPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Repos: []RepoConfig{
			{Name: "backend", Path: "does-not-exist/"},
		},
	}
	errs := cfg.ValidatePaths(tmpDir)
	if len(errs) == 0 {
		t.Fatal("expected an error for non-existent repo path")
	}
	if errs[0].Field != "repos[0].path" {
		t.Errorf("Field = %q, want repos[0].path", errs[0].Field)
	}
}

// =============================================================================
// ValidationError.Error – edge cases
// =============================================================================

// TestValidationError_WithExpected ensures the Expected string appears in
// the formatted message with the correct format.
func TestValidationError_WithExpected(t *testing.T) {
	ve := ValidationError{
		Field:    "project.type",
		Message:  "invalid value: bad",
		Expected: "monorepo or single-repo",
	}
	got := ve.Error()
	if !strings.Contains(got, "expected:") {
		t.Errorf("Error() should contain 'expected:', got %q", got)
	}
	if !strings.Contains(got, "monorepo or single-repo") {
		t.Errorf("Error() should contain the Expected value, got %q", got)
	}
}

// TestValidationError_WithoutExpected ensures absence of Expected omits the
// "(expected: …)" suffix.
func TestValidationError_WithoutExpected(t *testing.T) {
	ve := ValidationError{
		Field:   "project.name",
		Message: "required field is missing",
	}
	got := ve.Error()
	if strings.Contains(got, "expected:") {
		t.Errorf("Error() should NOT contain 'expected:' when Expected is empty, got %q", got)
	}
	if !strings.Contains(got, "project.name") {
		t.Errorf("Error() should contain the field name, got %q", got)
	}
}

// =============================================================================
// StateManager – atomic write behaviour
// =============================================================================

// TestStateManager_SaveState_NoTmpLeftover verifies the .tmp file is removed
// after a successful atomic write.
func TestStateManager_SaveState_NoTmpLeftover(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	mgr := NewStateManager(stateFile)

	if err := mgr.SaveState(&RunState{Phase: "test"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	tmpFile := stateFile + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error(".tmp file should not exist after successful SaveState")
	}
}

// TestStateManager_SaveState_UpdatesTimestamp verifies SavedAt is set to a
// recent time on every save.
func TestStateManager_SaveState_UpdatesTimestamp(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")
	mgr := NewStateManager(stateFile)

	before := time.Now()
	if err := mgr.SaveState(&RunState{Phase: "x"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	after := time.Now()

	loaded, err := mgr.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if loaded.SavedAt.Before(before) || loaded.SavedAt.After(after) {
		t.Errorf("SavedAt %v is outside expected range [%v, %v]", loaded.SavedAt, before, after)
	}
}

// =============================================================================
// FanInManager – additional cases
// =============================================================================

// TestFanInManager_SetLogDir verifies SetLogDir stores the directory.
func TestFanInManager_SetLogDir(t *testing.T) {
	fanIn := NewFanInManager(10)
	defer fanIn.Stop()

	fanIn.SetLogDir("/some/log/dir")

	fanIn.mu.Lock()
	got := fanIn.logDir
	fanIn.mu.Unlock()

	if got != "/some/log/dir" {
		t.Errorf("logDir = %q, want %q", got, "/some/log/dir")
	}
}

// TestFanInManager_FullBufferDrop verifies that SendClaudeLine does not
// block when the channel buffer is full (drops the message).
func TestFanInManager_FullBufferDrop(t *testing.T) {
	// Buffer of 1 so it fills immediately.
	fanIn := NewFanInManager(1)
	defer fanIn.Stop()

	// Fill the buffer with one message.
	fanIn.SendClaudeLine("first")

	// This should not block; it should drop silently.
	done := make(chan struct{})
	go func() {
		fanIn.SendClaudeLine("should-drop")
		close(done)
	}()

	select {
	case <-done:
		// correct – returned without blocking
	case <-time.After(200 * time.Millisecond):
		t.Error("SendClaudeLine blocked on a full channel")
	}
}

// TestFanInManager_SendClaudeLine_ThreadSafety calls SendClaudeLine from
// multiple goroutines concurrently and expects no panics/races.
func TestFanInManager_SendClaudeLine_ThreadSafety(t *testing.T) {
	fanIn := NewFanInManager(200)
	defer fanIn.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fanIn.SendClaudeLine("msg")
		}(i)
	}
	wg.Wait()
}

// =============================================================================
// OutputFormatter – ColorizeLogLine
// =============================================================================

// TestOutputFormatter_ColorizeLogLine verifies PRINCIPAL/WORKER prefixes are
// colourised and plain lines pass through unchanged.
func TestOutputFormatter_ColorizeLogLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantColor string
		noColor   bool
	}{
		{
			name:      "PRINCIPAL prefix gets blue",
			line:      "[PRINCIPAL] some message",
			wantColor: colorBlue,
		},
		{
			name:      "WORKER prefix gets green",
			line:      "[WORKER] some message",
			wantColor: colorGreen,
		},
		{
			name:    "plain line is unchanged",
			line:    "just a regular line",
			noColor: true,
		},
		{
			name:    "empty line is unchanged",
			line:    "",
			noColor: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &OutputFormatter{useColors: true}
			result := f.ColorizeLogLine(tt.line)

			if tt.noColor {
				if result != tt.line {
					t.Errorf("plain line should pass through unchanged, got %q", result)
				}
				return
			}

			if !strings.Contains(result, tt.wantColor) {
				t.Errorf("expected ANSI code %q in result %q", tt.wantColor, result)
			}
			if !strings.Contains(result, colorReset) {
				t.Errorf("expected reset code in result %q", result)
			}
		})
	}
}

// TestOutputFormatter_ColorizeLogLine_NoColors verifies that when colours are
// disabled the line is returned verbatim regardless of prefix.
func TestOutputFormatter_ColorizeLogLine_NoColors(t *testing.T) {
	f := &OutputFormatter{useColors: false}

	for _, line := range []string{"[PRINCIPAL] msg", "[WORKER] msg", "plain"} {
		got := f.ColorizeLogLine(line)
		if got != line {
			t.Errorf("with colours disabled, line %q should be unchanged, got %q", line, got)
		}
	}
}

// TestOutputFormatter_Success_WithColors verifies ANSI codes are present when
// colours are enabled.
func TestOutputFormatter_Success_WithColors(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{writer: &buf, useColors: true}
	f.Success("all good")

	out := buf.String()
	if !strings.Contains(out, colorGreen) {
		t.Error("Success with colors should contain green ANSI code")
	}
	if !strings.Contains(out, "all good") {
		t.Error("Success should contain the message")
	}
}

// TestOutputFormatter_Error_WithColors verifies ANSI codes are present when
// colours are enabled.
func TestOutputFormatter_Error_WithColors(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{writer: &buf, useColors: true}
	f.Error("something broke")

	out := buf.String()
	if !strings.Contains(out, colorRed) {
		t.Error("Error with colors should contain red ANSI code")
	}
}

// TestOutputFormatter_Warning_WithColors verifies ANSI codes appear for warnings.
func TestOutputFormatter_Warning_WithColors(t *testing.T) {
	var buf bytes.Buffer
	f := &OutputFormatter{writer: &buf, useColors: true}
	f.Warning("watch out")

	out := buf.String()
	if !strings.Contains(out, colorYellow) {
		t.Error("Warning with colors should contain yellow ANSI code")
	}
}

// =============================================================================
// OutputParser – tailer callback paths
// =============================================================================

// TestOutputParser_TailerCallbacks_OnDispatch verifies onDispatchWorker is
// called with the correct issue ID from STEP-3 and dispatch patterns.
func TestOutputParser_TailerCallbacks_OnDispatch(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		expectedIssue int
	}{
		{
			name:          "STEP-3 triggers dispatch callback",
			line:          "[PRINCIPAL] 10:00:00 | STEP-3    | issue #7",
			expectedIssue: 7,
		},
		{
			name:          "dispatch_worker.sh triggers dispatch callback",
			line:          `[EXEC] bash .ai/scripts/dispatch_worker.sh "99"`,
			expectedIssue: 99,
		},
		{
			name:          "Chinese dispatch pattern triggers callback",
			line:          "[PRINCIPAL] 10:00:05 | 派工 Issue #42",
			expectedIssue: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dispatchedIssue int
			p := NewOutputParserWithTailerCallbacks(
				nil,
				nil,
				func(id int) { dispatchedIssue = id },
				nil,
			)
			p.Parse(tt.line)

			if dispatchedIssue != tt.expectedIssue {
				t.Errorf("dispatch callback issue = %d, want %d", dispatchedIssue, tt.expectedIssue)
			}
		})
	}
}

// TestOutputParser_TailerCallbacks_OnStatus verifies onWorkerStatus is called
// by STEP-4 and worker-complete patterns.
func TestOutputParser_TailerCallbacks_OnStatus(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "STEP-4 triggers status callback",
			line: "[PRINCIPAL] 10:44:30 | STEP-4    | done",
		},
		{
			name: "WORKER_STATUS=success triggers status callback",
			line: "WORKER_STATUS=success",
		},
		{
			name: "Worker 執行完成 triggers status callback",
			line: "[PRINCIPAL] 10:10:30 | Worker 執行完成 (exit code: 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusCalled := false
			p := NewOutputParserWithTailerCallbacks(
				nil,
				nil,
				nil,
				func() { statusCalled = true },
			)
			p.Parse(tt.line)

			if !statusCalled {
				t.Error("onWorkerStatus callback was not called")
			}
		})
	}
}

// TestOutputParser_NilTailerCallbacks_NoPanic verifies that nil tailer
// callbacks do not cause a panic when the matching patterns are encountered.
func TestOutputParser_NilTailerCallbacks_NoPanic(t *testing.T) {
	p := NewOutputParserWithTailerCallbacks(nil, nil, nil, nil)

	lines := []string{
		"[PRINCIPAL] 10:43:45 | STEP-3    | issue #42",
		"[PRINCIPAL] 10:44:30 | STEP-4    | done",
		"[PRINCIPAL] 10:00:05 | 派工 Issue #15",
		"WORKER_STATUS=failed",
	}
	for _, l := range lines {
		p.Parse(l) // must not panic
	}
}

// =============================================================================
// SignalHandler – SetExecutor / SetFanInManager / Cleanup with nil components
// =============================================================================

// TestSignalHandler_SetExecutor verifies SetExecutor updates the executor field.
func TestSignalHandler_SetExecutor(t *testing.T) {
	h := NewSignalHandler(nil, nil, nil)

	exec1, _ := NewPTYExecutor("echo", []string{"hi"})
	h.SetExecutor(exec1)

	h.mu.Lock()
	got := h.executor
	h.mu.Unlock()

	if got != exec1 {
		t.Error("SetExecutor did not update executor")
	}
}

// TestSignalHandler_SetFanInManager verifies SetFanInManager stores the instance.
func TestSignalHandler_SetFanInManager(t *testing.T) {
	h := NewSignalHandler(nil, nil, nil)
	fanIn := NewFanInManager(10)

	h.SetFanInManager(fanIn)

	h.mu.Lock()
	got := h.fanIn
	h.mu.Unlock()

	if got != fanIn {
		t.Error("SetFanInManager did not store the FanInManager")
	}
}

// TestSignalHandler_Cleanup_NilComponents verifies the cleanup helper does not
// panic when executor, fanIn, lock, and state are all nil.
func TestSignalHandler_Cleanup_NilComponents(t *testing.T) {
	h := NewSignalHandler(nil, nil, nil)

	// cleanup is an internal method; exercise it via the exported no-op path.
	// We call cleanup with an empty monitor list to verify nil guards work.
	h.cleanup(nil, true)
	h.cleanup(nil, false)
}

// =============================================================================
// RotatingLogger – concurrent writes and FilePath after close
// =============================================================================

// TestRotatingLogger_ConcurrentWrites verifies the logger does not panic or
// corrupt when multiple goroutines write simultaneously.
func TestRotatingLogger_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Write([]byte("concurrent log line\n"))
		}()
	}
	wg.Wait()
}

// TestRotatingLogger_FilePathAfterClose verifies FilePath returns empty string
// after Close.
func TestRotatingLogger_FilePathAfterClose(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewRotatingLogger(tmpDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}

	if logger.FilePath() == "" {
		t.Error("FilePath should be non-empty before Close")
	}

	logger.Close()

	if logger.FilePath() != "" {
		t.Error("FilePath should be empty after Close")
	}
}

// =============================================================================
// LogTailer – StopAndWait completes without deadlock
// =============================================================================

// TestLogTailer_StopAndWait_Completes verifies StopAndWait returns promptly
// when the tailer is watching a file that does not yet exist.
func TestLogTailer_StopAndWait_Completes(t *testing.T) {
	tmpDir := t.TempDir()
	ch := make(chan LogLine, 10)
	tailer := NewLogTailer(filepath.Join(tmpDir, "never.log"), "test", 0, ch)
	tailer.Start()

	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		tailer.StopAndWait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Error("StopAndWait did not complete within 2 seconds")
	}
}

// =============================================================================
// LogLine struct
// =============================================================================

// TestLogLine_Fields confirms that the LogLine struct stores Source, IssueID,
// and Text as assigned.
func TestLogLine_Fields(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		issueID int
		text    string
	}{
		{"claude source", "claude", 0, "some text"},
		{"principal source", "principal", 0, "[PRINCIPAL] msg"},
		{"worker source", "worker", 42, "[WORKER] msg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ll := LogLine{
				Source:  tt.source,
				IssueID: tt.issueID,
				Text:    tt.text,
			}
			if ll.Source != tt.source {
				t.Errorf("Source = %q, want %q", ll.Source, tt.source)
			}
			if ll.IssueID != tt.issueID {
				t.Errorf("IssueID = %d, want %d", ll.IssueID, tt.issueID)
			}
			if ll.Text != tt.text {
				t.Errorf("Text = %q, want %q", ll.Text, tt.text)
			}
		})
	}
}

// =============================================================================
// Preflight – CheckRepoPaths with empty repo list
// =============================================================================

// TestPreflightChecker_CheckRepoPaths_EmptyRepos verifies that an empty repo
// list causes CheckRepoPaths to pass (nothing to validate).
func TestPreflightChecker_CheckRepoPaths_EmptyRepos(t *testing.T) {
	tmpDir := t.TempDir()
	aiConfigDir := filepath.Join(tmpDir, ".ai", "config")
	os.MkdirAll(aiConfigDir, 0755)
	configPath := filepath.Join(aiConfigDir, "workflow.yaml")

	checker := NewPreflightChecker(configPath, "")
	checker.config = &Config{Repos: []RepoConfig{}}

	result := checker.CheckRepoPaths()
	if !result.Passed {
		t.Errorf("empty repos should pass CheckRepoPaths: %s", result.Message)
	}
}

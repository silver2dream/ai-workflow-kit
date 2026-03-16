package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// GetStartedAtTime
// ---------------------------------------------------------------------------

func TestCov_GetStartedAtTime(t *testing.T) {
	tests := []struct {
		name      string
		startedAt string
		wantErr   bool
	}{
		{"rfc3339", "2024-01-15T09:30:00Z", false},
		{"rfc3339 offset", "2024-01-15T09:30:00+08:00", false},
		{"rfc3339 nano", "2024-01-15T09:30:00.123456789Z", false},
		{"z suffix", "2024-01-15T09:30:00Z", false},
		{"empty", "", true},
		{"invalid", "not-a-date", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := &ExecutionTrace{StartedAt: tt.startedAt}
			tm, err := trace.GetStartedAtTime()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tm.IsZero() {
				t.Error("expected non-zero time")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadResult
// ---------------------------------------------------------------------------

func TestCov_LoadResult_Extended(t *testing.T) {
	t.Run("valid result", func(t *testing.T) {
		dir := t.TempDir()
		resultDir := filepath.Join(dir, ".ai", "results")
		os.MkdirAll(resultDir, 0700)

		result := &IssueResult{
			IssueID:  "42",
			Status:   "success",
			PRURL:    "https://github.com/org/repo/pull/99",
			Repo:     "backend",
			RepoType: "directory",
			Branch:   "feat/ai-issue-42",
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		os.WriteFile(filepath.Join(resultDir, "issue-42.json"), data, 0600)

		loaded, err := LoadResult(dir, 42)
		if err != nil {
			t.Fatalf("LoadResult failed: %v", err)
		}
		if loaded.IssueID != "42" {
			t.Errorf("expected IssueID '42', got %q", loaded.IssueID)
		}
		if loaded.Status != "success" {
			t.Errorf("expected status 'success', got %q", loaded.Status)
		}
		if loaded.PRURL != "https://github.com/org/repo/pull/99" {
			t.Errorf("expected PRURL, got %q", loaded.PRURL)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadResult(dir, 999)
		if err == nil {
			t.Error("expected error for missing file")
		}
		if !os.IsNotExist(err) {
			t.Errorf("expected IsNotExist error, got: %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		dir := t.TempDir()
		resultDir := filepath.Join(dir, ".ai", "results")
		os.MkdirAll(resultDir, 0700)
		os.WriteFile(filepath.Join(resultDir, "issue-1.json"), []byte("not json"), 0600)

		_, err := LoadResult(dir, 1)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

// ---------------------------------------------------------------------------
// LoadTrace
// ---------------------------------------------------------------------------

func TestCov_LoadTrace_Extended(t *testing.T) {
	t.Run("valid trace", func(t *testing.T) {
		dir := t.TempDir()
		traceDir := filepath.Join(dir, ".ai", "state", "traces")
		os.MkdirAll(traceDir, 0755)

		trace := &ExecutionTrace{
			TraceID:   "trace-1",
			IssueID:   "42",
			Status:    "running",
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			WorkerPID: 12345,
		}
		data, _ := json.MarshalIndent(trace, "", "  ")
		os.WriteFile(filepath.Join(traceDir, "issue-42.json"), data, 0644)

		loaded, err := LoadTrace(dir, 42)
		if err != nil {
			t.Fatalf("LoadTrace failed: %v", err)
		}
		if loaded.TraceID != "trace-1" {
			t.Errorf("expected trace-1, got %q", loaded.TraceID)
		}
		if loaded.WorkerPID != 12345 {
			t.Errorf("expected PID 12345, got %d", loaded.WorkerPID)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadTrace(dir, 999)
		if err == nil {
			t.Error("expected error for missing trace")
		}
	})
}

// ---------------------------------------------------------------------------
// ReadFailCount / ResetFailCount
// ---------------------------------------------------------------------------

func TestCov_ReadFailCount_Extended(t *testing.T) {
	t.Run("no file returns 0", func(t *testing.T) {
		dir := t.TempDir()
		count := ReadFailCount(dir, 99)
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("valid count", func(t *testing.T) {
		dir := t.TempDir()
		runDir := filepath.Join(dir, ".ai", "runs", "issue-5")
		os.MkdirAll(runDir, 0755)
		os.WriteFile(filepath.Join(runDir, "fail_count.txt"), []byte("3"), 0644)

		count := ReadFailCount(dir, 5)
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("invalid content returns 0", func(t *testing.T) {
		dir := t.TempDir()
		runDir := filepath.Join(dir, ".ai", "runs", "issue-5")
		os.MkdirAll(runDir, 0755)
		os.WriteFile(filepath.Join(runDir, "fail_count.txt"), []byte("not-a-number"), 0644)

		count := ReadFailCount(dir, 5)
		if count != 0 {
			t.Errorf("expected 0 for invalid content, got %d", count)
		}
	})
}

func TestCov_ResetFailCount_Extended(t *testing.T) {
	t.Run("removes file", func(t *testing.T) {
		dir := t.TempDir()
		runDir := filepath.Join(dir, ".ai", "runs", "issue-5")
		os.MkdirAll(runDir, 0755)
		failPath := filepath.Join(runDir, "fail_count.txt")
		os.WriteFile(failPath, []byte("3"), 0644)

		err := ResetFailCount(dir, 5)
		if err != nil {
			t.Fatalf("ResetFailCount failed: %v", err)
		}
		if _, err := os.Stat(failPath); !os.IsNotExist(err) {
			t.Error("expected file to be removed")
		}
	})

	t.Run("no error for missing file", func(t *testing.T) {
		dir := t.TempDir()
		err := ResetFailCount(dir, 999)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// ReadConsecutiveFailures / ResetConsecutiveFailures
// ---------------------------------------------------------------------------

func TestCov_ReadConsecutiveFailures(t *testing.T) {
	t.Run("no file returns 0", func(t *testing.T) {
		dir := t.TempDir()
		count := ReadConsecutiveFailures(dir)
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("valid count", func(t *testing.T) {
		dir := t.TempDir()
		stateDir := filepath.Join(dir, ".ai", "state")
		os.MkdirAll(stateDir, 0700)
		os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("5"), 0644)

		count := ReadConsecutiveFailures(dir)
		if count != 5 {
			t.Errorf("expected 5, got %d", count)
		}
	})
}

func TestCov_ResetConsecutiveFailures(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0700)
	os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("5"), 0644)

	err := ResetConsecutiveFailures(dir)
	if err != nil {
		t.Fatalf("ResetConsecutiveFailures failed: %v", err)
	}

	count := ReadConsecutiveFailures(dir)
	if count != 0 {
		t.Errorf("expected 0 after reset, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// WriteResultAtomic
// ---------------------------------------------------------------------------

func TestCov_WriteResultAtomic_Extended(t *testing.T) {
	t.Run("basic write", func(t *testing.T) {
		dir := t.TempDir()
		result := &IssueResult{
			IssueID:      "42",
			Status:       "success",
			PRURL:        "https://github.com/org/repo/pull/99",
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		}

		err := WriteResultAtomic(dir, 42, result)
		if err != nil {
			t.Fatalf("WriteResultAtomic failed: %v", err)
		}

		loaded, err := LoadResult(dir, 42)
		if err != nil {
			t.Fatalf("LoadResult failed: %v", err)
		}
		if loaded.Status != "success" {
			t.Errorf("expected status 'success', got %q", loaded.Status)
		}
	})

	t.Run("preserves existing PRURL", func(t *testing.T) {
		dir := t.TempDir()

		// Write initial result with PRURL
		initial := &IssueResult{
			IssueID:      "42",
			Status:       "success",
			PRURL:        "https://github.com/org/repo/pull/99",
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		}
		WriteResultAtomic(dir, 42, initial)

		// Write new result without PRURL
		updated := &IssueResult{
			IssueID:      "42",
			Status:       "failed",
			ErrorMessage: "test failure",
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		}
		err := WriteResultAtomic(dir, 42, updated)
		if err != nil {
			t.Fatalf("WriteResultAtomic failed: %v", err)
		}

		loaded, _ := LoadResult(dir, 42)
		if loaded.PRURL != "https://github.com/org/repo/pull/99" {
			t.Errorf("expected preserved PRURL, got %q", loaded.PRURL)
		}
		if loaded.Status != "failed" {
			t.Errorf("expected status 'failed', got %q", loaded.Status)
		}
	})

	t.Run("new PRURL overwrites old", func(t *testing.T) {
		dir := t.TempDir()

		initial := &IssueResult{
			IssueID:      "42",
			Status:       "success",
			PRURL:        "https://github.com/org/repo/pull/99",
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		}
		WriteResultAtomic(dir, 42, initial)

		updated := &IssueResult{
			IssueID:      "42",
			Status:       "success",
			PRURL:        "https://github.com/org/repo/pull/100",
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		}
		WriteResultAtomic(dir, 42, updated)

		loaded, _ := LoadResult(dir, 42)
		if loaded.PRURL != "https://github.com/org/repo/pull/100" {
			t.Errorf("expected new PRURL, got %q", loaded.PRURL)
		}
	})
}

// ---------------------------------------------------------------------------
// IssueResult JSON serialization
// ---------------------------------------------------------------------------

func TestCov_IssueResultJSON(t *testing.T) {
	result := &IssueResult{
		IssueID:           "42",
		Status:            "success",
		Repo:              "backend",
		RepoType:          "directory",
		WorkDir:           "/tmp/work",
		Branch:            "feat/ai-issue-42",
		BaseBranch:        "develop",
		HeadSHA:           "abc123",
		PRURL:             "https://github.com/org/repo/pull/99",
		TimestampUTC:      "2024-01-15T09:30:00Z",
		ErrorMessage:      "",
		Recoverable:       false,
		ConsistencyStatus: "consistent",
		Session: SessionInfo{
			WorkerSessionID: "worker-123",
			AttemptNumber:   1,
		},
		Metrics: ResultMetrics{
			DurationSeconds: 300,
			RetryCount:      0,
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded IssueResult
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if loaded.IssueID != "42" {
		t.Errorf("IssueID mismatch: %q", loaded.IssueID)
	}
	if loaded.Session.WorkerSessionID != "worker-123" {
		t.Errorf("WorkerSessionID mismatch: %q", loaded.Session.WorkerSessionID)
	}
	if loaded.Metrics.DurationSeconds != 300 {
		t.Errorf("DurationSeconds mismatch: %d", loaded.Metrics.DurationSeconds)
	}
}

// ---------------------------------------------------------------------------
// ExecutionTrace JSON
// ---------------------------------------------------------------------------

func TestCov_ExecutionTraceJSON(t *testing.T) {
	trace := &ExecutionTrace{
		TraceID:     "trace-1",
		IssueID:     "42",
		Repo:        "backend",
		Branch:      "feat/ai-issue-42",
		BaseBranch:  "develop",
		Status:      "running",
		StartedAt:   "2024-01-15T09:30:00Z",
		WorkerPID:   12345,
		WorkerStart: time.Now().Unix(),
		Steps: []TraceStep{
			{
				Name:      "preflight",
				Status:    "success",
				StartedAt: "2024-01-15T09:30:00Z",
				EndedAt:   "2024-01-15T09:30:01Z",
				Duration:  1,
			},
		},
	}

	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var loaded ExecutionTrace
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(loaded.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(loaded.Steps))
	}
	if loaded.Steps[0].Name != "preflight" {
		t.Errorf("step name mismatch: %q", loaded.Steps[0].Name)
	}
}

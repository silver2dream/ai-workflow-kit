package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WriteFileAtomic
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")

	if err := WriteFileAtomic(target, []byte(`{"ok":true}`), 0644); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("content = %q, want %q", got, `{"ok":true}`)
	}
}

func TestWriteFileAtomic_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")

	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestWriteFileAtomic_CreatesNestedDirs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c", "file.txt")

	if err := WriteFileAtomic(target, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestWriteFileAtomic_NoLeftoverTmpFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")

	if err := WriteFileAtomic(target, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	tmpPath := target + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("leftover .tmp file should not exist after successful write")
	}
}

func TestWriteFileAtomic_NoLeftoverBakFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "file.txt")

	// Write twice to exercise the overwrite path (which creates a .bak during the operation)
	if err := WriteFileAtomic(target, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}

	bakPath := target + ".bak"
	if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
		t.Errorf("leftover .bak file should not exist after successful overwrite")
	}
}

// ---------------------------------------------------------------------------
// ReadFailCount / ResetFailCount
// ---------------------------------------------------------------------------

func TestReadFailCount_MalformedContent_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	runsDir := filepath.Join(dir, ".ai", "runs", "issue-7")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "fail_count.txt"), []byte("not-a-number"), 0644); err != nil {
		t.Fatal(err)
	}

	count := ReadFailCount(dir, 7)
	if count != 0 {
		t.Errorf("ReadFailCount with malformed content = %d, want 0", count)
	}
}

func TestReadFailCount_MissingFile_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	count := ReadFailCount(dir, 42)
	if count != 0 {
		t.Errorf("ReadFailCount for missing file = %d, want 0", count)
	}
}

func TestResetFailCount_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	runsDir := filepath.Join(dir, ".ai", "runs", "issue-5")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "fail_count.txt"), []byte("3"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ResetFailCount(dir, 5); err != nil {
		t.Fatalf("ResetFailCount failed: %v", err)
	}

	// After reset, reading should return 0
	if got := ReadFailCount(dir, 5); got != 0 {
		t.Errorf("ReadFailCount after reset = %d, want 0", got)
	}
}

func TestResetFailCount_Idempotent_NoError(t *testing.T) {
	dir := t.TempDir()

	// Calling reset when file doesn't exist should not error
	if err := ResetFailCount(dir, 99); err != nil {
		t.Errorf("ResetFailCount on non-existent file error = %v, want nil", err)
	}

	// Calling a second time should also succeed
	if err := ResetFailCount(dir, 99); err != nil {
		t.Errorf("second ResetFailCount error = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// ReadConsecutiveFailures / ResetConsecutiveFailures
// ---------------------------------------------------------------------------

func TestReadConsecutiveFailures_MissingFile_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	count := ReadConsecutiveFailures(dir)
	if count != 0 {
		t.Errorf("ReadConsecutiveFailures for missing file = %d, want 0", count)
	}
}

func TestReadConsecutiveFailures_ReadsValue(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("5"), 0644); err != nil {
		t.Fatal(err)
	}

	count := ReadConsecutiveFailures(dir)
	if count != 5 {
		t.Errorf("ReadConsecutiveFailures = %d, want 5", count)
	}
}

func TestResetConsecutiveFailures_WritesZero(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("3"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ResetConsecutiveFailures(dir); err != nil {
		t.Fatalf("ResetConsecutiveFailures failed: %v", err)
	}

	count := ReadConsecutiveFailures(dir)
	if count != 0 {
		t.Errorf("ReadConsecutiveFailures after reset = %d, want 0", count)
	}
}

// ---------------------------------------------------------------------------
// WriteResultAtomic – PR URL preservation
// ---------------------------------------------------------------------------

func TestWriteResultAtomic_PreservesPRURL_WhenNewResultHasNone(t *testing.T) {
	dir := t.TempDir()

	// Write initial result with PR URL
	initial := &IssueResult{
		IssueID:      "10",
		Status:       "success",
		PRURL:        "https://github.com/owner/repo/pull/99",
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteResultAtomic(dir, 10, initial); err != nil {
		t.Fatalf("WriteResultAtomic initial: %v", err)
	}

	// Write a new result without a PR URL
	updated := &IssueResult{
		IssueID:      "10",
		Status:       "failed",
		PRURL:        "", // intentionally blank
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteResultAtomic(dir, 10, updated); err != nil {
		t.Fatalf("WriteResultAtomic updated: %v", err)
	}

	loaded, err := LoadResult(dir, 10)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if loaded.PRURL != "https://github.com/owner/repo/pull/99" {
		t.Errorf("pr_url = %q, want %q", loaded.PRURL, "https://github.com/owner/repo/pull/99")
	}
}

func TestWriteResultAtomic_DoesNotOverwriteExistingPRURL_WhenNewHasOne(t *testing.T) {
	dir := t.TempDir()

	first := &IssueResult{
		IssueID:      "11",
		Status:       "success",
		PRURL:        "https://github.com/owner/repo/pull/1",
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteResultAtomic(dir, 11, first); err != nil {
		t.Fatal(err)
	}

	second := &IssueResult{
		IssueID:      "11",
		Status:       "success",
		PRURL:        "https://github.com/owner/repo/pull/2",
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteResultAtomic(dir, 11, second); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadResult(dir, 11)
	if err != nil {
		t.Fatal(err)
	}
	// When the new result already has a PR URL it should be used as-is
	if loaded.PRURL != "https://github.com/owner/repo/pull/2" {
		t.Errorf("pr_url = %q, want %q", loaded.PRURL, "https://github.com/owner/repo/pull/2")
	}
}

// ---------------------------------------------------------------------------
// WriteResultAtomic – JSON round-trip
// ---------------------------------------------------------------------------

func TestWriteResultAtomic_JSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &IssueResult{
		IssueID:      "20",
		Status:       "success",
		Repo:         "backend",
		RepoType:     "directory",
		Branch:       "feat/ai-issue-20",
		BaseBranch:   "develop",
		HeadSHA:      "deadbeef",
		TimestampUTC: "2026-01-01T00:00:00Z",
		PRURL:        "https://github.com/o/r/pull/20",
		Metrics:      ResultMetrics{DurationSeconds: 42, RetryCount: 1},
		Session:      SessionInfo{WorkerSessionID: "worker-abc123", AttemptNumber: 2},
	}

	if err := WriteResultAtomic(dir, 20, original); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := LoadResult(dir, 20)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.IssueID != original.IssueID {
		t.Errorf("IssueID: got %q want %q", loaded.IssueID, original.IssueID)
	}
	if loaded.Metrics.DurationSeconds != 42 {
		t.Errorf("DurationSeconds: got %d want 42", loaded.Metrics.DurationSeconds)
	}
	if loaded.Session.WorkerSessionID != "worker-abc123" {
		t.Errorf("WorkerSessionID: got %q want %q", loaded.Session.WorkerSessionID, "worker-abc123")
	}
	if loaded.Session.AttemptNumber != 2 {
		t.Errorf("AttemptNumber: got %d want 2", loaded.Session.AttemptNumber)
	}
}

// ---------------------------------------------------------------------------
// LoadResult / LoadTrace – malformed JSON
// ---------------------------------------------------------------------------

func TestLoadResult_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	resultsDir := filepath.Join(dir, ".ai", "results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, "issue-55.json"), []byte("{broken json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadResult(dir, 55)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "unmarshal") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error message %q should mention parsing", err.Error())
	}
}

func TestLoadTrace_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, ".ai", "state", "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tracesDir, "issue-56.json"), []byte("not json at all"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTrace(dir, 56)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// PID file operations
// ---------------------------------------------------------------------------

func TestWriteAndReadPIDFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)

	info := &PIDFile{
		PID:         12345,
		StartTime:   now.Unix(),
		IssueNumber: 7,
		SessionID:   "worker-20260101-120000-aabb",
		StartedAt:   now,
	}

	if err := WritePIDFile(dir, 7, info); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	loaded, err := ReadPIDFile(dir, 7)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}

	if loaded.PID != info.PID {
		t.Errorf("PID: got %d want %d", loaded.PID, info.PID)
	}
	if loaded.IssueNumber != info.IssueNumber {
		t.Errorf("IssueNumber: got %d want %d", loaded.IssueNumber, info.IssueNumber)
	}
	if loaded.SessionID != info.SessionID {
		t.Errorf("SessionID: got %q want %q", loaded.SessionID, info.SessionID)
	}
	if loaded.StartTime != info.StartTime {
		t.Errorf("StartTime: got %d want %d", loaded.StartTime, info.StartTime)
	}
}

func TestReadPIDFile_MissingFile_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadPIDFile(dir, 999)
	if err == nil {
		t.Fatal("expected error for missing PID file, got nil")
	}
}

func TestCleanupPIDFile_RemovesFile(t *testing.T) {
	dir := t.TempDir()

	info := &PIDFile{PID: 1, IssueNumber: 3, SessionID: "s"}
	if err := WritePIDFile(dir, 3, info); err != nil {
		t.Fatal(err)
	}

	if err := CleanupPIDFile(dir, 3); err != nil {
		t.Fatalf("CleanupPIDFile: %v", err)
	}

	if _, err := ReadPIDFile(dir, 3); err == nil {
		t.Error("expected error reading PID file after cleanup")
	}
}

func TestCleanupPIDFile_NonExistentFile_NoError(t *testing.T) {
	dir := t.TempDir()
	if err := CleanupPIDFile(dir, 888); err != nil {
		t.Errorf("CleanupPIDFile non-existent: %v", err)
	}
}

// ---------------------------------------------------------------------------
// IsProcessRunning – invalid PID
// ---------------------------------------------------------------------------

func TestIsProcessRunning_InvalidPID_ReturnsFalse(t *testing.T) {
	tests := []struct {
		name string
		pid  int
	}{
		{"zero", 0},
		{"negative", -1},
		{"large negative", -9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsProcessRunning(tt.pid, 0) {
				t.Errorf("IsProcessRunning(%d, 0) = true, want false", tt.pid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BackendRegistry – concurrent registration and overwrite
// ---------------------------------------------------------------------------

func TestBackendRegistry_ConcurrentRegister_NoPanic(t *testing.T) {
	reg := NewBackendRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.Register(NewCodexBackend())
		}()
	}
	wg.Wait()

	// Should still have exactly one "codex" entry
	names := reg.Names()
	count := 0
	for _, n := range names {
		if n == "codex" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'codex' registration, got %d", count)
	}
}

func TestBackendRegistry_OverwriteRegistration(t *testing.T) {
	reg := NewBackendRegistry()
	reg.Register(NewClaudeCodeBackend("opus", 10, false))
	reg.Register(NewClaudeCodeBackend("sonnet", 20, true))

	b, err := reg.Get("claude-code")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	cc, ok := b.(*ClaudeCodeBackend)
	if !ok {
		t.Fatalf("expected *ClaudeCodeBackend, got %T", b)
	}
	// The second registration should have won
	if cc.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", cc.Model, "sonnet")
	}
	if cc.MaxTurns != 20 {
		t.Errorf("MaxTurns = %d, want 20", cc.MaxTurns)
	}
}

// ---------------------------------------------------------------------------
// ClaudeCodeBackend – custom values and negative turns default
// ---------------------------------------------------------------------------

func TestClaudeCodeBackend_CustomModelAndTurns(t *testing.T) {
	b := NewClaudeCodeBackend("opus", 100, true)
	if b.Model != "opus" {
		t.Errorf("Model = %q, want %q", b.Model, "opus")
	}
	if b.MaxTurns != 100 {
		t.Errorf("MaxTurns = %d, want 100", b.MaxTurns)
	}
	if !b.DangerouslySkipPermissions {
		t.Error("DangerouslySkipPermissions should be true")
	}
}

func TestClaudeCodeBackend_NegativeTurns_UsesDefault(t *testing.T) {
	b := NewClaudeCodeBackend("", -5, false)
	if b.MaxTurns != 50 {
		t.Errorf("MaxTurns with negative input = %d, want 50", b.MaxTurns)
	}
}

// ---------------------------------------------------------------------------
// generateSessionID – format and uniqueness
// ---------------------------------------------------------------------------

func TestGenerateSessionID_Format(t *testing.T) {
	id := generateSessionID("worker")

	// Expected format: worker-YYYYMMDD-HHMMSS-xxxxxxxx
	if !strings.HasPrefix(id, "worker-") {
		t.Errorf("session ID %q should start with 'worker-'", id)
	}

	// Match pattern: role-date-time-hexsuffix
	re := regexp.MustCompile(`^worker-\d{8}-\d{6}-[0-9a-f]+$`)
	if !re.MatchString(id) {
		t.Errorf("session ID %q does not match expected pattern", id)
	}
}

func TestGenerateSessionID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateSessionID("test")
		if seen[id] {
			t.Errorf("duplicate session ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestGenerateSessionID_DifferentRoles(t *testing.T) {
	id1 := generateSessionID("worker")
	id2 := generateSessionID("principal")

	if !strings.HasPrefix(id1, "worker-") {
		t.Errorf("id1 %q should start with 'worker-'", id1)
	}
	if !strings.HasPrefix(id2, "principal-") {
		t.Errorf("id2 %q should start with 'principal-'", id2)
	}
}

// ---------------------------------------------------------------------------
// parseBool – table-driven edge cases
// ---------------------------------------------------------------------------

func TestParseBool_TableDriven(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"no", false},
		{"No", false},
		{"NO", false},
		{"0", false},
		{"", false},
		{"  true  ", true},  // trimmed whitespace
		{"  false  ", false},
		{"2", false},        // only "1" is truthy, not other numbers
		{"on", false},
		{"off", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got := parseBool(tt.input)
			if got != tt.expected {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// containsString – edge cases
// ---------------------------------------------------------------------------

func TestContainsString_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		target   string
		expected bool
	}{
		{"found in middle", []string{"a", "b", "c"}, "b", true},
		{"found at start", []string{"x", "y"}, "x", true},
		{"found at end", []string{"m", "n"}, "n", true},
		{"not found", []string{"a", "b"}, "z", false},
		{"empty slice", []string{}, "a", false},
		{"nil slice", nil, "a", false},
		{"empty target found", []string{"", "b"}, "", true},
		{"empty target not found", []string{"a", "b"}, "", false},
		{"case sensitive – no match", []string{"Root"}, "root", false},
		{"exact match required", []string{"backend"}, "back", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsString(tt.slice, tt.target)
			if got != tt.expected {
				t.Errorf("containsString(%v, %q) = %v, want %v", tt.slice, tt.target, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractTicketValue – table-driven
// ---------------------------------------------------------------------------

func TestExtractTicketValue_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		key      string
		expected string
	}{
		{
			name:     "bold format",
			content:  "**Release**: true\n",
			key:      "Release",
			expected: "true",
		},
		{
			name:     "list format with dash",
			content:  "- Repo: backend\n",
			key:      "Repo",
			expected: "backend",
		},
		{
			name:     "list format with asterisk",
			content:  "* Severity: P1\n",
			key:      "Severity",
			expected: "P1",
		},
		{
			name:     "key not present returns empty",
			content:  "- Repo: backend\n",
			key:      "Release",
			expected: "",
		},
		{
			name:     "empty content returns empty",
			content:  "",
			key:      "Release",
			expected: "",
		},
		{
			name:     "trailing whitespace trimmed",
			content:  "- Repo: backend   \n",
			key:      "Repo",
			expected: "backend",
		},
		{
			name:     "mixed case key match is case insensitive for content",
			content:  "**Source**: audit:xyz\n",
			key:      "Source",
			expected: "audit:xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTicketValue(tt.content, tt.key)
			if got != tt.expected {
				t.Errorf("extractTicketValue(%q, %q) = %q, want %q", tt.content, tt.key, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CleanupManager – LIFO order and double-cleanup idempotency
// ---------------------------------------------------------------------------

func TestCleanupManager_LIFO_Order(t *testing.T) {
	cm := NewCleanupManager()
	// Use runCleanup directly to avoid channel-close side effects of Cleanup()
	defer cm.cancel()

	var order []int
	mu := sync.Mutex{}

	cm.Register(func() {
		mu.Lock()
		order = append(order, 1)
		mu.Unlock()
	})
	cm.Register(func() {
		mu.Lock()
		order = append(order, 2)
		mu.Unlock()
	})
	cm.Register(func() {
		mu.Lock()
		order = append(order, 3)
		mu.Unlock()
	})

	// runCleanup is the internal LIFO executor; Cleanup() wraps it and also
	// closes the signal channel (not safe to call twice).
	cm.runCleanup()

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 cleanup calls, got %d", len(order))
	}
	// LIFO: 3, 2, 1
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("LIFO order wrong: got %v, want [3 2 1]", order)
	}
}

func TestCleanupManager_DoubleRunCleanup_RunsOnce(t *testing.T) {
	// runCleanup is idempotent (guarded by the done flag).
	// Calling Cleanup() twice would panic (closes channel twice), so we test
	// runCleanup directly to verify the idempotency guarantee.
	cm := NewCleanupManager()
	defer cm.cancel()

	callCount := 0
	cm.Register(func() {
		callCount++
	})

	cm.runCleanup()
	cm.runCleanup() // second call must be a no-op

	if callCount != 1 {
		t.Errorf("cleanup fn called %d times, want 1", callCount)
	}
}

func TestCleanupManager_NoRegistrations_RunCleanup_NoPanic(t *testing.T) {
	cm := NewCleanupManager()
	defer cm.cancel()

	// Should not panic even with no registered functions
	cm.runCleanup()
}

// ---------------------------------------------------------------------------
// buildValidRepos – various config shapes
// ---------------------------------------------------------------------------

func TestBuildValidRepos_NilConfig_ReturnsDefaults(t *testing.T) {
	repos := buildValidRepos(nil)
	// nil config must return root, backend, frontend
	if !containsString(repos, "root") {
		t.Errorf("expected 'root' in %v", repos)
	}
	if !containsString(repos, "backend") {
		t.Errorf("expected 'backend' in %v", repos)
	}
	if !containsString(repos, "frontend") {
		t.Errorf("expected 'frontend' in %v", repos)
	}
}

func TestBuildValidRepos_ConfigWithRepos_IncludesRoot(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "api"},
			{Name: "web"},
		},
	}
	repos := buildValidRepos(cfg)
	if !containsString(repos, "root") {
		t.Errorf("expected 'root' to always be present in %v", repos)
	}
	if !containsString(repos, "api") {
		t.Errorf("expected 'api' in %v", repos)
	}
	if !containsString(repos, "web") {
		t.Errorf("expected 'web' in %v", repos)
	}
}

func TestBuildValidRepos_DeduplicatesRootEntry(t *testing.T) {
	// A repo named "root" in config should not cause duplicate "root" entries
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "root"},
			{Name: "backend"},
		},
	}
	repos := buildValidRepos(cfg)
	count := 0
	for _, r := range repos {
		if r == "root" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("'root' appears %d times in %v, want exactly 1", count, repos)
	}
}

// ---------------------------------------------------------------------------
// ExecutionTrace.GetStartedAtTime – valid and empty
// ---------------------------------------------------------------------------

func TestExecutionTrace_GetStartedAtTime_ValidRFC3339(t *testing.T) {
	trace := &ExecutionTrace{StartedAt: "2026-01-15T09:30:00Z"}
	tm, err := trace.GetStartedAtTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tm.Year() != 2026 || tm.Month() != 1 || tm.Day() != 15 {
		t.Errorf("parsed time %v doesn't match expected 2026-01-15", tm)
	}
}

func TestExecutionTrace_GetStartedAtTime_EmptyField_ReturnsError(t *testing.T) {
	trace := &ExecutionTrace{StartedAt: ""}
	_, err := trace.GetStartedAtTime()
	if err == nil {
		t.Error("expected error for empty started_at, got nil")
	}
}

// ---------------------------------------------------------------------------
// IssueResult JSON serialization – omitempty fields
// ---------------------------------------------------------------------------

func TestIssueResult_JSONSerialization_OmitemptyFields(t *testing.T) {
	result := &IssueResult{
		IssueID:      "1",
		Status:       "success",
		TimestampUTC: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	jsonStr := string(data)
	// Fields with omitempty and zero values must not appear
	if strings.Contains(jsonStr, `"pr_url"`) {
		t.Error("empty pr_url should be omitted from JSON")
	}
	if strings.Contains(jsonStr, `"failure_stage"`) {
		t.Error("empty failure_stage should be omitted from JSON")
	}
	// Required fields must be present
	if !strings.Contains(jsonStr, `"issue_id"`) {
		t.Error("issue_id must be present in JSON")
	}
	if !strings.Contains(jsonStr, `"status"`) {
		t.Error("status must be present in JSON")
	}
}

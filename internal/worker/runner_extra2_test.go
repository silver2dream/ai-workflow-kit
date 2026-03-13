package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

// ---------------------------------------------------------------------------
// workflowFeedback.isEnabled
// ---------------------------------------------------------------------------

func TestWorkflowFeedback_IsEnabled_NilPointer(t *testing.T) {
	f := &workflowFeedback{Enabled: nil}
	if !f.isEnabled() {
		t.Error("isEnabled() with nil Enabled should return true (default on)")
	}
}

func TestWorkflowFeedback_IsEnabled_False(t *testing.T) {
	v := false
	f := &workflowFeedback{Enabled: &v}
	if f.isEnabled() {
		t.Error("isEnabled() with Enabled=false should return false")
	}
}

func TestWorkflowFeedback_IsEnabled_True(t *testing.T) {
	v := true
	f := &workflowFeedback{Enabled: &v}
	if !f.isEnabled() {
		t.Error("isEnabled() with Enabled=true should return true")
	}
}

// ---------------------------------------------------------------------------
// workflowFeedback.maxHistory
// ---------------------------------------------------------------------------

func TestWorkflowFeedback_MaxHistory_Zero(t *testing.T) {
	f := &workflowFeedback{MaxHistoryInPrompt: 0}
	if got := f.maxHistory(); got != 10 {
		t.Errorf("maxHistory() with 0 = %d, want 10", got)
	}
}

func TestWorkflowFeedback_MaxHistory_Negative(t *testing.T) {
	f := &workflowFeedback{MaxHistoryInPrompt: -5}
	if got := f.maxHistory(); got != 10 {
		t.Errorf("maxHistory() with -5 = %d, want 10", got)
	}
}

func TestWorkflowFeedback_MaxHistory_Positive(t *testing.T) {
	f := &workflowFeedback{MaxHistoryInPrompt: 7}
	if got := f.maxHistory(); got != 7 {
		t.Errorf("maxHistory() with 7 = %d, want 7", got)
	}
}

// ---------------------------------------------------------------------------
// buildValidRepos
// ---------------------------------------------------------------------------

func TestBuildValidRepos_NilConfig(t *testing.T) {
	repos := buildValidRepos(nil)
	// Should contain at least root, backend, frontend
	if len(repos) < 3 {
		t.Errorf("buildValidRepos(nil) = %v, want at least 3 items", repos)
	}
	if repos[0] != "root" {
		t.Errorf("first item = %q, want root", repos[0])
	}
}

func TestBuildValidRepos_EmptyConfig(t *testing.T) {
	cfg := &workflowConfig{}
	repos := buildValidRepos(cfg)
	// Always starts with "root"
	if repos[0] != "root" {
		t.Errorf("first item = %q, want root", repos[0])
	}
}

func TestBuildValidRepos_WithRepos(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "backend"},
			{Name: "frontend"},
		},
	}
	repos := buildValidRepos(cfg)
	found := map[string]bool{}
	for _, r := range repos {
		found[r] = true
	}
	if !found["root"] {
		t.Error("buildValidRepos should always include root")
	}
	if !found["backend"] {
		t.Error("buildValidRepos should include backend")
	}
	if !found["frontend"] {
		t.Error("buildValidRepos should include frontend")
	}
}

func TestBuildValidRepos_DuplicateSkipped(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "root"}, // duplicate of always-present "root"
			{Name: "api"},
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
		t.Errorf("root appears %d times, want 1", count)
	}
}

func TestBuildValidRepos_EmptyNameSkipped(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: ""},
			{Name: "api"},
		},
	}
	repos := buildValidRepos(cfg)
	for _, r := range repos {
		if r == "" {
			t.Error("buildValidRepos should skip empty repo names")
		}
	}
}

// ---------------------------------------------------------------------------
// resolveRepoConfig
// ---------------------------------------------------------------------------

func TestResolveRepoConfig2_NilConfig(t *testing.T) {
	repoType, repoPath := resolveRepoConfig(nil, "backend")
	if repoType != "directory" {
		t.Errorf("repoType = %q, want directory", repoType)
	}
	if repoPath != "./" {
		t.Errorf("repoPath = %q, want ./", repoPath)
	}
}

func TestResolveRepoConfig2_NotFound(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "backend", Type: "root", Path: "backend/"},
		},
	}
	repoType, repoPath := resolveRepoConfig(cfg, "notexist")
	if repoType != "directory" {
		t.Errorf("repoType = %q, want directory", repoType)
	}
	if repoPath != "./" {
		t.Errorf("repoPath = %q, want ./", repoPath)
	}
}

func TestResolveRepoConfig_Found(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "backend", Type: "submodule", Path: "backend/"},
		},
	}
	repoType, repoPath := resolveRepoConfig(cfg, "backend")
	if repoType != "submodule" {
		t.Errorf("repoType = %q, want submodule", repoType)
	}
	if repoPath != "backend/" {
		t.Errorf("repoPath = %q, want backend/", repoPath)
	}
}

func TestResolveRepoConfig_DefaultsApplied(t *testing.T) {
	// Repo exists but type and path are empty — defaults should apply
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "api", Type: "", Path: ""},
		},
	}
	repoType, repoPath := resolveRepoConfig(cfg, "api")
	if repoType != "directory" {
		t.Errorf("repoType = %q, want directory", repoType)
	}
	if repoPath != "./" {
		t.Errorf("repoPath = %q, want ./", repoPath)
	}
}

// ---------------------------------------------------------------------------
// getConfigVerifyCommands
// ---------------------------------------------------------------------------

func TestGetConfigVerifyCommands_NilConfig(t *testing.T) {
	cmds := getConfigVerifyCommands(nil, "backend")
	if cmds != nil {
		t.Errorf("expected nil for nil config, got %v", cmds)
	}
}

func TestGetConfigVerifyCommands_RepoNotFound(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{{Name: "backend", Verify: workflowRepoVerify{Build: "go build ./..."}}},
	}
	cmds := getConfigVerifyCommands(cfg, "frontend")
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands for missing repo, got %v", cmds)
	}
}

func TestGetConfigVerifyCommands_BothCommands(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{
				Name: "backend",
				Verify: workflowRepoVerify{
					Build: "go build ./...",
					Test:  "go test ./...",
				},
			},
		},
	}
	cmds := getConfigVerifyCommands(cfg, "backend")
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %v", cmds)
	}
	if cmds[0] != "go build ./..." {
		t.Errorf("cmds[0] = %q, want go build ./...", cmds[0])
	}
	if cmds[1] != "go test ./..." {
		t.Errorf("cmds[1] = %q, want go test ./...", cmds[1])
	}
}

func TestGetConfigVerifyCommands_OnlyBuild(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "api", Verify: workflowRepoVerify{Build: "make build"}},
		},
	}
	cmds := getConfigVerifyCommands(cfg, "api")
	if len(cmds) != 1 || cmds[0] != "make build" {
		t.Errorf("expected [make build], got %v", cmds)
	}
}

// ---------------------------------------------------------------------------
// inferSetupCommand
// ---------------------------------------------------------------------------

func TestInferSetupCommand_NodeDefault(t *testing.T) {
	cmd := inferSetupCommand("node", "")
	if !strings.Contains(cmd, "npm") {
		t.Errorf("node with no pm = %q, want npm", cmd)
	}
}

func TestInferSetupCommand_NodeYarn(t *testing.T) {
	cmd := inferSetupCommand("node", "yarn")
	if !strings.Contains(cmd, "yarn") {
		t.Errorf("node + yarn = %q, want yarn", cmd)
	}
}

func TestInferSetupCommand_NodePnpm(t *testing.T) {
	cmd := inferSetupCommand("node", "pnpm")
	if !strings.Contains(cmd, "pnpm") {
		t.Errorf("node + pnpm = %q, want pnpm", cmd)
	}
}

func TestInferSetupCommand_TypeScript(t *testing.T) {
	cmd := inferSetupCommand("typescript", "")
	if !strings.Contains(cmd, "npm") {
		t.Errorf("typescript = %q, want npm", cmd)
	}
}

func TestInferSetupCommand_React(t *testing.T) {
	cmd := inferSetupCommand("react", "")
	if !strings.Contains(cmd, "npm") {
		t.Errorf("react = %q, want npm", cmd)
	}
}

func TestInferSetupCommand_Python(t *testing.T) {
	cmd := inferSetupCommand("python", "")
	if !strings.Contains(cmd, "pip") {
		t.Errorf("python = %q, want pip", cmd)
	}
}

func TestInferSetupCommand_Django(t *testing.T) {
	cmd := inferSetupCommand("django", "")
	if !strings.Contains(cmd, "pip") {
		t.Errorf("django = %q, want pip", cmd)
	}
}

func TestInferSetupCommand_DotNet(t *testing.T) {
	cmd := inferSetupCommand("dotnet", "")
	if cmd != "dotnet restore" {
		t.Errorf("dotnet = %q, want 'dotnet restore'", cmd)
	}
}

func TestInferSetupCommand_CSharp(t *testing.T) {
	cmd := inferSetupCommand("csharp", "")
	if cmd != "dotnet restore" {
		t.Errorf("csharp = %q, want 'dotnet restore'", cmd)
	}
}

func TestInferSetupCommand_Go(t *testing.T) {
	cmd := inferSetupCommand("go", "")
	if cmd != "" {
		t.Errorf("go = %q, want empty (no setup needed)", cmd)
	}
}

func TestInferSetupCommand_Rust(t *testing.T) {
	cmd := inferSetupCommand("rust", "")
	if cmd != "" {
		t.Errorf("rust = %q, want empty (no setup needed)", cmd)
	}
}

func TestInferSetupCommand_Generic(t *testing.T) {
	cmd := inferSetupCommand("generic", "")
	if cmd != "" {
		t.Errorf("generic = %q, want empty", cmd)
	}
}

func TestInferSetupCommand_Unknown(t *testing.T) {
	cmd := inferSetupCommand("cobol", "")
	if cmd != "" {
		t.Errorf("unknown language = %q, want empty", cmd)
	}
}

func TestInferSetupCommand_CaseInsensitive(t *testing.T) {
	cmd := inferSetupCommand("Python", "")
	if !strings.Contains(cmd, "pip") {
		t.Errorf("Python (caps) = %q, want pip", cmd)
	}
}

// ---------------------------------------------------------------------------
// getSetupCommand
// ---------------------------------------------------------------------------

func TestGetSetupCommand_NilConfig(t *testing.T) {
	cmd := getSetupCommand(nil, "api")
	if cmd != "" {
		t.Errorf("nil config = %q, want empty", cmd)
	}
}

func TestGetSetupCommand_RepoNotFound(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{{Name: "backend", Language: "go"}},
	}
	cmd := getSetupCommand(cfg, "notexist")
	if cmd != "" {
		t.Errorf("missing repo = %q, want empty", cmd)
	}
}

func TestGetSetupCommand_ExplicitSetup(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "backend", Verify: workflowRepoVerify{Setup: "make deps"}},
		},
	}
	cmd := getSetupCommand(cfg, "backend")
	if cmd != "make deps" {
		t.Errorf("explicit setup = %q, want 'make deps'", cmd)
	}
}

func TestGetSetupCommand_InferredFromLanguage(t *testing.T) {
	cfg := &workflowConfig{
		Repos: []workflowRepo{
			{Name: "web", Language: "react"},
		},
	}
	cmd := getSetupCommand(cfg, "web")
	if !strings.Contains(cmd, "npm") {
		t.Errorf("inferred from react language = %q, want npm", cmd)
	}
}

// ---------------------------------------------------------------------------
// extractTitleLine
// ---------------------------------------------------------------------------

func TestExtractTitleLine_Simple(t *testing.T) {
	content := "# My Title\nBody here"
	got := extractTitleLine(content)
	if got != "My Title" {
		t.Errorf("extractTitleLine = %q, want 'My Title'", got)
	}
}

func TestExtractTitleLine_NoTitle(t *testing.T) {
	content := "Just a body\nNo heading here"
	got := extractTitleLine(content)
	if got != "" {
		t.Errorf("extractTitleLine with no heading = %q, want empty", got)
	}
}

func TestExtractTitleLine_EmptyTitle(t *testing.T) {
	content := "# \nBody"
	got := extractTitleLine(content)
	if got != "" {
		t.Errorf("extractTitleLine with empty heading = %q, want empty", got)
	}
}

func TestExtractTitleLine_MultipleHeadings(t *testing.T) {
	content := "# First\n# Second\n"
	got := extractTitleLine(content)
	if got != "First" {
		t.Errorf("extractTitleLine with multiple headings = %q, want 'First'", got)
	}
}

func TestExtractTitleLine_CarriageReturn(t *testing.T) {
	content := "# Title\r\nBody"
	got := extractTitleLine(content)
	if got != "Title" {
		t.Errorf("extractTitleLine with CRLF = %q, want 'Title'", got)
	}
}

// ---------------------------------------------------------------------------
// buildWorkDirInstruction
// ---------------------------------------------------------------------------

func TestBuildWorkDirInstruction_Directory(t *testing.T) {
	got := buildWorkDirInstruction("directory", "backend/", "/tmp/worktree", "backend")
	if !strings.Contains(got, "MONOREPO") {
		t.Errorf("directory type = %q, should contain MONOREPO", got)
	}
	if !strings.Contains(got, "/tmp/worktree") {
		t.Errorf("should contain workDir /tmp/worktree, got %q", got)
	}
}

func TestBuildWorkDirInstruction_Submodule(t *testing.T) {
	got := buildWorkDirInstruction("submodule", "backend/", "/tmp/worktree", "backend")
	if !strings.Contains(got, "SUBMODULE") {
		t.Errorf("submodule type = %q, should contain SUBMODULE", got)
	}
}

func TestBuildWorkDirInstruction_Root(t *testing.T) {
	// root type returns empty string
	got := buildWorkDirInstruction("root", "./", "/tmp/worktree", "root")
	if got != "" {
		t.Errorf("root type = %q, want empty", got)
	}
}

func TestBuildWorkDirInstruction_PathTraversal(t *testing.T) {
	// Path traversal should fall back to repoName
	got := buildWorkDirInstruction("directory", "../../etc", "/tmp/worktree", "backend")
	if strings.Contains(got, "../../") {
		t.Errorf("path traversal not sanitized, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// buildRetryConfig
// ---------------------------------------------------------------------------

func TestBuildRetryConfig_Defaults(t *testing.T) {
	cfg := buildRetryConfig(workflowTimeouts{})
	def := ghutil.DefaultRetryConfig()
	if cfg.MaxAttempts != def.MaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d (default)", cfg.MaxAttempts, def.MaxAttempts)
	}
}

func TestBuildRetryConfig_CustomRetryCount(t *testing.T) {
	cfg := buildRetryConfig(workflowTimeouts{GHRetryCount: 7})
	if cfg.MaxAttempts != 7 {
		t.Errorf("MaxAttempts = %d, want 7", cfg.MaxAttempts)
	}
}

func TestBuildRetryConfig_CustomBaseDelay(t *testing.T) {
	cfg := buildRetryConfig(workflowTimeouts{GHRetryBaseDelay: 5})
	want := 5 * time.Second
	if cfg.BaseDelay != want {
		t.Errorf("BaseDelay = %v, want %v", cfg.BaseDelay, want)
	}
}

func TestBuildRetryConfig_ZeroValuesUseDefaults(t *testing.T) {
	cfg := buildRetryConfig(workflowTimeouts{GHRetryCount: 0, GHRetryBaseDelay: 0})
	def := ghutil.DefaultRetryConfig()
	if cfg.MaxAttempts != def.MaxAttempts {
		t.Errorf("zero GHRetryCount should use default MaxAttempts, got %d", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != def.BaseDelay {
		t.Errorf("zero GHRetryBaseDelay should use default BaseDelay, got %v", cfg.BaseDelay)
	}
}

// ---------------------------------------------------------------------------
// IssueResult / LoadResult / WriteResultAtomic
// ---------------------------------------------------------------------------

func TestLoadResult_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadResult(dir, 99)
	if err == nil {
		t.Error("expected error for missing result file")
	}
}

func TestWriteResultAtomic_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	result := &IssueResult{
		IssueID: "42",
		Status:  "success",
		Repo:    "backend",
		PRURL:   "https://github.com/owner/repo/pull/1",
	}
	if err := WriteResultAtomic(dir, 42, result); err != nil {
		t.Fatalf("WriteResultAtomic: %v", err)
	}
	loaded, err := LoadResult(dir, 42)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if loaded.Status != "success" {
		t.Errorf("Status = %q, want success", loaded.Status)
	}
	if loaded.PRURL != "https://github.com/owner/repo/pull/1" {
		t.Errorf("PRURL = %q, want correct URL", loaded.PRURL)
	}
}

func TestWriteResultAtomic_PreservesPRURL(t *testing.T) {
	dir := t.TempDir()
	// Write first with a PR URL
	first := &IssueResult{
		IssueID: "10",
		Status:  "success",
		PRURL:   "https://github.com/owner/repo/pull/99",
	}
	if err := WriteResultAtomic(dir, 10, first); err != nil {
		t.Fatalf("WriteResultAtomic (first): %v", err)
	}
	// Write second without a PR URL — should preserve the existing one
	second := &IssueResult{
		IssueID: "10",
		Status:  "failed",
		PRURL:   "",
	}
	if err := WriteResultAtomic(dir, 10, second); err != nil {
		t.Fatalf("WriteResultAtomic (second): %v", err)
	}
	loaded, err := LoadResult(dir, 10)
	if err != nil {
		t.Fatalf("LoadResult: %v", err)
	}
	if loaded.PRURL != "https://github.com/owner/repo/pull/99" {
		t.Errorf("PRURL should be preserved; got %q", loaded.PRURL)
	}
}

// ---------------------------------------------------------------------------
// ExecutionTrace.GetStartedAtTime
// ---------------------------------------------------------------------------

func TestGetStartedAtTime_RFC3339(t *testing.T) {
	tr := &ExecutionTrace{StartedAt: "2024-01-15T09:30:00Z"}
	tm, err := tr.GetStartedAtTime()
	if err != nil {
		t.Fatalf("GetStartedAtTime: %v", err)
	}
	if tm.Year() != 2024 {
		t.Errorf("Year = %d, want 2024", tm.Year())
	}
}

func TestGetStartedAtTime_Empty(t *testing.T) {
	tr := &ExecutionTrace{StartedAt: ""}
	_, err := tr.GetStartedAtTime()
	if err == nil {
		t.Error("expected error for empty StartedAt")
	}
}

func TestGetStartedAtTime_InvalidFormat(t *testing.T) {
	tr := &ExecutionTrace{StartedAt: "not-a-date"}
	_, err := tr.GetStartedAtTime()
	if err == nil {
		t.Error("expected error for invalid date format")
	}
}

// ---------------------------------------------------------------------------
// ReadFailCount / ResetFailCount
// ---------------------------------------------------------------------------

func TestReadFailCount_MissingFile(t *testing.T) {
	dir := t.TempDir()
	count := ReadFailCount(dir, 99)
	if count != 0 {
		t.Errorf("ReadFailCount missing = %d, want 0", count)
	}
}

func TestReadFailCount_ValidFile(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, ".ai", "runs", "issue-5")
	os.MkdirAll(runDir, 0755)
	os.WriteFile(filepath.Join(runDir, "fail_count.txt"), []byte("3"), 0644)

	count := ReadFailCount(dir, 5)
	if count != 3 {
		t.Errorf("ReadFailCount = %d, want 3", count)
	}
}

func TestResetFailCount_ResetsToZero(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, ".ai", "runs", "issue-7")
	os.MkdirAll(runDir, 0755)
	os.WriteFile(filepath.Join(runDir, "fail_count.txt"), []byte("5"), 0644)

	if err := ResetFailCount(dir, 7); err != nil {
		t.Fatalf("ResetFailCount: %v", err)
	}
	count := ReadFailCount(dir, 7)
	if count != 0 {
		t.Errorf("after reset, count = %d, want 0", count)
	}
}

func TestResetFailCount_NoFile_NoError(t *testing.T) {
	dir := t.TempDir()
	// Should not error if file doesn't exist
	if err := ResetFailCount(dir, 99); err != nil {
		t.Errorf("ResetFailCount with missing file: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReadConsecutiveFailures / ResetConsecutiveFailures
// ---------------------------------------------------------------------------

func TestReadConsecutiveFailures_Missing(t *testing.T) {
	dir := t.TempDir()
	if n := ReadConsecutiveFailures(dir); n != 0 {
		t.Errorf("missing file = %d, want 0", n)
	}
}

func TestResetConsecutiveFailures(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	// Pre-populate with non-zero value
	os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("7"), 0644)

	if err := ResetConsecutiveFailures(dir); err != nil {
		t.Fatalf("ResetConsecutiveFailures: %v", err)
	}
	n := ReadConsecutiveFailures(dir)
	if n != 0 {
		t.Errorf("after reset = %d, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// LoadTrace
// ---------------------------------------------------------------------------

func TestLoadTrace_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadTrace(dir, 99)
	if err == nil {
		t.Error("expected error for missing trace file")
	}
}

func TestLoadTrace_ValidFile(t *testing.T) {
	dir := t.TempDir()
	traceDir := filepath.Join(dir, ".ai", "state", "traces")
	os.MkdirAll(traceDir, 0755)
	traceJSON := `{"trace_id":"abc","issue_id":"5","status":"success","started_at":"2024-01-15T10:00:00Z","steps":[]}`
	os.WriteFile(filepath.Join(traceDir, "issue-5.json"), []byte(traceJSON), 0644)

	tr, err := LoadTrace(dir, 5)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if tr.TraceID != "abc" {
		t.Errorf("TraceID = %q, want abc", tr.TraceID)
	}
	if tr.Status != "success" {
		t.Errorf("Status = %q, want success", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// workerLogger
// ---------------------------------------------------------------------------

func TestWorkerLogger_NilFiles(t *testing.T) {
	// Should not panic when files are nil
	l := &workerLogger{}
	l.Log("test %s", "message") // no-op
	if err := l.Close(); err != nil {
		t.Errorf("Close on nil logger: %v", err)
	}
}

func TestNewWorkerLogger_EmptyPaths(t *testing.T) {
	l := newWorkerLogger("", "")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	if l.file != nil {
		t.Error("file should be nil for empty path")
	}
	if l.summary != nil {
		t.Error("summary should be nil for empty path")
	}
}

func TestNewWorkerLogger_WithFiles(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker.log")
	sumPath := filepath.Join(dir, "summary.log")

	l := newWorkerLogger(logPath, sumPath)
	defer l.Close()

	if l.file == nil {
		t.Error("file should be non-nil")
	}
	if l.summary == nil {
		t.Error("summary should be non-nil")
	}

	l.Log("hello %s", "world")
	l.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log file content = %q, want to contain 'hello world'", string(data))
	}
}

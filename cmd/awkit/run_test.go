package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

func TestExtractTextFromStreamJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "non-json text",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "system init event",
			input:    `{"type":"system","subtype":"init","cwd":"/test","session_id":"abc123"}`,
			expected: "",
		},
		{
			name:     "assistant message with text (skipped - Claude narration)",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello from Claude!"}]}}`,
			expected: "",
		},
		{
			name:     "assistant message with multiple text blocks (skipped)",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"Line 1"},{"type":"text","text":"Line 2"}]}}`,
			expected: "",
		},
		{
			name:     "result event (should be skipped)",
			input:    `{"type":"result","subtype":"success","result":"Final result text"}`,
			expected: "",
		},
		{
			name:     "invalid json",
			input:    `{invalid json}`,
			expected: "",
		},
		{
			name:     "assistant message without content",
			input:    `{"type":"assistant","message":{}}`,
			expected: "",
		},
		{
			name:     "tool_use Bash command (capital B)",
			input:    `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"bash .ai/scripts/dispatch_worker.sh 15"}}]}}`,
			expected: "[EXEC] bash .ai/scripts/dispatch_worker.sh 15",
		},
		{
			name:     "tool_use bash command (lowercase)",
			input:    `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"bash","input":{"command":"go test ./..."}}]}}`,
			expected: "[EXEC] go test ./...",
		},
		{
			name:     "user message with tool_result (skipped - tailers handle logs)",
			input:    `{"type":"user","message":{"content":[{"type":"tool_result","content":"[WORKER] worker_session_id=worker-123\nWorker completed"}]}}`,
			expected: "",
		},
		{
			name:     "user message with tool_result and whitespace (skipped)",
			input:    `{"type":"user","message":{"content":[{"type":"tool_result","content":"[PRINCIPAL] 10:00:05 | test\r\n"}]}}`,
			expected: "",
		},
		{
			name:     "mixed text and tool_use in assistant (only EXEC)",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"Running dispatch..."},{"type":"tool_use","name":"Bash","input":{"command":"dispatch_worker.sh 10"}}]}}`,
			expected: "[EXEC] dispatch_worker.sh 10",
		},
		{
			name:     "content_block_delta with text (skipped - Claude narration)",
			input:    `{"type":"content_block_delta","delta":{"text":"streaming text"}}`,
			expected: "",
		},
		{
			name:     "content_block_delta without text",
			input:    `{"type":"content_block_delta","delta":{}}`,
			expected: "",
		},
		{
			name:     "tool_use non-bash (should be ignored)",
			input:    `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"test.txt"}}]}}`,
			expected: "",
		},
		{
			// A PTY (Windows ConPTY) injects ANSI escapes around/into the
			// stream; without stripping, json.Unmarshal fails and the event is
			// lost. With stripping, the command still extracts.
			name:     "ANSI-wrapped tool_use Bash still extracts EXEC",
			input:    "\x1b[1m{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"tool_use\",\"name\":\"Bash\",\"input\":{\"command\":\"awkit analyze-next --json\"}}]}}\x1b[0m",
			expected: "[EXEC] awkit analyze-next --json",
		},
		{
			name:     "ANSI-only control line is skipped, not dumped",
			input:    "\x1b[2K\x1b[1G",
			expected: "",
		},
		{
			name:     "JSON with a trailing carriage return is parsed",
			input:    "{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"tool_use\",\"name\":\"bash\",\"input\":{\"command\":\"go build ./...\"}}]}}\r",
			expected: "[EXEC] go build ./...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTextFromStreamJSON(tt.input)
			if result != tt.expected {
				t.Errorf("extractTextFromStreamJSON(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeStreamLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json untouched", `{"a":1}`, `{"a":1}`},
		{"strips SGR color", "\x1b[1mhello\x1b[0m", "hello"},
		{"strips cursor moves", "\x1b[2K\x1b[1Gtext", "text"},
		{"strips carriage return", "line\r", "line"},
		{"strips NUL bytes", "a\x00b", "ab"},
		{"trims surrounding space", "  {\"x\":1}  ", `{"x":1}`},
		{"ansi-only becomes empty", "\x1b[2K\x1b[1G", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeStreamLine(tt.input); got != tt.want {
				t.Errorf("sanitizeStreamLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSummarizeCommand(t *testing.T) {
	// Short single-line command is unchanged.
	if got := summarizeCommand("awkit analyze-next --json"); got != "awkit analyze-next --json" {
		t.Errorf("short cmd changed: %q", got)
	}
	// A multi-line heredoc collapses to its first line; the body must not leak.
	got := summarizeCommand("cat > /tmp/x.md << 'EOF'\n### huge review body\nline2\nEOF")
	if !strings.HasPrefix(got, "cat > /tmp/x.md << 'EOF'") {
		t.Errorf("heredoc first line lost: %q", got)
	}
	if strings.Contains(got, "review body") || strings.Contains(got, "line2") {
		t.Errorf("heredoc body leaked into [EXEC]: %q", got)
	}
	// A very long single line is truncated to bounded output.
	if got := summarizeCommand("echo " + strings.Repeat("x", 500)); len([]rune(got)) > 210 {
		t.Errorf("long cmd not truncated: %d runes", len([]rune(got)))
	}
}

func TestGetEnvString(t *testing.T) {
	const key = "AWKIT_TEST_PERMISSION_MODE"

	// Empty value falls back to the default.
	t.Setenv(key, "")
	if got := getEnvString(key, "auto"); got != "auto" {
		t.Errorf("empty env: got %q, want auto", got)
	}

	// Surrounding whitespace is trimmed.
	t.Setenv(key, "  bypassPermissions  ")
	if got := getEnvString(key, "auto"); got != "bypassPermissions" {
		t.Errorf("override: got %q, want bypassPermissions", got)
	}

	// An unset variable returns the default.
	if got := getEnvString("AWKIT_DEFINITELY_UNSET_KEY", "auto"); got != "auto" {
		t.Errorf("unset env: got %q, want auto", got)
	}
}

func TestWorkflowLabelSpecs(t *testing.T) {
	labels := analyzer.DefaultLabels()
	specs := workflowLabelSpecs(labels)

	byName := make(map[string]worker.LabelSpec, len(specs))
	for _, s := range specs {
		byName[s.Name] = s
	}

	// Every state-machine label the workflow relies on must have a spec, or it
	// won't be created and gh edits against it will fail with 'not found'.
	want := []string{
		labels.Task, labels.InProgress, labels.PRReady, labels.WorkerFailed,
		labels.NeedsHumanReview, labels.ReviewFailed, labels.MergeConflict,
		labels.NeedsRebase, labels.Completed,
	}
	for _, name := range want {
		s, ok := byName[name]
		if !ok {
			t.Errorf("label %q has no spec", name)
			continue
		}
		if len(s.Color) != 6 {
			t.Errorf("label %q color %q must be 6-hex", name, s.Color)
		}
		if s.Description == "" {
			t.Errorf("label %q has an empty description", name)
		}
	}
}

func TestFormatAnalyzeNextContext(t *testing.T) {
	ctx := formatAnalyzeNextContext(analyzeNextVars{
		NextAction:  "dispatch_worker",
		IssueNumber: "7",
		PRNumber:    "",
		ExitReason:  "",
		MergeIssue:  "",
	})
	if ctx != " (issue=7)" {
		t.Fatalf("formatAnalyzeNextContext() = %q, want %q", ctx, " (issue=7)")
	}
	if ctx := formatAnalyzeNextContext(analyzeNextVars{NextAction: "all_complete"}); ctx != "" {
		t.Fatalf("formatAnalyzeNextContext() with nothing to name = %q, want nothing", ctx)
	}
}

func TestFormatAnalyzeNextContextWithMergeIssue(t *testing.T) {
	ctx := formatAnalyzeNextContext(analyzeNextVars{
		NextAction:  "dispatch_worker",
		IssueNumber: "27",
		PRNumber:    "30",
		MergeIssue:  "conflict",
	})
	if ctx != " (issue=27 pr=30 merge=conflict)" {
		t.Fatalf("formatAnalyzeNextContext() = %q, want %q", ctx, " (issue=27 pr=30 merge=conflict)")
	}
}

// A run for one ticket says so to the Principal. A run for all of them says
// nothing about a limit.
func TestPrincipalPromptFor(t *testing.T) {
	if got := principalPromptFor(0); got != principalPrompt {
		t.Errorf("principalPromptFor(0) = %q, want the plain prompt", got)
	}
	got := principalPromptFor(42)
	if !strings.HasPrefix(got, principalPrompt) {
		t.Errorf("the prompt of a limited run lost the plain prompt: %q", got)
	}
	for _, want := range []string{"limited to issue #42", "awkit analyze-next"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt of a limited run should say %q: %q", want, got)
		}
	}
}

// A run can be limited to an open issue that is a ticket, and to nothing else.
func TestRunTicketProblem(t *testing.T) {
	tests := []struct {
		name        string
		info        worker.IssueInfo
		wantProblem string
	}{
		{"an open ticket", worker.IssueInfo{Number: 7, State: "OPEN", Labels: []string{"bug", "ai-task"}}, ""},
		{"a ticket in any state of the workflow", worker.IssueInfo{Number: 7, State: "OPEN", Labels: []string{"ai-task", "pr-ready"}}, ""},
		{"a closed ticket", worker.IssueInfo{Number: 7, State: "CLOSED", Labels: []string{"ai-task"}}, "issue #7 is closed"},
		{"an issue that is not a ticket", worker.IssueInfo{Number: 7, State: "OPEN", Labels: []string{"bug"}}, "issue #7 is not a ticket: it does not carry the `ai-task` label"},
		{"an issue without labels", worker.IssueInfo{Number: 7, State: "OPEN"}, "is not a ticket"},
	}
	for _, tt := range tests {
		got := runTicketProblem(&tt.info, "ai-task")
		if (tt.wantProblem == "") != (got == "") || !strings.Contains(got, tt.wantProblem) {
			t.Errorf("%s: problem = %q, want %q", tt.name, got, tt.wantProblem)
		}
	}
	// The label is the project's own.
	custom := worker.IssueInfo{Number: 7, State: "OPEN", Labels: []string{"work-item"}}
	if got := runTicketProblem(&custom, "work-item"); got != "" {
		t.Errorf("a ticket under the project's own label was refused: %q", got)
	}
	if got := runTicketProblem(&custom, "ai-task"); !strings.Contains(got, "`ai-task` label") {
		t.Errorf("problem = %q, want it to name the label that is missing", got)
	}
}

// analyze-next refuses a limit it cannot read, from the flag or from the run
// it was started in, before it decides anything.
func TestCmdAnalyzeNext_LimitMustBeAnIssueNumber(t *testing.T) {
	dir := t.TempDir()
	if exitCode := cmdAnalyzeNext([]string{"--issue", "-1", "--state-root", dir}); exitCode != 2 {
		t.Errorf("cmdAnalyzeNext(--issue -1) = %d, want 2", exitCode)
	}
	if exitCode := cmdAnalyzeNext([]string{"--issue", "abc", "--state-root", dir}); exitCode != 2 {
		t.Errorf("cmdAnalyzeNext(--issue abc) = %d, want 2", exitCode)
	}
	t.Setenv(analyzer.ScopeEnv, "not-a-number")
	if exitCode := cmdAnalyzeNext([]string{"--state-root", dir}); exitCode != 1 {
		t.Errorf("cmdAnalyzeNext with %s=not-a-number = %d, want 1", analyzer.ScopeEnv, exitCode)
	}
}

// The limit of a run reaches the decision engine from the flag, or from the
// run the command was started in. The flag has the last word.
func TestAnalyzerFor(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name      string
		env       string
		flag      int
		wantLimit int
	}{
		{"no limit", "", 0, 0},
		{"the limit of the run", "12", 0, 12},
		{"the flag", "", 7, 7},
		{"the flag over the run's", "12", 7, 7},
	}
	for _, tt := range tests {
		t.Setenv(analyzer.ScopeEnv, tt.env)
		a, err := analyzerFor(dir, tt.flag)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got := a.Limit(); got != tt.wantLimit {
			t.Errorf("%s: the engine decides for #%d, want #%d", tt.name, got, tt.wantLimit)
		}
	}

	// A limit that cannot be read is an error, not a run without one.
	t.Setenv(analyzer.ScopeEnv, "twelve")
	if a, err := analyzerFor(dir, 0); err == nil {
		t.Errorf("an unreadable limit was taken for #%d", a.Limit())
	}
	// The flag does not need the environment to make sense.
	if a, err := analyzerFor(dir, 7); err != nil || a.Limit() != 7 {
		t.Errorf("with the flag: %v", err)
	}
}

// awkit run --issue N tells the commands it starts; a run without the flag
// tells them there is no limit, whatever the shell had set.
func TestSetRunScope(t *testing.T) {
	t.Setenv(analyzer.ScopeEnv, "99")

	setRunScope(42)
	if got, err := analyzer.ScopeFromEnv(); err != nil || got != 42 {
		t.Errorf("after setRunScope(42): the limit is %d, %v", got, err)
	}

	setRunScope(0)
	if got, err := analyzer.ScopeFromEnv(); err != nil || got != 0 {
		t.Errorf("after setRunScope(0): the limit is %d, %v; want none", got, err)
	}
}

// A Worker may take an hour, and `awkit dispatch-worker` waits for it. The
// shell tool of the session a run starts gives a command two minutes and then
// ten more in the background before it stops it, unless these two settings
// say otherwise. Measured with Claude Code 2.1.321: a 13 minute command is
// killed with the defaults and finishes with both settings at 15 minutes.
func TestSetPrincipalShellTimeouts(t *testing.T) {
	want := strconv.FormatInt((worker.DefaultWorkerTimeout + 5*time.Minute).Milliseconds(), 10)
	if want != "3900000" {
		t.Fatalf("the shell timeout is %s ms, want 65 minutes: a dispatch waits %s for its Worker", want, worker.DefaultWorkerTimeout)
	}

	for _, tc := range []struct {
		name, before, after string
		set                 bool
	}{
		{name: "not set", after: want},
		{name: "the default of two minutes", before: "120000", set: true, after: want},
		{name: "just below", before: "3899999", set: true, after: want},
		{name: "exactly as long", before: want, set: true, after: want},
		{name: "longer is the user's", before: "7200000", set: true, after: "7200000"},
		{name: "longer, with a space", before: " 7200000 ", set: true, after: " 7200000 "},
		{name: "not a number", before: "soon", set: true, after: want},
		{name: "empty", before: "", set: true, after: want},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range shellTimeoutSettings {
				t.Setenv(name, tc.before)
				if !tc.set {
					os.Unsetenv(name)
				}
			}
			setPrincipalShellTimeouts()
			for _, name := range shellTimeoutSettings {
				if got := os.Getenv(name); got != tc.after {
					t.Errorf("%s = %q, want %q", name, got, tc.after)
				}
			}
		})
	}

	// Both settings, and each on its own: the default is what a command gets
	// when the model names no timeout, the maximum is what caps it.
	if len(shellTimeoutSettings) != 2 || shellTimeoutSettings[0] != "BASH_DEFAULT_TIMEOUT_MS" || shellTimeoutSettings[1] != "BASH_MAX_TIMEOUT_MS" {
		t.Errorf("the settings are %v", shellTimeoutSettings)
	}
	t.Setenv("BASH_DEFAULT_TIMEOUT_MS", "7200000")
	t.Setenv("BASH_MAX_TIMEOUT_MS", "600000")
	setPrincipalShellTimeouts()
	if got := os.Getenv("BASH_DEFAULT_TIMEOUT_MS"); got != "7200000" {
		t.Errorf("BASH_DEFAULT_TIMEOUT_MS = %q, want the user's", got)
	}
	if got := os.Getenv("BASH_MAX_TIMEOUT_MS"); got != want {
		t.Errorf("BASH_MAX_TIMEOUT_MS = %q, want %s: it was below what a Worker may take", got, want)
	}
}

package main

import (
	"strings"
	"testing"

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

func TestSameDecision(t *testing.T) {
	base := analyzeNextVars{NextAction: "create_task", SpecName: "tennis-arena", TaskLine: "15"}

	if !sameDecision(base, analyzeNextVars{NextAction: "create_task", SpecName: "tennis-arena", TaskLine: "15"}) {
		t.Error("identical decisions should compare equal")
	}

	// Any single differing field breaks equality.
	diffs := map[string]analyzeNextVars{
		"action": {NextAction: "dispatch_worker", SpecName: "tennis-arena", TaskLine: "15"},
		"line":   {NextAction: "create_task", SpecName: "tennis-arena", TaskLine: "16"},
		"spec":   {NextAction: "create_task", SpecName: "other", TaskLine: "15"},
		"issue":  {NextAction: "create_task", SpecName: "tennis-arena", TaskLine: "15", IssueNumber: "7"},
	}
	for name, d := range diffs {
		if sameDecision(base, d) {
			t.Errorf("differing %s should not compare equal", name)
		}
	}
}

func TestIsCreationAction(t *testing.T) {
	for _, a := range []string{"create_task", "generate_tasks", "audit_epic"} {
		if !isCreationAction(a) {
			t.Errorf("%q should be a creation action", a)
		}
	}
	for _, a := range []string{"dispatch_worker", "check_result", "review_pr", "all_complete", "none", ""} {
		if isCreationAction(a) {
			t.Errorf("%q should not be a creation action", a)
		}
	}
}

func TestFormatAnalyzeNextContext(t *testing.T) {
	ctx := formatAnalyzeNextContext(analyzeNextVars{
		NextAction:  "create_task",
		SpecName:    "snake-arena",
		TaskLine:    "7",
		IssueNumber: "",
		PRNumber:    "",
		ExitReason:  "",
		MergeIssue:  "",
	})
	if ctx != " (spec=snake-arena line=7)" {
		t.Fatalf("formatAnalyzeNextContext() = %q, want %q", ctx, " (spec=snake-arena line=7)")
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

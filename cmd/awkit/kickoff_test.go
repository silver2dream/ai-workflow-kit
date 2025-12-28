package main

import (
	"testing"
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

func TestParseAnalyzeNextOutput(t *testing.T) {
	out := `
NEXT_ACTION=create_task
ISSUE_NUMBER=
PR_NUMBER=
SPEC_NAME=snake-arena
TASK_LINE=7
EXIT_REASON=
`

	got := parseAnalyzeNextOutput(out)
	if got.NextAction != "create_task" {
		t.Fatalf("NextAction = %q, want %q", got.NextAction, "create_task")
	}
	if got.SpecName != "snake-arena" {
		t.Fatalf("SpecName = %q, want %q", got.SpecName, "snake-arena")
	}
	if got.TaskLine != "7" {
		t.Fatalf("TaskLine = %q, want %q", got.TaskLine, "7")
	}
	if got.ExitReason != "" {
		t.Fatalf("ExitReason = %q, want empty", got.ExitReason)
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
	})
	if ctx != " (spec=snake-arena line=7)" {
		t.Fatalf("formatAnalyzeNextContext() = %q, want %q", ctx, " (spec=snake-arena line=7)")
	}
}

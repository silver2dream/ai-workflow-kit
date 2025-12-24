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
			name:     "assistant message with text",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello from Claude!"}]}}`,
			expected: "Hello from Claude!",
		},
		{
			name:     "assistant message with multiple text blocks",
			input:    `{"type":"assistant","message":{"content":[{"type":"text","text":"Line 1"},{"type":"text","text":"Line 2"}]}}`,
			expected: "Line 1\nLine 2",
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

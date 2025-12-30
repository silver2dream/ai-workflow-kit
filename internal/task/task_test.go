package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasIssueRef(t *testing.T) {
	tests := []struct {
		line  string
		num   int
		found bool
	}{
		{"- [ ] Task <!-- Issue #123 -->", 123, true},
		{"- [ ] Task <!-- Issue #456 -->", 456, true},
		{"- [ ] Task without ref", 0, false},
		{"- [x] Completed <!-- Issue #789 -->", 789, true},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			num, found := HasIssueRef(tt.line)
			if found != tt.found {
				t.Errorf("HasIssueRef(%q) found = %v, want %v", tt.line, found, tt.found)
			}
			if found && num != tt.num {
				t.Errorf("HasIssueRef(%q) num = %d, want %d", tt.line, num, tt.num)
			}
		})
	}
}

func TestExtractTaskText(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"- [ ] Simple task", "Simple task"},
		{"- [x] Completed task", "Completed task"},
		{"- [ ] Task <!-- Issue #123 -->", "Task"},
		{"  - [ ] Indented task", "Indented task"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := ExtractTaskText(tt.line)
			got = strings.TrimSpace(got)
			if got != tt.want {
				t.Errorf("ExtractTaskText(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestDefaultIssueTitle(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"implement feature X", "[feat] implement feature x"},
		{"fix bug in Y", "[feat] fix bug in y"},
		{"add tests for Z", "[feat] add tests for z"},
		{"update documentation", "[feat] update documentation"},
		{"refactor module A", "[feat] refactor module a"},
		{"some other task", "[feat] some other task"},
		{"[fix] already prefixed", "[fix] already prefixed"},
		{"", "[feat] implement task"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := DefaultIssueTitle(tt.text)
			if got != tt.want {
				t.Errorf("DefaultIssueTitle(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestValidateBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "valid body with all sections",
			body: `## Summary
Test summary
## Scope
Test scope
## Acceptance Criteria
- [ ] Test checkbox
## Testing Requirements
Test requirements
## Metadata
Test metadata`,
			wantErr: false,
		},
		{
			name:    "empty body",
			body:    "",
			wantErr: true,
		},
		{
			name: "missing sections",
			body: `## Summary
Test`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBody(tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBody() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnsureAWKMetadata(t *testing.T) {
	body := "## Summary\nTest"
	result := EnsureAWKMetadata(body, "test-spec", 5)

	if !strings.Contains(result, "**Spec**: test-spec") {
		t.Error("should contain Spec")
	}
	if !strings.Contains(result, "**Task Line**: 5") {
		t.Error("should contain Task Line")
	}
}

func TestAppendIssueRef(t *testing.T) {
	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "tasks.md")

	content := "# Tasks\n- [ ] First task\n- [ ] Second task\n"
	os.WriteFile(tasksPath, []byte(content), 0644)

	err := AppendIssueRef(tasksPath, 2, 123)
	if err != nil {
		t.Fatalf("AppendIssueRef() error = %v", err)
	}

	updated, _ := os.ReadFile(tasksPath)
	if !strings.Contains(string(updated), "<!-- Issue #123 -->") {
		t.Error("should contain issue reference")
	}
}

func TestParseIssueOutput(t *testing.T) {
	tests := []struct {
		output  string
		wantNum int
		wantErr bool
	}{
		{
			output:  "https://github.com/owner/repo/issues/123",
			wantNum: 123,
			wantErr: false,
		},
		{
			output:  "Created issue #456\nhttps://github.com/owner/repo/issues/456",
			wantNum: 456,
			wantErr: false,
		},
		{
			output:  "no issue number here",
			wantNum: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			num, _, err := parseIssueOutput(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIssueOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && num != tt.wantNum {
				t.Errorf("parseIssueOutput() num = %d, want %d", num, tt.wantNum)
			}
		})
	}
}

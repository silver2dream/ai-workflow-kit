package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTicketMetadata(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected *TicketMetadata
	}{
		{
			name: "basic metadata with bold format",
			body: `# [bug] Fix login issue

- **Repo**: backend
- **Severity**: P0
- **Source**: audit:finding-123
- **Release**: true

## Objective
Fix the login issue.
`,
			expected: &TicketMetadata{
				Repo:     "backend",
				Severity: "P0",
				Source:   "audit:finding-123",
				Release:  true,
			},
		},
		{
			name: "metadata with spec and task line",
			body: `# [feat] Add feature

- **Repo**: frontend
- **Severity**: P1
- **Spec**: my-project
- **Task Line**: 42

## Objective
Add a new feature.
`,
			expected: &TicketMetadata{
				Repo:     "frontend",
				Severity: "P1",
				SpecName: "my-project",
				TaskLine: 42,
			},
		},
		{
			name: "default repo when not specified",
			body: `# [chore] Update docs

## Objective
Update documentation.
`,
			expected: &TicketMetadata{
				Repo: "root",
			},
		},
		{
			name: "constraints with special flags",
			body: `# [fix] Dangerous fix

- **Repo**: backend

## Constraints
- obey AGENTS.md
- allow-parent-changes
- allow-script-changes
- allow-secrets

## Objective
Fix something dangerous.
`,
			expected: &TicketMetadata{
				Repo:               "backend",
				AllowParentChanges: true,
				AllowScriptChanges: true,
				AllowSecrets:       true,
			},
		},
		{
			name: "spec with path traversal should be ignored",
			body: `- **Spec**: ../malicious
- **Task Line**: 10
`,
			expected: &TicketMetadata{
				Repo:     "root",
				TaskLine: 10,
				// SpecName should be empty due to path traversal
			},
		},
		{
			name: "alternative format without bold",
			body: `- Repo: backend
- Severity: P2
`,
			expected: &TicketMetadata{
				Repo:     "backend",
				Severity: "P2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseTicketMetadata(tt.body)

			if result.Repo != tt.expected.Repo {
				t.Errorf("Repo = %q, want %q", result.Repo, tt.expected.Repo)
			}
			if result.Severity != tt.expected.Severity {
				t.Errorf("Severity = %q, want %q", result.Severity, tt.expected.Severity)
			}
			if result.Source != tt.expected.Source {
				t.Errorf("Source = %q, want %q", result.Source, tt.expected.Source)
			}
			if result.Release != tt.expected.Release {
				t.Errorf("Release = %v, want %v", result.Release, tt.expected.Release)
			}
			if result.SpecName != tt.expected.SpecName {
				t.Errorf("SpecName = %q, want %q", result.SpecName, tt.expected.SpecName)
			}
			if result.TaskLine != tt.expected.TaskLine {
				t.Errorf("TaskLine = %d, want %d", result.TaskLine, tt.expected.TaskLine)
			}
			if result.AllowParentChanges != tt.expected.AllowParentChanges {
				t.Errorf("AllowParentChanges = %v, want %v", result.AllowParentChanges, tt.expected.AllowParentChanges)
			}
			if result.AllowScriptChanges != tt.expected.AllowScriptChanges {
				t.Errorf("AllowScriptChanges = %v, want %v", result.AllowScriptChanges, tt.expected.AllowScriptChanges)
			}
			if result.AllowSecrets != tt.expected.AllowSecrets {
				t.Errorf("AllowSecrets = %v, want %v", result.AllowSecrets, tt.expected.AllowSecrets)
			}
		})
	}
}

func TestSaveAndLoadTicketFile(t *testing.T) {
	tmpDir := t.TempDir()

	body := `# [bug] Fix issue

- **Repo**: backend

## Objective
Fix the issue.
`

	// Save ticket
	ticketPath, err := SaveTicketFile(tmpDir, 123, body)
	if err != nil {
		t.Fatalf("SaveTicketFile failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".ai", "temp", "ticket-123.md")
	if ticketPath != expectedPath {
		t.Errorf("ticketPath = %q, want %q", ticketPath, expectedPath)
	}

	// Verify file exists
	if _, err := os.Stat(ticketPath); os.IsNotExist(err) {
		t.Error("ticket file does not exist")
	}

	// Load ticket
	loaded, err := LoadTicketFile(tmpDir, 123)
	if err != nil {
		t.Fatalf("LoadTicketFile failed: %v", err)
	}

	if loaded != body {
		t.Errorf("loaded content mismatch")
	}

	// Cleanup
	if err := CleanupTicketFile(tmpDir, 123); err != nil {
		t.Errorf("CleanupTicketFile failed: %v", err)
	}

	// Verify cleanup
	if _, err := os.Stat(ticketPath); !os.IsNotExist(err) {
		t.Error("ticket file should have been removed")
	}

	// Cleanup non-existent file should not error
	if err := CleanupTicketFile(tmpDir, 999); err != nil {
		t.Errorf("CleanupTicketFile for non-existent file should not error: %v", err)
	}
}

func TestLoadTicketFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := LoadTicketFile(tmpDir, 999)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist error, got %v", err)
	}
}

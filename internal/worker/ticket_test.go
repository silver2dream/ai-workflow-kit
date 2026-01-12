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

func TestParseVerificationCommands(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected []VerificationCommands
	}{
		{
			name: "basic verification section",
			body: `# [bug] Fix issue

## Verification
- backend: ` + "`go build ./...`" + ` and ` + "`go test ./...`" + `
- frontend: ` + "`npm run build`" + ` and ` + "`npm run test`" + `

## Acceptance Criteria
- [ ] Tests pass
`,
			expected: []VerificationCommands{
				{Repo: "backend", Commands: []string{"go build ./...", "go test ./..."}},
				{Repo: "frontend", Commands: []string{"npm run build", "npm run test"}},
			},
		},
		{
			name: "single command per repo",
			body: `## Verification
- root: ` + "`make test`" + `
`,
			expected: []VerificationCommands{
				{Repo: "root", Commands: []string{"make test"}},
			},
		},
		{
			name: "no verification section",
			body: `# [bug] Fix issue

## Objective
Fix something.
`,
			expected: []VerificationCommands{},
		},
		{
			name: "empty verification section",
			body: `## Verification

## Next Section
`,
			expected: []VerificationCommands{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseVerificationCommands(tt.body)

			if len(result) != len(tt.expected) {
				t.Fatalf("got %d entries, want %d", len(result), len(tt.expected))
			}

			for i, exp := range tt.expected {
				if result[i].Repo != exp.Repo {
					t.Errorf("entry %d: Repo = %q, want %q", i, result[i].Repo, exp.Repo)
				}
				if len(result[i].Commands) != len(exp.Commands) {
					t.Errorf("entry %d: got %d commands, want %d", i, len(result[i].Commands), len(exp.Commands))
					continue
				}
				for j, cmd := range exp.Commands {
					if result[i].Commands[j] != cmd {
						t.Errorf("entry %d, cmd %d: got %q, want %q", i, j, result[i].Commands[j], cmd)
					}
				}
			}
		})
	}
}

func TestGetVerificationCommandsForRepo(t *testing.T) {
	body := `## Verification
- backend: ` + "`go test ./...`" + `
- frontend: ` + "`npm test`" + `
- root: ` + "`make check`" + `
`

	tests := []struct {
		repo     string
		expected []string
	}{
		{"backend", []string{"go test ./..."}},
		{"frontend", []string{"npm test"}},
		{"root", []string{"make check"}},
		{"", []string{"make check"}},       // empty defaults to root
		{"unknown", nil},                   // no match
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			result := GetVerificationCommandsForRepo(body, tt.repo)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("got %d commands, want %d", len(result), len(tt.expected))
			}

			for i, cmd := range tt.expected {
				if result[i] != cmd {
					t.Errorf("cmd %d: got %q, want %q", i, result[i], cmd)
				}
			}
		})
	}
}

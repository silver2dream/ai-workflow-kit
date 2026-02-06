package reviewer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetTestCommandFromConfig(t *testing.T) {
	tests := []struct {
		name         string
		configYAML   string
		worktreePath string
		want         string
	}{
		{
			name:         "no config file",
			configYAML:   "",
			worktreePath: "/some/worktree",
			want:         "", // Now returns empty when config not found
		},
		{
			name: "root type repo",
			configYAML: `repos:
  - name: myproject
    path: "./"
    type: root
    verify:
      test: "go test ./..."
`,
			worktreePath: "/some/worktree",
			want:         "go test ./...",
		},
		{
			name: "directory type repo",
			configYAML: `repos:
  - name: backend
    path: "backend"
    type: directory
    verify:
      test: "go test ./..."
`,
			worktreePath: "/some/worktree/backend",
			want:         "cd backend && go test ./...",
		},
		{
			name: "submodule type repo",
			configYAML: `repos:
  - name: frontend
    path: "frontend"
    type: submodule
    verify:
      test: "npm test"
`,
			worktreePath: "/some/worktree/frontend",
			want:         "cd frontend && npm test",
		},
		{
			name: "multiple repos - match by path",
			configYAML: `repos:
  - name: backend
    path: "backend"
    type: directory
    verify:
      test: "go test ./..."
  - name: frontend
    path: "frontend"
    type: directory
    verify:
      test: "npm test"
`,
			worktreePath: "/some/worktree/frontend/issue-1",
			want:         "cd frontend && npm test",
		},
		{
			name: "multiple repos - match by name",
			configYAML: `repos:
  - name: backend
    path: "be"
    type: directory
    verify:
      test: "go test ./..."
  - name: frontend
    path: "fe"
    type: directory
    verify:
      test: "npm test"
`,
			worktreePath: "/some/worktree/backend/issue-1",
			want:         "cd be && go test ./...",
		},
		{
			name: "no matching repo - returns empty (no false positive)",
			configYAML: `repos:
  - name: backend
    path: "backend"
    type: directory
    verify:
      test: "go test ./..."
`,
			worktreePath: "/unrelated/path",
			want:         "", // Should NOT fallback to first repo - this was the bug!
		},
		{
			name: "worktree NOT_FOUND - returns empty",
			configYAML: `repos:
  - name: backend
    path: "backend"
    type: directory
    verify:
      test: "go test ./..."
`,
			worktreePath: "NOT_FOUND",
			want:         "", // Should NOT fallback to first repo
		},
		{
			name:         "invalid yaml",
			configYAML:   "invalid: yaml: content: [",
			worktreePath: "/some/path",
			want:         "", // Now returns empty when config parsing fails
		},
		{
			name: "repo without test command",
			configYAML: `repos:
  - name: docs
    path: "docs"
    type: directory
    verify:
      build: "mkdocs build"
`,
			worktreePath: "/some/path",
			want:         "", // Now returns empty when no repo has test command
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.configYAML != "" {
				configDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(configDir, 0755); err != nil {
					t.Fatalf("failed to create config dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(tt.configYAML), 0644); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
			}

			got := getTestCommandFromConfig(tmpDir, tt.worktreePath)
			if got != tt.want {
				t.Errorf("getTestCommandFromConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetRepoSettingsFromConfig(t *testing.T) {
	tests := []struct {
		name         string
		configYAML   string
		worktreePath string
		wantCmd      string
		wantLang     string
	}{
		{
			name:         "no config file returns unknown language",
			configYAML:   "",
			worktreePath: "/some/worktree",
			wantCmd:      "",
			wantLang:     "unknown",
		},
		{
			name:         "invalid yaml returns unknown language",
			configYAML:   "invalid: yaml: content: [",
			worktreePath: "/some/path",
			wantCmd:      "",
			wantLang:     "unknown",
		},
		{
			name: "go repo returns go language",
			configYAML: `repos:
  - name: backend
    path: "backend"
    type: directory
    language: go
    verify:
      test: "go test ./..."
`,
			worktreePath: "/some/worktree/backend",
			wantCmd:      "cd backend && go test ./...",
			wantLang:     "go",
		},
		{
			name: "typescript repo returns typescript language",
			configYAML: `repos:
  - name: frontend
    path: "frontend"
    type: directory
    language: typescript
    verify:
      test: "npm test"
`,
			worktreePath: "/some/worktree/frontend",
			wantCmd:      "cd frontend && npm test",
			wantLang:     "typescript",
		},
		{
			name: "no matching repo returns unknown language",
			configYAML: `repos:
  - name: docs
    path: "docs"
    type: directory
    verify:
      build: "mkdocs build"
`,
			worktreePath: "/some/path",
			wantCmd:      "",
			wantLang:     "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.configYAML != "" {
				configDir := filepath.Join(tmpDir, ".ai", "config")
				if err := os.MkdirAll(configDir, 0755); err != nil {
					t.Fatalf("failed to create config dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(tt.configYAML), 0644); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
			}

			settings := getRepoSettingsFromConfig(tmpDir, tt.worktreePath)
			if settings.TestCommand != tt.wantCmd {
				t.Errorf("TestCommand = %q, want %q", settings.TestCommand, tt.wantCmd)
			}
			if settings.Language != tt.wantLang {
				t.Errorf("Language = %q, want %q", settings.Language, tt.wantLang)
			}
		})
	}
}

func TestBuildTestCommand(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		repoType string
		testCmd  string
		want     string
	}{
		{
			name:     "root type with ./ path",
			repoPath: "./",
			repoType: "root",
			testCmd:  "go test ./...",
			want:     "go test ./...",
		},
		{
			name:     "root type with empty path",
			repoPath: "",
			repoType: "root",
			testCmd:  "npm test",
			want:     "npm test",
		},
		{
			name:     "root type with . path",
			repoPath: ".",
			repoType: "root",
			testCmd:  "make test",
			want:     "make test",
		},
		{
			name:     "directory type",
			repoPath: "backend",
			repoType: "directory",
			testCmd:  "go test ./...",
			want:     "cd backend && go test ./...",
		},
		{
			name:     "submodule type",
			repoPath: "frontend",
			repoType: "submodule",
			testCmd:  "npm test",
			want:     "cd frontend && npm test",
		},
		{
			name:     "path with trailing slash",
			repoPath: "backend/",
			repoType: "directory",
			testCmd:  "go test ./...",
			want:     "cd backend && go test ./...",
		},
		{
			name:     "nested path",
			repoPath: "services/api",
			repoType: "directory",
			testCmd:  "go test ./...",
			want:     "cd services/api && go test ./...",
		},
		{
			name:     "empty path non-root type defaults to command as-is",
			repoPath: "",
			repoType: "directory",
			testCmd:  "go test ./...",
			want:     "go test ./...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTestCommand(tt.repoPath, tt.repoType, tt.testCmd)
			if got != tt.want {
				t.Errorf("buildTestCommand(%q, %q, %q) = %q, want %q",
					tt.repoPath, tt.repoType, tt.testCmd, got, tt.want)
			}
		})
	}
}

func TestPrepareReview_Validation(t *testing.T) {
	tests := []struct {
		name    string
		opts    PrepareReviewOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing state root",
			opts:    PrepareReviewOptions{PRNumber: 1, IssueNumber: 1},
			wantErr: true,
			errMsg:  "state root is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PrepareReview(nil, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWorkflowConfigParsing(t *testing.T) {
	// Test that workflowConfig struct correctly unmarshals YAML
	configYAML := `repos:
  - name: backend
    path: "backend"
    type: directory
    verify:
      test: "go test ./..."
  - name: frontend
    path: "frontend"
    type: submodule
    verify:
      test: "npm test"
`
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".ai", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "workflow.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Test backend matching
	cmd := getTestCommandFromConfig(tmpDir, "/path/with/backend")
	if cmd != "cd backend && go test ./..." {
		t.Errorf("backend match failed: got %q", cmd)
	}

	// Test frontend matching
	cmd = getTestCommandFromConfig(tmpDir, "/path/with/frontend")
	if cmd != "cd frontend && npm test" {
		t.Errorf("frontend match failed: got %q", cmd)
	}
}

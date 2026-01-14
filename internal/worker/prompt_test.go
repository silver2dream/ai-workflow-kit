package worker

import (
	"strings"
	"testing"
)

// GenerateWorkDirInstruction generates work directory instruction for worker prompt.
// This is an exported wrapper for testing the internal buildWorkDirInstruction function.
// Property 9: Worker Prompt Contains Repo Context
func GenerateWorkDirInstruction(repoType, repoPath, workDir string) string {
	// Use repoPath as repoName for simplicity in tests
	return buildWorkDirInstruction(repoType, repoPath, workDir, repoPath)
}

// BuildWorkerPrompt builds complete worker prompt with repo context.
func BuildWorkerPrompt(repoType, repoPath, workDir, taskContent string) string {
	workDirInstruction := GenerateWorkDirInstruction(repoType, repoPath, workDir)

	var builder strings.Builder
	builder.WriteString("You are an automated coding agent running inside a git worktree.\n\n")
	builder.WriteString("Repo rules:\n")
	builder.WriteString("- Read and follow CLAUDE.md and AGENTS.md.\n")
	builder.WriteString("- Keep changes minimal and strictly within ticket scope.\n")
	builder.WriteString(workDirInstruction)
	builder.WriteString("IMPORTANT: Do NOT run any git commands (commit, push, etc.) or create PRs.\n")
	builder.WriteString("The runner script will handle git operations after you complete the code changes.\n")
	builder.WriteString("Your job is ONLY to:\n")
	builder.WriteString("1. Write/modify code files\n")
	builder.WriteString("2. Run verification commands\n")
	builder.WriteString("3. Report the results\n\n")
	builder.WriteString("Ticket:\n")
	builder.WriteString(taskContent)
	builder.WriteString("\n\nAfter making changes:\n")
	builder.WriteString("- Print: git status --porcelain\n")
	builder.WriteString("- Print: git diff\n")
	builder.WriteString("- Run verification commands from the ticket.\n")
	builder.WriteString("- Do NOT commit or push - the runner will handle that.\n")

	return builder.String()
}

// TestWorkerPromptRepoContext tests worker prompt contains repo context.
// Property 9: Worker Prompt Contains Repo Context
func TestWorkerPromptRepoContext(t *testing.T) {
	t.Run("root_type_no_special_instructions", func(t *testing.T) {
		// Test root type has no special path instructions (Req 10.1)
		instruction := GenerateWorkDirInstruction("root", "./", "/worktree")

		if instruction != "" {
			t.Errorf("Expected empty instruction for root type, got '%s'", instruction)
		}
	})

	t.Run("directory_type_has_work_dir", func(t *testing.T) {
		// Test directory type includes WORK_DIR path (Req 10.2)
		workDir := "/worktree/backend"
		instruction := GenerateWorkDirInstruction("directory", "backend", workDir)

		if !strings.Contains(instruction, workDir) {
			t.Errorf("Expected instruction to contain work_dir '%s'", workDir)
		}
		if !strings.Contains(instruction, "Working directory:") {
			t.Error("Expected instruction to contain 'Working directory:'")
		}
	})

	t.Run("directory_type_has_path_relativity", func(t *testing.T) {
		// Test directory type explains path relativity (Req 10.4)
		instruction := GenerateWorkDirInstruction("directory", "backend", "/worktree/backend")

		if !strings.Contains(instruction, "relative to the worktree root") {
			t.Error("Expected instruction to explain path relativity")
		}
		if !strings.Contains(instruction, "backend/") {
			t.Error("Expected instruction to contain repo path 'backend/'")
		}
	})

	t.Run("directory_type_has_example", func(t *testing.T) {
		// Test directory type includes example path
		instruction := GenerateWorkDirInstruction("directory", "backend", "/worktree/backend")

		if !strings.Contains(instruction, "Example:") {
			t.Error("Expected instruction to contain example")
		}
		if !strings.Contains(instruction, "backend/") {
			t.Error("Expected instruction to contain 'backend/' in example")
		}
	})

	t.Run("submodule_type_has_work_dir", func(t *testing.T) {
		// Test submodule type includes WORK_DIR path (Req 10.3)
		workDir := "/worktree/libs/shared"
		instruction := GenerateWorkDirInstruction("submodule", "libs/shared", workDir)

		if !strings.Contains(instruction, workDir) {
			t.Errorf("Expected instruction to contain work_dir '%s'", workDir)
		}
		if !strings.Contains(instruction, "Working directory:") {
			t.Error("Expected instruction to contain 'Working directory:'")
		}
	})

	t.Run("submodule_type_has_warning", func(t *testing.T) {
		// Test submodule type has warning about file boundary (Req 10.5)
		instruction := GenerateWorkDirInstruction("submodule", "libs/shared", "/worktree/libs/shared")

		if !strings.Contains(instruction, "WARNING") {
			t.Error("Expected instruction to contain 'WARNING'")
		}
		if !strings.Contains(instruction, "Do NOT modify files outside") {
			t.Error("Expected instruction to warn about file boundary")
		}
	})

	t.Run("submodule_type_has_boundary_info", func(t *testing.T) {
		// Test submodule type specifies boundary
		instruction := GenerateWorkDirInstruction("submodule", "libs/shared", "/worktree/libs/shared")

		if !strings.Contains(instruction, "libs/shared/") {
			t.Error("Expected instruction to specify boundary path")
		}
		if !strings.Contains(instruction, "All changes must be within") {
			t.Error("Expected instruction to contain boundary requirement")
		}
	})
}

// TestWorkerPromptComplete tests complete worker prompt generation.
func TestWorkerPromptComplete(t *testing.T) {
	t.Run("prompt_includes_repo_rules", func(t *testing.T) {
		// Test prompt includes repo rules section
		prompt := BuildWorkerPrompt("root", "./", "/worktree", "Test task")

		if !strings.Contains(prompt, "Repo rules:") {
			t.Error("Expected prompt to contain 'Repo rules:'")
		}
		if !strings.Contains(prompt, "CLAUDE.md") {
			t.Error("Expected prompt to reference CLAUDE.md")
		}
		if !strings.Contains(prompt, "AGENTS.md") {
			t.Error("Expected prompt to reference AGENTS.md")
		}
	})

	t.Run("prompt_includes_task_content", func(t *testing.T) {
		// Test prompt includes task content
		task := "# Test Task\n\nDo something"
		prompt := BuildWorkerPrompt("root", "./", "/worktree", task)

		if !strings.Contains(prompt, "Ticket:") {
			t.Error("Expected prompt to contain 'Ticket:'")
		}
		if !strings.Contains(prompt, task) {
			t.Error("Expected prompt to contain task content")
		}
	})

	t.Run("prompt_includes_git_warning", func(t *testing.T) {
		// Test prompt warns not to run git commands
		prompt := BuildWorkerPrompt("root", "./", "/worktree", "Test task")

		if !strings.Contains(prompt, "Do NOT run any git commands") {
			t.Error("Expected prompt to warn about git commands")
		}
		if !strings.Contains(prompt, "Do NOT commit or push") {
			t.Error("Expected prompt to warn about commit/push")
		}
	})

	t.Run("prompt_includes_verification_instructions", func(t *testing.T) {
		// Test prompt includes verification instructions
		prompt := BuildWorkerPrompt("root", "./", "/worktree", "Test task")

		if !strings.Contains(prompt, "git status --porcelain") {
			t.Error("Expected prompt to contain 'git status --porcelain'")
		}
		if !strings.Contains(prompt, "git diff") {
			t.Error("Expected prompt to contain 'git diff'")
		}
		if !strings.Contains(prompt, "verification commands") {
			t.Error("Expected prompt to mention verification commands")
		}
	})

	t.Run("prompt_has_work_dir_instruction_based_on_type", func(t *testing.T) {
		// Test prompt includes work dir instruction based on repo type
		testCases := []struct {
			repoType             string
			shouldHaveInstruction bool
		}{
			{"root", false},
			{"directory", true},
			{"submodule", true},
		}

		for _, tc := range testCases {
			t.Run(tc.repoType, func(t *testing.T) {
				prompt := BuildWorkerPrompt(tc.repoType, "backend", "/worktree/backend", "Test task")

				hasInstruction := strings.Contains(prompt, "IMPORTANT: You are working in")
				if hasInstruction != tc.shouldHaveInstruction {
					if tc.shouldHaveInstruction {
						t.Errorf("Expected work dir instruction for repo type '%s'", tc.repoType)
					} else {
						t.Errorf("Did not expect work dir instruction for repo type '%s'", tc.repoType)
					}
				}
			})
		}
	})
}

// TestWorkerPromptEdgeCases tests edge cases for worker prompt generation.
func TestWorkerPromptEdgeCases(t *testing.T) {
	t.Run("unknown_repo_type_returns_empty", func(t *testing.T) {
		// Test unknown repo type returns empty instruction
		instruction := GenerateWorkDirInstruction("unknown", "path", "/worktree")

		if instruction != "" {
			t.Errorf("Expected empty instruction for unknown repo type, got '%s'", instruction)
		}
	})

	t.Run("empty_repo_path", func(t *testing.T) {
		// Test empty repo path is handled
		// Note: The Go implementation falls back to repoName when repoPath is empty
		instruction := GenerateWorkDirInstruction("directory", "", "/worktree")

		if !strings.Contains(instruction, "Working directory:") {
			t.Error("Expected instruction to contain 'Working directory:'")
		}
	})

	t.Run("nested_repo_path", func(t *testing.T) {
		// Test nested repo path is handled
		instruction := GenerateWorkDirInstruction("submodule", "libs/shared/core", "/worktree/libs/shared/core")

		if !strings.Contains(instruction, "libs/shared/core") {
			t.Error("Expected instruction to contain nested path 'libs/shared/core'")
		}
	})
}

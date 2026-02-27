package awkit

import (
	"io/fs"
	"testing"
)

// TestKitFSContainsRequiredFiles verifies that all required files are embedded.
// This prevents issues where awkit init fails because files are missing.
func TestKitFSContainsRequiredFiles(t *testing.T) {
	requiredFiles := []string{
		// Config files
		".ai/config/workflow.schema.json",
		".ai/config/audit.schema.json",
		".ai/config/repo_scan.schema.json",
		".ai/config/execution_trace.schema.json",
		".ai/config/failure_patterns.json",

		// Rules
		".ai/rules/_kit/git-workflow.md",

		// Skills
		".ai/skills/principal-workflow/SKILL.md",
		".ai/skills/principal-workflow/phases/main-loop.md",

		// Subagents
		".claude/agents/pr-reviewer.md",
	}

	for _, path := range requiredFiles {
		_, err := fs.Stat(KitFS, path)
		if err != nil {
			t.Errorf("required file not embedded: %s", path)
		}
	}
}

// TestKitFSContainsRequiredDirectories verifies directory structure is embedded.
func TestKitFSContainsRequiredDirectories(t *testing.T) {
	requiredDirs := []string{
		".ai/config",
		".ai/templates",
		".ai/rules/_kit",
		".ai/rules/_examples",
		// Skills
		".ai/skills/principal-workflow",
		".ai/skills/principal-workflow/phases",
		".ai/skills/principal-workflow/tasks",
		".ai/skills/principal-workflow/references",
		".ai/skills/post-mortem",
		".ai/skills/post-mortem/phases",
		".ai/skills/release-checklist",
		".ai/skills/release-checklist/phases",
		// Subagents
		".claude/agents",
	}

	for _, dir := range requiredDirs {
		entries, err := fs.ReadDir(KitFS, dir)
		if err != nil {
			t.Errorf("required directory not embedded or empty: %s (error: %v)", dir, err)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("required directory is empty: %s", dir)
		}
	}
}

// TestKitFSPythonLibModuleComplete was removed - Python lib module migrated to Go implementations.

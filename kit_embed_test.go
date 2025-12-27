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

		// Core scripts (shell)
		".ai/scripts/generate.sh",
		".ai/scripts/kickoff.sh",
		".ai/scripts/audit_project.sh",
		".ai/scripts/scan_repo.sh",
		".ai/scripts/evaluate.sh",
		".ai/scripts/cleanup.sh",
		".ai/scripts/rollback.sh",
		".ai/scripts/analyze_failure.sh",
		".ai/scripts/write_result.sh",
		".ai/scripts/stats.sh",

		// Core scripts (Python)
		".ai/scripts/validate_config.py",
		".ai/scripts/audit_project.py",
		".ai/scripts/scan_repo.py",
		".ai/scripts/parse_tasks.py",
		".ai/scripts/query_traces.py",

		// Python lib module (critical - was missing before)
		".ai/scripts/lib/__init__.py",
		".ai/scripts/lib/errors.py",
		".ai/scripts/lib/logger.py",
		".ai/scripts/lib/run_with_timeout.py",
		".ai/scripts/lib/timeout.sh",
		".ai/scripts/lib/hash.sh",

		// Other script files
		".ai/scripts/principal_boot.txt",

		// Templates
		".ai/templates/CLAUDE.md.j2",
		".ai/templates/AGENTS.md.j2",
		".ai/templates/git-workflow.md.j2",

		// Commands (deprecated - all moved to scripts and skills)
		// preflight.md also deprecated - preflight now handled by Go code

		// New scripts (from skills refactor)
		".ai/scripts/analyze_next.sh",
		".ai/scripts/dispatch_worker.sh",
		".ai/scripts/check_result.sh",
		".ai/scripts/stop_work.sh",

		// Rules
		".ai/rules/_kit/git-workflow.md",

		// Docs
		".ai/docs/evaluate.md",

		// Tests
		".ai/tests/run_all_tests.sh",
		".ai/tests/conftest.py",
		".ai/tests/pytest.ini",
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
		".ai/scripts",
		".ai/scripts/lib",
		".ai/templates",
		".ai/rules/_kit",
		".ai/rules/_examples",
		".ai/docs",
		".ai/tests",
		".ai/tests/fixtures",
		".ai/tests/unit",
		// Skills (source of truth in .ai/skills/, symlinked to .claude/skills/)
		".ai/skills/principal-workflow",
		".ai/skills/principal-workflow/phases",
		".ai/skills/principal-workflow/tasks",
		".ai/skills/principal-workflow/references",
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

// TestKitFSPythonLibModuleComplete ensures the lib module has all necessary files.
func TestKitFSPythonLibModuleComplete(t *testing.T) {
	libFiles := []string{
		".ai/scripts/lib/__init__.py",
		".ai/scripts/lib/errors.py",
		".ai/scripts/lib/logger.py",
	}

	for _, path := range libFiles {
		content, err := fs.ReadFile(KitFS, path)
		if err != nil {
			t.Errorf("lib module file not embedded: %s", path)
			continue
		}
		if len(content) == 0 {
			t.Errorf("lib module file is empty: %s", path)
		}
	}
}

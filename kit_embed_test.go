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

// Core scripts - migrated to Go (awkit commands)
// All scripts moved to .ai/scripts/_deprecated/

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

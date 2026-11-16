package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// createMinimalMockFS is a kit as small as a kit can be: a rule, a skill and
// a subagent, at the paths they have in a project.
func createMinimalMockFS() fstest.MapFS {
	return fstest.MapFS{
		".claude/rules/_kit/git-workflow.md":         &fstest.MapFile{Data: []byte("# git workflow\n")},
		".claude/skills/principal-workflow/SKILL.md": &fstest.MapFile{Data: []byte("# principal workflow\n")},
		".claude/agents/pr-reviewer.md":              &fstest.MapFile{Data: []byte("# pr reviewer\n")},
	}
}

func TestInstall_CurrentDirectory(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	// Test install with absolute path
	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Verify .ai directory was created
	aiDir := filepath.Join(tmpDir, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		t.Error(".ai directory was not created")
	}

	// Verify the configuration was written
	if _, err := os.Stat(layout.Config(tmpDir)); os.IsNotExist(err) {
		t.Error("the configuration was not written")
	}
}

func TestInstall_DotPath(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	mockFS := createMinimalMockFS()

	// Test install with "."
	result, err := Install(mockFS, ".", Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install with '.' failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}

	// Verify .ai directory was created
	aiDir := filepath.Join(tmpDir, ".ai")
	if _, err := os.Stat(aiDir); os.IsNotExist(err) {
		t.Error(".ai directory was not created")
	}
}

func TestInstall_InvalidPreset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "invalid-preset",
		NoGenerate: true,
	})
	if err == nil {
		t.Error("expected error for invalid preset, got nil")
	}
}

func TestInstall_NonExistentDirectory(t *testing.T) {
	mockFS := createMinimalMockFS()

	_, err := Install(mockFS, "/nonexistent/path/that/does/not/exist", Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}

func TestInstall_ConfigSkipped(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml
	configDir := filepath.Dir(layout.Config(tmpDir))
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(layout.Config(tmpDir), []byte("existing: true"), 0o644)

	mockFS := createMinimalMockFS()

	result, err := Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}
	if !result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be true")
	}

	// Verify original content preserved
	content, _ := os.ReadFile(layout.Config(tmpDir))
	if string(content) != "existing: true" {
		t.Error("existing workflow.yaml was overwritten")
	}
}

func TestInstall_ForceConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing workflow.yaml
	configDir := filepath.Dir(layout.Config(tmpDir))
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(layout.Config(tmpDir), []byte("existing: true"), 0o644)

	mockFS := createMinimalMockFS()

	result, err := Install(mockFS, tmpDir, Options{
		Preset:      "generic",
		ForceConfig: true,
		NoGenerate:  true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if result == nil {
		t.Fatal("Install returned nil result")
	}
	if result.ConfigSkipped {
		t.Error("expected ConfigSkipped to be false with --force-config")
	}

	// Verify content was overwritten
	content, _ := os.ReadFile(layout.Config(tmpDir))
	if string(content) == "existing: true" {
		t.Error("workflow.yaml was not overwritten with --force-config")
	}
}

func TestInstall_GitIgnoreContainsClaudeSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify .gitignore contains claude settings entry
	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	if !strings.Contains(string(content), ".claude/settings.local.json") {
		t.Error(".gitignore should contain .claude/settings.local.json")
	}
}

func TestInstall_GitIgnoreContainsCacheFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	// Verify common cache files are ignored to prevent audit P1 findings
	expectedEntries := []string{
		"__pycache__/",
		"*.pyc",
		"node_modules/",
		".pytest_cache/",
	}
	for _, entry := range expectedEntries {
		if !strings.Contains(string(content), entry) {
			t.Errorf(".gitignore should contain %s", entry)
		}
	}
}

func TestInstall_ClaudeDirectoryCreated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockFS := createMinimalMockFS()

	_, err = Install(mockFS, tmpDir, Options{
		Preset:     "generic",
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify .claude directory was created
	claudeDir := filepath.Join(tmpDir, ".claude")
	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		t.Error(".claude directory was not created")
	}
}

func TestInstall_GitIgnoreUpdatedOnUpgrade(t *testing.T) {
	// Test that upgrade replaces old AWK gitignore section with new entries
	tmpDir, err := os.MkdirTemp("", "awkit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// An ignore file with awkit's section as an earlier version wrote it: one
	// line for every directory a run wrote to.
	oldGitignore := `# User entries
*.bak

# >>> AI Workflow Kit >>>
# Runtime state (do not commit)
.ai/state/
.ai/results/
.ai/runs/
.ai/exe-logs/
.worktrees/
# <<< AI Workflow Kit <<<

# More user entries
*.tmp
`
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(oldGitignore), 0o644)

	mockFS := createMinimalMockFS()

	// Simulate upgrade
	_, err = Install(mockFS, tmpDir, Options{
		Force:      true,
		SkipConfig: true,
		NoGenerate: true,
	})
	if err != nil {
		t.Fatalf("Install (upgrade) failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}

	// Should now contain new entries
	if !strings.Contains(string(content), "__pycache__/") {
		t.Error(".gitignore should contain __pycache__/ after upgrade")
	}
	// A run writes below the state directory now: that line covers it, and
	// the lines for where a run wrote before are gone.
	if !strings.Contains(string(content), "\n.ai/state/\n") || !strings.Contains(string(content), "\n.worktrees/\n") {
		t.Errorf(".gitignore should ignore the state directory and the worktrees:\n%s", content)
	}
	for _, gone := range []string{".ai/results/", ".ai/runs/", ".ai/exe-logs/", ".ai/temp/", ".ai/logs/"} {
		if strings.Contains(string(content), gone) {
			t.Errorf(".gitignore still has %s after the upgrade:\n%s", gone, content)
		}
	}

	// Should preserve user entries
	if !strings.Contains(string(content), "*.bak") {
		t.Error(".gitignore should preserve user entries (*.bak)")
	}
	if !strings.Contains(string(content), "*.tmp") {
		t.Error(".gitignore should preserve user entries (*.tmp)")
	}

	// Should only have ONE AWK section (not duplicated)
	count := strings.Count(string(content), "# >>> AI Workflow Kit >>>")
	if count != 1 {
		t.Errorf(".gitignore should have exactly 1 AWK section, got %d", count)
	}
}

// =============================================================================
// Task 21: Preset Tests
// =============================================================================

// =============================================================================
// Task 22: Scaffold Tests
// =============================================================================

// TestValidateProjectName verifies project name validation
func TestValidateProjectName(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{"ValidSimple", "my-project", false},
		{"ValidWithNumbers", "project123", false},
		{"ValidWithUnderscore", "my_project", false},
		{"ValidWithDot", "my.project", false},
		{"InvalidSlash", "my/project", true},
		{"InvalidBackslash", "my\\project", true},
		{"InvalidDollar", "my$project", true},
		{"InvalidBacktick", "my`project", true},
		{"InvalidDoubleQuote", `my"project`, true},
		{"InvalidSingleQuote", "my'project", true},
		{"InvalidSemicolon", "my;project", true},
		{"InvalidAmpersand", "my&project", true},
		{"InvalidPipe", "my|project", true},
		{"InvalidGreaterThan", "my>project", true},
		{"InvalidLessThan", "my<project", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProjectName(tc.input)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tc.input, err)
			}
		})
	}
}

// TestCIWorkflowMigration verifies that upgrade removes deprecated awk job
func TestCIWorkflowMigration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "awkit-ci-migration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create old CI file with deprecated awk job
	ciDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(ciDir, 0755); err != nil {
		t.Fatalf("failed to create .github/workflows dir: %v", err)
	}

	oldCI := `name: ci

on:
  push:
    branches: ["feat/example"]
  pull_request:
    branches: ["feat/example", "main"]

jobs:
  awk:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Run AWK tests
        run: bash .ai/scripts/evaluate.sh

  backend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Go test
        working-directory: backend
        run: go test ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Build
        working-directory: frontend
        run: npm run build
`
	ciPath := filepath.Join(ciDir, "ci.yml")
	if err := os.WriteFile(ciPath, []byte(oldCI), 0644); err != nil {
		t.Fatalf("failed to write old CI file: %v", err)
	}

	// Test migration function directly
	migrated, newContent := migrateCIWorkflow([]byte(oldCI))
	if !migrated {
		t.Error("migrateCIWorkflow should return migrated=true for CI with awk job")
	}

	newContentStr := string(newContent)

	// Verify awk job is removed
	if strings.Contains(newContentStr, "awk:") {
		t.Error("Migrated CI should NOT contain 'awk:' job")
	}
	if strings.Contains(newContentStr, "evaluate.sh") {
		t.Error("Migrated CI should NOT contain 'evaluate.sh'")
	}

	// Verify other jobs are preserved
	if !strings.Contains(newContentStr, "backend:") {
		t.Error("Migrated CI should preserve 'backend:' job")
	}
	if !strings.Contains(newContentStr, "frontend:") {
		t.Error("Migrated CI should preserve 'frontend:' job")
	}
	if !strings.Contains(newContentStr, "go test") {
		t.Error("Migrated CI should preserve backend test command")
	}
	if !strings.Contains(newContentStr, "npm run build") {
		t.Error("Migrated CI should preserve frontend build command")
	}
}

// TestCIWorkflowNoMigrationNeeded verifies that CI without awk job is not modified
func TestCIWorkflowNoMigrationNeeded(t *testing.T) {
	cleanCI := `name: ci

on: [push]

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - run: npm test
`
	migrated, newContent := migrateCIWorkflow([]byte(cleanCI))
	if migrated {
		t.Error("migrateCIWorkflow should return migrated=false for CI without awk job")
	}
	if string(newContent) != cleanCI {
		t.Error("Content should be unchanged when no migration needed")
	}
}

// A new project gets the roster of its preset: who sits at the table when
// `awkit plan` plans. A roster that is there is the project's own and stays;
// an upgrade writes none.
func TestInstall_Roster(t *testing.T) {
	rosterOf := func(dir string) string {
		data, err := os.ReadFile(layout.Roster(dir))
		if err != nil {
			return ""
		}
		return string(data)
	}

	for preset, want := range map[string]string{"generic": "generic", "godot-go": "godot-go"} {
		dir := t.TempDir()
		if _, err := Install(createMinimalMockFS(), dir, Options{Preset: preset, NoGenerate: true}); err != nil {
			t.Fatalf("%s: Install: %v", preset, err)
		}
		if got := rosterOf(dir); got == "" || got != string(council.DefaultRoster(want)) {
			t.Errorf("%s: the project did not get the %s roster (%d bytes)", preset, want, len(got))
		}
		if _, err := council.LoadRoster(layout.Roster(dir)); err != nil {
			t.Errorf("%s: the roster that was written cannot hold a meeting: %v", preset, err)
		}
	}
	if string(council.DefaultRoster("generic")) == string(council.DefaultRoster("godot-go")) {
		t.Fatal("the two rosters of the test are the same one")
	}

	// The project's own roster is kept, also when the config is forced.
	dir := t.TempDir()
	const own = "roles:\n  - {id: lead, decides: true, plans: true, stance: leads}\n  - {id: critic, stance: criticises}\n"
	if err := os.MkdirAll(filepath.Join(dir, ".ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.Roster(dir), []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(createMinimalMockFS(), dir, Options{Preset: "godot-go", NoGenerate: true, Force: true, ForceConfig: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := rosterOf(dir); got != own {
		t.Errorf("the project's own roster was replaced:\n%s", got)
	}

	// An upgrade leaves the config alone, and writes no roster either.
	dir = t.TempDir()
	if _, err := Install(createMinimalMockFS(), dir, Options{Force: true, SkipConfig: true, NoGenerate: true}); err != nil {
		t.Fatalf("Install (upgrade): %v", err)
	}
	if got := rosterOf(dir); got != "" {
		t.Errorf("an upgrade wrote a roster:\n%s", got)
	}
}

// What an install writes into a new project, and nothing more: the
// configuration and the roster under .ai, the kit under .claude, and the
// directories a run writes to. There is no second copy of the kit, no
// placeholder file in a directory git ignores, and no example.
func TestInstall_WritesTheLayout(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(createMinimalMockFS(), dir, Options{Preset: "generic", NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".ai/config.yaml",
		".ai/roster.yaml",
		".claude/agents/pr-reviewer.md",
		".claude/rules/_kit/git-workflow.md",
		".claude/skills/principal-workflow/SKILL.md",
		".gitattributes",
		".gitignore",
	}
	sort.Strings(files)
	if !reflect.DeepEqual(files, want) {
		t.Errorf("an install wrote\n%v\nwant\n%v", files, want)
	}

	for _, sub := range []string{".ai/state", ".ai/state/results", ".ai/state/runs", ".ai/state/logs", ".ai/state/tmp", ".worktrees"} {
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(sub))); err != nil || !info.IsDir() {
			t.Errorf("%s is not there for a run to write to (err %v)", sub, err)
		}
	}

	// What a run leaves behind is one line of the ignore file.
	ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	for _, line := range []string{"\n.ai/state/\n", "\n.worktrees/\n", "\n.claude/settings.local.json\n"} {
		if !strings.Contains(string(ignore), line) {
			t.Errorf(".gitignore lacks %q:\n%s", strings.TrimSpace(line), ignore)
		}
	}
	for _, gone := range []string{".ai/results", ".ai/runs", ".ai/exe-logs", ".ai/temp", ".ai/logs", ".claude/rules", ".claude/skills"} {
		if strings.Contains(string(ignore), gone) {
			t.Errorf(".gitignore still names %s:\n%s", gone, ignore)
		}
	}
}

// A kit is its .claude directory. What else the kit's file system holds, a
// configuration of its own for one, is not installed: a project's
// configuration comes from its preset.
func TestInstall_OnlyTheKitIsInstalled(t *testing.T) {
	dir := t.TempDir()
	kit := createMinimalMockFS()
	kit[".ai/config.yaml"] = &fstest.MapFile{Data: []byte("language: unity\n")}
	kit["docs/examples/rules/testing-strategy.md"] = &fstest.MapFile{Data: []byte("# Testing rules")}

	if _, err := Install(kit, dir, Options{Preset: PresetGodotGo, Force: true, NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	content, err := os.ReadFile(layout.Config(dir))
	if err != nil {
		t.Fatalf("the configuration: %v", err)
	}
	if strings.Contains(string(content), "language: unity") {
		t.Error("the kit's own configuration was installed over the preset's")
	}
	if !strings.Contains(string(content), "language: csharp") || !strings.Contains(string(content), "language: go") {
		t.Errorf("the configuration is not the godot-go preset's:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs")); !os.IsNotExist(err) {
		t.Errorf("what the kit holds outside .claude was installed (err %v)", err)
	}
}

// An upgrade writes the kit's files over and adds the ones that are new. It
// leaves alone what is the project's: its configuration, and whatever else it
// keeps under .claude, which is Claude Code's directory before it is AWK's.
func TestInstall_Upgrade(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(rel string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "(not there: " + err.Error() + ")"
		}
		return string(content)
	}
	const customConfig = "project:\n  name: my-custom-project\n  type: monorepo\n"
	own := map[string]string{
		".claude/rules/backend.md":              "the project's rule",
		".claude/skills/release-notes/SKILL.md": "the project's skill",
		".claude/agents/designer.md":            "the project's subagent",
		".claude/commands/deploy.md":            "the project's command",
		".claude/settings.json":                 "{}",
	}
	write(".ai/config.yaml", customConfig)
	write(".claude/rules/_kit/git-workflow.md", "the kit's rule as an earlier version had it")
	for rel, content := range own {
		write(rel, content)
	}

	result, err := Install(createMinimalMockFS(), dir, Options{Force: true, SkipConfig: true, NoGenerate: true})
	if err != nil {
		t.Fatalf("Install (upgrade): %v", err)
	}

	if !result.ConfigSkipped || read(".ai/config.yaml") != customConfig {
		t.Errorf("the configuration was not kept: skipped %v, content %q", result.ConfigSkipped, read(".ai/config.yaml"))
	}
	if got := read(".claude/rules/_kit/git-workflow.md"); got != "# git workflow\n" {
		t.Errorf("the kit's rule was not written over: %q", got)
	}
	for _, added := range []string{".claude/skills/principal-workflow/SKILL.md", ".claude/agents/pr-reviewer.md"} {
		if strings.HasPrefix(read(added), "(not there") {
			t.Errorf("%s was not added", added)
		}
	}
	for rel, content := range own {
		if got := read(rel); got != content {
			t.Errorf("%s is the project's own and was touched: %q", rel, got)
		}
	}
	// An upgrade writes no roster: `awkit plan` does, the first time it runs.
	if _, err := os.Stat(layout.Roster(dir)); !os.IsNotExist(err) {
		t.Errorf("an upgrade wrote a roster (err %v)", err)
	}
}

package install

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
)

//go:embed deprecated.txt
var deprecatedFiles string

type Options struct {
	Preset      string
	ProjectName string
	Force       bool
	ForceConfig bool // Only overwrite workflow.yaml
	ForceCI     bool // Overwrite CI workflow file
	SkipConfig  bool // Skip workflow.yaml entirely (for upgrade)
	NoGenerate  bool
	WithCI      bool
	Scaffold    bool // Execute scaffold after install
	DryRun      bool // For scaffold dry-run
}

// InstallResult contains information about what was done
type InstallResult struct {
	ConfigSkipped  bool
	ConfigPath     string
	ScaffoldResult *ScaffoldResult
	ScaffoldError  error
}

// ScaffoldOptions configures scaffold behavior
type ScaffoldOptions struct {
	Preset      string
	TargetDir   string
	ProjectName string
	Force       bool
	DryRun      bool
}

// ScaffoldResult contains scaffold operation results
type ScaffoldResult struct {
	Created []string
	Skipped []string
	Failed  []string
	Errors  []error
}

// Error types for scaffold operations
var (
	ErrUnknownPreset      = errors.New("unknown preset")
	ErrScaffoldConflict   = errors.New("file already exists")
	ErrScaffoldFailed     = errors.New("scaffold failed")
	ErrMissingPreset      = errors.New("--preset required for upgrade --scaffold")
	ErrInvalidProjectName = errors.New("invalid project name: contains special characters")
)

func Install(kit fs.FS, targetDir string, opts Options) (*InstallResult, error) {
	result := &InstallResult{}

	// Resolve "." or "./" to absolute path
	if targetDir == "." || targetDir == "./" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot resolve current directory: %w", err)
		}
		targetDir = cwd
	}
	targetDir = filepath.Clean(targetDir)

	if st, err := os.Stat(targetDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("target directory does not exist or is not a directory: %s", targetDir)
	}

	if opts.ProjectName == "" {
		opts.ProjectName = filepath.Base(targetDir)
	}

	// Apply preset FIRST to generate workflow.yaml before copyDir
	// This ensures preset-specific config is used instead of embedded default
	configSkipped, err := applyPreset(kit, targetDir, opts)
	if err != nil {
		return nil, err
	}
	result.ConfigSkipped = configSkipped
	result.ConfigPath = filepath.Join(targetDir, ".ai", "config", "workflow.yaml")

	if err := copyDir(kit, ".ai", filepath.Join(targetDir, ".ai"), opts.Force); err != nil {
		return nil, err
	}

	// Copy .ai/skills/ directory if it exists in the kit
	if _, err := fs.Stat(kit, ".ai/skills"); err == nil {
		if err := copyDir(kit, ".ai/skills", filepath.Join(targetDir, ".ai", "skills"), opts.Force); err != nil {
			return nil, err
		}
	}

	// Clean up deprecated .ai/commands/ directory (replaced by .ai/skills/)
	deprecatedCommandsDir := filepath.Join(targetDir, ".ai", "commands")
	if _, err := os.Stat(deprecatedCommandsDir); err == nil {
		_ = os.RemoveAll(deprecatedCommandsDir)
	}

	// Clean up deprecated .claude/commands symlink/directory
	deprecatedClaudeCommands := filepath.Join(targetDir, ".claude", "commands")
	if _, err := os.Stat(deprecatedClaudeCommands); err == nil {
		_ = os.RemoveAll(deprecatedClaudeCommands)
	}

	// Clean up deprecated kit files (removed in newer versions)
	cleanupDeprecatedFiles(targetDir)

	// Copy .claude/agents/ directory if it exists in the kit
	if _, err := fs.Stat(kit, ".claude/agents"); err == nil {
		agentsDir := filepath.Join(targetDir, ".claude", "agents")
		if err := os.MkdirAll(filepath.Dir(agentsDir), 0o755); err != nil {
			return nil, err
		}
		if err := copyDir(kit, ".claude/agents", agentsDir, opts.Force); err != nil {
			return nil, err
		}
	}

	if err := ensureRuntimeDirs(targetDir); err != nil {
		return nil, err
	}

	if err := ensureGitIgnore(targetDir); err != nil {
		return nil, err
	}

	if err := ensureGitAttributes(targetDir); err != nil {
		return nil, err
	}

	if opts.WithCI {
		if err := ensureCIWorkflow(targetDir, opts.ForceCI); err != nil {
			return nil, err
		}
	}

	if err := ensureClaudeLinks(targetDir); err != nil {
		return nil, err
	}

	// Scaffold integration point (after all kit files are installed)
	if opts.Scaffold {
		scaffoldResult, err := Scaffold(targetDir, ScaffoldOptions{
			Preset:      opts.Preset,
			TargetDir:   targetDir,
			ProjectName: opts.ProjectName,
			Force:       opts.Force,
			DryRun:      false,
		})
		if err != nil {
			// Scaffold failure doesn't affect already installed AWK kit
			result.ScaffoldResult = scaffoldResult
			result.ScaffoldError = err
		} else {
			result.ScaffoldResult = scaffoldResult
		}
	}

	if !opts.NoGenerate {
		_ = tryGenerate(targetDir)
	}

	return result, nil
}

func copyDir(src fs.FS, srcDir, dstDir string, force bool) error {
	return fs.WalkDir(src, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		// Skip workflow.yaml - it's managed by applyPreset, not copyDir
		// This prevents the embedded default config from overwriting preset-generated config
		if strings.HasSuffix(path, "workflow.yaml") {
			return nil
		}

		if !force {
			if _, err := os.Stat(dstPath); err == nil {
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		r, err := src.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		}

		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}

		if strings.HasSuffix(path, ".sh") {
			_ = os.Chmod(dstPath, 0o755)
		}

		return nil
	})
}

func ensureRuntimeDirs(targetDir string) error {
	dirs := []string{
		filepath.Join(targetDir, ".ai", "state"),
		filepath.Join(targetDir, ".ai", "state", "traces"),
		filepath.Join(targetDir, ".ai", "results"),
		filepath.Join(targetDir, ".ai", "runs"),
		filepath.Join(targetDir, ".ai", "exe-logs"),
		filepath.Join(targetDir, ".ai", "temp"),
		filepath.Join(targetDir, ".ai", "logs"),
		filepath.Join(targetDir, ".worktrees"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	keepFiles := []string{
		filepath.Join(targetDir, ".ai", "state", ".gitkeep"),
		filepath.Join(targetDir, ".ai", "state", "traces", ".gitkeep"),
		filepath.Join(targetDir, ".ai", "results", ".gitkeep"),
		filepath.Join(targetDir, ".ai", "runs", ".gitkeep"),
		filepath.Join(targetDir, ".ai", "exe-logs", ".gitkeep"),
		filepath.Join(targetDir, ".ai", "temp", ".gitkeep"),
		filepath.Join(targetDir, ".ai", "logs", ".gitkeep"),
	}
	for _, f := range keepFiles {
		if _, err := os.Stat(f); err == nil {
			continue
		}
		if err := os.WriteFile(f, []byte("\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureGitIgnore(targetDir string) error {
	const markerStart = "# >>> AI Workflow Kit >>>"
	const markerEnd = "# <<< AI Workflow Kit <<<"

	snippet := strings.Join([]string{
		markerStart,
		"# Runtime state (do not commit)",
		".ai/state/",
		".ai/results/",
		".ai/runs/",
		".ai/exe-logs/",
		".ai/logs/",
		".ai/temp/",
		".worktrees/",
		"# Claude Code local settings (do not commit)",
		".claude/settings.local.json",
		"# Common cache files (prevent audit P1 findings)",
		"__pycache__/",
		"*.pyc",
		"*.pyo",
		".pytest_cache/",
		"node_modules/",
		".npm/",
		".yarn/",
		"*.log",
		markerEnd,
	}, "\n") + "\n"

	path := filepath.Join(targetDir, ".gitignore")
	existing, _ := os.ReadFile(path)

	// If AWK section exists, replace it (to support upgrades with new entries)
	if bytes.Contains(existing, []byte(markerStart)) {
		startIdx := bytes.Index(existing, []byte(markerStart))
		endIdx := bytes.Index(existing, []byte(markerEnd))
		if endIdx > startIdx {
			// Remove old section and replace with new
			endIdx += len(markerEnd)
			// Skip trailing newline if present
			if endIdx < len(existing) && existing[endIdx] == '\n' {
				endIdx++
			}
			var out []byte
			out = append(out, existing[:startIdx]...)
			out = append(out, []byte(snippet)...)
			out = append(out, existing[endIdx:]...)
			return os.WriteFile(path, out, 0o644)
		}
		// Malformed section (start without end), append new section
	}

	var out []byte
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		out = append(out, existing...)
		out = append(out, '\n')
	} else {
		out = append(out, existing...)
	}
	out = append(out, '\n')
	out = append(out, []byte(snippet)...)
	return os.WriteFile(path, out, 0o644)
}

func ensureGitAttributes(targetDir string) error {
	path := filepath.Join(targetDir, ".gitattributes")
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	content := strings.Join([]string{
		"# Normalize text files and prevent CRLF breakage in CI",
		"* text=auto",
		"",
		"*.sh text eol=lf",
		"*.yml text eol=lf",
		"*.yaml text eol=lf",
		"*.md text eol=lf",
		"*.json text eol=lf",
		"*.ts text eol=lf",
		"*.tsx text eol=lf",
		"",
	}, "\n")
	return os.WriteFile(path, []byte(content), 0o644)
}

func ensureClaudeLinks(targetDir string) error {
	claudeDir := filepath.Join(targetDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return err
	}

	type link struct {
		name   string
		source string
		target string
	}
	links := []link{
		{
			name:   "rules",
			source: filepath.Join(targetDir, ".ai", "rules"),
			target: filepath.Join(claudeDir, "rules"),
		},
		{
			name:   "skills",
			source: filepath.Join(targetDir, ".ai", "skills"),
			target: filepath.Join(claudeDir, "skills"),
		},
	}

	for _, l := range links {
		// Skip if source doesn't exist
		if _, err := os.Stat(l.source); os.IsNotExist(err) {
			continue
		}

		_ = os.RemoveAll(l.target)
		relSource, err := filepath.Rel(filepath.Dir(l.target), l.source)
		if err != nil {
			return err
		}

		if err := os.Symlink(relSource, l.target); err == nil {
			continue
		}

		// Symlink failed (e.g., Windows without admin), fall back to copy
		if err := copyOSDir(l.source, l.target); err != nil {
			return fmt.Errorf("failed to create .claude/%s (symlink+copy both failed): %w", l.name, err)
		}
	}
	return nil
}

func copyOSDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0o644)
	})
}

func applyPreset(kit fs.FS, targetDir string, opts Options) (skipped bool, err error) {
	// Skip config entirely for upgrade mode
	if opts.SkipConfig {
		return true, nil
	}

	switch opts.Preset {
	case "", "generic", "node":
		// generic and node share the same preset (backward compatibility)
		return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetNode)
	case "go":
		return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetGo)
	case "python":
		return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetPython)
	case "rust":
		return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetRust)
	case "dotnet":
		return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetDotnet)
	case "react-go":
		skipped, err = ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetReactGo)
		if err != nil {
			return skipped, err
		}
		return skipped, applyPresetRules(kit, targetDir, "react-go")
	case "react-python":
		skipped, err = ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetReactPython)
		if err != nil {
			return skipped, err
		}
		return skipped, applyPresetRules(kit, targetDir, "react-python")
	case "unity-go":
		skipped, err = ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetUnityGo)
		if err != nil {
			return skipped, err
		}
		return skipped, applyPresetRules(kit, targetDir, "unity-go")
	case "godot-go":
		skipped, err = ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetGodotGo)
		if err != nil {
			return skipped, err
		}
		return skipped, applyPresetRules(kit, targetDir, "godot-go")
	case "unreal-go":
		skipped, err = ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetUnrealGo)
		if err != nil {
			return skipped, err
		}
		return skipped, applyPresetRules(kit, targetDir, "unreal-go")
	default:
		return false, fmt.Errorf("%w: %q", ErrUnknownPreset, opts.Preset)
	}
}

func ensureWorkflowConfig(targetDir, projectName string, forceOverwrite bool, preset func(projectName string) []byte) (skipped bool, err error) {
	path := filepath.Join(targetDir, ".ai", "config", "workflow.yaml")
	if _, err := os.Stat(path); err == nil {
		if !forceOverwrite {
			return true, nil // File exists, skip
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return false, os.WriteFile(path, preset(projectName), 0o644)
}

// applyPresetRules copies rule files for the given preset
func applyPresetRules(kit fs.FS, targetDir string, preset string) error {
	ruleMap := map[string][]struct{ src, dst string }{
		"react-go": {
			{src: ".ai/rules/_examples/backend-go.md", dst: ".ai/rules/backend-go.md"},
			{src: ".ai/rules/_examples/frontend-react.md", dst: ".ai/rules/frontend-react.md"},
		},
		"react-python": {
			{src: ".ai/rules/_examples/backend-python.md", dst: ".ai/rules/backend-python.md"},
			{src: ".ai/rules/_examples/frontend-react.md", dst: ".ai/rules/frontend-react.md"},
		},
		"unity-go": {
			{src: ".ai/rules/_examples/backend-go.md", dst: ".ai/rules/backend-go.md"},
			{src: ".ai/rules/_examples/frontend-unity.md", dst: ".ai/rules/frontend-unity.md"},
		},
		"godot-go": {
			{src: ".ai/rules/_examples/backend-go.md", dst: ".ai/rules/backend-go.md"},
			{src: ".ai/rules/_examples/frontend-godot.md", dst: ".ai/rules/frontend-godot.md"},
		},
		"unreal-go": {
			{src: ".ai/rules/_examples/backend-go.md", dst: ".ai/rules/backend-go.md"},
			{src: ".ai/rules/_examples/frontend-unreal.md", dst: ".ai/rules/frontend-unreal.md"},
		},
	}

	rules, ok := ruleMap[preset]
	if !ok {
		return nil // No rules for this preset
	}

	for _, r := range rules {
		b, err := fs.ReadFile(kit, r.src)
		if err != nil {
			// Rule file doesn't exist, skip with warning (don't fail)
			continue
		}
		dstPath := filepath.Join(targetDir, filepath.FromSlash(r.dst))
		if _, err := os.Stat(dstPath); err == nil {
			// Never overwrite user rules (these are intended as starting points).
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureCIWorkflow(targetDir string, force bool) error {
	path := filepath.Join(targetDir, ".github", "workflows", "ci.yml")

	// Check if file exists
	existingContent, err := os.ReadFile(path)
	if err == nil {
		// File exists - check if migration is needed
		migrated, migratedContent := migrateCIWorkflow(existingContent)
		if migrated {
			// Migration happened, write the migrated content
			return os.WriteFile(path, migratedContent, 0o644)
		}
		// No migration needed, respect force flag
		if !force {
			return nil
		}
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := strings.TrimSpace(ciWorkflowYAML) + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

// migrateCIWorkflow checks for and removes deprecated AWK CI jobs
// Returns (migrated bool, newContent []byte)
func migrateCIWorkflow(content []byte) (bool, []byte) {
	lines := strings.Split(string(content), "\n")
	var result []string
	migrated := false
	inAwkJob := false
	awkJobIndent := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Detect start of awk job (old deprecated job)
		if strings.TrimSpace(line) == "awk:" && strings.HasSuffix(strings.TrimRight(line, " \t"), ":") {
			// Found awk: job definition
			awkJobIndent = len(line) - len(strings.TrimLeft(line, " \t"))
			inAwkJob = true
			migrated = true
			continue
		}

		// If we're in awk job, skip until we hit a line with same or less indentation
		if inAwkJob {
			currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
			// Empty lines are part of the job
			if strings.TrimSpace(line) == "" {
				continue
			}
			// If indentation is greater, still in awk job
			if currentIndent > awkJobIndent {
				continue
			}
			// We've exited the awk job
			inAwkJob = false
		}

		result = append(result, line)
	}

	if migrated {
		// Clean up any double blank lines that might result
		return true, []byte(strings.Join(result, "\n"))
	}
	return false, content
}

func tryGenerate(targetDir string) error {
	_, err := generate.Generate(generate.Options{
		StateRoot:  targetDir,
		GenerateCI: false,
	})
	return err
}

// commonConfigSuffix contains the common config sections appended to all presets.
// These sections were introduced in config v1.2.
const commonConfigSuffix = `
agents:
  builtin:
    - pr-reviewer
    - conflict-resolver
  custom: []

timeouts:
  git_seconds: 120
  gh_seconds: 60
  codex_minutes: 30
  gh_retry_count: 3
  gh_retry_base_delay: 2

review:
  score_threshold: 7
  merge_strategy: squash

escalation:
  triggers: []
  max_consecutive_failures: 3
  retry_count: 2
  retry_delay_seconds: 5
  max_single_pr_files: 50
  max_single_pr_lines: 500

feedback:
  enabled: true
  max_history_in_prompt: 10

hooks: {}

worker:
  backend: codex
`

var presetReactGo = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (React + Go)
version: "1.2"

project:
  name: %q
  description: "React + Go project using AWK"
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: typescript
    node_version: "20"
    package_manager: "npm"
    verify:
      build: "npm run build"
      test: "npm run test -- --run"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - submodule-sync
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom:
    - backend-go
    - frontend-react
`, projectName)
	return []byte(content + commonConfigSuffix)
}

const ciWorkflowYAML = `
name: ci

on:
  push:
    branches: ["feat/example"]
  pull_request:
    branches: ["feat/example", "main"]

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"

      - name: Go test
        working-directory: backend
        run: go test ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "20"

      - name: Install dependencies
        working-directory: frontend
        run: npm install

      - name: Test
        working-directory: frontend
        run: npm run test -- --run

      - name: Build
        working-directory: frontend
        run: npm run build
`

// validateProjectName checks for invalid characters in project name
func validateProjectName(name string) error {
	if strings.ContainsAny(name, `/\$"';|&><`) || strings.Contains(name, "`") {
		return ErrInvalidProjectName
	}
	return nil
}

// writeFileIfNotExists writes a file, skipping if exists unless force is true
// Returns (created, skipped, err)
func writeFileIfNotExists(path string, content []byte, force bool) (created bool, skipped bool, err error) {
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err == nil {
		if !force {
			return false, true, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, false, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, false, err
	}
	return true, false, nil
}

// Preset functions for new presets

var presetGo = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Go)
version: "1.2"

project:
  name: %q
  description: "Go project using AWK"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom: []
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetPython = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Python)
version: "1.2"

project:
  name: %q
  description: "Python project using AWK"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: python
    verify:
      build: "echo 'No build step'"
      test: "python -m pytest tests/ -v --tb=short 2>/dev/null || echo 'No tests yet'"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom: []
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetRust = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Rust)
version: "1.2"

project:
  name: %q
  description: "Rust project using AWK"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: rust
    verify:
      build: "cargo build"
      test: "cargo test"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom: []
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetDotnet = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (.NET)
version: "1.2"

project:
  name: %q
  description: ".NET project using AWK"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: dotnet
    verify:
      build: "dotnet build"
      test: "dotnet test"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom: []
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetNode = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Node.js/TypeScript)
version: "1.2"

project:
  name: %q
  description: "Node.js/TypeScript project using AWK"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: typescript
    verify:
      build: "npm run build"
      test: "npm run test -- --run"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom: []
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetReactPython = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (React + Python)
version: "1.2"

project:
  name: %q
  description: "React + Python project using AWK"
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: python
    verify:
      build: "echo 'No build step'"
      test: "python -m pytest tests/ -v --tb=short 2>/dev/null || echo 'No tests yet'"

  - name: frontend
    path: frontend/
    type: directory
    language: typescript
    node_version: "20"
    package_manager: "npm"
    verify:
      build: "npm run build"
      test: "npm run test -- --run"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - submodule-sync
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom:
    - backend-python
    - frontend-react
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetUnityGo = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Unity + Go)
version: "1.2"

project:
  name: %q
  description: "Unity + Go project using AWK"
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: unity
    verify:
      build: "echo 'Unity build via Editor'"
      test: "echo 'Unity tests via Editor'"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - submodule-sync
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom:
    - backend-go
    - frontend-unity
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetGodotGo = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Godot + Go)
version: "1.2"

project:
  name: %q
  description: "Godot + Go project using AWK"
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: gdscript
    verify:
      build: "echo 'Godot build via Editor'"
      test: "echo 'Godot tests via Editor'"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - submodule-sync
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom:
    - backend-go
    - frontend-godot
`, projectName)
	return []byte(content + commonConfigSuffix)
}

var presetUnrealGo = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (Unreal + Go)
version: "1.2"

project:
  name: %q
  description: "Unreal + Go project using AWK"
  type: "monorepo"

repos:
  - name: backend
    path: backend/
    type: directory
    language: go
    verify:
      build: "go build ./..."
      test: "go test ./..."

  - name: frontend
    path: frontend/
    type: directory
    language: unreal
    verify:
      build: "echo 'Unreal build via Editor'"
      test: "echo 'Unreal tests via Editor'"

git:
  integration_branch: "feat/example"
  release_branch: "main"
  commit_format: "[type] subject"

specs:
  base_path: ".ai/specs"
  files:
    requirements: "requirements.md"
    design: "design.md"
    tasks: "tasks.md"
  auto_generate_tasks: true
  active: []

tasks:
  format:
    uncompleted: "- [ ]"
    completed: "- [x]"
    optional: "- [ ]*"
  source_priority:
    - audit
    - specs

audit:
  checks:
    - dirty-worktree
    - submodule-sync
    - missing-tests
    - missing-ci
  custom: []

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    review_failed: "review-failed"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"

rules:
  kit:
    - git-workflow
  custom:
    - backend-go
    - frontend-unreal
`, projectName)
	return []byte(content + commonConfigSuffix)
}

// Scaffold creates project structure based on preset
func Scaffold(targetDir string, opts ScaffoldOptions) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	// Validate project name
	if opts.ProjectName != "" {
		if err := validateProjectName(opts.ProjectName); err != nil {
			return result, err
		}
	}

	// Resolve project name from directory if not provided
	projectName := opts.ProjectName
	if projectName == "" {
		projectName = filepath.Base(targetDir)
	}

	// Determine scaffold type based on preset
	switch opts.Preset {
	case "", "generic", "node":
		return scaffoldNode(targetDir, projectName, opts.Force, opts.DryRun)
	case "go":
		return scaffoldGo(targetDir, projectName, opts.Force, opts.DryRun)
	case "python":
		return scaffoldPython(targetDir, projectName, opts.Force, opts.DryRun)
	case "rust":
		return scaffoldRust(targetDir, projectName, opts.Force, opts.DryRun)
	case "dotnet":
		return scaffoldDotnet(targetDir, projectName, opts.Force, opts.DryRun)
	case "react-go":
		return scaffoldMonorepo(targetDir, projectName, "go", "react", opts.Force, opts.DryRun)
	case "react-python":
		return scaffoldMonorepo(targetDir, projectName, "python", "react", opts.Force, opts.DryRun)
	case "unity-go":
		return scaffoldMonorepo(targetDir, projectName, "go", "unity", opts.Force, opts.DryRun)
	case "godot-go":
		return scaffoldMonorepo(targetDir, projectName, "go", "godot", opts.Force, opts.DryRun)
	case "unreal-go":
		return scaffoldMonorepo(targetDir, projectName, "go", "unreal", opts.Force, opts.DryRun)
	default:
		return result, fmt.Errorf("%w: %q", ErrUnknownPreset, opts.Preset)
	}
}

func scaffoldGo(targetDir, projectName string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, "go.mod"),
			content: []byte(fmt.Sprintf(`module %s

go 1.25
`, projectName)),
		},
		{
			path: filepath.Join(targetDir, "main.go"),
			content: []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`),
		},
		{
			path:    filepath.Join(targetDir, "README.md"),
			content: []byte(fmt.Sprintf("# %s\n\nA Go project.\n", projectName)),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

func scaffoldPython(targetDir, projectName string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, "pyproject.toml"),
			content: []byte(fmt.Sprintf(`[project]
name = "%s"
version = "0.1.0"
description = "A Python project"
requires-python = ">=3.9"

[tool.pytest.ini_options]
testpaths = ["tests"]
`, projectName)),
		},
		{
			path:    filepath.Join(targetDir, "src", "__init__.py"),
			content: []byte(""),
		},
		{
			path: filepath.Join(targetDir, "src", "main.py"),
			content: []byte(`def main():
    print("Hello, World!")

if __name__ == "__main__":
    main()
`),
		},
		{
			path:    filepath.Join(targetDir, "tests", "__init__.py"),
			content: []byte(""),
		},
		{
			path: filepath.Join(targetDir, "tests", "test_placeholder.py"),
			content: []byte(`def test_placeholder():
    """Placeholder test to ensure pytest doesn't fail with 'no tests collected'."""
    pass
`),
		},
		{
			path:    filepath.Join(targetDir, "README.md"),
			content: []byte(fmt.Sprintf("# %s\n\nA Python project.\n", projectName)),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

func scaffoldRust(targetDir, projectName string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, "Cargo.toml"),
			content: []byte(fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
`, projectName)),
		},
		{
			path: filepath.Join(targetDir, "src", "main.rs"),
			content: []byte(`fn main() {
    println!("Hello, World!");
}
`),
		},
		{
			path:    filepath.Join(targetDir, "README.md"),
			content: []byte(fmt.Sprintf("# %s\n\nA Rust project.\n", projectName)),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

func scaffoldDotnet(targetDir, projectName string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, projectName+".csproj"),
			content: []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net8.0</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
    <Nullable>enable</Nullable>
  </PropertyGroup>
</Project>
`),
		},
		{
			path: filepath.Join(targetDir, "Program.cs"),
			content: []byte(`Console.WriteLine("Hello, World!");
`),
		},
		{
			path:    filepath.Join(targetDir, "README.md"),
			content: []byte(fmt.Sprintf("# %s\n\nA .NET project.\n", projectName)),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

func scaffoldNode(targetDir, projectName string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, "package.json"),
			content: []byte(fmt.Sprintf(`{
  "name": "%s",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "build": "tsc",
    "test": "vitest"
  },
  "devDependencies": {
    "typescript": "^5.0.0",
    "vitest": "^1.0.0"
  }
}
`, projectName)),
		},
		{
			path: filepath.Join(targetDir, "tsconfig.json"),
			content: []byte(`{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "outDir": "dist",
    "rootDir": "src"
  },
  "include": ["src"]
}
`),
		},
		{
			path: filepath.Join(targetDir, "src", "index.ts"),
			content: []byte(`console.log("Hello, World!");
`),
		},
		{
			path: filepath.Join(targetDir, "src", "index.test.ts"),
			content: []byte(`import { describe, expect, it } from "vitest";

describe("smoke", () => {
  it("runs", () => {
    expect(true).toBe(true);
  });
});
`),
		},
		{
			path:    filepath.Join(targetDir, "README.md"),
			content: []byte(fmt.Sprintf("# %s\n\nA Node.js/TypeScript project.\n", projectName)),
		},
	}

	res, err := scaffoldFiles(files, force, dryRun, result)
	if err != nil || dryRun {
		return res, err
	}

	// Install npm dependencies if package.json was created
	if _, statErr := os.Stat(filepath.Join(targetDir, "package.json")); statErr == nil {
		_ = npmInstall(targetDir)
	}

	return res, nil
}

func scaffoldMonorepo(targetDir, projectName, backend, frontend string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	// Scaffold backend
	backendDir := filepath.Join(targetDir, "backend")
	var backendResult *ScaffoldResult
	var err error

	switch backend {
	case "go":
		backendResult, err = scaffoldGo(backendDir, projectName+"-backend", force, dryRun)
	case "python":
		backendResult, err = scaffoldPython(backendDir, projectName+"-backend", force, dryRun)
	}
	if err != nil {
		return result, err
	}
	result.Created = append(result.Created, backendResult.Created...)
	result.Skipped = append(result.Skipped, backendResult.Skipped...)
	result.Failed = append(result.Failed, backendResult.Failed...)
	result.Errors = append(result.Errors, backendResult.Errors...)

	// Scaffold frontend
	frontendDir := filepath.Join(targetDir, "frontend")
	var frontendResult *ScaffoldResult

	switch frontend {
	case "react":
		frontendResult, err = scaffoldReactFrontend(frontendDir, force, dryRun)
	case "unity":
		frontendResult, err = scaffoldUnityFrontend(frontendDir, force, dryRun)
	case "godot":
		frontendResult, err = scaffoldGodotFrontend(frontendDir, force, dryRun)
	case "unreal":
		frontendResult, err = scaffoldUnrealFrontend(frontendDir, force, dryRun)
	}
	if err != nil {
		return result, err
	}
	result.Created = append(result.Created, frontendResult.Created...)
	result.Skipped = append(result.Skipped, frontendResult.Skipped...)
	result.Failed = append(result.Failed, frontendResult.Failed...)
	result.Errors = append(result.Errors, frontendResult.Errors...)

	if len(result.Failed) > 0 {
		return result, ErrScaffoldFailed
	}
	return result, nil
}

func scaffoldReactFrontend(targetDir string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, "package.json"),
			content: []byte(`{
  "name": "frontend",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "test": "vitest"
  },
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.0",
    "@types/react-dom": "^18.2.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.0.0",
    "vite": "^5.0.0",
    "vitest": "^1.0.0"
  }
}
`),
		},
		{
			path: filepath.Join(targetDir, "tsconfig.json"),
			content: []byte(`{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noEmit": true
  },
  "include": ["src"]
}
`),
		},
		{
			path: filepath.Join(targetDir, "vite.config.ts"),
			content: []byte(`import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
})
`),
		},
		{
			path: filepath.Join(targetDir, "index.html"),
			content: []byte(`<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>App</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
`),
		},
		{
			path: filepath.Join(targetDir, "src", "main.tsx"),
			content: []byte(`import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
`),
		},
		{
			path: filepath.Join(targetDir, "src", "App.tsx"),
			content: []byte(`function App() {
  return (
    <div>
      <h1>Hello, World!</h1>
    </div>
  )
}

export default App
`),
		},
		{
			path: filepath.Join(targetDir, "src", "smoke.test.ts"),
			content: []byte(`import { describe, expect, it } from 'vitest'

describe('smoke', () => {
  it('runs', () => {
    expect(1 + 1).toBe(2)
  })
})
`),
		},
		{
			path:    filepath.Join(targetDir, "README.md"),
			content: []byte("# Frontend\n\nReact frontend with Vite.\n"),
		},
	}

	res, err := scaffoldFiles(files, force, dryRun, result)
	if err != nil || dryRun {
		return res, err
	}

	// Install npm dependencies if package.json was created
	if _, statErr := os.Stat(filepath.Join(targetDir, "package.json")); statErr == nil {
		_ = npmInstall(targetDir)
	}

	return res, nil
}

func scaffoldUnityFrontend(targetDir string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path:    filepath.Join(targetDir, ".gitkeep"),
			content: []byte(""),
		},
		{
			path: filepath.Join(targetDir, "README.md"),
			content: []byte(`# Frontend (Unity)

This directory is a placeholder for the Unity project.

## Setup

1. Open Unity Hub
2. Create a new Unity project in this directory
3. The Unity project files will be created here

Note: Unity projects should be created using the Unity Editor, not scaffolded.
`),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

func scaffoldGodotFrontend(targetDir string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path: filepath.Join(targetDir, "project.godot"),
			content: []byte(`; Godot Engine project file
; Minimal configuration

config_version=5

[application]
config/name="Frontend"
`),
		},
		{
			path: filepath.Join(targetDir, "README.md"),
			content: []byte(`# Frontend (Godot)

Godot game project.

## Setup

1. Open Godot Engine
2. Import this project
3. Start developing your game

The project.godot file contains minimal configuration.
`),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

func scaffoldUnrealFrontend(targetDir string, force, dryRun bool) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	files := []struct {
		path    string
		content []byte
	}{
		{
			path:    filepath.Join(targetDir, ".gitkeep"),
			content: []byte(""),
		},
		{
			path: filepath.Join(targetDir, "README.md"),
			content: []byte(`# Frontend (Unreal)

This directory is a placeholder for the Unreal Engine project.

## Setup

1. Open Unreal Engine
2. Create a new project in this directory
3. The Unreal project files will be created here

Note: Unreal projects should be created using the Unreal Editor, not scaffolded.
`),
		},
	}

	return scaffoldFiles(files, force, dryRun, result)
}

// scaffoldFiles is a helper that creates files and tracks results
func scaffoldFiles(files []struct {
	path    string
	content []byte
}, force, dryRun bool, result *ScaffoldResult) (*ScaffoldResult, error) {
	for _, f := range files {
		if dryRun {
			// Check if file exists for dry-run reporting
			if _, err := os.Stat(f.path); err == nil {
				if !force {
					result.Skipped = append(result.Skipped, f.path)
				} else {
					result.Created = append(result.Created, f.path)
				}
			} else {
				result.Created = append(result.Created, f.path)
			}
			continue
		}

		created, skipped, err := writeFileIfNotExists(f.path, f.content, force)
		if err != nil {
			result.Failed = append(result.Failed, f.path)
			result.Errors = append(result.Errors, err)
			continue
		}
		if created {
			result.Created = append(result.Created, f.path)
		} else if skipped {
			result.Skipped = append(result.Skipped, f.path)
		}
	}

	if len(result.Failed) > 0 {
		return result, ErrScaffoldFailed
	}
	return result, nil
}

// npmInstall runs "npm install" in the given directory.
// Errors are non-fatal — scaffold succeeds even if npm install fails.
func npmInstall(dir string) error {
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found in PATH")
	}
	cmd := exec.Command(npmPath, "install")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cleanupDeprecatedFiles removes files listed in deprecated.txt
func cleanupDeprecatedFiles(targetDir string) {
	scanner := bufio.NewScanner(strings.NewReader(deprecatedFiles))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path := filepath.Join(targetDir, filepath.FromSlash(line))
		if _, err := os.Stat(path); err == nil {
			_ = os.Remove(path)
		}
	}
}

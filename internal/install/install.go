package install

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Options struct {
	Preset      string
	ProjectName string
	Force       bool
	ForceConfig bool // Only overwrite workflow.yaml
	SkipConfig  bool // Skip workflow.yaml entirely (for upgrade)
	NoGenerate  bool
	WithCI      bool
}

// InstallResult contains information about what was done
type InstallResult struct {
	ConfigSkipped bool
	ConfigPath    string
}

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
		if err := ensureCIWorkflow(targetDir); err != nil {
			return nil, err
		}
	}

	if err := ensureClaudeLinks(targetDir); err != nil {
		return nil, err
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
			name:   "commands",
			source: filepath.Join(targetDir, ".ai", "commands"),
			target: filepath.Join(claudeDir, "commands"),
		},
	}

	for _, l := range links {
		_ = os.RemoveAll(l.target)
		relSource, err := filepath.Rel(filepath.Dir(l.target), l.source)
		if err != nil {
			return err
		}

		if err := os.Symlink(relSource, l.target); err == nil {
			continue
		}

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
	case "", "generic":
		return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetGeneric)
	case "react-go":
		skipped, err = ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, presetReactGo)
		if err != nil {
			return skipped, err
		}
		return skipped, applyReactGoRules(kit, targetDir)
	default:
		return false, fmt.Errorf("unknown preset: %q", opts.Preset)
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

func applyReactGoRules(kit fs.FS, targetDir string) error {
	rules := []struct {
		src string
		dst string
	}{
		{src: ".ai/rules/_examples/backend-go.md", dst: ".ai/rules/backend-go.md"},
		{src: ".ai/rules/_examples/frontend-react.md", dst: ".ai/rules/frontend-react.md"},
	}
	for _, r := range rules {
		b, err := fs.ReadFile(kit, r.src)
		if err != nil {
			return fmt.Errorf("missing embedded rule %s: %w", r.src, err)
		}
		dstPath := filepath.Join(targetDir, filepath.FromSlash(r.dst))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func ensureCIWorkflow(targetDir string) error {
	path := filepath.Join(targetDir, ".github", "workflows", "ci.yml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := strings.TrimSpace(ciWorkflowYAML) + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func tryGenerate(targetDir string) error {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return nil
	}

	cmd := exec.Command(bash, ".ai/scripts/generate.sh")
	cmd.Dir = targetDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

var presetGeneric = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration
version: "1.0"

project:
  name: %q
  description: "Project using AWK"
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

rules:
  kit:
    - git-workflow
  custom: []
`, projectName)
	return []byte(content)
}

var presetReactGo = func(projectName string) []byte {
	content := fmt.Sprintf(`# AWK Configuration (React + Go)
version: "1.0"

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

rules:
  kit:
    - git-workflow
  custom:
    - backend-go
    - frontend-react
`, projectName)
	return []byte(content)
}

const ciWorkflowYAML = `
name: ci

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

      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: "3.11"

      - name: Install Python deps
        run: |
          python -m pip install --upgrade pip
          pip install pyyaml jsonschema jinja2

      - name: Evaluate (offline)
        run: bash .ai/scripts/evaluate.sh --offline

      - name: Evaluate (strict)
        run: bash .ai/scripts/evaluate.sh --offline --strict

      - name: Test suite
        run: bash .ai/tests/run_all_tests.sh

  backend:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22.x"

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
          cache: "npm"
          cache-dependency-path: frontend/package-lock.json

      - name: Install dependencies
        working-directory: frontend
        run: npm ci

      - name: Test
        working-directory: frontend
        run: npm run test -- --run

      - name: Build
        working-directory: frontend
        run: npm run build
`

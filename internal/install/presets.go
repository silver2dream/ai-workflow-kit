package install

import (
	"fmt"
	"strings"
)

// A preset decides what a new project starts with: its workflow config and,
// with --scaffold, a project skeleton. Everything else, CI workflows
// included, is generated from that config.
const (
	// PresetGeneric assumes nothing about the stack: one repository, and the
	// build and test commands are left for the project to fill in.
	PresetGeneric = "generic"
	// PresetGodotGo is a monorepo with a Go backend and a Godot .NET frontend
	// written in C#.
	PresetGodotGo = "godot-go"
)

// Presets lists the preset names in the order they are shown.
var Presets = []string{PresetGeneric, PresetGodotGo}

// presetName resolves the preset a project gets when none is named.
func presetName(preset string) string {
	if preset == "" {
		return PresetGeneric
	}
	return preset
}

// presetConfigs builds the workflow config of each preset.
var presetConfigs = map[string]func(projectName string) []byte{
	PresetGeneric: genericConfig,
	PresetGodotGo: godotGoConfig,
}

// applyPreset writes the preset's workflow config unless the project already
// has one.
func applyPreset(targetDir string, opts Options) (skipped bool, err error) {
	// Skip config entirely for upgrade mode
	if opts.SkipConfig {
		return true, nil
	}
	build, ok := presetConfigs[presetName(opts.Preset)]
	if !ok {
		return false, unknownPreset(opts.Preset)
	}
	return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, build)
}

func unknownPreset(preset string) error {
	return fmt.Errorf("%w: %q (available: %s)", ErrUnknownPreset, preset, strings.Join(Presets, ", "))
}

// configSections are the sections every preset shares.
const configSections = `
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
  custom: []

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

func genericConfig(projectName string) []byte {
	head := fmt.Sprintf(`# AWK Configuration
version: "1.2"

project:
  name: %q
  description: "Project using AWK"
  type: "single-repo"

repos:
  - name: root
    path: ./
    type: root
    language: generic
    verify:
      # The commands that build and test this project. The Worker runs them
      # before it opens a pull request and the review runs the tests again,
      # so both must be set before the first run.
      build: ""
      test: ""
`, projectName)
	return []byte(head + configSections)
}

func godotGoConfig(projectName string) []byte {
	head := fmt.Sprintf(`# AWK Configuration (Go backend + Godot C# frontend)
version: "1.2"

project:
  name: %q
  description: "Go backend and Godot C# frontend"
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
    language: csharp
    verify:
      build: "dotnet build"
      test: "dotnet test"
`, projectName)
	return []byte(head + configSections)
}

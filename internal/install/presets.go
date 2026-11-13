package install

import (
	"fmt"
	"strconv"
	"strings"
)

// A preset decides what a new project starts with: its workflow config, the
// rules for its stack (starter_rules.go) and, with --scaffold, a project
// skeleton. Everything else, CI workflows included, is generated from that
// config.
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
var presetConfigs = map[string]func(projectName string, branches Branches) []byte{
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
	branches := opts.Branches.orDefault()
	return ensureWorkflowConfig(targetDir, opts.ProjectName, opts.Force || opts.ForceConfig, func(projectName string) []byte {
		return build(projectName, branches)
	})
}

func unknownPreset(preset string) error {
	return fmt.Errorf("%w: %q (available: %s)", ErrUnknownPreset, preset, strings.Join(Presets, ", "))
}

// sharedSections are the sections every preset shares. The rules a project's
// agents are told to read are the kit's and the ones the preset starts the
// project with: the list is taken from the files, so the two cannot differ.
func sharedSections(preset string, branches Branches) string {
	custom := "  custom: []\n"
	if rules := StarterRules(preset); len(rules) > 0 {
		custom = "  custom:\n"
		for _, rule := range rules {
			custom += "    - " + rule + "\n"
		}
	}
	return gitSection(branches) + configBeforeRules + "\nrules:\n  kit:\n    - git-workflow\n" + custom + configAfterRules
}

// gitSection says where pull requests go. The names are the caller's: init
// takes them from the repository, so that they are branches that exist.
func gitSection(branches Branches) string {
	branches = branches.orDefault()
	return fmt.Sprintf(`
git:
  # Pull requests are merged into the integration branch. The release branch
  # is where releases are cut from; with a single trunk the two are the same.
  integration_branch: %s
  release_branch: %s
  commit_format: "[type] subject"
`, strconv.Quote(branches.Integration), strconv.Quote(branches.Release))
}

const configBeforeRules = `

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
`

const configAfterRules = `
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

func genericConfig(projectName string, branches Branches) []byte {
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
	return []byte(head + sharedSections(PresetGeneric, branches))
}

func godotGoConfig(projectName string, branches Branches) []byte {
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
	return []byte(head + sharedSections(PresetGodotGo, branches))
}

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

// configHeader opens every preset's configuration. A new file holds what a
// person sets and nothing else: a key that is left out has its default, and
// no key is written that nothing reads.
const configHeader = `# The configuration of this project for AWK. A key that is left out takes its
# default; every key is described in
# https://github.com/silver2dream/ai-workflow-kit/blob/main/docs/user/configuration.md
#
# After a change, awkit generate writes CLAUDE.md, AGENTS.md and the CI
# workflows again, and awkit validate checks this file.
version: "1.2"
`

// sharedSections are the sections every preset shares. The rules a project's
// agents are told to read are the ones the preset starts the project with:
// the list is taken from the files, so the two cannot differ.
func sharedSections(preset string, branches Branches) string {
	custom := "  custom: []\n"
	if rules := StarterRules(preset); len(rules) > 0 {
		custom = "  custom:\n"
		for _, rule := range rules {
			custom += "    - " + rule + "\n"
		}
	}
	return gitSection(branches) + rulesComment + custom + configAfterRules
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
`, strconv.Quote(branches.Integration), strconv.Quote(branches.Release))
}

const rulesComment = `
rules:
  # The project's own rules: a name here is the file .claude/rules/<name>.md,
  # and a Worker is told to read the rules that are listed.
`

const configAfterRules = `
worker:
  # Who writes the code: codex or claude-code.
  backend: codex

review:
  # The score out of 10 a review has to give before a pull request is merged,
  # and how it is merged: squash, merge or rebase.
  score_threshold: 7
  merge_strategy: squash
`

func genericConfig(projectName string, branches Branches) []byte {
	head := fmt.Sprintf(configHeader+`
project:
  name: %q
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
	head := fmt.Sprintf(configHeader+`
project:
  name: %q
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

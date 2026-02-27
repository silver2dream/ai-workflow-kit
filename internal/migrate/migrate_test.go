package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const v1_0Config = `version: "1.0"

project:
  name: "test-project"
  type: "single-repo"

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    review_failed: "review-fail"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"

timeouts:
  git_seconds: 120
  gh_seconds: 60
  codex_minutes: 30
`

const v1_0ConfigFull = `version: "1.0"

project:
  name: "test-project"
  type: "single-repo"

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    review_failed: "review-fail"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
    merge_conflict: "merge-conflict"
    needs_rebase: "needs-rebase"
    completed: "completed"

timeouts:
  git_seconds: 120
  gh_seconds: 60
  codex_minutes: 30
  gh_retry_count: 3
  gh_retry_base_delay: 2

review:
  score_threshold: 7
  merge_strategy: squash
`

const v1_1Config = `version: "1.1"

project:
  name: "test-project"
  type: "single-repo"

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    review_failed: "review-failed"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"
`

const v1_1ConfigWithSections = `version: "1.1"

project:
  name: "test-project"
  type: "single-repo"

github:
  repo: ""
  labels:
    task: "ai-task"
    in_progress: "in-progress"
    pr_ready: "pr-ready"
    review_failed: "review-failed"
    worker_failed: "worker-failed"
    needs_human_review: "needs-human-review"

agents:
  builtin:
    - pr-reviewer
  custom: []

escalation:
  triggers: []
  max_consecutive_failures: 5

feedback:
  enabled: false
  max_history_in_prompt: 20

hooks: {}

worker:
  backend: claude-code
`

const v1_2Config = `version: "1.2"

project:
  name: "test-project"
  type: "single-repo"

github:
  repo: ""
  labels:
    task: "ai-task"

agents:
  builtin:
    - pr-reviewer
    - conflict-resolver
  custom: []

worker:
  backend: codex
`

// --- v1.0 → v1.1 Tests (run full chain to v1.2) ---

func TestMigrateV1_0ToV1_1_Basic(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_0Config), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Full chain: v1.0 → v1.1 → v1.2
	if len(applied) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(applied))
	}
	if applied[0].FromVersion != "1.0" || applied[0].ToVersion != "1.1" {
		t.Errorf("unexpected first migration: %+v", applied[0])
	}
	if applied[1].FromVersion != "1.1" || applied[1].ToVersion != "1.2" {
		t.Errorf("unexpected second migration: %+v", applied[1])
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Final version after full chain
	assertYAMLVersion(t, data, "1.2")

	// v1.0→v1.1 changes
	assertYAMLLabelValue(t, data, "review_failed", "review-failed")
	assertYAMLLabelValue(t, data, "merge_conflict", "merge-conflict")
	assertYAMLLabelValue(t, data, "needs_rebase", "needs-rebase")
	assertYAMLLabelValue(t, data, "completed", "completed")
	assertYAMLTimeoutValue(t, data, "gh_retry_count", 3)
	assertYAMLTimeoutValue(t, data, "gh_retry_base_delay", 2)
	assertYAMLReviewValue(t, data, "score_threshold", "7")
	assertYAMLReviewValue(t, data, "merge_strategy", "squash")

	// v1.1→v1.2 changes
	assertYAMLSectionExists(t, data, "agents")
	assertYAMLSectionExists(t, data, "escalation")
	assertYAMLSectionExists(t, data, "feedback")
	assertYAMLSectionExists(t, data, "hooks")
	assertYAMLSectionExists(t, data, "worker")
}

func TestMigrateV1_0ToV1_1_AlreadyHasFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_0ConfigFull), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Full chain: v1.0 → v1.1 → v1.2
	if len(applied) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(applied))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.2")
	assertYAMLLabelValue(t, data, "review_failed", "review-failed")

	// Existing fields should not be duplicated
	count := strings.Count(string(data), "merge_conflict")
	if count != 1 {
		t.Errorf("merge_conflict should appear exactly once, got %d", count)
	}
	count = strings.Count(string(data), "gh_retry_count")
	if count != 1 {
		t.Errorf("gh_retry_count should appear exactly once, got %d", count)
	}
}

// --- v1.1 → v1.2 Tests ---

func TestMigrateV1_1ToV1_2_Basic(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_1Config), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(applied) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(applied))
	}
	if applied[0].FromVersion != "1.1" || applied[0].ToVersion != "1.2" {
		t.Errorf("unexpected migration: %+v", applied[0])
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.2")

	// Verify new sections added
	assertYAMLSectionExists(t, data, "agents")
	assertYAMLSectionExists(t, data, "escalation")
	assertYAMLSectionExists(t, data, "feedback")
	assertYAMLSectionExists(t, data, "hooks")
	assertYAMLSectionExists(t, data, "worker")

	// Verify default values
	assertYAMLMapValue(t, data, "feedback", "enabled", "true")
	assertYAMLMapValue(t, data, "feedback", "max_history_in_prompt", "10")
	assertYAMLMapValue(t, data, "worker", "backend", "codex")
	assertYAMLMapValue(t, data, "escalation", "max_consecutive_failures", "3")
	assertYAMLMapValue(t, data, "escalation", "retry_count", "2")
	assertYAMLMapValue(t, data, "escalation", "retry_delay_seconds", "5")
	assertYAMLMapValue(t, data, "escalation", "max_single_pr_files", "50")
	assertYAMLMapValue(t, data, "escalation", "max_single_pr_lines", "500")
}

func TestMigrateV1_1ToV1_2_AlreadyHasSections(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_1ConfigWithSections), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(applied) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(applied))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.2")

	// Existing sections should not be duplicated
	for _, section := range []string{"agents", "escalation", "feedback", "hooks", "worker"} {
		count := strings.Count(string(data), section+":")
		if count != 1 {
			t.Errorf("%s should appear exactly once as a top-level key, got %d", section, count)
		}
	}

	// User-customized values should be preserved
	assertYAMLMapValue(t, data, "feedback", "enabled", "false")
	assertYAMLMapValue(t, data, "feedback", "max_history_in_prompt", "20")
	assertYAMLMapValue(t, data, "worker", "backend", "claude-code")
	assertYAMLMapValue(t, data, "escalation", "max_consecutive_failures", "5")
}

func TestMigrateV1_0ToV1_2_Chain(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_0Config), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should apply both migrations in one pass
	if len(applied) != 2 {
		t.Fatalf("expected 2 migrations (v1.0→v1.1→v1.2), got %d", len(applied))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.2")

	// v1.0→v1.1 changes present
	assertYAMLLabelValue(t, data, "review_failed", "review-failed")
	assertYAMLReviewValue(t, data, "score_threshold", "7")

	// v1.1→v1.2 changes present
	assertYAMLSectionExists(t, data, "agents")
	assertYAMLSectionExists(t, data, "escalation")
	assertYAMLSectionExists(t, data, "feedback")
	assertYAMLSectionExists(t, data, "hooks")
	assertYAMLSectionExists(t, data, "worker")
	assertYAMLMapValue(t, data, "worker", "backend", "codex")
}

// --- General migration tests ---

func TestMigrateAlreadyLatest(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_2Config), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(applied) != 0 {
		t.Fatalf("expected 0 migrations for already-latest config, got %d", len(applied))
	}
}

func TestMigrateMissingVersion(t *testing.T) {
	config := `project:
  name: "old-project"
  type: "single-repo"

github:
  repo: ""
  labels:
    task: "ai-task"
    review_failed: "review-fail"

timeouts:
  git_seconds: 120
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Full chain from assumed 1.0: v1.0 → v1.1 → v1.2
	if len(applied) != 2 {
		t.Fatalf("expected 2 migrations (from assumed 1.0), got %d", len(applied))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.2")
}

func TestMigrateDryRun(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_0Config), 0644); err != nil {
		t.Fatal(err)
	}

	applied, err := Run(configPath, true)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Full chain in dry-run
	if len(applied) != 2 {
		t.Fatalf("expected 2 migrations in dry-run, got %d", len(applied))
	}

	// File should NOT be modified
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.0")
}

func TestMigrateBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	original := []byte(v1_0Config)
	if err := os.WriteFile(configPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	bakPath := configPath + ".bak"
	bakData, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup file not found: %v", err)
	}

	if string(bakData) != string(original) {
		t.Error("backup content does not match original")
	}
}

func TestNeedsMigration(t *testing.T) {
	dir := t.TempDir()

	// v1.0 needs migration
	configPath := filepath.Join(dir, "v1_0.yaml")
	if err := os.WriteFile(configPath, []byte(v1_0Config), 0644); err != nil {
		t.Fatal(err)
	}
	ver, needed, err := NeedsMigration(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !needed {
		t.Error("v1.0 config should need migration")
	}
	if ver != "1.0" {
		t.Errorf("expected version 1.0, got %s", ver)
	}

	// v1.1 now needs migration (to v1.2)
	configPath2 := filepath.Join(dir, "v1_1.yaml")
	if err := os.WriteFile(configPath2, []byte(v1_1Config), 0644); err != nil {
		t.Fatal(err)
	}
	ver, needed, err = NeedsMigration(configPath2)
	if err != nil {
		t.Fatal(err)
	}
	if !needed {
		t.Error("v1.1 config should need migration (to v1.2)")
	}
	if ver != "1.1" {
		t.Errorf("expected version 1.1, got %s", ver)
	}

	// v1.2 does NOT need migration
	configPath3 := filepath.Join(dir, "v1_2.yaml")
	if err := os.WriteFile(configPath3, []byte(v1_2Config), 0644); err != nil {
		t.Fatal(err)
	}
	ver, needed, err = NeedsMigration(configPath3)
	if err != nil {
		t.Fatal(err)
	}
	if needed {
		t.Error("v1.2 config should not need migration")
	}
	if ver != "1.2" {
		t.Errorf("expected version 1.2, got %s", ver)
	}
}

func TestMigratePreservesUserValues(t *testing.T) {
	config := `version: "1.0"

project:
  name: "custom-project"
  type: "monorepo"

github:
  repo: "myorg/myrepo"
  labels:
    task: "custom-task"
    in_progress: "wip"
    pr_ready: "ready"
    review_failed: "review-fail"
    worker_failed: "failed"
    needs_human_review: "human"

timeouts:
  git_seconds: 300
  gh_seconds: 120
  codex_minutes: 60
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(configPath, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.2")

	// User-customized values should be preserved
	assertYAMLLabelValue(t, data, "task", "custom-task")
	assertYAMLLabelValue(t, data, "in_progress", "wip")
	assertYAMLLabelValue(t, data, "worker_failed", "failed")

	// review_failed should be fixed
	assertYAMLLabelValue(t, data, "review_failed", "review-failed")

	// Custom timeout values should be preserved
	assertYAMLTimeoutValue(t, data, "git_seconds", 300)
	assertYAMLTimeoutValue(t, data, "gh_seconds", 120)
}

// --- Helpers ---

func assertYAMLVersion(t *testing.T, data []byte, expected string) {
	t.Helper()
	var doc struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Version != expected {
		t.Errorf("expected version=%q, got %q", expected, doc.Version)
	}
}

func assertYAMLLabelValue(t *testing.T, data []byte, key, expected string) {
	t.Helper()
	var doc struct {
		GitHub struct {
			Labels map[string]string `yaml:"labels"`
		} `yaml:"github"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := doc.GitHub.Labels[key]
	if !ok {
		t.Errorf("label %q not found", key)
		return
	}
	if got != expected {
		t.Errorf("label %q: expected %q, got %q", key, expected, got)
	}
}

func assertYAMLTimeoutValue(t *testing.T, data []byte, key string, expected int) {
	t.Helper()
	var doc struct {
		Timeouts map[string]int `yaml:"timeouts"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := doc.Timeouts[key]
	if !ok {
		t.Errorf("timeout %q not found", key)
		return
	}
	if got != expected {
		t.Errorf("timeout %q: expected %d, got %d", key, expected, got)
	}
}

func assertYAMLReviewValue(t *testing.T, data []byte, key, expected string) {
	t.Helper()
	var doc struct {
		Review map[string]interface{} `yaml:"review"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := doc.Review[key]
	if !ok {
		t.Errorf("review %q not found", key)
		return
	}
	gotStr := fmt.Sprintf("%v", got)
	if gotStr != expected {
		t.Errorf("review %q: expected %q, got %q", key, expected, gotStr)
	}
}

func assertYAMLSectionExists(t *testing.T, data []byte, section string) {
	t.Helper()
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := doc[section]; !ok {
		t.Errorf("section %q not found in config", section)
	}
}

func assertYAMLMapValue(t *testing.T, data []byte, section, key, expected string) {
	t.Helper()
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sectionRaw, ok := doc[section]
	if !ok {
		t.Errorf("section %q not found", section)
		return
	}
	sectionMap, ok := sectionRaw.(map[string]interface{})
	if !ok {
		t.Errorf("section %q is not a map", section)
		return
	}
	got, ok := sectionMap[key]
	if !ok {
		t.Errorf("%s.%s not found", section, key)
		return
	}
	gotStr := fmt.Sprintf("%v", got)
	if gotStr != expected {
		t.Errorf("%s.%s: expected %q, got %q", section, key, expected, gotStr)
	}
}

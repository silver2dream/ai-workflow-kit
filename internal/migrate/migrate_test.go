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

	if len(applied) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(applied))
	}
	if applied[0].FromVersion != "1.0" || applied[0].ToVersion != "1.1" {
		t.Errorf("unexpected migration: %+v", applied[0])
	}

	// Read migrated file
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify version bumped
	assertYAMLVersion(t, data, "1.1")

	// Verify label value fixed
	assertYAMLLabelValue(t, data, "review_failed", "review-failed")

	// Verify missing labels added
	assertYAMLLabelValue(t, data, "merge_conflict", "merge-conflict")
	assertYAMLLabelValue(t, data, "needs_rebase", "needs-rebase")
	assertYAMLLabelValue(t, data, "completed", "completed")

	// Verify timeout fields added
	assertYAMLTimeoutValue(t, data, "gh_retry_count", 3)
	assertYAMLTimeoutValue(t, data, "gh_retry_base_delay", 2)

	// Verify review section added
	assertYAMLReviewValue(t, data, "score_threshold", "7")
	assertYAMLReviewValue(t, data, "merge_strategy", "squash")
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

	if len(applied) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(applied))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.1")
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

func TestMigrateAlreadyLatest(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(configPath, []byte(v1_1Config), 0644); err != nil {
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

	if len(applied) != 1 {
		t.Fatalf("expected 1 migration (from assumed 1.0), got %d", len(applied))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	assertYAMLVersion(t, data, "1.1")
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

	if len(applied) != 1 {
		t.Fatalf("expected 1 migration in dry-run, got %d", len(applied))
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

	configPath2 := filepath.Join(dir, "v1_1.yaml")
	if err := os.WriteFile(configPath2, []byte(v1_1Config), 0644); err != nil {
		t.Fatal(err)
	}
	ver, needed, err = NeedsMigration(configPath2)
	if err != nil {
		t.Fatal(err)
	}
	if needed {
		t.Error("v1.1 config should not need migration")
	}
	if ver != "1.1" {
		t.Errorf("expected version 1.1, got %s", ver)
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

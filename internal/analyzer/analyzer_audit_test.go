package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// readAuditMilestone / writeAuditMilestone (analyzer.go)
// ---------------------------------------------------------------------------

func TestReadAuditMilestone_NoFile(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, nil)
	val := a.readAuditMilestone("my-spec")
	if val != 0 {
		t.Errorf("readAuditMilestone with no file = %d, want 0", val)
	}
}

func TestReadAuditMilestone_WithFile(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "audit-milestone-my-spec.txt"), []byte("50"), 0644)

	a := New(dir, nil)
	val := a.readAuditMilestone("my-spec")
	if val != 50 {
		t.Errorf("readAuditMilestone = %d, want 50", val)
	}
}

func TestWriteAuditMilestone_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, nil)

	a.writeAuditMilestone("spec-x", 75)

	val := a.readAuditMilestone("spec-x")
	if val != 75 {
		t.Errorf("readAuditMilestone after write = %d, want 75", val)
	}
}

// ---------------------------------------------------------------------------
// shouldTriggerAudit (analyzer.go)
// ---------------------------------------------------------------------------

func TestShouldTriggerAudit_ZeroTotal(t *testing.T) {
	dir := t.TempDir()
	// Need non-nil config since shouldTriggerAudit accesses a.Config.Specs.Tracking.Audit.MilestoneInterval
	a := New(dir, &Config{})
	got := a.shouldTriggerAudit("spec", 0, 0)
	if got {
		t.Error("shouldTriggerAudit with total=0 should return false")
	}
}

func TestShouldTriggerAudit_BelowInterval(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, &Config{})
	// 2/10 = 20% which is below the default 25% interval
	got := a.shouldTriggerAudit("spec", 2, 10)
	if got {
		t.Error("shouldTriggerAudit at 20% should return false (below 25% threshold)")
	}
}

func TestShouldTriggerAudit_AtThreshold(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, &Config{})
	// 3/10 = 30% which is >= first 25% milestone
	got := a.shouldTriggerAudit("spec", 3, 10)
	if !got {
		t.Error("shouldTriggerAudit at 30% should return true (first 25% milestone)")
	}
}

func TestShouldTriggerAudit_AlreadyTriggered(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, &Config{})
	// First trigger at 30%
	a.shouldTriggerAudit("spec", 3, 10)
	// Second call at same progress — should NOT re-trigger
	got := a.shouldTriggerAudit("spec", 3, 10)
	if got {
		t.Error("shouldTriggerAudit should not re-trigger for the same milestone")
	}
}

func TestShouldTriggerAudit_CustomInterval(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	cfg.Specs.Tracking.Audit.MilestoneInterval = 50
	a := New(dir, cfg)

	// 4/10 = 40%, below custom 50% interval
	got := a.shouldTriggerAudit("spec", 4, 10)
	if got {
		t.Error("shouldTriggerAudit at 40% with 50% interval should return false")
	}

	// 6/10 = 60%, >= first 50% milestone
	got = a.shouldTriggerAudit("spec", 6, 10)
	if !got {
		t.Error("shouldTriggerAudit at 60% with 50% interval should return true")
	}
}

// ---------------------------------------------------------------------------
// maxReviewAttempts (analyzer.go)
// ---------------------------------------------------------------------------

func TestMaxReviewAttempts_Default(t *testing.T) {
	dir := t.TempDir()
	a := New(dir, nil)
	got := a.maxReviewAttempts()
	if got != DefaultMaxReviewAttempts {
		t.Errorf("maxReviewAttempts() = %d, want %d", got, DefaultMaxReviewAttempts)
	}
}

func TestMaxReviewAttempts_FromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	cfg.Escalation.MaxReviewAttempts = 5
	a := New(dir, cfg)
	got := a.maxReviewAttempts()
	if got != 5 {
		t.Errorf("maxReviewAttempts() = %d, want 5", got)
	}
}

func TestMaxReviewAttempts_ZeroUsesDefault(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{}
	cfg.Escalation.MaxReviewAttempts = 0
	a := New(dir, cfg)
	got := a.maxReviewAttempts()
	if got != DefaultMaxReviewAttempts {
		t.Errorf("maxReviewAttempts() with zero config = %d, want %d", got, DefaultMaxReviewAttempts)
	}
}

package analyzer

import (
	"testing"
)

// ---------------------------------------------------------------------------
// HooksConfig.GetHooks (config.go)
// ---------------------------------------------------------------------------

func TestGetHooks_PreDispatch(t *testing.T) {
	h := &HooksConfig{
		PreDispatch: []HookDef{{Command: "echo pre-dispatch"}},
	}
	hooks := h.GetHooks("pre_dispatch")
	if len(hooks) != 1 {
		t.Errorf("GetHooks(pre_dispatch) = %d hooks, want 1", len(hooks))
	}
}

func TestGetHooks_PostDispatch(t *testing.T) {
	h := &HooksConfig{
		PostDispatch: []HookDef{{Command: "echo post-dispatch"}},
	}
	hooks := h.GetHooks("post_dispatch")
	if len(hooks) != 1 {
		t.Errorf("GetHooks(post_dispatch) = %d hooks, want 1", len(hooks))
	}
}

func TestGetHooks_PreReview(t *testing.T) {
	h := &HooksConfig{
		PreReview: []HookDef{{Command: "lint"}},
	}
	hooks := h.GetHooks("pre_review")
	if len(hooks) != 1 {
		t.Errorf("GetHooks(pre_review) = %d hooks, want 1", len(hooks))
	}
}

func TestGetHooks_PostReview(t *testing.T) {
	h := &HooksConfig{
		PostReview: []HookDef{{Command: "notify"}},
	}
	hooks := h.GetHooks("post_review")
	if len(hooks) != 1 {
		t.Errorf("GetHooks(post_review) = %d hooks, want 1", len(hooks))
	}
}

func TestGetHooks_OnMerge(t *testing.T) {
	h := &HooksConfig{
		OnMerge: []HookDef{{Command: "deploy"}},
	}
	hooks := h.GetHooks("on_merge")
	if len(hooks) != 1 {
		t.Errorf("GetHooks(on_merge) = %d hooks, want 1", len(hooks))
	}
}

func TestGetHooks_OnFailure(t *testing.T) {
	h := &HooksConfig{
		OnFailure: []HookDef{{Command: "alert"}},
	}
	hooks := h.GetHooks("on_failure")
	if len(hooks) != 1 {
		t.Errorf("GetHooks(on_failure) = %d hooks, want 1", len(hooks))
	}
}

func TestGetHooks_UnknownEvent(t *testing.T) {
	h := &HooksConfig{}
	hooks := h.GetHooks("nonexistent_event")
	if hooks != nil {
		t.Errorf("GetHooks(unknown) = %v, want nil", hooks)
	}
}

func TestGetHooks_EmptyHooks(t *testing.T) {
	h := &HooksConfig{}
	hooks := h.GetHooks("pre_dispatch")
	if len(hooks) != 0 {
		t.Errorf("GetHooks on empty config = %d hooks, want 0", len(hooks))
	}
}

// ---------------------------------------------------------------------------
// CodexConfig.IsFullAuto (config.go)
// ---------------------------------------------------------------------------

func TestIsFullAuto_NilDefault(t *testing.T) {
	c := &CodexConfig{FullAuto: nil}
	if !c.IsFullAuto() {
		t.Error("IsFullAuto with nil = false, want true (default)")
	}
}

func TestIsFullAuto_ExplicitTrue(t *testing.T) {
	b := true
	c := &CodexConfig{FullAuto: &b}
	if !c.IsFullAuto() {
		t.Error("IsFullAuto with true = false, want true")
	}
}

func TestIsFullAuto_ExplicitFalse(t *testing.T) {
	b := false
	c := &CodexConfig{FullAuto: &b}
	if c.IsFullAuto() {
		t.Error("IsFullAuto with false = true, want false")
	}
}

// ---------------------------------------------------------------------------
// FeedbackConfig.IsEnabled (config.go)
// ---------------------------------------------------------------------------

func TestFeedbackConfig_IsEnabled_NilDefault(t *testing.T) {
	c := &FeedbackConfig{Enabled: nil}
	if !c.IsEnabled() {
		t.Error("IsEnabled with nil = false, want true (default)")
	}
}

func TestFeedbackConfig_IsEnabled_ExplicitTrue(t *testing.T) {
	b := true
	c := &FeedbackConfig{Enabled: &b}
	if !c.IsEnabled() {
		t.Error("IsEnabled with true = false, want true")
	}
}

func TestFeedbackConfig_IsEnabled_ExplicitFalse(t *testing.T) {
	b := false
	c := &FeedbackConfig{Enabled: &b}
	if c.IsEnabled() {
		t.Error("IsEnabled with false = true, want false")
	}
}

// ---------------------------------------------------------------------------
// EpicAuditConfig.IsAuditEnabled (config.go)
// ---------------------------------------------------------------------------

func TestIsAuditEnabled_NilDefault(t *testing.T) {
	c := &EpicAuditConfig{Enabled: nil}
	if !c.IsAuditEnabled() {
		t.Error("IsAuditEnabled with nil = false, want true (default)")
	}
}

func TestIsAuditEnabled_ExplicitTrue(t *testing.T) {
	b := true
	c := &EpicAuditConfig{Enabled: &b}
	if !c.IsAuditEnabled() {
		t.Error("IsAuditEnabled with true = false, want true")
	}
}

func TestIsAuditEnabled_ExplicitFalse(t *testing.T) {
	b := false
	c := &EpicAuditConfig{Enabled: &b}
	if c.IsAuditEnabled() {
		t.Error("IsAuditEnabled with false = true, want false")
	}
}

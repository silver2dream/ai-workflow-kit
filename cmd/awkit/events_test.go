package main

import (
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// --- colorizeLevel ---

func TestColorizeLevel_Error(t *testing.T) {
	got := colorizeLevel("error")
	if !strings.Contains(got, "ERROR") {
		t.Errorf("colorizeLevel(error) = %q, want ERROR", got)
	}
}

func TestColorizeLevel_Warn(t *testing.T) {
	got := colorizeLevel("warn")
	if !strings.Contains(got, "WARN") {
		t.Errorf("colorizeLevel(warn) = %q, want WARN", got)
	}
}

func TestColorizeLevel_Decision(t *testing.T) {
	got := colorizeLevel("decision")
	if !strings.Contains(got, "DECISION") {
		t.Errorf("colorizeLevel(decision) = %q, want DECISION", got)
	}
}

func TestColorizeLevel_Default(t *testing.T) {
	got := colorizeLevel("info")
	if !strings.Contains(got, "INFO") {
		t.Errorf("colorizeLevel(info) = %q, want INFO", got)
	}
}

// --- colorizeComponent ---

func TestColorizeComponent_Principal(t *testing.T) {
	got := colorizeComponent("principal")
	if !strings.Contains(got, "principal") {
		t.Errorf("colorizeComponent(principal) = %q", got)
	}
}

func TestColorizeComponent_Worker(t *testing.T) {
	got := colorizeComponent("worker")
	if !strings.Contains(got, "worker") {
		t.Errorf("colorizeComponent(worker) = %q", got)
	}
}

func TestColorizeComponent_Reviewer(t *testing.T) {
	got := colorizeComponent("reviewer")
	if !strings.Contains(got, "reviewer") {
		t.Errorf("colorizeComponent(reviewer) = %q", got)
	}
}

func TestColorizeComponent_GitHub(t *testing.T) {
	got := colorizeComponent("github")
	if !strings.Contains(got, "github") {
		t.Errorf("colorizeComponent(github) = %q", got)
	}
}

func TestColorizeComponent_Default(t *testing.T) {
	got := colorizeComponent("custom-component")
	if got != "custom-component" {
		t.Errorf("colorizeComponent(custom-component) = %q, want pass-through", got)
	}
}

// --- colorizeDecisionResult ---

func TestColorizeDecisionResult_Continue(t *testing.T) {
	got := colorizeDecisionResult("CONTINUE")
	if !strings.Contains(got, "CONTINUE") {
		t.Errorf("colorizeDecisionResult(CONTINUE) = %q", got)
	}
}

func TestColorizeDecisionResult_Retry(t *testing.T) {
	got := colorizeDecisionResult("RETRY")
	if !strings.Contains(got, "RETRY") {
		t.Errorf("colorizeDecisionResult(RETRY) = %q", got)
	}
}

func TestColorizeDecisionResult_Success(t *testing.T) {
	got := colorizeDecisionResult("SUCCESS")
	if !strings.Contains(got, "SUCCESS") {
		t.Errorf("colorizeDecisionResult(SUCCESS) = %q", got)
	}
}

func TestColorizeDecisionResult_StopComplete(t *testing.T) {
	got := colorizeDecisionResult("STOP_COMPLETE")
	if !strings.Contains(got, "STOP_COMPLETE") {
		t.Errorf("colorizeDecisionResult(STOP_COMPLETE) = %q", got)
	}
}

func TestColorizeDecisionResult_StopNone(t *testing.T) {
	got := colorizeDecisionResult("STOP_NONE")
	if !strings.Contains(got, "STOP_NONE") {
		t.Errorf("colorizeDecisionResult(STOP_NONE) = %q", got)
	}
}

func TestColorizeDecisionResult_FailFinal(t *testing.T) {
	got := colorizeDecisionResult("FAIL_FINAL")
	if !strings.Contains(got, "FAIL_FINAL") {
		t.Errorf("colorizeDecisionResult(FAIL_FINAL) = %q", got)
	}
}

func TestColorizeDecisionResult_Default(t *testing.T) {
	got := colorizeDecisionResult("UNKNOWN_STATE")
	if !strings.Contains(got, "UNKNOWN_STATE") {
		t.Errorf("colorizeDecisionResult(UNKNOWN_STATE) = %q", got)
	}
}

// --- formatConditions ---

func TestFormatConditions_Empty(t *testing.T) {
	got := formatConditions(nil)
	if got != "{}" {
		t.Errorf("formatConditions(nil) = %q, want {}", got)
	}
}

func TestFormatConditions_EmptyMap(t *testing.T) {
	got := formatConditions(map[string]any{})
	if got != "{}" {
		t.Errorf("formatConditions({}) = %q, want {}", got)
	}
}

func TestFormatConditions_WithEntries(t *testing.T) {
	got := formatConditions(map[string]any{"key": "value"})
	if !strings.Contains(got, "key=value") {
		t.Errorf("formatConditions({key:value}) = %q, want key=value", got)
	}
}

// --- formatEventData ---

func TestFormatEventData_NilData(t *testing.T) {
	e := trace.Event{Data: nil}
	got := formatEventData(e)
	if got != "" {
		t.Errorf("formatEventData(nil data) = %q, want empty", got)
	}
}

func TestFormatEventData_MapStringAny(t *testing.T) {
	e := trace.Event{Data: map[string]any{"key": "val"}}
	got := formatEventData(e)
	if !strings.Contains(got, "key=val") {
		t.Errorf("formatEventData(map[string]any) = %q, want key=val", got)
	}
}

func TestFormatEventData_MapStringAny_EmptyValues(t *testing.T) {
	e := trace.Event{Data: map[string]any{"a": "", "b": nil, "c": 0}}
	got := formatEventData(e)
	// All values are empty/nil/zero, should return ""
	if got != "" {
		t.Errorf("formatEventData(all empty) = %q, want empty", got)
	}
}

func TestFormatEventData_MapStringString(t *testing.T) {
	e := trace.Event{Data: map[string]string{"foo": "bar"}}
	got := formatEventData(e)
	if !strings.Contains(got, "foo=bar") {
		t.Errorf("formatEventData(map[string]string) = %q, want foo=bar", got)
	}
}

func TestFormatEventData_MapStringString_Empty(t *testing.T) {
	e := trace.Event{Data: map[string]string{"a": ""}}
	got := formatEventData(e)
	if got != "" {
		t.Errorf("formatEventData(empty string values) = %q, want empty", got)
	}
}

func TestFormatEventData_OtherType(t *testing.T) {
	e := trace.Event{Data: "some string"}
	got := formatEventData(e)
	// Other types fall through to return ""
	if got != "" {
		t.Errorf("formatEventData(string type) = %q, want empty", got)
	}
}

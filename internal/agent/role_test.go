package agent

import (
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	impl := Profile{Backend: ClaudeCode, Model: "sonnet", Effort: "high"}
	if p := Defaults(Implementer, impl); p.Role != Implementer || p.Backend != ClaudeCode || p.Model != "sonnet" || p.Tools != ShellTools || p.Minutes != 30 {
		t.Errorf("implementer: %+v", p)
	}
	if p := Defaults(Implementer, Profile{}); p.Backend != Codex {
		t.Errorf("an implementer with no backend runs on the project's default, codex: %+v", p)
	}
	// The reviewer reads, with another model than the implementer's.
	if p := Defaults(Reviewer, impl); p.Backend != ClaudeCode || p.Tools != ReadTools || p.Model == impl.Model {
		t.Errorf("reviewer: %+v", p)
	}
	if p := Defaults(Reviewer, Profile{Backend: ClaudeCode, Model: "opus"}); p.Model == "opus" {
		t.Errorf("reviewer of an opus implementer: %+v", p)
	}
	if p := Defaults(Resolver, impl); p.Backend != ClaudeCode || p.Model != "sonnet" || p.Tools != ShellTools {
		t.Errorf("resolver works as the implementer does: %+v", p)
	}
	for _, role := range []Role{Triage, Interpreter, Judge, Distiller} {
		if p := Defaults(role, impl); p.Tools != NoTools || p.Backend != ClaudeCode || p.SchemaRetries != 1 {
			t.Errorf("%s: %+v", role, p)
		}
	}
}

func TestResolve(t *testing.T) {
	impl := Profile{Backend: Codex}
	p, err := Resolve(Implementer, impl, Override{Backend: ClaudeCode, Model: "opus", Effort: "xhigh", Minutes: 45, USD: 2.5})
	if err != nil || p.Backend != ClaudeCode || p.Model != "opus" || p.Effort != "xhigh" || p.Minutes != 45 || p.USD != 2.5 {
		t.Fatalf("override: %+v, %v", p, err)
	}
	if p, err := Resolve(Reviewer, impl, Override{Tools: NoTools}); err != nil || p.Tools != NoTools {
		t.Errorf("narrowing a reviewer's tools: %+v, %v", p, err)
	}
	// Another backend does not take the default's model: a judge on codex is
	// no haiku.
	if p, err := Resolve(Judge, impl, Override{Backend: Codex}); err != nil || p.Model != "" || p.Effort != "" {
		t.Errorf("a judge moved to codex: %+v, %v", p, err)
	}
	if p, err := Resolve(Judge, impl, Override{Backend: Codex, Model: "gpt-5.5"}); err != nil || p.Model != "gpt-5.5" {
		t.Errorf("a judge moved to codex with a model: %+v, %v", p, err)
	}

	// Past a role's limit: refused, not trimmed.
	for name, c := range map[string]struct {
		role Role
		o    Override
		says string
	}{
		"a reviewer that writes":   {Reviewer, Override{Tools: EditTools}, "at most"},
		"a reviewer that runs":     {Reviewer, Override{Tools: ShellTools}, "at most"},
		"a judge with tools":       {Judge, Override{Tools: ReadTools}, "at most"},
		"unknown tools":            {Implementer, Override{Tools: "all"}, "none of"},
		"unknown backend":          {Implementer, Override{Backend: "gemini"}, "cannot run on"},
		"too long":                 {Reviewer, Override{Minutes: 600}, "1 to 120"},
		"a claude effort it lacks": {Reviewer, Override{Effort: "minimal"}, "claude CLI"},
		"a codex effort it lacks":  {Implementer, Override{Effort: "max"}, "codex takes"},
		"a negative spend":         {Implementer, Override{USD: -1}, "negative"},
	} {
		if _, err := Resolve(c.role, impl, c.o); err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Resolve("principal", impl, Override{}); err == nil || !strings.Contains(err.Error(), "no agent role") {
		t.Errorf("an unknown role: %v", err)
	}
}

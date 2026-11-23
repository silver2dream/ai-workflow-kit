// Package guard is what the guardrails of AWK share: what a check says about
// what it checked (a verdict), the pieces of context an agent is given with
// where each came from and how far it is trusted, and the checks that run on
// those pieces before they reach a prompt.
//
// A check is deterministic where it can be: a rule decides before a model
// does. A check that cannot decide does not let the thing through (fail
// closed), and says that it was the check that failed, not the content. See
// docs/developer/specs/agent-engine/design.md §9.
package guard

import (
	"fmt"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// What a check decided.
const (
	// Pass lets the thing through.
	Pass = "pass"
	// Flag lets it through and records why it was suspect.
	Flag = "flag"
	// Block stops it: the lane waits for a person.
	Block = "block"
)

// What a verdict is about.
const (
	// Security: something an attacker would do. A block stops the lane.
	Security = "security"
	// Quality: work not done as asked. A block sends the work back.
	Quality = "quality"
	// Info: worth knowing, never a block on its own.
	Info = "info"
)

// Where in the pipeline a check ran.
const (
	StageInput   = "input"
	StageTool    = "tool"
	StageOutput  = "output"
	StageMemory  = "memory"
	StageMonitor = "monitor"
)

// Verdict is what one check said about one thing.
type Verdict struct {
	Result string `json:"result"`
	Class  string `json:"class"`
	Stage  string `json:"stage"`
	// Rule names the rule or the check, with its version.
	Rule string `json:"rule"`
	// Piece names what was checked: a piece of context by its id, a file.
	Piece string `json:"piece,omitempty"`
	// Source is where what was checked came from, for a person to look at.
	Source string  `json:"source,omitempty"`
	Score  float64 `json:"score,omitempty"`
	// Reason says, for a person, why.
	Reason string `json:"reason"`
	// Evidence is the part that matched, shortened and with hidden
	// characters made visible. It is kept on this computer only: it is
	// never written to GitHub, where an agent could read it back.
	Evidence string `json:"evidence,omitempty"`
}

// Blocks reports whether any verdict blocks.
func Blocks(verdicts []Verdict) bool {
	for _, v := range verdicts {
		if v.Result == Block {
			return true
		}
	}
	return false
}

// Blocking returns the verdicts that block.
func Blocking(verdicts []Verdict) []Verdict {
	var out []Verdict
	for _, v := range verdicts {
		if v.Result == Block {
			out = append(out, v)
		}
	}
	return out
}

// Summary says what blocked, by rule and source, without the matched text:
// it is what goes to GitHub and to the result of a run.
func Summary(verdicts []Verdict) string {
	var parts []string
	for _, v := range Blocking(verdicts) {
		part := v.Rule
		if v.Source != "" {
			part += " in " + v.Source
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("guardrail blocked: %s", strings.Join(parts, "; "))
}

// Session is the event file a verdict goes to when the process has no event
// stream of its own (a run-issue started by hand, say): a verdict is a
// record of security, and is kept whoever ran the check.
const Session = "guard"

// Record writes each verdict that is not a pass to the event stream of the
// process, or, when it has none, to the guard's own event file of the
// project at stateRoot. `awkit events --issue N` finds it either way.
func Record(stateRoot string, verdicts []Verdict, issue int) {
	var kept []Verdict
	for _, v := range verdicts {
		if v.Result != Pass {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		return
	}
	w := trace.GetGlobalWriter()
	if w == nil {
		if stateRoot == "" {
			return
		}
		own, err := trace.NewEventWriter(stateRoot, Session)
		if err != nil {
			return
		}
		defer own.Close()
		w = own
	}
	for _, v := range kept {
		level := trace.LevelWarn
		if v.Result == Block {
			level = trace.LevelError
		}
		opts := []trace.EventOption{trace.WithData(v)}
		if issue > 0 {
			opts = append(opts, trace.WithIssue(issue))
		}
		_ = w.Write(trace.ComponentGuard, trace.TypeGuardrailVerdict, level, opts...)
	}
}

// evidence shortens what matched for a verdict and makes characters a
// person cannot see visible.
func evidence(text string) string {
	var b strings.Builder
	for _, r := range text {
		if hidden(r) {
			fmt.Fprintf(&b, "<U+%04X>", r)
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	const max = 160
	if runes := []rune(out); len(runes) > max {
		out = string(runes[:max]) + "…"
	}
	return out
}

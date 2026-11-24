package main

import (
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
)

// AWKIT_ENGINE says how `awkit run` runs the workflow: the engine unless it
// says otherwise, the engine with its shadow, or the Principal's workflow,
// kept for one version. A word that is no mode is given back, and the engine
// runs the workflow.
func TestEngineMode(t *testing.T) {
	for value, want := range map[string][2]string{
		"":          {modeEngine, ""},
		"engine":    {modeEngine, ""},
		" Engine ":  {modeEngine, ""},
		"shadow":    {modeShadow, ""},
		"SHADOW":    {modeShadow, ""},
		"principal": {modePrincipal, ""},
		"Principal": {modePrincipal, ""},
		"engines":   {modeEngine, "engines"},
	} {
		t.Setenv("AWKIT_ENGINE", value)
		if mode, unknown := engineMode(); mode != want[0] || unknown != want[1] {
			t.Errorf("AWKIT_ENGINE=%q: %q, %q; want %q, %q", value, mode, unknown, want[0], want[1])
		}
	}
}

// A use of the Principal's workflow is recorded in the run's session, so
// that the events say whether anything still uses it before it is removed;
// outside a run, a person is told instead.
func TestNoteDeprecated(t *testing.T) {
	root := t.TempDir()

	// Outside a run: no session, a word on stderr, no event.
	out := captureStderr(func() { noteDeprecated(root, "awkit analyze-next") })
	if !strings.Contains(out, "awkit analyze-next is part of the Principal's workflow, which the next version removes") {
		t.Errorf("outside a run, a person is not told: %q", out)
	}

	// In a run: an event in its session.
	id, err := session.NewManager(root).InitPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	if out := captureStderr(func() { noteDeprecated(root, "awkit check-result") }); out != "" {
		t.Errorf("in a run, stderr is written: %q", out)
	}
	// The same through the writer a command opened for the session.
	if err := trace.InitGlobalWriter(root, id); err != nil {
		t.Fatal(err)
	}
	noteDeprecated(root, "awkit stop-workflow")
	trace.CloseGlobalWriter()

	events, err := trace.NewEventReader(root).ReadSession(id)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range events {
		if e.Type == trace.TypeDeprecatedPath {
			paths = append(paths, e.Data.(map[string]any)["path"].(string))
		}
	}
	if got := strings.Join(paths, ", "); got != "awkit check-result, awkit stop-workflow" {
		t.Errorf("recorded: %q", got)
	}
}

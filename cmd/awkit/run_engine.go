package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/engine"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
	"github.com/silver2dream/ai-workflow-kit/internal/workflow"
)

// How `awkit run` runs the workflow (AWKIT_ENGINE).
const (
	// modeEngine, the default: the engine runs it.
	modeEngine = "engine"
	// modeShadow: the engine runs it, and each of its decisions is compared
	// with the one the Principal's analyzer would make (task 1.7).
	modeShadow = "shadow"
	// modePrincipal: the Principal's workflow runs it, in a Claude Code
	// session. It is kept for the one version after the engine became the
	// default, and removed in the next (design.md §14).
	modePrincipal = "principal"
)

// engineMode is how AWKIT_ENGINE asks `awkit run` to run the workflow. The
// engine runs it unless it says otherwise; a word that is no mode is given
// back in unknown, and the engine runs it.
func engineMode() (mode, unknown string) {
	switch value := strings.ToLower(strings.TrimSpace(os.Getenv("AWKIT_ENGINE"))); value {
	case "", modeEngine:
		return modeEngine, ""
	case modeShadow, modePrincipal:
		return value, ""
	default:
		return modeEngine, strings.TrimSpace(os.Getenv("AWKIT_ENGINE"))
	}
}

// noteDeprecated records that a part of the Principal's workflow was used.
// AWKIT_ENGINE=principal, the principal-workflow skill and the commands only
// it calls (analyze-next, check-result, stop-workflow) are removed in the
// version after the engine became the default (design.md §14); these events
// say whether anything still uses them. They go to the run's session; used
// outside a run, where there is no session to write to, a person is told.
func noteDeprecated(stateRoot, what string) {
	data := trace.WithData(map[string]any{"path": what, "removed": "in the version after the one the engine became the default in"})
	if trace.GetGlobalWriter() != nil {
		trace.WriteEvent(trace.ComponentPrincipal, trace.TypeDeprecatedPath, trace.LevelWarn, data)
		return
	}
	if session := readCurrentPrincipalSession(stateRoot); session != "" {
		if w, err := trace.NewEventWriter(stateRoot, session); err == nil {
			_ = w.Write(trace.ComponentPrincipal, trace.TypeDeprecatedPath, trace.LevelWarn, data)
			_ = w.Close()
			return
		}
	}
	fmt.Fprintf(os.Stderr, "awkit: %s is part of the Principal's workflow, which the next version removes: awkit run runs the engine without it\n", what)
}

// runEngine runs the engine as the project's run, in the run's lock that
// cmdRun holds, and ends it as the Principal ends one: `stop-workflow`'s
// report and marker, then how it ended.
func runEngine(mode string, issue int, logger *kickoff.RotatingLogger, output *kickoff.OutputFormatter, endSession func(string)) int {
	root, err := filepath.Abs(".")
	if err != nil {
		output.Error(fmt.Sprintf("The project's folder: %v", err))
		endSession("failed")
		return 1
	}
	var log io.Writer = os.Stdout
	if logger != nil {
		log = logger
	}
	e, err := engine.New(engine.Options{Root: root, Only: issue, Log: log, Shadow: mode == modeShadow})
	if err != nil {
		output.Error(fmt.Sprintf("The engine could not start: %v", err))
		endSession("failed")
		return 1
	}
	defer e.Journal.Close()

	fmt.Println("")
	output.Info("The engine runs the workflow. `awkit stop` stops it when its step is done, `awkit stop --now` at once.")
	if mode == modeShadow {
		output.Info("AWKIT_ENGINE=shadow: each of its decisions is compared with the one the Principal's workflow would make; `awkit status` says how they compared.")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	reason, err := engine.Serve(ctx, e, engine.ServeOptions{})
	if err != nil {
		output.Error(fmt.Sprintf("The engine stopped: %v", err))
	}
	if reason == "" || !workflow.IsValidExitReason(reason) {
		reason = "error_exit"
	}
	if _, err := workflow.StopWorkflow(context.Background(), workflow.StopWorkflowOptions{Reason: reason, StateRoot: root, GHTimeout: 60 * time.Second}); err != nil {
		output.Warning(fmt.Sprintf("The run's report could not be written: %v", err))
	}
	code := reportEnding(output, reason, "")
	switch {
	case reason == engine.EndComplete:
		endSession("completed")
	case err != nil:
		endSession("failed")
	default:
		endSession("stopped")
	}
	return code
}

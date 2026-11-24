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
	"github.com/silver2dream/ai-workflow-kit/internal/workflow"
)

// How `awkit run` runs the workflow (AWKIT_ENGINE).
const (
	// modeEngine: the engine runs it in the Principal's place.
	modeEngine = "engine"
	// modeShadow: the engine runs it, and each of its decisions is compared
	// with the one the Principal's analyzer would make (task 1.7).
	modeShadow = "shadow"
)

// engineMode is how AWKIT_ENGINE asks `awkit run` to run the workflow:
// modeEngine, modeShadow, or "" for the Principal. The Principal stays the
// default until the shadow has compared the two (task 1.7).
func engineMode() string {
	switch mode := strings.ToLower(strings.TrimSpace(os.Getenv("AWKIT_ENGINE"))); mode {
	case modeEngine, modeShadow:
		return mode
	}
	return ""
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
	output.Info(fmt.Sprintf("The engine runs the workflow (AWKIT_ENGINE=%s). `awkit stop` stops it when its step is done, `awkit stop --now` at once.", mode))
	if mode == modeShadow {
		output.Info("Each of its decisions is compared with the one the Principal's workflow would make; `awkit status` says how they compared.")
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

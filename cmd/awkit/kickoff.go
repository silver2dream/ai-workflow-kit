package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

func usageKickoff() {
	fmt.Fprint(os.Stderr, `Start the AI workflow with PTY and progress monitoring

Usage:
  awkit kickoff [options]

Options:
  --dry-run     Only perform pre-flight checks without starting the workflow
  --background  Run the workflow in background mode (output to log file)
  --resume      Resume from the last saved state
  --fresh       Ignore saved state and start fresh
  --force       Auto-delete STOP marker without asking

Examples:
  awkit kickoff
  awkit kickoff --dry-run
  awkit kickoff --background
  awkit kickoff --resume
  awkit kickoff --fresh
  awkit kickoff --force
`)
}

func cmdKickoff(args []string) int {
	fs := flag.NewFlagSet("kickoff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageKickoff

	dryRun := fs.Bool("dry-run", false, "")
	background := fs.Bool("background", false, "")
	resume := fs.Bool("resume", false, "")
	fresh := fs.Bool("fresh", false, "")
	force := fs.Bool("force", false, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showHelp || *showHelpShort {
		usageKickoff()
		return 0
	}

	// Paths
	configPath := filepath.Join(".ai", "config", "workflow.yaml")
	lockFile := filepath.Join(".ai", "state", "kickoff.lock")
	stateFile := filepath.Join(".ai", "state", "last_run.json")
	logDir := filepath.Join(".ai", "exe-logs")

	output := kickoff.NewOutputFormatter(os.Stdout)

	fmt.Println("")
	fmt.Println("AWK Kickoff")
	fmt.Println("")

	// Pre-flight checks
	preflight := kickoff.NewPreflightChecker(configPath, lockFile)
	preflight.SetForceDelete(*force)
	results, err := preflight.RunAll()

	for _, r := range results {
		if r.Passed {
			if r.Warning {
				output.Warning(fmt.Sprintf("%s: %s", r.Name, r.Message))
			} else {
				output.Success(fmt.Sprintf("%s: %s", r.Name, r.Message))
			}
		} else {
			output.Error(fmt.Sprintf("%s: %s", r.Name, r.Message))
		}
	}

	if err != nil {
		fmt.Println("")
		output.Error(fmt.Sprintf("Pre-flight check failed: %v", err))
		return 1
	}

	if *dryRun {
		fmt.Println("")
		output.Success("Dry run complete. All pre-flight checks passed.")
		return 0
	}

	// Lock manager
	lock := kickoff.NewLockManager(lockFile)
	lock.SetupSignalHandler()

	if err := lock.Acquire(); err != nil {
		output.Error(fmt.Sprintf("Failed to acquire lock: %v", err))
		return 1
	}
	defer lock.Release()

	// State manager
	state := kickoff.NewStateManager(stateFile)

	// Check for existing state
	if !*fresh && !*resume && state.HasState() {
		if state.IsStale() {
			output.Warning("Found saved state older than 24 hours. Consider starting fresh.")
		}

		reader := bufio.NewReader(os.Stdin)
		shouldResume, err := kickoff.PromptResumeOrFresh(reader)
		if err != nil {
			output.Error(fmt.Sprintf("Failed to read input: %v", err))
			return 1
		}
		*resume = shouldResume
	}

	if *resume {
		savedState, err := state.LoadState()
		if err != nil {
			output.Error(fmt.Sprintf("Failed to load state: %v", err))
			return 1
		}
		output.Info(fmt.Sprintf("Resuming from phase: %s", savedState.Phase))
	}

	// Logger
	var logger *kickoff.RotatingLogger
	if *background {
		var err error
		logger, err = kickoff.NewRotatingLogger(logDir)
		if err != nil {
			output.Error(fmt.Sprintf("Failed to create logger: %v", err))
			return 1
		}
		defer logger.Close()
		output.Info(fmt.Sprintf("Background mode: logging to %s", logger.FilePath()))
	}

	// Get config
	config := preflight.Config()
	if config == nil {
		output.Error("Configuration not loaded")
		return 1
	}

	// Build Claude CLI command
	// Use -p to pass the prompt directly (like: echo "/start-work --autonomous" | claude --print)
	claudeCmd := "claude"
	claudeArgs := []string{
		"--print",
		"-p", "/start-work --autonomous",
	}

	fmt.Println("")
	output.Info(fmt.Sprintf("Starting workflow for project: %s", config.Project.Name))
	fmt.Println("")

	// Create PTY executor
	executor, err := kickoff.NewPTYExecutor(claudeCmd, claudeArgs)
	if err != nil {
		output.Error(fmt.Sprintf("Failed to create executor: %v", err))
		return 1
	}

	// Signal handler
	signalHandler := kickoff.NewSignalHandler(executor, state, lock)
	signalHandler.Setup()

	// Start executor
	if err := executor.Start(); err != nil {
		output.Error(fmt.Sprintf("Failed to start Claude CLI: %v", err))
		return 1
	}

	if executor.IsFallback() {
		output.Warning("PTY initialization failed, using standard execution")
	}

	// Output parser and monitor management
	var currentMonitor *kickoff.IssueMonitor
	var currentSpinner *kickoff.Spinner

	parser := kickoff.NewOutputParser(
		func(issueID int) {
			// Stop previous monitor if any
			if currentMonitor != nil {
				currentMonitor.Stop("new_issue")
			}
			if currentSpinner != nil {
				currentSpinner.Stop("")
			}

			// Create new spinner and monitor
			currentSpinner = kickoff.NewSpinner(issueID, os.Stdout)
			currentMonitor = kickoff.NewIssueMonitor(issueID, currentSpinner)

			currentMonitor.SetCommentCallback(func(commentType, prURL string) {
				if currentSpinner != nil {
					currentSpinner.Pause()
					currentSpinner.ClearLine()
				}

				if prURL != "" {
					output.WorkerMessage(issueID, fmt.Sprintf("%s (PR: %s)", commentType, prURL))
				} else {
					output.WorkerMessage(issueID, commentType)
				}

				if currentSpinner != nil {
					currentSpinner.Resume()
				}
			})

			signalHandler.AddMonitor(currentMonitor)
			currentSpinner.Start()
			currentMonitor.Start()
		},
		func() {
			// STEP-4 detected
			if currentMonitor != nil {
				currentMonitor.Stop("step4_detected")
				signalHandler.RemoveMonitor(currentMonitor)
			}
			if currentSpinner != nil {
				duration := currentSpinner.Duration()
				currentSpinner.Stop(fmt.Sprintf("✓ [#%d] Worker 完成 (%s)",
					currentMonitor.IssueID(),
					formatDuration(duration)))
			}
			currentMonitor = nil
			currentSpinner = nil
		},
	)

	// Read and process output
	outputReader := executor.Output()
	scanner := bufio.NewScanner(outputReader)

	// Also write to logger if in background mode
	var writers []io.Writer
	writers = append(writers, os.Stdout)
	if logger != nil {
		writers = append(writers, logger)
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Pause spinner while printing
		if currentSpinner != nil {
			currentSpinner.Pause()
			currentSpinner.ClearLine()
		}

		// Write to all outputs
		for _, w := range writers {
			fmt.Fprintln(w, line)
		}

		// Resume spinner
		if currentSpinner != nil {
			currentSpinner.Resume()
		}

		// Parse for STEP-3/STEP-4
		parser.Parse(line)
	}

	// Wait for executor to finish
	if err := executor.Wait(); err != nil {
		// Check if it was a signal
		if !signalHandler.IsShutdown() {
			output.Error(fmt.Sprintf("Claude CLI exited with error: %v", err))
		}
	}

	// Cleanup
	if currentMonitor != nil {
		currentMonitor.Stop("process_exit")
	}
	if currentSpinner != nil {
		currentSpinner.Stop("")
	}

	executor.Close()

	fmt.Println("")
	output.Success("Workflow completed")

	return 0
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

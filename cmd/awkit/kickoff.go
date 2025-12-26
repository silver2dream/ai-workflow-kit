package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Initialize loop_count for Loop Safety mechanism
	loopCountFile := filepath.Join(".ai", "state", "loop_count")
	if err := os.MkdirAll(filepath.Dir(loopCountFile), 0755); err != nil {
		output.Error(fmt.Sprintf("Failed to create state directory: %v", err))
		return 1
	}
	if err := os.WriteFile(loopCountFile, []byte("0"), 0644); err != nil {
		output.Error(fmt.Sprintf("Failed to initialize loop_count: %v", err))
		return 1
	}

	// Initialize consecutive_failures
	consecutiveFailuresFile := filepath.Join(".ai", "state", "consecutive_failures")
	if err := os.WriteFile(consecutiveFailuresFile, []byte("0"), 0644); err != nil {
		output.Error(fmt.Sprintf("Failed to initialize consecutive_failures: %v", err))
		return 1
	}

	// Initialize Principal session
	sessionCmd := exec.Command("bash", ".ai/scripts/session_manager.sh", "init_principal_session")
	sessionOutput, err := sessionCmd.Output()
	if err != nil {
		output.Warning(fmt.Sprintf("Failed to initialize session: %v", err))
		// Continue without session - not fatal
	} else {
		sessionID := strings.TrimSpace(string(sessionOutput))
		if sessionID != "" {
			output.Success(fmt.Sprintf("Session: %s", sessionID))
		}
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
	// Use stream-json format for real-time streaming output
	// Use principal-workflow Skill for deterministic workflow execution
	claudeCmd := "claude"
	claudeArgs := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"-p", "Use the principal-workflow Skill. Start the main loop immediately.",
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

	// Create fan-in manager for log aggregation
	fanIn := kickoff.NewFanInManager(1000)
	defer fanIn.Stop()

	// Register fan-in manager with signal handler for cleanup
	signalHandler.SetFanInManager(fanIn)

	// Start principal log tailer
	fanIn.StartPrincipalTailer(logDir)

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

	parser := kickoff.NewOutputParserWithTailerCallbacks(
		func(issueID int) {
			// onIssueStart - Start spinner and monitor
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
			// onIssueEnd - Worker complete detected
			if currentMonitor != nil {
				currentMonitor.Stop("worker_complete")
				signalHandler.RemoveMonitor(currentMonitor)
			}
			if currentSpinner != nil {
				duration := currentSpinner.Duration()
				currentSpinner.Stop(fmt.Sprintf("✓ [#%d] Worker completed (%s)",
					currentMonitor.IssueID(),
					formatDuration(duration)))
			}
			currentMonitor = nil
			currentSpinner = nil
		},
		func(issueID int) {
			// onDispatchWorker - Start worker tailer
			fanIn.StartWorkerTailer(logDir, issueID)
		},
		func() {
			// onWorkerStatus - Stop worker tailer
			fanIn.StopWorkerTailer()
		},
	)

	// Read and process output (stream-json format) via goroutine
	outputReader := executor.Output()

	// Increase scanner buffer for large JSON lines
	const maxScanTokenSize = 1024 * 1024 // 1MB

	// Claude stream producer goroutine
	go func() {
		defer fanIn.Stop() // Close channel when executor finishes (unblocks main loop)

		scanner := bufio.NewScanner(outputReader)
		buf := make([]byte, maxScanTokenSize)
		scanner.Buffer(buf, maxScanTokenSize)

		for scanner.Scan() {
			line := scanner.Text()

			// Parse JSON to extract text content
			text := extractTextFromStreamJSON(line)
			if text == "" {
				continue
			}

			// Send to fan-in channel
			fanIn.SendClaudeLine(text)
		}
	}()

	// Also write to logger if in background mode
	var writers []io.Writer
	if logger != nil {
		writers = append(writers, logger)
	}

	// Main goroutine: consume from fan-in channel
	for logLine := range fanIn.Channel() {
		// Pause spinner while printing
		if currentSpinner != nil {
			currentSpinner.Pause()
			currentSpinner.ClearLine()
		}

		// Colorize log labels and write to stdout
		colorizedLine := output.ColorizeLogLine(logLine.Text)
		fmt.Println(colorizedLine)

		// Write to logger if in background mode (no color)
		for _, w := range writers {
			fmt.Fprintln(w, logLine.Text)
		}

		// Resume spinner
		if currentSpinner != nil {
			currentSpinner.Resume()
		}

		// Parse for workflow events
		parser.Parse(logLine.Text)
	}

	// Wait for executor to finish
	exitCode := 0
	if err := executor.Wait(); err != nil {
		// Check if it was a signal
		if !signalHandler.IsShutdown() {
			output.Error(fmt.Sprintf("Claude CLI exited with error: %v", err))
		}
		exitCode = 1
	}

	// Cleanup
	if currentMonitor != nil {
		currentMonitor.Stop("process_exit")
	}
	if currentSpinner != nil {
		currentSpinner.Stop("")
	}

	executor.Close()

	if exitCode == 0 {
		fmt.Println("")
		output.Success("Workflow completed")
	}

	return exitCode
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// extractTextFromStreamJSON extracts text content from Claude CLI stream-json output
// Stream JSON format has different event types:
// - {"type":"system","subtype":"init",...} - initialization
// - {"type":"assistant","message":{...}} - assistant response with content
// - {"type":"result","subtype":"success",...} - final result
func extractTextFromStreamJSON(line string) string {
	if line == "" {
		return ""
	}

	// Quick check if it's JSON
	if line[0] != '{' {
		return line // Not JSON, return as-is
	}

	// Parse JSON
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return "" // Invalid JSON, skip
	}

	eventType, _ := event["type"].(string)

	switch eventType {
	case "assistant":
		// Extract text from assistant message
		message, ok := event["message"].(map[string]any)
		if !ok {
			return ""
		}
		content, ok := message["content"].([]any)
		if !ok {
			return ""
		}

		var texts []string
		for _, item := range content {
			if contentItem, ok := item.(map[string]any); ok {
				contentType, _ := contentItem["type"].(string)
				switch contentType {
				case "text":
					if text, ok := contentItem["text"].(string); ok {
						texts = append(texts, text)
					}
				case "tool_use":
					// Show bash commands being executed
					toolName, _ := contentItem["name"].(string)
					if toolName == "Bash" || toolName == "bash" || toolName == "execute_bash" {
						if input, ok := contentItem["input"].(map[string]any); ok {
							if cmd, ok := input["command"].(string); ok {
								texts = append(texts, fmt.Sprintf("[EXEC] %s", cmd))
							}
						}
					}
				}
			}
		}
		return strings.Join(texts, "\n")

	case "user":
		// Extract tool_result from user message (contains bash output)
		message, ok := event["message"].(map[string]any)
		if !ok {
			return ""
		}
		content, ok := message["content"].([]any)
		if !ok {
			return ""
		}

		var texts []string
		for _, item := range content {
			if contentItem, ok := item.(map[string]any); ok {
				contentType, _ := contentItem["type"].(string)
				if contentType == "tool_result" {
					if output, ok := contentItem["content"].(string); ok && output != "" {
						texts = append(texts, strings.TrimSpace(output))
					}
				}
			}
		}
		return strings.Join(texts, "\n")

	case "content_block_delta":
		// Handle streaming content deltas
		if delta, ok := event["delta"].(map[string]any); ok {
			if text, ok := delta["text"].(string); ok {
				return text
			}
		}
		return ""

	case "result":
		// Skip result event - it's just a summary of what was already output
		// The actual content was already streamed via "assistant" events
		return ""

	default:
		// Skip other event types (system, etc.)
		return ""
	}
}

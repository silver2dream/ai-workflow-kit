package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/session"
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

	if err := os.MkdirAll(logDir, 0755); err != nil {
		output.Error(fmt.Sprintf("Failed to create log directory: %v", err))
		return 1
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

	principalSessionID := ""
	var endSessionOnce sync.Once
	sessionMgr := session.NewManager(".")
	endPrincipalSession := func(reason string) {
		if principalSessionID == "" {
			return
		}
		endSessionOnce.Do(func() {
			// Try Go implementation first
			if err := sessionMgr.EndPrincipal(principalSessionID, reason); err == nil {
				return
			}
			// Fallback to bash script
			if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
				cmd := exec.Command("bash", ".ai/scripts/session_manager.sh", "end_principal_session", principalSessionID, reason)
				cmd.Stdout = io.Discard
				cmd.Stderr = io.Discard
				_ = cmd.Run()
			}
		})
	}

	// Initialize Principal session
	sessionID, err := sessionMgr.InitPrincipal()
	if err != nil {
		// Fallback to bash script
		if os.Getenv("AWKIT_USE_SCRIPT") == "1" {
			sessionCmd := exec.Command("bash", ".ai/scripts/session_manager.sh", "init_principal_session")
			sessionOutput, err := sessionCmd.Output()
			if err != nil {
				output.Warning(fmt.Sprintf("Failed to initialize session: %v", err))
			} else {
				sessionID = strings.TrimSpace(string(sessionOutput))
			}
		} else {
			output.Warning(fmt.Sprintf("Failed to initialize session: %v", err))
		}
	}
	if sessionID != "" {
		principalSessionID = sessionID
		defer endPrincipalSession("aborted")
		output.Success(fmt.Sprintf("Session: %s", sessionID))
	}

	// Lock manager
	lock := kickoff.NewLockManager(lockFile)

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
		// G14 fix: validate state integrity
		if savedState.Phase == "" {
			output.Warning("Saved state has empty phase, starting fresh")
			*resume = false
		} else if savedState.SavedAt.IsZero() {
			output.Warning("Saved state has invalid timestamp, starting fresh")
			*resume = false
		} else {
			output.Info(fmt.Sprintf("Resuming from phase: %s", savedState.Phase))
		}
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
		"-p", "Run the AWK principal workflow.",
	}

	fmt.Println("")
	output.Info(fmt.Sprintf("Starting workflow for project: %s", config.Project.Name))
	fmt.Println("")

	if os.Getenv("AWKIT_KICKOFF_LEGACY") == "1" {
	// Create PTY executor
	executor, err := kickoff.NewPTYExecutor(claudeCmd, claudeArgs)
	if err != nil {
		output.Error(fmt.Sprintf("Failed to create executor: %v", err))
		return 1
	}

	// Signal handler
	signalHandler := kickoff.NewSignalHandler(executor, state, lock)
	signalHandler.SetCleanupCallback(func() {
		endPrincipalSession("interrupted")
	})
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

	if exitCode != 0 {
		endPrincipalSession("failed")
		return exitCode
	}

	// Respect STOP marker mid-run (created by Ctrl+C or manual request).
	if fileExists(filepath.Join(".ai", "state", "STOP")) {
		fmt.Println("")
		output.Warning("Workflow stopped (STOP marker present)")
		endPrincipalSession("stopped")
		return 0
	}

	next, err := runAnalyzeNext(context.Background(), analyzeNextArgs{
		Timeout:            30 * time.Second,
		PrincipalSessionID: principalSessionID,
	})
	if err != nil {
		fmt.Println("")
		output.Warning(fmt.Sprintf("Workflow paused (failed to determine next action: %v)", err))
		output.Info("Run `awkit status` for offline details.")
		endPrincipalSession("paused")
		return 1
	}

	fmt.Println("")
	if next.NextAction == "all_complete" {
		output.Success("Workflow completed")
		endPrincipalSession("completed")
		return 0
	}

	if next.NextAction == "none" {
		reason := strings.TrimSpace(next.ExitReason)
		if reason == "" {
			reason = "none"
		}
		output.Warning(fmt.Sprintf("Workflow stopped (%s)%s", reason, formatAnalyzeNextContext(next)))
		endPrincipalSession("stopped")
		return 1
	}

	output.Warning(fmt.Sprintf("Workflow paused (pending: %s)%s", next.NextAction, formatAnalyzeNextContext(next)))
	endPrincipalSession("paused")
	return 1
	}

	// Multi-session loop: restart Principal when pending work remains.
	stopMarker := filepath.Join(".ai", "state", "STOP")
	maxSessions := getEnvInt("AWKIT_MAX_SESSIONS", 50)

	// G8 fix: exponential backoff for session restarts
	const (
		minRestartDelay = 3 * time.Second
		maxRestartDelay = 60 * time.Second
	)
	restartDelay := minRestartDelay

	signalHandler := kickoff.NewSignalHandler(nil, state, lock)
	signalHandler.SetCleanupCallback(func() {
		endPrincipalSession("interrupted")
	})
	signalHandler.Setup()

	var lastNext analyzeNextVars

	for sessionIndex := 1; sessionIndex <= maxSessions; sessionIndex++ {
		if fileExists(stopMarker) {
			fmt.Println("")
			output.Warning("Workflow stopped (STOP marker present)")
			endPrincipalSession("stopped")
			return 0
		}

		if sessionIndex == 1 {
			output.Info(fmt.Sprintf("Starting Principal session (1/%d)...", maxSessions))
		} else {
			output.Info(fmt.Sprintf("Restarting Principal session (%d/%d)...", sessionIndex, maxSessions))
		}

		exitCode, err := runClaudeSession(runClaudeSessionArgs{
			ClaudeCmd:     claudeCmd,
			ClaudeArgs:    claudeArgs,
			LogDir:        logDir,
			Logger:        logger,
			SignalHandler: signalHandler,
			Output:        output,
		})
		if err != nil {
			fmt.Println("")
			output.Error(fmt.Sprintf("Workflow failed (Claude session error): %v", err))
			endPrincipalSession("failed")
			return 1
		}
		if exitCode != 0 {
			fmt.Println("")
			output.Error("Workflow failed (Claude session exited with error)")
			endPrincipalSession("failed")
			return 1
		}

		if fileExists(stopMarker) {
			fmt.Println("")
			output.Warning("Workflow stopped (STOP marker present)")
			endPrincipalSession("stopped")
			return 0
		}

		next, err := runAnalyzeNext(context.Background(), analyzeNextArgs{
			Timeout:            30 * time.Second,
			PrincipalSessionID: principalSessionID,
		})
		if err != nil {
			fmt.Println("")
			output.Warning(fmt.Sprintf("Workflow paused (failed to determine next action: %v)", err))
			output.Info("Run `awkit status` for offline details.")
			endPrincipalSession("paused")
			return 1
		}
		lastNext = next

		switch next.NextAction {
		case "all_complete":
			fmt.Println("")
			output.Success("Workflow completed")
			endPrincipalSession("completed")
			return 0
		case "none":
			reason := strings.TrimSpace(next.ExitReason)
			if reason == "" {
				reason = "none"
			}
			fmt.Println("")
			output.Warning(fmt.Sprintf("Workflow stopped (%s)%s", reason, formatAnalyzeNextContext(next)))
			endPrincipalSession("stopped")
			return 1
		default:
			output.Info(fmt.Sprintf("Pending: %s%s (restarting in %s)", next.NextAction, formatAnalyzeNextContext(next), restartDelay))
			time.Sleep(restartDelay)
			// G8 fix: exponential backoff - double delay for next restart, capped at max
			restartDelay = min(restartDelay*2, maxRestartDelay)
		}
	}

	fmt.Println("")
	output.Warning(fmt.Sprintf("Workflow paused (max sessions reached: %d)%s", maxSessions, formatAnalyzeNextContext(lastNext)))
	output.Info("Run `awkit status` for offline details.")
	endPrincipalSession("paused")
	return 1
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
		// Only extract Bash tool_use commands, skip text content (Claude's narration)
		// The tailers handle [PRINCIPAL] and [WORKER] log output
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
				// Only extract Bash commands, skip "text" (Claude's narration)
				if contentType == "tool_use" {
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
		// Skip tool_result extraction - the tailers handle log output
		// The eval-able variables (NEXT_ACTION=...) are for Claude's context, not user display
		return ""

	case "content_block_delta":
		// Skip streaming text deltas - we don't want Claude's narration
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

type analyzeNextVars struct {
	NextAction  string
	IssueNumber string
	PRNumber    string
	SpecName    string
	TaskLine    string
	ExitReason  string
}

type analyzeNextArgs struct {
	Timeout            time.Duration
	PrincipalSessionID string
}

func runAnalyzeNext(ctx context.Context, args analyzeNextArgs) (analyzeNextVars, error) {
	ctx, cancel := context.WithTimeout(ctx, args.Timeout)
	defer cancel()

	a := analyzer.New(".", nil)
	decision, err := a.Decide(ctx)
	if err != nil {
		return analyzeNextVars{}, fmt.Errorf("analyze-next failed: %w", err)
	}

	return analyzeNextVars{
		NextAction:  decision.NextAction,
		IssueNumber: strconv.Itoa(decision.IssueNumber),
		PRNumber:    strconv.Itoa(decision.PRNumber),
		SpecName:    decision.SpecName,
		TaskLine:    strconv.Itoa(decision.TaskLine),
		ExitReason:  decision.ExitReason,
	}, nil
}

func parseAnalyzeNextOutput(out string) analyzeNextVars {
	var v analyzeNextVars
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "NEXT_ACTION":
			v.NextAction = strings.TrimSpace(value)
		case "ISSUE_NUMBER":
			v.IssueNumber = strings.TrimSpace(value)
		case "PR_NUMBER":
			v.PRNumber = strings.TrimSpace(value)
		case "SPEC_NAME":
			v.SpecName = strings.TrimSpace(value)
		case "TASK_LINE":
			v.TaskLine = strings.TrimSpace(value)
		case "EXIT_REASON":
			v.ExitReason = strings.TrimSpace(value)
		}
	}
	return v
}

func formatAnalyzeNextContext(v analyzeNextVars) string {
	parts := make([]string, 0, 4)
	if strings.TrimSpace(v.SpecName) != "" {
		parts = append(parts, "spec="+v.SpecName)
	}
	if strings.TrimSpace(v.TaskLine) != "" {
		parts = append(parts, "line="+v.TaskLine)
	}
	if strings.TrimSpace(v.IssueNumber) != "" {
		parts = append(parts, "issue="+v.IssueNumber)
	}
	if strings.TrimSpace(v.PRNumber) != "" {
		parts = append(parts, "pr="+v.PRNumber)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, " ") + ")"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getEnvInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

type runClaudeSessionArgs struct {
	ClaudeCmd     string
	ClaudeArgs    []string
	LogDir        string
	Logger        *kickoff.RotatingLogger
	SignalHandler *kickoff.SignalHandler
	Output        *kickoff.OutputFormatter
}

func runClaudeSession(args runClaudeSessionArgs) (int, error) {
	executor, err := kickoff.NewPTYExecutor(args.ClaudeCmd, args.ClaudeArgs)
	if err != nil {
		return 1, fmt.Errorf("failed to create executor: %w", err)
	}
	defer executor.Close()

	args.SignalHandler.SetExecutor(executor)

	fanIn := kickoff.NewFanInManager(1000)
	defer fanIn.Stop()

	args.SignalHandler.SetFanInManager(fanIn)
	fanIn.StartPrincipalTailer(args.LogDir)

	if err := executor.Start(); err != nil {
		return 1, fmt.Errorf("failed to start Claude CLI: %w", err)
	}

	if executor.IsFallback() {
		args.Output.Warning("PTY initialization failed, using standard execution")
	}

	var currentMonitor *kickoff.IssueMonitor
	var currentSpinner *kickoff.Spinner

	parser := kickoff.NewOutputParserWithTailerCallbacks(
		func(issueID int) {
			if currentMonitor != nil {
				currentMonitor.Stop("new_issue")
			}
			if currentSpinner != nil {
				currentSpinner.Stop("")
			}

			currentSpinner = kickoff.NewSpinner(issueID, os.Stdout)
			currentMonitor = kickoff.NewIssueMonitor(issueID, currentSpinner)

			currentMonitor.SetCommentCallback(func(commentType, prURL string) {
				if currentSpinner != nil {
					currentSpinner.Pause()
					currentSpinner.ClearLine()
				}

				if prURL != "" {
					args.Output.WorkerMessage(issueID, fmt.Sprintf("%s (PR: %s)", commentType, prURL))
				} else {
					args.Output.WorkerMessage(issueID, commentType)
				}

				if currentSpinner != nil {
					currentSpinner.Resume()
				}
			})

			args.SignalHandler.AddMonitor(currentMonitor)
			currentSpinner.Start()
			currentMonitor.Start()
		},
		func() {
			if currentMonitor != nil {
				currentMonitor.Stop("worker_complete")
				args.SignalHandler.RemoveMonitor(currentMonitor)
			}
			if currentSpinner != nil {
				duration := currentSpinner.Duration()
				currentSpinner.Stop(fmt.Sprintf("??[#%d] Worker completed (%s)",
					currentMonitor.IssueID(),
					formatDuration(duration)))
			}
			currentMonitor = nil
			currentSpinner = nil
		},
		func(issueID int) {
			fanIn.StartWorkerTailer(args.LogDir, issueID)
		},
		func() {
			fanIn.StopWorkerTailer()
		},
	)

	outputReader := executor.Output()
	const maxScanTokenSize = 1024 * 1024 // 1MB

	go func() {
		defer fanIn.Stop()

		scanner := bufio.NewScanner(outputReader)
		buf := make([]byte, maxScanTokenSize)
		scanner.Buffer(buf, maxScanTokenSize)

		for scanner.Scan() {
			line := scanner.Text()
			text := extractTextFromStreamJSON(line)
			if text == "" {
				continue
			}
			fanIn.SendClaudeLine(text)
		}
	}()

	var writers []io.Writer
	if args.Logger != nil {
		writers = append(writers, args.Logger)
	}

	for logLine := range fanIn.Channel() {
		if currentSpinner != nil {
			currentSpinner.Pause()
			currentSpinner.ClearLine()
		}

		colorizedLine := args.Output.ColorizeLogLine(logLine.Text)
		fmt.Println(colorizedLine)

		for _, w := range writers {
			fmt.Fprintln(w, logLine.Text)
		}

		if currentSpinner != nil {
			currentSpinner.Resume()
		}

		parser.Parse(logLine.Text)
	}

	exitCode := 0
	if err := executor.Wait(); err != nil {
		if !args.SignalHandler.IsShutdown() {
			args.Output.Error(fmt.Sprintf("Claude CLI exited with error: %v", err))
		}
		exitCode = 1
	}

	if currentMonitor != nil {
		currentMonitor.Stop("process_exit")
	}
	if currentSpinner != nil {
		currentSpinner.Stop("")
	}

	return exitCode, nil
}

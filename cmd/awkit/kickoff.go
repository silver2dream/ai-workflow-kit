package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/session"
	"github.com/silver2dream/ai-workflow-kit/internal/trace"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
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
			// Write session_end event
			trace.WriteEvent(trace.ComponentPrincipal, trace.TypeSessionEnd, trace.LevelInfo,
				trace.WithData(map[string]string{"reason": reason}))
			trace.CloseGlobalWriter()

			if err := sessionMgr.EndPrincipal(principalSessionID, reason); err != nil {
				// Log error but continue - session end is best-effort
				fmt.Fprintf(os.Stderr, "Warning: failed to end principal session: %v\n", err)
			}
		})
	}

	// Initialize Principal session
	sessionID, err := sessionMgr.InitPrincipal()
	if err != nil {
		output.Warning(fmt.Sprintf("Failed to initialize session: %v", err))
	}
	if sessionID != "" {
		principalSessionID = sessionID
		defer endPrincipalSession("aborted")
		output.Success(fmt.Sprintf("Session: %s", sessionID))

		// Initialize event stream for this session
		if err := trace.InitGlobalWriter(".", sessionID); err != nil {
			output.Warning(fmt.Sprintf("Failed to initialize event stream: %v", err))
		}
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

	// Write session_start event (after config is loaded)
	trace.WriteEvent(trace.ComponentPrincipal, trace.TypeSessionStart, trace.LevelInfo,
		trace.WithData(map[string]string{
			"project": config.Project.Name,
			"session": principalSessionID,
		}))

	// Ensure the workflow's state-machine labels exist before the Principal
	// starts. The worker and reviewer add/remove these as issues move through the
	// pipeline; if they're missing, every `gh issue edit` logs a noisy
	// '<label> not found'. Best-effort — a permission failure is just a warning.
	ensureWorkflowLabels(configPath, output)

	// Build Claude CLI command
	// Use stream-json format for real-time streaming output
	// Use principal-workflow Skill for deterministic workflow execution
	//
	// The Principal runs fully autonomously: `awkit kickoff` has no human to
	// answer permission prompts. It therefore MUST run with a non-interactive
	// permission mode, or the first tool call requiring approval hangs forever.
	// Default to `auto` (Claude Code v2.1.83+): a background classifier
	// auto-approves routine actions and blocks genuinely dangerous ones, so we
	// get autonomy with a safety net. Override via AWKIT_PRINCIPAL_PERMISSION_MODE
	// (e.g. `bypassPermissions` — equivalent to the Worker's
	// --dangerously-skip-permissions — for locked-down or older environments
	// where `auto` is unavailable).
	permissionMode := getEnvString("AWKIT_PRINCIPAL_PERMISSION_MODE", "auto")
	claudeCmd := "claude"
	claudeArgs := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", permissionMode,
		"-p", "Run the AWK principal workflow.",
	}

	fmt.Println("")
	output.Info(fmt.Sprintf("Starting workflow for project: %s", config.Project.Name))
	fmt.Println("")


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
	// No-progress guard: a "creation" decision (create_task/generate_tasks/
	// audit_epic) always changes repo state on success, so if the analyzer keeps
	// returning the exact same one, the workflow is stalled — classically a
	// create_task whose `gh issue create` fails on a permission error, which is
	// invisible to the analyzer (it only sees that the issue still doesn't
	// exist) and never counts as a consecutive_failure. Count identical repeats
	// so we can stop with a clear message instead of burning every session.
	var sameDecisionStreak int
	maxSameDecision := getEnvInt("AWKIT_MAX_SAME_DECISION", 3)

	for sessionIndex := 1; sessionIndex <= maxSessions; sessionIndex++ {
		// Write loop_start event
		trace.WriteEvent(trace.ComponentPrincipal, trace.TypeLoopStart, trace.LevelInfo,
			trace.WithData(map[string]any{
				"loop":         sessionIndex,
				"max_sessions": maxSessions,
			}))

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
			// Write loop_decision event for error case
			trace.WriteDecisionEvent(trace.ComponentPrincipal, trace.TypeLoopDecision, trace.Decision{
				Rule:       "analyze_next failed",
				Conditions: map[string]any{"error": err.Error()},
				Result:     "PAUSE",
			})
			fmt.Println("")
			output.Warning(fmt.Sprintf("Workflow paused (failed to determine next action: %v)", err))
			output.Info("Run `awkit status` for offline details.")
			endPrincipalSession("paused")
			return 1
		}
		if sessionIndex > 1 && sameDecision(next, lastNext) {
			sameDecisionStreak++
		} else {
			sameDecisionStreak = 1
		}
		lastNext = next

		// Write loop_decision event
		trace.WriteDecisionEvent(trace.ComponentPrincipal, trace.TypeLoopDecision, trace.Decision{
			Rule: "next_action determines loop continuation",
			Conditions: map[string]any{
				"next_action":  next.NextAction,
				"issue_number": next.IssueNumber,
				"pr_number":    next.PRNumber,
				"exit_reason":  next.ExitReason,
				"loop":         sessionIndex,
			},
			Result: func() string {
				switch next.NextAction {
				case "all_complete":
					return "STOP_COMPLETE"
				case "none":
					return "STOP_NONE"
				default:
					return "CONTINUE"
				}
			}(),
		})

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
			if isCreationAction(next.NextAction) && sameDecisionStreak >= maxSameDecision {
				fmt.Println("")
				output.Error(fmt.Sprintf("Workflow stopped: no progress — '%s'%s repeated %d× without advancing.", next.NextAction, formatAnalyzeNextContext(next), sameDecisionStreak))
				output.Info("This usually means gh lacks write access to the repo (check `gh auth status` and the repo's permissions) or a label/config problem. See `awkit doctor`.")
				endPrincipalSession("no_progress")
				return 1
			}
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
// ansiEscape matches ANSI/VT escape sequences (SGR colors, cursor moves, ...).
// When claude runs inside a PTY it detects a TTY and injects these into the
// stream-json output; they must be removed before a line can be parsed as JSON
// or the whole event is dropped/garbled.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

// sanitizeStreamLine strips ANSI escapes and stray carriage returns/NULs that
// a PTY may interleave with the JSON stream.
func sanitizeStreamLine(line string) string {
	line = ansiEscape.ReplaceAllString(line, "")
	line = strings.ReplaceAll(line, "\r", "")
	line = strings.ReplaceAll(line, "\x00", "")
	return strings.TrimSpace(line)
}

// summarizeCommand collapses a shell command to a single length-bounded line for
// [EXEC] display. Without it a multi-line command — e.g. a reviewer heredoc that
// inlines an entire review body — dumps hundreds of lines to the console.
func summarizeCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if i := strings.IndexAny(cmd, "\r\n"); i >= 0 {
		cmd = strings.TrimSpace(cmd[:i]) + " …"
	}
	const maxRunes = 200
	if r := []rune(cmd); len(r) > maxRunes {
		cmd = strings.TrimSpace(string(r[:maxRunes])) + " …"
	}
	return cmd
}

func extractTextFromStreamJSON(line string) string {
	line = sanitizeStreamLine(line)
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
								texts = append(texts, fmt.Sprintf("[EXEC] %s", summarizeCommand(cmd)))
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
		// The content itself was already streamed via "assistant" events,
		// but the result event carries the session's token/cost usage.
		return recordSessionUsage(event)

	default:
		// Skip other event types (system, etc.)
		return ""
	}
}

// recordSessionUsage extracts token/cost usage from a Claude stream-json
// "result" event, emits a session_usage trace event, and returns a one-line
// summary for display. Returns "" when the event carries no usage data.
func recordSessionUsage(event map[string]any) string {
	cost, _ := event["total_cost_usd"].(float64)
	var tokensIn, tokensOut int64
	if usage, ok := event["usage"].(map[string]any); ok {
		if f, ok := usage["input_tokens"].(float64); ok {
			tokensIn = int64(f)
		}
		if f, ok := usage["output_tokens"].(float64); ok {
			tokensOut = int64(f)
		}
	}
	if cost == 0 && tokensIn == 0 && tokensOut == 0 {
		return ""
	}

	trace.WriteEvent(trace.ComponentPrincipal, trace.TypeSessionUsage, trace.LevelInfo,
		trace.WithData(map[string]any{
			"cost_usd":   cost,
			"tokens_in":  tokensIn,
			"tokens_out": tokensOut,
		}))

	return fmt.Sprintf("[COST] session usage: $%.4f (in: %d, out: %d tokens)", cost, tokensIn, tokensOut)
}

type analyzeNextVars struct {
	NextAction  string
	IssueNumber string
	PRNumber    string
	SpecName    string
	TaskLine    string
	ExitReason  string
	MergeIssue  string // conflict | rebase - indicates Worker needs to fix merge issues
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
		MergeIssue:  decision.MergeIssue,
	}, nil
}

func formatAnalyzeNextContext(v analyzeNextVars) string {
	parts := make([]string, 0, 5)
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
	if strings.TrimSpace(v.MergeIssue) != "" {
		parts = append(parts, "merge="+v.MergeIssue)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, " ") + ")"
}

// sameDecision reports whether two analyze-next results are identical across
// every field that identifies the work to be done. Used to detect a stalled
// loop that keeps deciding the same thing without making progress.
func sameDecision(a, b analyzeNextVars) bool {
	return a.NextAction == b.NextAction &&
		a.IssueNumber == b.IssueNumber &&
		a.PRNumber == b.PRNumber &&
		a.SpecName == b.SpecName &&
		a.TaskLine == b.TaskLine &&
		a.MergeIssue == b.MergeIssue
}

// isCreationAction reports whether an action creates GitHub state (an issue or
// task list) that, on success, necessarily advances the workflow. Repeating one
// of these unchanged is a hard stall, so the no-progress guard only trips on
// them — actions like check_result or dispatch_worker can legitimately recur
// while a worker runs.
func isCreationAction(action string) bool {
	switch action {
	case "create_task", "generate_tasks", "audit_epic":
		return true
	default:
		return false
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ensureWorkflowLabels creates the state-machine labels up front so the worker
// and reviewer don't spam '<label> not found' as they move issues through the
// pipeline. Best-effort: config/permission problems are a warning, not a blocker.
func ensureWorkflowLabels(configPath string, output *kickoff.OutputFormatter) {
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil {
		return // config errors are surfaced by preflight already
	}
	ghc := worker.NewGitHubClient(30 * time.Second)
	if err := ghc.EnsureLabels(context.Background(), cfg.GitHub.Repo, workflowLabelSpecs(cfg.GitHub.Labels)); err != nil {
		output.Warning(fmt.Sprintf("Some workflow labels could not be created: %v", err))
	}
}

// workflowLabelSpecs maps the configured label names to create specs with stable
// colors and descriptions.
func workflowLabelSpecs(l analyzer.LabelsConfig) []worker.LabelSpec {
	return []worker.LabelSpec{
		{Name: l.Task, Color: "1d76db", Description: "AWK automated task"},
		{Name: l.InProgress, Color: "fbca04", Description: "Worker is implementing this issue"},
		{Name: l.PRReady, Color: "0e8a16", Description: "PR is ready for review"},
		{Name: l.WorkerFailed, Color: "d93f0b", Description: "Worker failed after retries"},
		{Name: l.NeedsHumanReview, Color: "d876e3", Description: "Needs human review"},
		{Name: l.ReviewFailed, Color: "b60205", Description: "Automated review failed"},
		{Name: l.MergeConflict, Color: "e99695", Description: "PR has merge conflicts"},
		{Name: l.NeedsRebase, Color: "c2e0c6", Description: "Branch needs rebase before merge"},
		{Name: l.Completed, Color: "5319e7", Description: "Task completed"},
	}
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

func getEnvString(name, defaultValue string) string {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		return raw
	}
	return defaultValue
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
	// Run the Principal headless (plain pipes, no PTY). A `claude --print
	// --output-format stream-json` child that detects a TTY switches to
	// interactive mode and blocks forever on tool-permission prompts nobody can
	// answer — the root cause of the silent multi-minute kickoff hang on Windows
	// (ConPTY) and native Unix PTYs. Pipes also emit clean JSON with no ANSI.
	executor.SetStandardMode(true)
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

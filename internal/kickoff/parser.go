package kickoff

import (
	"regexp"
	"strconv"
)

// OutputParser parses Principal stdout to detect Worker dispatch and completion events
type OutputParser struct {
	onIssueStart     func(issueID int) // Called when Worker dispatch detected (for spinner)
	onIssueEnd       func()            // Called when Worker completion detected (for spinner)
	onDispatchWorker func(issueID int) // Called to start worker tailer
	onWorkerStatus   func()            // Called to stop worker tailer
}

// Patterns for old format (STEP-3/STEP-4)
var step3Pattern = regexp.MustCompile(`\[PRINCIPAL\].*\|\s*STEP-3\s*\|.*issue #(\d+)`)
var step4Pattern = regexp.MustCompile(`\[PRINCIPAL\].*\|\s*STEP-4\s*\|`)

// Patterns for new Skills format (G10 fix: generic patterns)
// Supports multiple dispatch formats:
// - [PRINCIPAL] HH:MM:SS | Dispatch Issue #N / 派工 Issue #N
// - [EXEC] bash .ai/scripts/dispatch_worker.sh "15"
// - [EXEC] awkit dispatch-worker --issue 15
// - dispatch_worker: issue=15
var dispatchPattern = regexp.MustCompile(`(?:派工\s*Issue\s*#|dispatch[-_]worker(?:\.sh)?\s+(?:--issue\s+)?["']?)(\d+)`)
var workerStartPattern = regexp.MustCompile(`\[WORKER\].*(?:worker_session_id|session_id)=`)
var workerCompletePattern = regexp.MustCompile(`(?:Worker\s*執行完成|WORKER_STATUS=(?:success|failed))`)

// NewOutputParser creates a new OutputParser with the given callbacks
func NewOutputParser(onStart func(int), onEnd func()) *OutputParser {
	return &OutputParser{
		onIssueStart: onStart,
		onIssueEnd:   onEnd,
	}
}

// NewOutputParserWithTailerCallbacks creates an OutputParser with tailer management callbacks
func NewOutputParserWithTailerCallbacks(
	onStart func(int),
	onEnd func(),
	onDispatch func(int),
	onStatus func(),
) *OutputParser {
	return &OutputParser{
		onIssueStart:     onStart,
		onIssueEnd:       onEnd,
		onDispatchWorker: onDispatch,
		onWorkerStatus:   onStatus,
	}
}

// Parse processes a line of output and triggers callbacks as needed
func (p *OutputParser) Parse(line string) {
	// Check for old STEP-3 format
	if matches := step3Pattern.FindStringSubmatch(line); len(matches) > 1 {
		if issueID, err := strconv.Atoi(matches[1]); err == nil {
			// Start tailer before spinner
			if p.onDispatchWorker != nil {
				p.onDispatchWorker(issueID)
			}
			if p.onIssueStart != nil {
				p.onIssueStart(issueID)
			}
		}
		return
	}

	// Check for new dispatch format (派工 or dispatch_worker.sh call)
	if matches := dispatchPattern.FindStringSubmatch(line); len(matches) > 1 {
		if issueID, err := strconv.Atoi(matches[1]); err == nil {
			// Start tailer before spinner
			if p.onDispatchWorker != nil {
				p.onDispatchWorker(issueID)
			}
			if p.onIssueStart != nil {
				p.onIssueStart(issueID)
			}
		}
		return
	}

	// Check for old STEP-4 format
	if step4Pattern.MatchString(line) {
		// Stop tailer before spinner stop
		if p.onWorkerStatus != nil {
			p.onWorkerStatus()
		}
		if p.onIssueEnd != nil {
			p.onIssueEnd()
		}
		return
	}

	// Check for new worker complete format
	if workerCompletePattern.MatchString(line) {
		// Stop tailer before spinner stop
		if p.onWorkerStatus != nil {
			p.onWorkerStatus()
		}
		if p.onIssueEnd != nil {
			p.onIssueEnd()
		}
		return
	}
}

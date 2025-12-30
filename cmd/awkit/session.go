package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/silver2dream/ai-workflow-kit/internal/session"
)

func usageSession() {
	fmt.Fprint(os.Stderr, `Session management commands

Usage:
  awkit session <command> [args]

Commands:
  init                              Initialize Principal session
  end <session_id> <reason>         End session with reason
  get-id                            Get current session ID
  append <session_id> <type> <json> Record action in session log
  update-result <issue_id> <session_id>  Update result with principal session
  update-review <issue_id> <reviewer_sid> <decision> <ci_status> <ci_timeout> [merge_ts]

Examples:
  awkit session init
  awkit session get-id
  awkit session end principal-20250101-120000-ab12 completed
  awkit session append principal-20250101-120000-ab12 worker_dispatched '{"issue_id":"25"}'
`)
}

func cmdSession(args []string) int {
	if len(args) == 0 {
		usageSession()
		return 2
	}

	stateRoot, err := resolveGitRoot()
	if err != nil {
		errorf("failed to resolve git root: %v\n", err)
		return 1
	}

	mgr := session.NewManager(stateRoot)

	switch args[0] {
	case "init":
		return cmdSessionInit(mgr)
	case "end":
		return cmdSessionEnd(mgr, args[1:])
	case "get-id":
		return cmdSessionGetID(mgr)
	case "append":
		return cmdSessionAppend(mgr, args[1:])
	case "update-result":
		return cmdSessionUpdateResult(mgr, args[1:])
	case "update-review":
		return cmdSessionUpdateReview(mgr, args[1:])
	case "-h", "--help", "help":
		usageSession()
		return 0
	default:
		errorf("unknown session command: %s\n", args[0])
		usageSession()
		return 2
	}
}

func cmdSessionInit(mgr *session.Manager) int {
	sessionID, err := mgr.InitPrincipal()
	if err != nil {
		errorf("failed to init session: %v\n", err)
		return 1
	}
	fmt.Println(sessionID)
	return 0
}

func cmdSessionEnd(mgr *session.Manager, args []string) int {
	fs := flag.NewFlagSet("session end", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	remaining := fs.Args()
	if len(remaining) < 2 {
		errorf("usage: awkit session end <session_id> <reason>\n")
		return 2
	}

	sessionID := remaining[0]
	reason := remaining[1]

	if err := mgr.EndPrincipal(sessionID, reason); err != nil {
		errorf("failed to end session: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "[SESSION] Ended session %s with reason: %s\n", sessionID, reason)
	return 0
}

func cmdSessionGetID(mgr *session.Manager) int {
	sessionID := mgr.GetCurrentSessionID()
	if sessionID == "" {
		return 1
	}
	fmt.Println(sessionID)
	return 0
}

func cmdSessionAppend(mgr *session.Manager, args []string) int {
	if len(args) < 3 {
		errorf("usage: awkit session append <session_id> <type> <json>\n")
		return 2
	}

	sessionID := args[0]
	actionType := args[1]
	actionData := args[2]

	if err := mgr.AppendAction(sessionID, actionType, actionData); err != nil {
		errorf("failed to append action: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "[SESSION] Recorded action: %s\n", actionType)
	return 0
}

func cmdSessionUpdateResult(mgr *session.Manager, args []string) int {
	if len(args) < 2 {
		errorf("usage: awkit session update-result <issue_id> <session_id>\n")
		return 2
	}

	issueID := args[0]
	sessionID := args[1]

	if err := mgr.UpdateResultWithPrincipalSession(issueID, sessionID); err != nil {
		errorf("failed to update result: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "[SESSION] Updated result with principal_session_id: %s\n", sessionID)
	return 0
}

func cmdSessionUpdateReview(mgr *session.Manager, args []string) int {
	if len(args) < 5 {
		errorf("usage: awkit session update-review <issue_id> <reviewer_sid> <decision> <ci_status> <ci_timeout> [merge_ts]\n")
		return 2
	}

	issueID := args[0]
	reviewerSID := args[1]
	decision := args[2]
	ciStatus := args[3]
	ciTimeout := args[4] == "true"
	mergeTS := ""
	if len(args) > 5 {
		mergeTS = args[5]
	}

	audit := &session.ReviewAudit{
		ReviewerSessionID: reviewerSID,
		Decision:          decision,
		CIStatus:          ciStatus,
		CITimeout:         ciTimeout,
		MergeTimestamp:    mergeTS,
	}

	if err := mgr.UpdateResultWithReviewAudit(issueID, audit); err != nil {
		errorf("failed to update review audit: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "[SESSION] Updated result with review_audit\n")
	return 0
}

// Placeholder for backwards compatibility with bash scripts
func cmdSessionCheckRunning(mgr *session.Manager, args []string) int {
	if len(args) < 2 {
		errorf("usage: awkit session check-running <pid> <start_time>\n")
		return 2
	}

	pid, err := strconv.Atoi(args[0])
	if err != nil {
		errorf("invalid pid: %s\n", args[0])
		return 2
	}

	startTime, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		errorf("invalid start_time: %s\n", args[1])
		return 2
	}

	if mgr.IsPrincipalRunning(pid, startTime) {
		return 0 // Running
	}
	return 1 // Not running
}

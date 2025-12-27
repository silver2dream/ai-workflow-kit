package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
)

type Options struct {
	IssueID int
	Now     time.Time
}

type Report struct {
	TimestampUTC string       `json:"timestamp_utc"`
	Root         string       `json:"root"`
	Run          RunInfo      `json:"run"`
	Control      ControlInfo  `json:"control"`
	Target       TargetInfo   `json:"target"`
	Artifacts    IssueDetails `json:"artifacts"`
	Suggestions  []string     `json:"suggestions,omitempty"`
	Warnings     []string     `json:"warnings,omitempty"`
}

type RunInfo struct {
	State        string      `json:"state"`
	LockFile     string      `json:"lock_file"`
	LockCorrupt  bool        `json:"lock_corrupt,omitempty"`
	PID          int         `json:"pid,omitempty"`
	ProcessAlive *bool       `json:"process_alive,omitempty"`
	StartTime    string      `json:"start_time,omitempty"`
	Hostname     string      `json:"hostname,omitempty"`
	Session      SessionInfo `json:"session"`
}

type SessionInfo struct {
	File         string         `json:"file"`
	SessionID    string         `json:"session_id,omitempty"`
	StartedAt    string         `json:"started_at,omitempty"`
	LogFile      string         `json:"log_file,omitempty"`
	EndedAt      string         `json:"ended_at,omitempty"`
	ExitReason   string         `json:"exit_reason,omitempty"`
	ActionsCount int            `json:"actions_count,omitempty"`
	LastAction   *SessionAction `json:"last_action,omitempty"`
}

type SessionAction struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

type ControlInfo struct {
	StopPresent         bool   `json:"stop_present"`
	StopFile            string `json:"stop_file"`
	LoopCount           *int   `json:"loop_count,omitempty"`
	ConsecutiveFailures *int   `json:"consecutive_failures,omitempty"`
}

type TargetInfo struct {
	IssueID int    `json:"issue_id,omitempty"`
	Source  string `json:"source,omitempty"` // "flag" | "trace" | "result" | "none"
}

type IssueDetails struct {
	IssueID int         `json:"issue_id,omitempty"`
	Result  *ResultInfo `json:"result,omitempty"`
	Trace   *TraceInfo  `json:"trace,omitempty"`
	RunDir  Artifact    `json:"run_dir"`
	Summary Artifact    `json:"summary"`
	Logs    IssueLogs   `json:"logs"`
}

type IssueLogs struct {
	Principal Artifact   `json:"principal"`
	Worker    Artifact   `json:"worker"`
	Codex     []Artifact `json:"codex,omitempty"`
}

type Artifact struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type ResultInfo struct {
	Path         string `json:"path"`
	Exists       bool   `json:"exists"`
	Status       string `json:"status,omitempty"`
	Repo         string `json:"repo,omitempty"`
	Branch       string `json:"branch,omitempty"`
	BaseBranch   string `json:"base_branch,omitempty"`
	HeadSHA      string `json:"head_sha,omitempty"`
	TimestampUTC string `json:"timestamp_utc,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
	DurationSec  int    `json:"duration_seconds,omitempty"`
	RetryCount   int    `json:"retry_count,omitempty"`
	SummaryFile  string `json:"summary_file,omitempty"`
}

type TraceInfo struct {
	Path           string `json:"path"`
	Exists         bool   `json:"exists"`
	Status         string `json:"status,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	EndedAt        string `json:"ended_at,omitempty"`
	DurationSec    int    `json:"duration_seconds,omitempty"`
	Error          string `json:"error,omitempty"`
	LastStepName   string `json:"last_step_name,omitempty"`
	LastStepStatus string `json:"last_step_status,omitempty"`
}

type rawSessionFile struct {
	SessionID    string `json:"session_id"`
	StartedAt    string `json:"started_at"`
	PID          int    `json:"pid"`
	PIDStartTime int64  `json:"pid_start_time"`
}

type rawSessionLog struct {
	SessionID  string          `json:"session_id"`
	StartedAt  string          `json:"started_at"`
	EndedAt    string          `json:"ended_at,omitempty"`
	ExitReason string          `json:"exit_reason,omitempty"`
	Actions    []SessionAction `json:"actions"`
}

type rawResultFile struct {
	IssueID      string `json:"issue_id"`
	Status       string `json:"status"`
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	BaseBranch   string `json:"base_branch"`
	HeadSHA      string `json:"head_sha"`
	TimestampUTC string `json:"timestamp_utc"`
	PRURL        string `json:"pr_url"`
	SummaryFile  string `json:"summary_file"`
	Metrics      struct {
		DurationSeconds int `json:"duration_seconds"`
		RetryCount      int `json:"retry_count"`
	} `json:"metrics"`
}

type rawTraceFile struct {
	IssueID         string `json:"issue_id"`
	Status          string `json:"status"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
	DurationSeconds int    `json:"duration_seconds"`
	Error           string `json:"error"`
	Steps           []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"steps"`
}

func Collect(root string, opts Options) (Report, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	report := Report{
		TimestampUTC: now.UTC().Format(time.RFC3339),
		Root:         root,
		Run: RunInfo{
			State:    "unknown",
			LockFile: filepath.Join(root, ".ai", "state", "kickoff.lock"),
			Session: SessionInfo{
				File: filepath.Join(root, ".ai", "state", "principal", "session.json"),
			},
		},
		Control: ControlInfo{
			StopFile: filepath.Join(root, ".ai", "state", "STOP"),
		},
		Artifacts: IssueDetails{
			RunDir:  Artifact{},
			Summary: Artifact{},
			Logs: IssueLogs{
				Principal: Artifact{Path: filepath.Join(root, ".ai", "exe-logs", "principal.log")},
			},
		},
	}

	warnings := make([]string, 0, 4)

	// -------------------------
	// Run state from lock file
	// -------------------------
	lockInfo, lockErr := readLockInfo(report.Run.LockFile)
	switch {
	case errors.Is(lockErr, os.ErrNotExist):
		report.Run.State = "no_lock"
	case lockErr != nil:
		report.Run.State = "lock_corrupt"
		report.Run.LockCorrupt = true
		warnings = append(warnings, fmt.Sprintf("cannot read lock file: %s", report.Run.LockFile))
	default:
		report.Run.PID = lockInfo.PID
		report.Run.StartTime = lockInfo.StartTime.UTC().Format(time.RFC3339)
		report.Run.Hostname = lockInfo.Hostname
		alive := kickoff.ProcessAlive(lockInfo.PID)
		report.Run.ProcessAlive = &alive
		if alive {
			report.Run.State = "running"
		} else {
			report.Run.State = "stale_lock"
		}
	}

	// -------------------------
	// Control files
	// -------------------------
	report.Control.StopPresent = fileExists(report.Control.StopFile)

	if loopCount, err := readIntFile(filepath.Join(root, ".ai", "state", "loop_count")); err == nil {
		report.Control.LoopCount = &loopCount
	}
	if consecutiveFailures, err := readIntFile(filepath.Join(root, ".ai", "state", "consecutive_failures")); err == nil {
		report.Control.ConsecutiveFailures = &consecutiveFailures
	}

	// -------------------------
	// Principal session
	// -------------------------
	if sessionFile, err := readSessionFile(report.Run.Session.File); err == nil {
		report.Run.Session.SessionID = sessionFile.SessionID
		report.Run.Session.StartedAt = sessionFile.StartedAt
		report.Run.Session.LogFile = filepath.Join(root, ".ai", "state", "principal", "sessions", sessionFile.SessionID+".json")

		if sessionLog, err := readSessionLog(report.Run.Session.LogFile); err == nil {
			report.Run.Session.EndedAt = sessionLog.EndedAt
			report.Run.Session.ExitReason = sessionLog.ExitReason
			report.Run.Session.ActionsCount = len(sessionLog.Actions)
			if len(sessionLog.Actions) > 0 {
				last := sessionLog.Actions[len(sessionLog.Actions)-1]
				report.Run.Session.LastAction = &last
			}
		}
	}

	// -------------------------
	// Select target issue
	// -------------------------
	var targetIssueID int
	var targetSource string
	if opts.IssueID > 0 {
		targetIssueID = opts.IssueID
		targetSource = "flag"
	} else if runningIssue, ok := findRunningTraceIssue(filepath.Join(root, ".ai", "state", "traces")); ok {
		targetIssueID = runningIssue
		targetSource = "trace"
	} else if lastIssue, ok := findLatestResultIssue(filepath.Join(root, ".ai", "results")); ok {
		targetIssueID = lastIssue
		targetSource = "result"
	} else {
		targetIssueID = 0
		targetSource = "none"
	}

	report.Target = TargetInfo{IssueID: targetIssueID, Source: targetSource}
	report.Artifacts.IssueID = targetIssueID

	// -------------------------
	// Populate issue artifacts
	// -------------------------
	if targetIssueID > 0 {
		report.Artifacts.RunDir = artifact(filepath.Join(root, ".ai", "runs", fmt.Sprintf("issue-%d", targetIssueID)))
		report.Artifacts.Summary = artifact(filepath.Join(root, ".ai", "runs", fmt.Sprintf("issue-%d", targetIssueID), "summary.txt"))

		report.Artifacts.Logs.Worker = artifact(filepath.Join(root, ".ai", "exe-logs", fmt.Sprintf("issue-%d.worker.log", targetIssueID)))
		report.Artifacts.Logs.Principal = artifact(filepath.Join(root, ".ai", "exe-logs", "principal.log"))
		report.Artifacts.Logs.Codex = listCodexLogs(filepath.Join(root, ".ai", "exe-logs"), targetIssueID)

		resultPath := filepath.Join(root, ".ai", "results", fmt.Sprintf("issue-%d.json", targetIssueID))
		resultInfo := &ResultInfo{Path: resultPath, Exists: fileExists(resultPath)}
		if resultInfo.Exists {
			rawResult, err := readResultFile(resultPath)
			if err == nil {
				resultInfo.Status = rawResult.Status
				resultInfo.Repo = rawResult.Repo
				resultInfo.Branch = rawResult.Branch
				resultInfo.BaseBranch = rawResult.BaseBranch
				resultInfo.HeadSHA = rawResult.HeadSHA
				resultInfo.TimestampUTC = rawResult.TimestampUTC
				resultInfo.PRURL = rawResult.PRURL
				resultInfo.SummaryFile = rawResult.SummaryFile
				resultInfo.DurationSec = rawResult.Metrics.DurationSeconds
				resultInfo.RetryCount = rawResult.Metrics.RetryCount
			}
		}
		report.Artifacts.Result = resultInfo

		tracePath := filepath.Join(root, ".ai", "state", "traces", fmt.Sprintf("issue-%d.json", targetIssueID))
		traceInfo := &TraceInfo{Path: tracePath, Exists: fileExists(tracePath)}
		if traceInfo.Exists {
			rawTrace, err := readTraceFile(tracePath)
			if err == nil {
				traceInfo.Status = rawTrace.Status
				traceInfo.StartedAt = rawTrace.StartedAt
				traceInfo.EndedAt = rawTrace.EndedAt
				traceInfo.DurationSec = rawTrace.DurationSeconds
				traceInfo.Error = rawTrace.Error
				if len(rawTrace.Steps) > 0 {
					last := rawTrace.Steps[len(rawTrace.Steps)-1]
					traceInfo.LastStepName = last.Name
					traceInfo.LastStepStatus = last.Status
				}
			}
		}
		report.Artifacts.Trace = traceInfo
	} else {
		// still fill principal log existence
		report.Artifacts.Logs.Principal = artifact(filepath.Join(root, ".ai", "exe-logs", "principal.log"))
	}

	// -------------------------
	// Suggestions / warnings
	// -------------------------
	suggestions := make([]string, 0, 6)

	if report.Control.StopPresent {
		suggestions = append(suggestions, "STOP marker is present; delete `.ai/state/STOP` to resume, then run `awkit kickoff --resume`.")
	}

	if report.Run.State == "stale_lock" {
		suggestions = append(suggestions, "Stale lock detected; rerun `awkit kickoff` (it will auto-clear stale lock), or delete `.ai/state/kickoff.lock` if needed.")
	}

	if report.Run.State == "running" {
		suggestions = append(suggestions, "Workflow is running; wait for completion or inspect the current issue artifacts.")
	}

	if targetIssueID > 0 && report.Artifacts.Result != nil && report.Artifacts.Result.Exists {
		switch report.Artifacts.Result.Status {
		case "failed":
			suggestions = append(suggestions, fmt.Sprintf("Issue #%d failed; check `%s` and `%s`.", targetIssueID, report.Artifacts.Summary.Path, report.Artifacts.Trace.Path))
		case "success":
			if report.Artifacts.Result.PRURL != "" {
				suggestions = append(suggestions, fmt.Sprintf("Issue #%d succeeded; next step is PR review: %s", targetIssueID, report.Artifacts.Result.PRURL))
			}
		}
	}

	if targetIssueID > 0 {
		if !report.Artifacts.Summary.Exists {
			warnings = append(warnings, fmt.Sprintf("missing summary file: %s", report.Artifacts.Summary.Path))
		}
		if report.Artifacts.Trace != nil && !report.Artifacts.Trace.Exists {
			warnings = append(warnings, fmt.Sprintf("missing trace file: %s", report.Artifacts.Trace.Path))
		}
		if report.Artifacts.Result != nil && !report.Artifacts.Result.Exists {
			warnings = append(warnings, fmt.Sprintf("missing result file: %s", report.Artifacts.Result.Path))
		}
	}

	report.Suggestions = suggestions
	report.Warnings = warnings
	return report, nil
}

func readLockInfo(path string) (*kickoff.LockInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info kickoff.LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func readSessionFile(path string) (*rawSessionFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var session rawSessionFile
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func readSessionLog(path string) (*rawSessionLog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var log rawSessionLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	if log.Actions == nil {
		log.Actions = []SessionAction{}
	}
	return &log, nil
}

func readResultFile(path string) (*rawResultFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result rawResultFile
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func readTraceFile(path string) (*rawTraceFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var trace rawTraceFile
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, err
	}
	return &trace, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func artifact(path string) Artifact {
	return Artifact{Path: path, Exists: fileExists(path)}
}

func readIntFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, fmt.Errorf("empty int file: %s", path)
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func findRunningTraceIssue(traceDir string) (int, bool) {
	entries, err := os.ReadDir(traceDir)
	if err != nil {
		return 0, false
	}
	type candidate struct {
		IssueID int
		Start   string
	}
	var running []candidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "issue-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		issueID := parseIssueIDFromFilename(name)
		if issueID == 0 {
			continue
		}
		path := filepath.Join(traceDir, name)
		raw, err := readTraceFile(path)
		if err != nil {
			continue
		}
		if raw.Status == "running" {
			running = append(running, candidate{IssueID: issueID, Start: raw.StartedAt})
		}
	}
	if len(running) == 0 {
		return 0, false
	}
	sort.Slice(running, func(i, j int) bool { return running[i].Start > running[j].Start })
	return running[0].IssueID, true
}

func findLatestResultIssue(resultsDir string) (int, bool) {
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return 0, false
	}
	type candidate struct {
		IssueID int
		TS      string
		ModTime time.Time
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "issue-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		issueID := parseIssueIDFromFilename(name)
		if issueID == 0 {
			continue
		}
		path := filepath.Join(resultsDir, name)
		raw, err := readResultFile(path)
		if err != nil {
			continue
		}
		info, _ := e.Info()
		modTime := time.Time{}
		if info != nil {
			modTime = info.ModTime()
		}
		candidates = append(candidates, candidate{IssueID: issueID, TS: raw.TimestampUTC, ModTime: modTime})
	}
	if len(candidates) == 0 {
		return 0, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].TS != "" && candidates[j].TS != "" && candidates[i].TS != candidates[j].TS {
			return candidates[i].TS > candidates[j].TS
		}
		return candidates[i].ModTime.After(candidates[j].ModTime)
	})
	return candidates[0].IssueID, true
}

func parseIssueIDFromFilename(name string) int {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(name, "issue-"), ".json")
	i, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return i
}

func listCodexLogs(exeLogsDir string, issueID int) []Artifact {
	entries, err := os.ReadDir(exeLogsDir)
	if err != nil {
		return nil
	}

	prefix := fmt.Sprintf("issue-%d.", issueID)
	var logs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !strings.Contains(name, ".codex") {
			continue
		}
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		logs = append(logs, name)
	}
	sort.Strings(logs)

	out := make([]Artifact, 0, len(logs))
	for _, name := range logs {
		path := filepath.Join(exeLogsDir, name)
		out = append(out, Artifact{Path: path, Exists: true})
	}
	return out
}

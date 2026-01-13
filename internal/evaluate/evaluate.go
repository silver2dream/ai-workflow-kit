// Package evaluate provides developer testing and evaluation functionality for AWK.
// It includes offline gates (O0-O10) and online gates (N1-N3) for quality assessment.
package evaluate

// GateResult represents the result of a single gate check.
type GateResult struct {
	Status string // "PASS", "FAIL", "SKIP"
	Reason string
}

// Pass creates a passing GateResult with the given reason.
func Pass(reason string) GateResult {
	return GateResult{Status: "PASS", Reason: reason}
}

// Fail creates a failing GateResult with the given reason.
func Fail(reason string) GateResult {
	return GateResult{Status: "FAIL", Reason: reason}
}

// Skip creates a skipped GateResult with the given reason.
func Skip(reason string) GateResult {
	return GateResult{Status: "SKIP", Reason: reason}
}

// OfflineGateResults contains results for offline gate checks (O0-O10).
type OfflineGateResults struct {
	O0  GateResult // Git ignore - checks that state/result dirs are ignored
	O1  GateResult // Scan repo - placeholder for repo scanning
	O3  GateResult // Audit project - placeholder for project audit
	O5  GateResult // Config validation - validates workflow.yaml
	O7  GateResult // Version sync - placeholder for version sync check
	O8  GateResult // File encoding - checks for CRLF/UTF-16
	O10 GateResult // Test suite - placeholder for test suite
}

// OnlineGateResults contains results for online gate checks (N1-N3).
type OnlineGateResults struct {
	N1 GateResult // Kickoff dry-run - placeholder
	N2 GateResult // Rollback output - placeholder
	N3 GateResult // Stats JSON - placeholder
}

// EvaluationResult contains the complete evaluation results.
type EvaluationResult struct {
	Offline  OfflineGateResults
	Online   OnlineGateResults
	ScoreCap float64
	Grade    string
}

// Evaluator performs gate checks on a project.
type Evaluator struct {
	RootPath string
}

// New creates a new Evaluator for the given project root path.
func New(rootPath string) *Evaluator {
	if rootPath == "" {
		rootPath = "."
	}
	return &Evaluator{RootPath: rootPath}
}

// RunOffline executes all offline gate checks.
func (e *Evaluator) RunOffline() OfflineGateResults {
	return OfflineGateResults{
		O0:  CheckO0GitIgnore(e.RootPath),
		O1:  Skip("scan repo not implemented"),
		O3:  Skip("audit project not implemented"),
		O5:  CheckO5ConfigValidation(e.RootPath),
		O7:  CheckO7VersionSync(e.RootPath),
		O8:  CheckO8FileEncoding(e.RootPath),
		O10: Skip("test suite not implemented"),
	}
}

// RunOnline executes all online gate checks.
func (e *Evaluator) RunOnline() OnlineGateResults {
	return OnlineGateResults{
		N1: Skip("kickoff dry-run not implemented"),
		N2: Skip("rollback output not implemented"),
		N3: Skip("stats JSON not implemented"),
	}
}

// Run executes all gate checks and calculates the score.
func (e *Evaluator) Run() EvaluationResult {
	offline := e.RunOffline()
	online := e.RunOnline()
	scoreCap := CalculateScoreCap(offline, online)
	grade := ScoreToGrade(scoreCap)

	return EvaluationResult{
		Offline:  offline,
		Online:   online,
		ScoreCap: scoreCap,
		Grade:    grade,
	}
}

package audit

// Severity represents the severity level of a finding.
type Severity string

const (
	SeverityP0 Severity = "P0"
	SeverityP1 Severity = "P1"
	SeverityP2 Severity = "P2"
)

// Finding IDs for P0 (Critical - blocks workflow)
const (
	FindingMissingClaudeMD     = "MISSING_CLAUDE_MD"
	FindingMissingAgentsMD     = "MISSING_AGENTS_MD"
	FindingMissingWorkflowYAML = "MISSING_WORKFLOW_YAML"
)

// Finding IDs for P1 (Warning - may cause issues)
const (
	FindingDirtyWorktree = "DIRTY_WORKTREE"
	FindingMissingAIDir  = "MISSING_AI_DIR"
)

// Finding IDs for P2 (Info)
const (
	FindingMissingREADME = "MISSING_README"
)

// Finding messages
var findingMessages = map[string]string{
	FindingMissingClaudeMD:     "Required file CLAUDE.md is missing",
	FindingMissingAgentsMD:     "Required file AGENTS.md is missing",
	FindingMissingWorkflowYAML: "Required file .ai/config/workflow.yaml is missing",
	FindingDirtyWorktree:       "Git worktree has uncommitted changes",
	FindingMissingAIDir:        ".ai directory structure is incomplete",
	FindingMissingREADME:       "README.md is missing",
}

// Finding represents a single audit finding.
type Finding struct {
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	File     string   `json:"file,omitempty"`
}

// Summary holds counts of findings by severity.
type Summary struct {
	P0Count int `json:"p0_count"`
	P1Count int `json:"p1_count"`
	P2Count int `json:"p2_count"`
}

// AuditResult contains the full audit results.
type AuditResult struct {
	Findings []Finding `json:"findings"`
	Summary  Summary   `json:"summary"`
	Passed   bool      `json:"passed"`
}

// NewFinding creates a new Finding with the given ID and severity.
// The message is looked up from the findingMessages map.
func NewFinding(id string, severity Severity, file string) Finding {
	msg := findingMessages[id]
	if msg == "" {
		msg = "Unknown finding"
	}
	return Finding{
		ID:       id,
		Severity: severity,
		Message:  msg,
		File:     file,
	}
}

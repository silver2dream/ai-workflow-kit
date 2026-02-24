// Package epicaudit provides epic coverage auditing against design.md.
package epicaudit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

// AuditOptions holds options for running an epic audit.
type AuditOptions struct {
	SpecName  string
	StateRoot string
}

// TaskStatus represents the status of an epic task.
type TaskStatus struct {
	Text         string `json:"text"`
	Completed    bool   `json:"completed"`
	IssueNumber  int    `json:"issue_number,omitempty"`
	HasResult    bool   `json:"has_result"`
	ResultStatus string `json:"result_status,omitempty"`
}

// AuditReport is the structured output of an epic audit.
type AuditReport struct {
	SpecName       string       `json:"spec_name"`
	EpicIssue      int          `json:"epic_issue"`
	DesignFile     string       `json:"design_file"`
	DesignExists   bool         `json:"design_exists"`
	DesignSections []string     `json:"design_sections"`
	Tasks          []TaskStatus `json:"tasks"`
	TotalTasks     int          `json:"total_tasks"`
	CompletedTasks int          `json:"completed_tasks"`
	PendingTasks   int          `json:"pending_tasks"`

	// Coverage analysis signals (for LLM to interpret)
	DesignRequirements []string `json:"design_requirements"`
	ReposInDesign      []string `json:"repos_in_design"`
	ReposInTasks       []string `json:"repos_in_tasks"`
	CompletedResults   []string `json:"completed_results"`

	// Suggested action
	SuggestedAction string   `json:"suggested_action"` // "ok" | "gaps_detected" | "needs_review"
	GapHints        []string `json:"gap_hints"`
}

// RunAudit executes an epic audit for the given spec.
func RunAudit(ctx context.Context, opts AuditOptions, ghClient analyzer.GitHubClientInterface) (*AuditReport, error) {
	if opts.SpecName == "" {
		return nil, fmt.Errorf("spec name is required")
	}

	// 1. Load config
	configPath := filepath.Join(opts.StateRoot, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if !cfg.IsEpicMode() {
		return nil, fmt.Errorf("audit-epic requires github_epic tracking mode (current: %s)", cfg.Specs.Tracking.Mode)
	}

	epicIssue := cfg.GetEpicIssue(opts.SpecName)
	if epicIssue == 0 {
		return nil, fmt.Errorf("no epic issue configured for spec %q", opts.SpecName)
	}

	report := &AuditReport{
		SpecName:  opts.SpecName,
		EpicIssue: epicIssue,
	}

	// 2. Read design.md
	designPath := filepath.Join(opts.StateRoot, cfg.Specs.BasePath, opts.SpecName, "design.md")
	report.DesignFile = designPath

	designContent, err := os.ReadFile(designPath)
	if err != nil {
		report.DesignExists = false
		// No design.md — cannot do coverage analysis, return early with "ok"
		report.SuggestedAction = "ok"
		report.GapHints = []string{"NO_DESIGN_FILE"}
		return report, nil
	}
	report.DesignExists = true
	designText := string(designContent)

	report.DesignSections = extractDesignSections(designText)
	report.DesignRequirements = extractRequirementLabels(designText)

	knownRepos := make([]string, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		knownRepos = append(knownRepos, r.Name)
	}
	report.ReposInDesign = extractRepoMentions(designText, knownRepos)

	// 3. Read Epic body from GitHub
	body, err := ghClient.GetIssueBody(ctx, epicIssue)
	if err != nil {
		return nil, fmt.Errorf("failed to read epic #%d: %w", epicIssue, err)
	}

	tasks := analyzer.ParseEpicBody(body)
	report.TotalTasks = len(tasks)

	// Collect all task text for repo mention analysis
	var allTaskText strings.Builder
	for _, t := range tasks {
		ts := TaskStatus{
			Text:        t.Text,
			Completed:   t.Completed,
			IssueNumber: t.IssueNumber,
		}
		if t.Completed {
			report.CompletedTasks++
		}
		allTaskText.WriteString(t.Text + " ")

		// Check results
		if t.IssueNumber > 0 {
			resultPath := filepath.Join(opts.StateRoot, ".ai", "results", fmt.Sprintf("issue-%d.json", t.IssueNumber))
			if _, err := os.Stat(resultPath); err == nil {
				ts.HasResult = true
				report.CompletedResults = append(report.CompletedResults, resultPath)
			}
		}

		report.Tasks = append(report.Tasks, ts)
	}
	report.PendingTasks = report.TotalTasks - report.CompletedTasks
	report.ReposInTasks = extractRepoMentions(allTaskText.String(), knownRepos)

	// 4. Generate gap hints
	report.GapHints = generateGapHints(report)

	// 5. Determine suggested action
	if len(report.GapHints) == 0 {
		report.SuggestedAction = "ok"
	} else {
		report.SuggestedAction = "gaps_detected"
	}

	return report, nil
}

// --- Extraction helpers ---

// headingRe matches markdown H2/H3 headings
var headingRe = regexp.MustCompile(`^#{2,3}\s+(.+)$`)

// extractDesignSections parses design.md and returns H2/H3 heading texts.
func extractDesignSections(content string) []string {
	var sections []string
	for _, line := range strings.Split(content, "\n") {
		if m := headingRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			sections = append(sections, strings.TrimSpace(m[1]))
		}
	}
	return sections
}

// requirementRe matches requirement labels like R1, R2, FR-1, REQ-01, etc.
var requirementRe = regexp.MustCompile(`\b((?:R|FR|REQ|SR|NFR)-?\d+)\b`)

// extractRequirementLabels extracts requirement labels from text.
func extractRequirementLabels(content string) []string {
	matches := requirementRe.FindAllString(content, -1)
	// Deduplicate
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		upper := strings.ToUpper(m)
		if !seen[upper] {
			seen[upper] = true
			result = append(result, upper)
		}
	}
	return result
}

// extractRepoMentions finds known repo names mentioned in text.
func extractRepoMentions(text string, knownRepos []string) []string {
	lower := strings.ToLower(text)
	var found []string
	for _, repo := range knownRepos {
		if strings.Contains(lower, strings.ToLower(repo)) {
			found = append(found, repo)
		}
	}
	return found
}

// --- Gap analysis ---

// integrationKeywords are terms that indicate an integration/wiring task.
var integrationKeywords = []string{
	"integrat", "wiring", "connect", "entry point", "bootstrap",
	"wire up", "hook up", "register", "main.go", "串接", "整合",
}

// generateGapHints compares design sections vs task coverage and returns hints.
// These are structural heuristics — semantic analysis is left to the Principal LLM.
func generateGapHints(report *AuditReport) []string {
	var hints []string

	// 1. Repo coverage check
	for _, repo := range report.ReposInDesign {
		found := false
		for _, taskRepo := range report.ReposInTasks {
			if strings.EqualFold(repo, taskRepo) {
				found = true
				break
			}
		}
		if !found {
			hints = append(hints, fmt.Sprintf("REPO_UNCOVERED:%s", repo))
		}
	}

	// 2. Requirement coverage check
	if len(report.DesignRequirements) > 0 {
		allTaskText := ""
		for _, t := range report.Tasks {
			allTaskText += strings.ToLower(t.Text) + " "
		}
		for _, req := range report.DesignRequirements {
			if !strings.Contains(allTaskText, strings.ToLower(req)) {
				hints = append(hints, fmt.Sprintf("REQ_UNCOVERED:%s", req))
			}
		}
	}

	// 3. Low task count warning
	if len(report.DesignSections) > 0 && report.TotalTasks < len(report.DesignSections) {
		hints = append(hints, "LOW_TASK_COUNT")
	}

	// 4. Missing integration/wiring task detection
	if len(report.ReposInDesign) > 1 {
		hasIntegrationTask := false
		for _, t := range report.Tasks {
			lower := strings.ToLower(t.Text)
			for _, kw := range integrationKeywords {
				if strings.Contains(lower, kw) {
					hasIntegrationTask = true
					break
				}
			}
			if hasIntegrationTask {
				break
			}
		}
		if !hasIntegrationTask {
			hints = append(hints, "MISSING_INTEGRATION_TASK")
		}
	}

	return hints
}

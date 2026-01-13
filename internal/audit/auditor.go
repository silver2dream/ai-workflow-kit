package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AuditProject performs a full audit of the project at the given path.
// It checks for required files, worktree status, and .ai directory structure.
func AuditProject(path string) (*AuditResult, error) {
	var allFindings []Finding

	// Check required files
	allFindings = append(allFindings, CheckRequiredFiles(path)...)

	// Check worktree status
	allFindings = append(allFindings, CheckWorktreeStatus(path)...)

	// Check .ai directory structure
	allFindings = append(allFindings, CheckAIDirectory(path)...)

	summary := CalculateSummary(allFindings)

	// Project passes if there are no P0 findings
	passed := summary.P0Count == 0

	return &AuditResult{
		Findings: allFindings,
		Summary:  summary,
		Passed:   passed,
	}, nil
}

// CheckRequiredFiles checks for required files (CLAUDE.md, AGENTS.md, workflow.yaml).
func CheckRequiredFiles(path string) []Finding {
	var findings []Finding

	// Check CLAUDE.md
	claudePath := filepath.Join(path, "CLAUDE.md")
	if !fileExists(claudePath) {
		findings = append(findings, NewFinding(FindingMissingClaudeMD, SeverityP0, claudePath))
	}

	// Check AGENTS.md
	agentsPath := filepath.Join(path, "AGENTS.md")
	if !fileExists(agentsPath) {
		findings = append(findings, NewFinding(FindingMissingAgentsMD, SeverityP0, agentsPath))
	}

	// Check .ai/config/workflow.yaml
	workflowPath := filepath.Join(path, ".ai", "config", "workflow.yaml")
	if !fileExists(workflowPath) {
		findings = append(findings, NewFinding(FindingMissingWorkflowYAML, SeverityP0, workflowPath))
	}

	// Check README.md (P2 - informational)
	readmePath := filepath.Join(path, "README.md")
	if !fileExists(readmePath) {
		findings = append(findings, NewFinding(FindingMissingREADME, SeverityP2, readmePath))
	}

	return findings
}

// CheckWorktreeStatus checks if the git worktree is clean.
func CheckWorktreeStatus(path string) []Finding {
	var findings []Finding

	// Run git status --porcelain to check for uncommitted changes
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		// If git command fails, we cannot determine worktree status
		// This could happen if path is not a git repository
		return findings
	}

	// If output is non-empty, there are uncommitted changes
	if len(strings.TrimSpace(string(output))) > 0 {
		findings = append(findings, NewFinding(FindingDirtyWorktree, SeverityP1, path))
	}

	return findings
}

// CheckAIDirectory checks if the .ai directory structure is complete.
func CheckAIDirectory(path string) []Finding {
	var findings []Finding

	// Required subdirectories in .ai
	requiredDirs := []string{
		"config",
		"state",
		"results",
		"exe-logs",
	}

	aiDir := filepath.Join(path, ".ai")
	if !dirExists(aiDir) {
		findings = append(findings, NewFinding(FindingMissingAIDir, SeverityP1, aiDir))
		return findings
	}

	// Check each required subdirectory
	for _, dir := range requiredDirs {
		dirPath := filepath.Join(aiDir, dir)
		if !dirExists(dirPath) {
			findings = append(findings, NewFinding(FindingMissingAIDir, SeverityP1, dirPath))
		}
	}

	return findings
}

// CalculateSummary calculates finding counts by severity.
func CalculateSummary(findings []Finding) Summary {
	var summary Summary

	for _, f := range findings {
		switch f.Severity {
		case SeverityP0:
			summary.P0Count++
		case SeverityP1:
			summary.P1Count++
		case SeverityP2:
			summary.P2Count++
		}
	}

	return summary
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists at the given path.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CheckUninitializedSubmodules checks for uninitialized submodules in the repository.
// Property 8: Audit Submodule Detection - Uninitialized submodules (P1) (Req 8.1)
func CheckUninitializedSubmodules(repoRoot string) []Finding {
	var findings []Finding

	// Check if .gitmodules exists
	gitmodulesPath := filepath.Join(repoRoot, ".gitmodules")
	if !fileExists(gitmodulesPath) {
		return findings
	}

	// Get submodule status
	cmd := exec.Command("git", "-C", repoRoot, "submodule", "status")
	output, err := cmd.Output()
	if err != nil {
		return findings
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Uninitialized submodules start with '-'
		if strings.HasPrefix(line, "-") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				path := parts[1]
				findings = append(findings, NewFinding(FindingUninitializedSubmodule, SeverityP1, path))
			}
		}
	}

	return findings
}

// CheckDirtySubmodules checks for dirty submodule working trees.
// Property 8: Audit Submodule Detection - Dirty submodule working trees (P1) (Req 8.2, 8.5)
func CheckDirtySubmodules(repoRoot string, submodulePaths []string) []Finding {
	var findings []Finding

	for _, subPath := range submodulePaths {
		submoduleDir := filepath.Join(repoRoot, subPath)
		if !dirExists(submoduleDir) {
			continue
		}

		// Check if submodule has uncommitted changes
		cmd := exec.Command("git", "-C", submoduleDir, "status", "--porcelain")
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		if len(strings.TrimSpace(string(output))) > 0 {
			findings = append(findings, NewFinding(FindingDirtySubmodule, SeverityP1, subPath))
		}
	}

	return findings
}

// CheckUnpushedSubmoduleCommits checks for unpushed commits in submodules.
// Property 8: Audit Submodule Detection - Unpushed submodule commits (P1) (Req 8.3)
func CheckUnpushedSubmoduleCommits(repoRoot string, submodulePaths []string) []Finding {
	var findings []Finding

	for _, subPath := range submodulePaths {
		submoduleDir := filepath.Join(repoRoot, subPath)
		if !dirExists(submoduleDir) {
			continue
		}

		// Check if submodule has unpushed commits
		cmd := exec.Command("git", "-C", submoduleDir, "log", "--oneline", "@{u}..HEAD")
		output, err := cmd.Output()
		if err != nil {
			// Command may fail if no upstream is set, which is ok
			continue
		}

		// If output is non-empty, there are unpushed commits
		if len(strings.TrimSpace(string(output))) > 0 {
			findings = append(findings, NewFinding(FindingUnpushedSubmoduleCommit, SeverityP1, subPath))
		}
	}

	return findings
}

// RunSubmoduleAudit runs a complete submodule audit on the repository.
// Returns findings from all submodule checks.
func RunSubmoduleAudit(repoRoot string, submodulePaths []string) []Finding {
	var findings []Finding
	findings = append(findings, CheckUninitializedSubmodules(repoRoot)...)
	findings = append(findings, CheckDirtySubmodules(repoRoot, submodulePaths)...)
	findings = append(findings, CheckUnpushedSubmoduleCommits(repoRoot, submodulePaths)...)
	return findings
}

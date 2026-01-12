package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TicketMetadata contains parsed metadata from a ticket/issue body
type TicketMetadata struct {
	Repo               string // root, backend, frontend, etc.
	Severity           string // P0, P1, P2
	Source             string // audit:<finding-id>, tasks.md #<n>
	Release            bool
	SpecName           string // Spec name for task mapping
	TaskLine           int    // Task line number for auto-marking
	AllowParentChanges bool   // Allow changes outside submodule boundary
	AllowScriptChanges bool   // Allow changes to .ai/scripts/
	AllowSecrets       bool   // Allow secrets in diff
}

// ParseTicketMetadata extracts metadata from issue body
func ParseTicketMetadata(body string) *TicketMetadata {
	meta := &TicketMetadata{
		Repo: "root", // Default repo
	}

	// Parse Repo: **Repo**: backend or - Repo: backend
	repoPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Repo\*\*\s*:\s*([^\s\r\n]+)`),
		regexp.MustCompile(`(?im)^[-*]\s*Repo\s*:\s*([^\s\r\n]+)`),
	}
	for _, pattern := range repoPatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Repo = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse Severity: **Severity**: P0 or - Severity: P0
	severityPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Severity\*\*\s*:\s*(P[0-3])`),
		regexp.MustCompile(`(?im)^[-*]\s*Severity\s*:\s*(P[0-3])`),
	}
	for _, pattern := range severityPatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Severity = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse Source
	sourcePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Source\*\*\s*:\s*([^\r\n]+)`),
		regexp.MustCompile(`(?im)^[-*]\s*Source\s*:\s*([^\r\n]+)`),
	}
	for _, pattern := range sourcePatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Source = strings.TrimSpace(matches[1])
			break
		}
	}

	// Parse Release: **Release**: true/false
	releasePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Release\*\*\s*:\s*(true|false)`),
		regexp.MustCompile(`(?im)^[-*]\s*Release\s*:\s*(true|false)`),
	}
	for _, pattern := range releasePatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			meta.Release = strings.ToLower(matches[1]) == "true"
			break
		}
	}

	// Parse Spec name: **Spec**: name or - Spec: name
	specPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Spec\*\*\s*:\s*([^\r\n]+)`),
		regexp.MustCompile(`(?im)^[-*]\s*Spec\s*:\s*([^\r\n]+)`),
	}
	for _, pattern := range specPatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			specName := strings.TrimSpace(matches[1])
			// Validate: no path traversal
			if !strings.Contains(specName, "/") && !strings.Contains(specName, "\\") && !strings.Contains(specName, "..") {
				meta.SpecName = specName
			}
			break
		}
	}

	// Parse Task Line: **Task Line**: 123 or - Task Line: 123
	taskLinePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?im)\*\*Task\s+Line\*\*\s*:\s*(\d+)`),
		regexp.MustCompile(`(?im)^[-*]\s*Task\s+Line\s*:\s*(\d+)`),
	}
	for _, pattern := range taskLinePatterns {
		if matches := pattern.FindStringSubmatch(body); len(matches) > 1 {
			var line int
			if _, err := fmt.Sscanf(matches[1], "%d", &line); err == nil {
				meta.TaskLine = line
			}
			break
		}
	}

	// Parse special flags in Constraints section
	// Look for: allow-parent-changes, allow-script-changes, allow-secrets
	constraintsPattern := regexp.MustCompile(`(?is)##\s*Constraints\s*\n(.*?)(?:\n##|\z)`)
	if matches := constraintsPattern.FindStringSubmatch(body); len(matches) > 1 {
		constraints := matches[1]
		meta.AllowParentChanges = strings.Contains(strings.ToLower(constraints), "allow-parent-changes")
		meta.AllowScriptChanges = strings.Contains(strings.ToLower(constraints), "allow-script-changes")
		meta.AllowSecrets = strings.Contains(strings.ToLower(constraints), "allow-secrets")
	}

	return meta
}

// SaveTicketFile saves the issue body to a ticket file
func SaveTicketFile(stateRoot string, issueNumber int, body string) (string, error) {
	tempDir := filepath.Join(stateRoot, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	ticketPath := filepath.Join(tempDir, fmt.Sprintf("ticket-%d.md", issueNumber))
	if err := os.WriteFile(ticketPath, []byte(body), 0644); err != nil {
		return "", fmt.Errorf("failed to write ticket file: %w", err)
	}

	return ticketPath, nil
}

// LoadTicketFile loads a ticket file content
func LoadTicketFile(stateRoot string, issueNumber int) (string, error) {
	ticketPath := filepath.Join(stateRoot, ".ai", "temp", fmt.Sprintf("ticket-%d.md", issueNumber))
	data, err := os.ReadFile(ticketPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CleanupTicketFile removes the ticket file
func CleanupTicketFile(stateRoot string, issueNumber int) error {
	ticketPath := filepath.Join(stateRoot, ".ai", "temp", fmt.Sprintf("ticket-%d.md", issueNumber))
	err := os.Remove(ticketPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// VerificationCommands contains parsed verification commands from a ticket
type VerificationCommands struct {
	Repo     string   // repo name (backend, frontend, root, etc.)
	Commands []string // list of commands to run
}

// ParseVerificationCommands extracts verification commands from ticket body
// Format: ## Verification
//
//	- backend: `go build ./...` and `go test ./...`
//	- frontend: `npm run build` and `npm run test`
func ParseVerificationCommands(body string) []VerificationCommands {
	var results []VerificationCommands

	// Find ## Verification section
	verificationPattern := regexp.MustCompile(`(?is)##\s*Verification\s*\n(.*?)(?:\n##|\z)`)
	matches := verificationPattern.FindStringSubmatch(body)
	if len(matches) < 2 {
		return results
	}

	section := matches[1]

	// Parse each line: - repo: `cmd1` and `cmd2`
	linePattern := regexp.MustCompile(`(?m)^[-*]\s*(\w+)\s*:\s*(.+)$`)
	lineMatches := linePattern.FindAllStringSubmatch(section, -1)

	for _, match := range lineMatches {
		if len(match) < 3 {
			continue
		}

		repo := strings.TrimSpace(match[1])
		cmdPart := match[2]

		// Extract commands from backticks
		cmdPattern := regexp.MustCompile("`([^`]+)`")
		cmdMatches := cmdPattern.FindAllStringSubmatch(cmdPart, -1)

		var commands []string
		for _, cm := range cmdMatches {
			if len(cm) > 1 {
				cmd := strings.TrimSpace(cm[1])
				if cmd != "" {
					commands = append(commands, cmd)
				}
			}
		}

		if len(commands) > 0 {
			results = append(results, VerificationCommands{
				Repo:     strings.ToLower(repo),
				Commands: commands,
			})
		}
	}

	return results
}

// GetVerificationCommandsForRepo returns verification commands for a specific repo
func GetVerificationCommandsForRepo(body, repoName string) []string {
	allCommands := ParseVerificationCommands(body)
	repoName = strings.ToLower(strings.TrimSpace(repoName))

	for _, vc := range allCommands {
		if vc.Repo == repoName {
			return vc.Commands
		}
	}

	// Fallback: if repo is empty or "root", try to find root commands
	if repoName == "" || repoName == "root" {
		for _, vc := range allCommands {
			if vc.Repo == "root" {
				return vc.Commands
			}
		}
	}

	return nil
}

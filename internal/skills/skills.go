// Package skills provides utilities for validating and working with AWK skill definitions.
package skills

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// SkillMetadata represents the parsed frontmatter from a SKILL.md file.
type SkillMetadata struct {
	Name         string
	Description  string
	AllowedTools []string
}

// RequiredFiles returns the list of required files for a skill.
func RequiredFiles() []string {
	return []string{
		"phases/main-loop.md",
		"tasks/generate-tasks.md",
		"tasks/create-task.md",
		"references/contracts.md",
	}
}

// ValidateSkillStructure validates that a skill directory has the required structure.
// Returns nil if valid, or an error describing what's missing.
func ValidateSkillStructure(skillDir string) error {
	// Check SKILL.md exists
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		return &SkillValidationError{Message: "SKILL.md does not exist"}
	}

	// Check required files
	for _, file := range RequiredFiles() {
		path := filepath.Join(skillDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return &SkillValidationError{Message: file + " does not exist"}
		}
	}

	return nil
}

// ParseSkillMetadata parses the YAML frontmatter from a SKILL.md file.
func ParseSkillMetadata(skillFile string) (*SkillMetadata, error) {
	f, err := os.Open(skillFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	meta := &SkillMetadata{}

	// Check for frontmatter start
	if !scanner.Scan() {
		return nil, &SkillValidationError{Message: "empty file"}
	}
	firstLine := strings.TrimRight(scanner.Text(), "\r\n")
	if firstLine != "---" {
		return nil, &SkillValidationError{Message: "frontmatter does not start with ---"}
	}

	// Parse frontmatter until closing ---
	inFrontmatter := true
	for scanner.Scan() && inFrontmatter {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if line == "---" {
			inFrontmatter = false
			break
		}

		// Parse key: value pairs
		if strings.HasPrefix(line, "name:") {
			meta.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "description:") {
			desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			// Check for multi-line indicator
			if strings.HasPrefix(desc, "|") || strings.HasPrefix(desc, ">") {
				return nil, &SkillValidationError{Message: "description should be single-line, not multi-line"}
			}
			meta.Description = desc
		} else if strings.HasPrefix(line, "allowed-tools:") {
			toolsStr := strings.TrimSpace(strings.TrimPrefix(line, "allowed-tools:"))
			meta.AllowedTools = parseCommaSeparatedList(toolsStr)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return meta, nil
}

// ValidateFrontmatter validates the frontmatter of a SKILL.md file.
func ValidateFrontmatter(skillFile string) error {
	meta, err := ParseSkillMetadata(skillFile)
	if err != nil {
		return err
	}

	if meta.Name == "" {
		return &SkillValidationError{Message: "name field is missing or empty"}
	}

	if meta.Description == "" {
		return &SkillValidationError{Message: "description field is missing or empty"}
	}

	if len(meta.AllowedTools) == 0 {
		return &SkillValidationError{Message: "allowed-tools field is missing or empty"}
	}

	return nil
}

// ValidateMainLoop validates that main-loop.md contains required content.
func ValidateMainLoop(mainLoopFile string) error {
	content, err := os.ReadFile(mainLoopFile)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// Check for required content
	// Note: The actual file uses "analyze-next" (with hyphen) in commands
	requiredContent := []struct {
		pattern string
		desc    string
	}{
		{"analyze-next", "analyze-next command reference"},
		{"next_action", "next_action routing"},
		{"Loop", "Loop Safety"},
	}

	for _, req := range requiredContent {
		if !strings.Contains(contentStr, req.pattern) {
			return &SkillValidationError{Message: "main-loop.md missing " + req.desc}
		}
	}

	return nil
}

// ValidateContracts validates that contracts.md contains required action definitions.
func ValidateContracts(contractsFile string) error {
	content, err := os.ReadFile(contractsFile)
	if err != nil {
		return err
	}

	contentStr := string(content)

	// Check for required actions
	requiredActions := []string{
		"generate_tasks",
		"create_task",
		"dispatch_worker",
		"check_result",
		"review_pr",
		"all_complete",
		"none",
	}

	for _, action := range requiredActions {
		if !strings.Contains(contentStr, action) {
			return &SkillValidationError{Message: "contracts.md missing action: " + action}
		}
	}

	return nil
}

// SkillValidationError represents a skill validation error.
type SkillValidationError struct {
	Message string
}

func (e *SkillValidationError) Error() string {
	return e.Message
}

// parseCommaSeparatedList parses a comma-separated string into a slice.
func parseCommaSeparatedList(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

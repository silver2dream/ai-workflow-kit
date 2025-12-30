package task

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var issueRefRe = regexp.MustCompile(`<!--\s*Issue\s*#(\d+)\s*-->`)

// ExtractTaskText removes the Issue reference and checkbox prefix from a task line.
func ExtractTaskText(line string) string {
	text := strings.TrimRight(line, "\r\n")
	text = issueRefRe.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)

	// Remove checkbox prefix: - [ ], - [x], - [X]
	checkboxRe := regexp.MustCompile(`^\s*-\s*\[[ xX]\]\s*`)
	text = checkboxRe.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// HasIssueRef checks if a line already has an Issue reference.
// Returns the issue number and true if found, otherwise 0 and false.
func HasIssueRef(line string) (int, bool) {
	matches := issueRefRe.FindStringSubmatch(line)
	if len(matches) < 2 {
		return 0, false
	}
	var num int
	fmt.Sscanf(matches[1], "%d", &num)
	return num, true
}

// DefaultIssueTitle generates a default issue title from task text.
func DefaultIssueTitle(taskText string) string {
	if taskText == "" {
		return "[feat] implement task"
	}

	trimmed := strings.TrimSpace(taskText)
	if strings.HasPrefix(trimmed, "[") {
		return trimmed
	}

	// Normalize whitespace and lowercase
	spaceRe := regexp.MustCompile(`\s+`)
	normalized := spaceRe.ReplaceAllString(trimmed, " ")
	normalized = strings.ToLower(normalized)
	return fmt.Sprintf("[feat] %s", normalized)
}

// AppendIssueRef appends <!-- Issue #N --> to the specified line in a tasks.md file.
func AppendIssueRef(tasksPath string, lineNumber, issueNumber int) error {
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return fmt.Errorf("failed to read tasks file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if lineNumber < 1 || lineNumber > len(lines) {
		return fmt.Errorf("task_line %d out of range (file has %d lines)", lineNumber, len(lines))
	}

	current := lines[lineNumber-1]
	if _, found := HasIssueRef(current); found {
		// Already has issue reference, nothing to do
		return nil
	}

	suffix := fmt.Sprintf(" <!-- Issue #%d -->", issueNumber)
	trimmed := strings.TrimRight(current, "\r\n\t ")
	lines[lineNumber-1] = trimmed + suffix

	output := strings.Join(lines, "\n")
	if err := os.WriteFile(tasksPath, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write tasks file: %w", err)
	}

	return nil
}

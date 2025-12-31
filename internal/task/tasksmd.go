package task

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

// CommitTasksUpdate commits the tasks.md file after an update.
// action should be "linked" (issue created) or "complete" (PR merged).
// This is a best-effort operation - errors are logged but not returned.
func CommitTasksUpdate(tasksPath string, issueNumber int, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get git root directory
	dir := filepath.Dir(tasksPath)
	rootCmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	rootCmd.Dir = dir
	rootOutput, err := rootCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get git root: %w", err)
	}
	gitRoot := strings.TrimSpace(string(rootOutput))

	// Get relative path from git root
	relPath, err := filepath.Rel(gitRoot, tasksPath)
	if err != nil {
		relPath = tasksPath
	}

	// Stage the file
	addCmd := exec.CommandContext(ctx, "git", "add", tasksPath)
	addCmd.Dir = gitRoot
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("failed to stage tasks.md: %w", err)
	}

	// Check if there are staged changes
	diffCmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet", "--", tasksPath)
	diffCmd.Dir = gitRoot
	if err := diffCmd.Run(); err == nil {
		// No changes staged, nothing to commit
		return nil
	}

	// Commit with appropriate message (follows [type] subject format)
	var commitMsg string
	switch action {
	case "linked":
		commitMsg = fmt.Sprintf("[chore] link task to issue #%d [skip ci]", issueNumber)
	case "complete":
		commitMsg = fmt.Sprintf("[chore] mark task for issue #%d complete [skip ci]", issueNumber)
	default:
		commitMsg = fmt.Sprintf("[chore] update %s [skip ci]", relPath)
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	commitCmd.Dir = gitRoot
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("failed to commit tasks.md: %w", err)
	}

	return nil
}

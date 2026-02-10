package analyzer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EpicTask represents a task entry in a GitHub tracking issue
type EpicTask struct {
	IssueNumber int    // 0 if no issue ref (plain text task)
	Text        string // task description text
	Completed   bool   // [x] vs [ ]
	RawLine     string // original line for reconstruction
}

// epicTaskRe matches GitHub task list items: - [ ] #42 description or - [x] plain text
var epicTaskRe = regexp.MustCompile(`^(\s*)-\s*\[([ xX])\]\s*(.*)$`)

// issueRefRe matches #N at the start of task text
var epicIssueRefRe = regexp.MustCompile(`^#(\d+)\s*(.*)$`)

// ParseEpicBody parses a GitHub tracking issue body for task list items.
// Non-task lines are ignored. Returns tasks in order of appearance.
func ParseEpicBody(body string) []EpicTask {
	lines := strings.Split(body, "\n")
	var tasks []EpicTask

	for _, line := range lines {
		matches := epicTaskRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		completed := matches[2] == "x" || matches[2] == "X"
		remainder := strings.TrimSpace(matches[3])

		task := EpicTask{
			Completed: completed,
			RawLine:   line,
		}

		// Check if remainder starts with #N (issue reference)
		if issueMatches := epicIssueRefRe.FindStringSubmatch(remainder); issueMatches != nil {
			num, _ := strconv.Atoi(issueMatches[1])
			task.IssueNumber = num
			task.Text = strings.TrimSpace(issueMatches[2])
		} else {
			task.Text = remainder
		}

		tasks = append(tasks, task)
	}

	return tasks
}

// AppendTaskToEpicBody appends a new `- [ ] #N description` line to the epic body.
// It inserts after the last task list entry to maintain grouping.
func AppendTaskToEpicBody(body string, issueNumber int, description string) string {
	newLine := fmt.Sprintf("- [ ] #%d", issueNumber)
	if description != "" {
		newLine += " " + description
	}

	lines := strings.Split(body, "\n")
	lastTaskIdx := -1
	for i, line := range lines {
		if epicTaskRe.MatchString(line) {
			lastTaskIdx = i
		}
	}

	if lastTaskIdx == -1 {
		// No existing tasks — append at end
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		return body + newLine + "\n"
	}

	// Insert after last task line
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:lastTaskIdx+1]...)
	result = append(result, newLine)
	result = append(result, lines[lastTaskIdx+1:]...)
	return strings.Join(result, "\n")
}

// CountEpicProgress returns (total, completed) counts from an epic body
func CountEpicProgress(body string) (total, completed int) {
	tasks := ParseEpicBody(body)
	total = len(tasks)
	for _, t := range tasks {
		if t.Completed {
			completed++
		}
	}
	return
}

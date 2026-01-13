package task

import (
	"regexp"
	"strings"
)

// Task represents a parsed task from tasks.md
type Task struct {
	ID        string   // Task ID (e.g., "1", "2.1")
	Title     string   // Task title text
	Completed bool     // Whether task is marked complete [x]
	DependsOn []string // List of task IDs this task depends on
	Subtasks  []*Task  // Nested subtasks
	Optional  bool     // Whether task is marked optional with asterisk
}

// ToMap converts a Task to a map representation
func (t *Task) ToMap() map[string]interface{} {
	subtasks := make([]map[string]interface{}, len(t.Subtasks))
	for i, st := range t.Subtasks {
		subtasks[i] = st.ToMap()
	}

	return map[string]interface{}{
		"id":         t.ID,
		"title":      t.Title,
		"completed":  t.Completed,
		"depends_on": t.DependsOn,
		"subtasks":   subtasks,
		"optional":   t.Optional,
	}
}

// Patterns for parsing tasks.md format
var (
	// Main task: - [ ] N. Title or - [x] N. Title
	mainTaskRe = regexp.MustCompile(`^-\s*\[([ xX])\]\s*(\d+)\.\s+(.+)$`)
	// Subtask: indented - [ ] N.M Title or - [ ]* N.M Title (optional)
	subtaskRe = regexp.MustCompile(`^\s+-\s*\[([ xX])\]\*?\s*(\d+\.\d+)\s+(.+)$`)
	// Dependencies: _depends_on: 1, 2_
	dependsOnRe = regexp.MustCompile(`_depends_on:\s*([^_]+)_`)
)

// ParseTasks parses tasks.md content and returns a list of tasks
func ParseTasks(content string) []*Task {
	lines := strings.Split(content, "\n")
	var tasks []*Task
	var currentTask *Task

	for _, line := range lines {
		// Try to match main task
		if matches := mainTaskRe.FindStringSubmatch(line); matches != nil {
			completed := matches[1] == "x" || matches[1] == "X"
			task := &Task{
				ID:        matches[2],
				Title:     strings.TrimSpace(matches[3]),
				Completed: completed,
				DependsOn: []string{},
				Subtasks:  []*Task{},
			}
			tasks = append(tasks, task)
			currentTask = task
			continue
		}

		// Try to match subtask
		if matches := subtaskRe.FindStringSubmatch(line); matches != nil {
			completed := matches[1] == "x" || matches[1] == "X"
			optional := strings.Contains(line, "]*")
			subtask := &Task{
				ID:        matches[2],
				Title:     strings.TrimSpace(matches[3]),
				Completed: completed,
				DependsOn: []string{},
				Subtasks:  []*Task{},
				Optional:  optional,
			}
			if currentTask != nil {
				currentTask.Subtasks = append(currentTask.Subtasks, subtask)
			}
			continue
		}

		// Try to match depends_on for current task
		if matches := dependsOnRe.FindStringSubmatch(line); matches != nil {
			if currentTask != nil {
				deps := strings.Split(matches[1], ",")
				for _, dep := range deps {
					dep = strings.TrimSpace(dep)
					if dep != "" {
						currentTask.DependsOn = append(currentTask.DependsOn, dep)
					}
				}
			}
		}
	}

	return tasks
}

// GetExecutableTasks returns tasks that are ready to execute:
// - Not completed
// - All dependencies are completed
func GetExecutableTasks(tasks []*Task) []*Task {
	// Build a map of completed task IDs
	completedIDs := make(map[string]bool)
	for _, t := range tasks {
		if t.Completed {
			completedIDs[t.ID] = true
		}
	}

	var executable []*Task
	for _, t := range tasks {
		if t.Completed {
			continue
		}

		// Check if all dependencies are completed
		allDepsSatisfied := true
		for _, dep := range t.DependsOn {
			if !completedIDs[dep] {
				allDepsSatisfied = false
				break
			}
		}

		if allDepsSatisfied {
			executable = append(executable, t)
		}
	}

	return executable
}

// GetParallelTasks groups tasks into parallel execution groups
// Tasks in the same group can run in parallel (no dependencies between them)
// Returns a slice of task groups, where each group can run after the previous
func GetParallelTasks(tasks []*Task) [][]*Task {
	if len(tasks) == 0 {
		return nil
	}

	// Build dependency graph and track completed tasks
	completedIDs := make(map[string]bool)
	taskMap := make(map[string]*Task)
	for _, t := range tasks {
		taskMap[t.ID] = t
		if t.Completed {
			completedIDs[t.ID] = true
		}
	}

	// Track which tasks have been assigned to groups
	assigned := make(map[string]bool)
	var groups [][]*Task

	// Keep creating groups until all incomplete tasks are assigned
	for {
		var group []*Task
		for _, t := range tasks {
			if t.Completed || assigned[t.ID] {
				continue
			}

			// Check if all dependencies are either completed or already assigned
			canExecute := true
			for _, dep := range t.DependsOn {
				if !completedIDs[dep] && !assigned[dep] {
					canExecute = false
					break
				}
			}

			if canExecute {
				group = append(group, t)
			}
		}

		if len(group) == 0 {
			break
		}

		groups = append(groups, group)

		// Mark tasks in this group as assigned
		for _, t := range group {
			assigned[t.ID] = true
		}
	}

	return groups
}

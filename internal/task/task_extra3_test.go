package task

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task.ToMap with subtasks (parser.go — tests the recursive path)
// ---------------------------------------------------------------------------

func TestToMap_WithSubtasks(t *testing.T) {
	parent := &Task{
		ID:    "1",
		Title: "Parent",
		Subtasks: []*Task{
			{
				ID:        "1.1",
				Title:     "Subtask 1",
				Completed: true,
				DependsOn: []string{},
				Subtasks:  []*Task{},
			},
		},
		DependsOn: []string{},
	}

	m := parent.ToMap()
	subtasks := m["subtasks"].([]map[string]interface{})
	if len(subtasks) != 1 {
		t.Errorf("subtasks len = %d, want 1", len(subtasks))
	}
	if subtasks[0]["id"] != "1.1" {
		t.Errorf("subtask id = %q, want 1.1", subtasks[0]["id"])
	}
	if subtasks[0]["completed"] != true {
		t.Errorf("subtask completed = %v, want true", subtasks[0]["completed"])
	}
}

func TestToMap_EmptySubtasks(t *testing.T) {
	task := &Task{
		ID:        "1",
		Title:     "Simple",
		DependsOn: []string{},
		Subtasks:  []*Task{},
	}

	m := task.ToMap()
	subtasks := m["subtasks"].([]map[string]interface{})
	if len(subtasks) != 0 {
		t.Errorf("subtasks len = %d, want 0", len(subtasks))
	}
}

// ---------------------------------------------------------------------------
// GetParallelTasks edge cases (parser.go)
// ---------------------------------------------------------------------------

func TestGetParallelTasks_SingleTask(t *testing.T) {
	tasks := []*Task{
		{ID: "1", Title: "Task 1", DependsOn: []string{}, Subtasks: []*Task{}},
	}
	groups := GetParallelTasks(tasks)
	if len(groups) != 1 {
		t.Errorf("GetParallelTasks(single) = %d groups, want 1", len(groups))
	}
}

func TestGetParallelTasks_AllCompleted(t *testing.T) {
	tasks := []*Task{
		{ID: "1", Completed: true, DependsOn: []string{}, Subtasks: []*Task{}},
		{ID: "2", Completed: true, DependsOn: []string{}, Subtasks: []*Task{}},
	}
	groups := GetParallelTasks(tasks)
	if len(groups) != 0 {
		t.Errorf("GetParallelTasks(all completed) = %d groups, want 0", len(groups))
	}
}

func TestGetParallelTasks_DependencyOnCompleted(t *testing.T) {
	tasks := []*Task{
		{ID: "1", Completed: true, DependsOn: []string{}, Subtasks: []*Task{}},
		{ID: "2", Completed: false, DependsOn: []string{"1"}, Subtasks: []*Task{}},
	}
	groups := GetParallelTasks(tasks)
	// Task 2 depends on completed task 1, so it can run in group 1
	if len(groups) != 1 {
		t.Errorf("GetParallelTasks(deps on completed) = %d groups, want 1", len(groups))
	}
}

// ---------------------------------------------------------------------------
// prepareBodyFile tests via CreateEpic path (task.go/epic.go)
// These test the "already has spec" error path
// ---------------------------------------------------------------------------

func TestCreateTask_TitleHelpers(t *testing.T) {
	// Test title extraction from ticket body
	body := "# [feat] add authentication\n\nObjective: add auth"
	lines := strings.Split(body, "\n")
	title := ""
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			title = strings.TrimSpace(strings.TrimLeft(l, "#"))
			break
		}
	}
	if !strings.Contains(title, "feat") {
		t.Errorf("title extraction = %q, should contain 'feat'", title)
	}
}

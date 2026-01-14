package task

import (
	"testing"
)

// ============================================================================
// TestParseTask_BasicParsing - Parse task from markdown line
// ============================================================================

func TestParseTask_UncompletedTask(t *testing.T) {
	content := "- [ ] 1. First task"
	tasks := ParseTasks(content)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", tasks[0].ID)
	}
	if tasks[0].Title != "First task" {
		t.Errorf("expected title 'First task', got '%s'", tasks[0].Title)
	}
	if tasks[0].Completed {
		t.Error("expected task to be uncompleted")
	}
}

func TestParseTask_CompletedTask(t *testing.T) {
	content := "- [x] 1. Done task"
	tasks := ParseTasks(content)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", tasks[0].ID)
	}
	if !tasks[0].Completed {
		t.Error("expected task to be completed")
	}
}

func TestParseTask_MultipleTasks(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
- [x] 3. Third task`

	tasks := ParseTasks(content)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", tasks[0].ID)
	}
	if tasks[1].ID != "2" {
		t.Errorf("expected ID '2', got '%s'", tasks[1].ID)
	}
	if tasks[2].ID != "3" {
		t.Errorf("expected ID '3', got '%s'", tasks[2].ID)
	}
	if !tasks[2].Completed {
		t.Error("expected task 3 to be completed")
	}
}

// ============================================================================
// TestParseTask_WithDependencies - Parse tasks with depends_on
// ============================================================================

func TestParseTask_WithSingleDependency(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_`

	tasks := ParseTasks(content)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if len(tasks[1].DependsOn) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(tasks[1].DependsOn))
	}
	if tasks[1].DependsOn[0] != "1" {
		t.Errorf("expected dependency '1', got '%s'", tasks[1].DependsOn[0])
	}
}

func TestParseTask_WithMultipleDependencies(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
- [ ] 3. Third task
  - _depends_on: 1, 2_`

	tasks := ParseTasks(content)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	deps := tasks[2].DependsOn
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}

	// Check both dependencies are present
	hasOne := false
	hasTwo := false
	for _, dep := range deps {
		if dep == "1" {
			hasOne = true
		}
		if dep == "2" {
			hasTwo = true
		}
	}
	if !hasOne {
		t.Error("expected dependency '1' to be present")
	}
	if !hasTwo {
		t.Error("expected dependency '2' to be present")
	}
}

// ============================================================================
// TestParseTask_Subtasks - Parse tasks with subtasks
// ============================================================================

func TestParseTask_Subtasks(t *testing.T) {
	content := `- [ ] 1. Main task
  - [ ] 1.1 Subtask one
  - [ ] 1.2 Subtask two`

	tasks := ParseTasks(content)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 main task, got %d", len(tasks))
	}
	if len(tasks[0].Subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(tasks[0].Subtasks))
	}
	if tasks[0].Subtasks[0].ID != "1.1" {
		t.Errorf("expected subtask ID '1.1', got '%s'", tasks[0].Subtasks[0].ID)
	}
	if tasks[0].Subtasks[1].ID != "1.2" {
		t.Errorf("expected subtask ID '1.2', got '%s'", tasks[0].Subtasks[1].ID)
	}
}

func TestParseTask_OptionalSubtask(t *testing.T) {
	content := `- [ ] 1. Main task
  - [ ]* 1.1 Optional subtask
  - [ ] 1.2 Required subtask`

	tasks := ParseTasks(content)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 main task, got %d", len(tasks))
	}
	if len(tasks[0].Subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(tasks[0].Subtasks))
	}
	// Both subtasks should be parsed
	if tasks[0].Subtasks[0].ID != "1.1" {
		t.Errorf("expected subtask ID '1.1', got '%s'", tasks[0].Subtasks[0].ID)
	}
	if tasks[0].Subtasks[1].ID != "1.2" {
		t.Errorf("expected subtask ID '1.2', got '%s'", tasks[0].Subtasks[1].ID)
	}
}

// ============================================================================
// TestGetExecutableTasks - Filter tasks ready to execute
// ============================================================================

func TestGetExecutableTasks_NoDeps(t *testing.T) {
	content := "- [ ] 1. First task"
	tasks := ParseTasks(content)
	executable := GetExecutableTasks(tasks)

	if len(executable) != 1 {
		t.Fatalf("expected 1 executable task, got %d", len(executable))
	}
	if executable[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", executable[0].ID)
	}
}

func TestGetExecutableTasks_DepsSatisfied(t *testing.T) {
	content := `- [x] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_`

	tasks := ParseTasks(content)
	executable := GetExecutableTasks(tasks)

	// Task 2 should be executable because task 1 is completed
	if len(executable) != 1 {
		t.Fatalf("expected 1 executable task, got %d", len(executable))
	}
	if executable[0].ID != "2" {
		t.Errorf("expected ID '2', got '%s'", executable[0].ID)
	}
}

func TestGetExecutableTasks_DepsNotSatisfied(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_`

	tasks := ParseTasks(content)
	executable := GetExecutableTasks(tasks)

	// Only task 1 should be executable
	if len(executable) != 1 {
		t.Fatalf("expected 1 executable task, got %d", len(executable))
	}
	if executable[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", executable[0].ID)
	}
}

func TestGetExecutableTasks_ExcludesCompleted(t *testing.T) {
	content := `- [x] 1. Completed task
- [ ] 2. Pending task`

	tasks := ParseTasks(content)
	executable := GetExecutableTasks(tasks)

	if len(executable) != 1 {
		t.Fatalf("expected 1 executable task, got %d", len(executable))
	}
	if executable[0].ID != "2" {
		t.Errorf("expected ID '2', got '%s'", executable[0].ID)
	}
}

func TestGetExecutableTasks_AllCompleted(t *testing.T) {
	content := `- [x] 1. Completed task
- [x] 2. Also completed`

	tasks := ParseTasks(content)
	executable := GetExecutableTasks(tasks)

	if len(executable) != 0 {
		t.Errorf("expected 0 executable tasks, got %d", len(executable))
	}
}

// ============================================================================
// TestGetParallelTasks - Group tasks for parallel execution
// ============================================================================

func TestGetParallelTasks_NoDeps(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
- [ ] 3. Third task`

	tasks := ParseTasks(content)
	groups := GetParallelTasks(tasks)

	// All tasks should be in one group (can run in parallel)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("expected 3 tasks in group, got %d", len(groups[0]))
	}
}

func TestGetParallelTasks_WithDeps(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
  - _depends_on: 1_
- [ ] 3. Third task
  - _depends_on: 2_`

	tasks := ParseTasks(content)
	groups := GetParallelTasks(tasks)

	// Should be 3 groups (sequential chain)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0][0].ID != "1" {
		t.Errorf("expected first group to contain task '1', got '%s'", groups[0][0].ID)
	}
	if groups[1][0].ID != "2" {
		t.Errorf("expected second group to contain task '2', got '%s'", groups[1][0].ID)
	}
	if groups[2][0].ID != "3" {
		t.Errorf("expected third group to contain task '3', got '%s'", groups[2][0].ID)
	}
}

func TestGetParallelTasks_Mixed(t *testing.T) {
	content := `- [ ] 1. First task
- [ ] 2. Second task
- [ ] 3. Third task
  - _depends_on: 1, 2_`

	tasks := ParseTasks(content)
	groups := GetParallelTasks(tasks)

	// Group 1: tasks 1 and 2 (parallel)
	// Group 2: task 3 (depends on both)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("expected 2 tasks in first group, got %d", len(groups[0]))
	}
	if len(groups[1]) != 1 {
		t.Errorf("expected 1 task in second group, got %d", len(groups[1]))
	}
}

// ============================================================================
// TestParseTask_MalformedInput - Handle invalid input
// ============================================================================

func TestParseTask_EmptyContent(t *testing.T) {
	content := ""
	tasks := ParseTasks(content)

	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestParseTask_NoTasksContent(t *testing.T) {
	content := `# Header

Some text but no tasks.`

	tasks := ParseTasks(content)

	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestParseTask_MalformedCheckbox(t *testing.T) {
	content := `- [ ] No number here
- [x] Also no number
- [] 1. Missing space in checkbox
- [y] 1. Invalid checkbox character
- [ ] 1. Valid task`

	tasks := ParseTasks(content)

	// Only the last one should be valid
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", tasks[0].ID)
	}
	if tasks[0].Title != "Valid task" {
		t.Errorf("expected title 'Valid task', got '%s'", tasks[0].Title)
	}
}

func TestParseTask_IgnoresNonTaskLines(t *testing.T) {
	content := `# Header

Some description text.

* Bullet point (not a task)
1. Numbered item without checkbox
- Regular dash item

- [ ] 1. Actual task

More text after.`

	tasks := ParseTasks(content)

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "1" {
		t.Errorf("expected ID '1', got '%s'", tasks[0].ID)
	}
	if tasks[0].Title != "Actual task" {
		t.Errorf("expected title 'Actual task', got '%s'", tasks[0].Title)
	}
}

// ============================================================================
// TestTask_ToMap - Test Task.ToMap() method
// ============================================================================

func TestTask_ToMap(t *testing.T) {
	task := &Task{
		ID:        "1",
		Title:     "Test task",
		Completed: true,
		DependsOn: []string{"0"},
		Subtasks:  []*Task{},
	}

	m := task.ToMap()

	if m["id"] != "1" {
		t.Errorf("expected id '1', got '%v'", m["id"])
	}
	if m["title"] != "Test task" {
		t.Errorf("expected title 'Test task', got '%v'", m["title"])
	}
	if m["completed"] != true {
		t.Errorf("expected completed true, got '%v'", m["completed"])
	}
	deps := m["depends_on"].([]string)
	if len(deps) != 1 || deps[0] != "0" {
		t.Errorf("expected depends_on ['0'], got '%v'", m["depends_on"])
	}
}

// ============================================================================
// TestParseTask_SampleFile - Test parsing a sample tasks.md format
// ============================================================================

func TestParseTask_SampleFile(t *testing.T) {
	content := `# Sample Feature - Implementation Plan

Repo: backend
Coordination: sequential
Sync: independent

## Objective

This is a sample tasks.md for testing parse_tasks.py.

---

## Tasks

- [ ] 1. First main task
  - Repo: backend
  - [ ] 1.1 First subtask
    - Implementation details here
    - _Requirements: R1_
  - [ ] 1.2 Second subtask
    - More details
    - _Requirements: R1_

- [ ] 2. Second main task
  - Repo: backend
  - _depends_on: 1_
  - [ ] 2.1 Dependent subtask
    - This depends on task 1
    - _Requirements: R2_
  - [ ]* 2.2 Optional subtask
    - This is optional (note the asterisk)
    - _Requirements: R2_

- [x] 3. Completed task
  - This task is already done
  - [ ] 3.1 Subtask of completed
    - Even subtasks can be marked

- [ ] 4. Task with multiple dependencies
  - _depends_on: 1, 2_
  - [ ] 4.1 Final implementation
    - _Requirements: R3_

- [ ] 5. Checkpoint
  - Ensure tests pass. In autonomous mode, log issues and continue.`

	tasks := ParseTasks(content)

	// Should have 5 main tasks
	if len(tasks) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(tasks))
	}

	// Task 1 should have subtasks
	if len(tasks[0].Subtasks) < 2 {
		t.Errorf("expected at least 2 subtasks for task 1, got %d", len(tasks[0].Subtasks))
	}

	// Task 2 should depend on task 1
	found := false
	for _, dep := range tasks[1].DependsOn {
		if dep == "1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected task 2 to depend on task 1")
	}

	// Task 3 should be completed
	if !tasks[2].Completed {
		t.Error("expected task 3 to be completed")
	}

	// Task 4 should depend on both 1 and 2
	hasOne := false
	hasTwo := false
	for _, dep := range tasks[3].DependsOn {
		if dep == "1" {
			hasOne = true
		}
		if dep == "2" {
			hasTwo = true
		}
	}
	if !hasOne {
		t.Error("expected task 4 to depend on task 1")
	}
	if !hasTwo {
		t.Error("expected task 4 to depend on task 2")
	}
}

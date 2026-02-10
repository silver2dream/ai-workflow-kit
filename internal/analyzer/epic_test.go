package analyzer

import (
	"context"
	"strings"
	"testing"
)

func TestParseEpicBody(t *testing.T) {
	body := `# My Epic

## Tasks

- [ ] #10 Implement login
- [x] #11 Add database schema
- [ ] #12
- [ ] Plain text task without issue ref
- [X] #13 Case insensitive check

## Notes

Some notes here.
`
	tasks := ParseEpicBody(body)

	if len(tasks) != 5 {
		t.Fatalf("ParseEpicBody() returned %d tasks, want 5", len(tasks))
	}

	// Task 0: unchecked with issue ref and text
	if tasks[0].IssueNumber != 10 || tasks[0].Completed || tasks[0].Text != "Implement login" {
		t.Errorf("Task 0 = %+v, want IssueNumber=10, Completed=false, Text='Implement login'", tasks[0])
	}

	// Task 1: checked with issue ref and text
	if tasks[1].IssueNumber != 11 || !tasks[1].Completed || tasks[1].Text != "Add database schema" {
		t.Errorf("Task 1 = %+v, want IssueNumber=11, Completed=true, Text='Add database schema'", tasks[1])
	}

	// Task 2: unchecked with issue ref but no text
	if tasks[2].IssueNumber != 12 || tasks[2].Completed || tasks[2].Text != "" {
		t.Errorf("Task 2 = %+v, want IssueNumber=12, Completed=false, Text=''", tasks[2])
	}

	// Task 3: plain text task (no issue ref)
	if tasks[3].IssueNumber != 0 || tasks[3].Completed || tasks[3].Text != "Plain text task without issue ref" {
		t.Errorf("Task 3 = %+v, want IssueNumber=0, Completed=false, Text='Plain text task without issue ref'", tasks[3])
	}

	// Task 4: uppercase X
	if tasks[4].IssueNumber != 13 || !tasks[4].Completed {
		t.Errorf("Task 4 = %+v, want IssueNumber=13, Completed=true", tasks[4])
	}
}

func TestParseEpicBody_Empty(t *testing.T) {
	tasks := ParseEpicBody("")
	if len(tasks) != 0 {
		t.Errorf("ParseEpicBody('') returned %d tasks, want 0", len(tasks))
	}
}

func TestParseEpicBody_NoTasks(t *testing.T) {
	body := `# My Epic

Just some text, no task list items.

- Regular list item (no checkbox)
`
	tasks := ParseEpicBody(body)
	if len(tasks) != 0 {
		t.Errorf("ParseEpicBody() returned %d tasks, want 0", len(tasks))
	}
}

func TestParseEpicBody_IndentedTasks(t *testing.T) {
	body := `- [ ] #1 Parent task
  - [ ] #2 Child task
    - [x] #3 Grandchild task
`
	tasks := ParseEpicBody(body)
	if len(tasks) != 3 {
		t.Fatalf("ParseEpicBody() returned %d tasks, want 3", len(tasks))
	}
	if tasks[0].IssueNumber != 1 {
		t.Errorf("Task 0 IssueNumber = %d, want 1", tasks[0].IssueNumber)
	}
	if tasks[1].IssueNumber != 2 {
		t.Errorf("Task 1 IssueNumber = %d, want 2", tasks[1].IssueNumber)
	}
	if tasks[2].IssueNumber != 3 || !tasks[2].Completed {
		t.Errorf("Task 2 = %+v, want IssueNumber=3, Completed=true", tasks[2])
	}
}

func TestAppendTaskToEpicBody(t *testing.T) {
	body := `# Epic

## Tasks

- [ ] #10 First task
- [x] #11 Second task

## Notes

End of document.
`
	result := AppendTaskToEpicBody(body, 12, "Third task")

	if !strings.Contains(result, "- [ ] #12 Third task") {
		t.Errorf("Result should contain new task line, got:\n%s", result)
	}

	// Verify it's inserted after the last task (line with #11), not at the very end
	lines := strings.Split(result, "\n")
	foundNew := false
	for i, line := range lines {
		if strings.Contains(line, "#12 Third task") {
			foundNew = true
			// Should be right after the #11 line
			if i > 0 && !strings.Contains(lines[i-1], "#11") {
				t.Errorf("New task not inserted after last task. Line %d: %q, prev: %q", i, line, lines[i-1])
			}
			break
		}
	}
	if !foundNew {
		t.Error("New task line not found in result")
	}
}

func TestAppendTaskToEpicBody_NoExistingTasks(t *testing.T) {
	body := `# Epic

Just some text.`

	result := AppendTaskToEpicBody(body, 42, "New task")

	if !strings.Contains(result, "- [ ] #42 New task") {
		t.Errorf("Result should contain new task, got:\n%s", result)
	}
	// Should be appended at end
	if !strings.HasSuffix(strings.TrimRight(result, "\n"), "- [ ] #42 New task") {
		t.Errorf("New task should be at end of body, got:\n%s", result)
	}
}

func TestAppendTaskToEpicBody_EmptyBody(t *testing.T) {
	result := AppendTaskToEpicBody("", 1, "First task")
	if !strings.Contains(result, "- [ ] #1 First task") {
		t.Errorf("Result should contain new task, got: %q", result)
	}
}

func TestAppendTaskToEpicBody_NoDescription(t *testing.T) {
	result := AppendTaskToEpicBody("", 42, "")
	if !strings.Contains(result, "- [ ] #42\n") {
		t.Errorf("Result should contain task without description, got: %q", result)
	}
}

func TestCountEpicProgress(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantTotal     int
		wantCompleted int
	}{
		{
			"all complete",
			"- [x] #1 Done\n- [x] #2 Also done\n",
			2, 2,
		},
		{
			"none complete",
			"- [ ] #1 Todo\n- [ ] #2 Also todo\n",
			2, 0,
		},
		{
			"mixed",
			"- [x] #1 Done\n- [ ] #2 Todo\n- [x] #3 Done\n",
			3, 2,
		},
		{
			"empty",
			"",
			0, 0,
		},
		{
			"no tasks",
			"# Just a header\nSome text\n",
			0, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, completed := CountEpicProgress(tt.body)
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if completed != tt.wantCompleted {
				t.Errorf("completed = %d, want %d", completed, tt.wantCompleted)
			}
		})
	}
}

func TestParseEpicBody_RawLinePreserved(t *testing.T) {
	line := "- [ ] #42 My task description"
	body := "# Header\n" + line + "\n"

	tasks := ParseEpicBody(body)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].RawLine != line {
		t.Errorf("RawLine = %q, want %q", tasks[0].RawLine, line)
	}
}

// --- checkEpicProgress tests ---

func TestCheckEpicProgress_AllComplete(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	mockClient.IssueBodies[100] = "- [x] #10 Done\n- [x] #11 Also done\n"

	decision := a.checkEpicProgress(context.Background())
	if decision != nil {
		t.Errorf("expected nil (all complete), got %+v", decision)
	}
}

func TestCheckEpicProgress_HasUnlinkedTask(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	mockClient.IssueBodies[100] = "- [x] #10 Done task\n- [ ] New unlinked task\n"

	decision := a.checkEpicProgress(context.Background())
	if decision == nil {
		t.Fatal("expected a create_task decision, got nil")
	}
	if decision.NextAction != ActionCreateTask {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.SpecName != "my-project" {
		t.Errorf("SpecName = %q, want %q", decision.SpecName, "my-project")
	}
	if decision.EpicIssue != 100 {
		t.Errorf("EpicIssue = %d, want 100", decision.EpicIssue)
	}
	if decision.TaskText != "New unlinked task" {
		t.Errorf("TaskText = %q, want %q", decision.TaskText, "New unlinked task")
	}
}

func TestCheckEpicProgress_InProgressTaskSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	// Task has issue ref but is not checked — means it's in-progress
	mockClient.IssueBodies[100] = "- [ ] #10 In progress task\n"

	decision := a.checkEpicProgress(context.Background())
	if decision != nil {
		t.Errorf("expected nil (task has issue ref, in-progress), got %+v", decision)
	}
}

func TestCheckEpicProgress_NoEpicConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{}},
		},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	decision := a.checkEpicProgress(context.Background())
	if decision != nil {
		t.Errorf("expected nil when no epic configured, got %+v", decision)
	}
}

func TestCheckEpicProgress_EmptyEpicBody(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)
	mockClient.IssueBodies[100] = ""

	decision := a.checkEpicProgress(context.Background())
	if decision != nil {
		t.Errorf("expected nil for empty epic body, got %+v", decision)
	}
}

func TestDecide_EpicMode_AllComplete(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
		GitHub: GitHubConfig{Labels: DefaultLabels()},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	mockClient.IssueBodies[100] = "- [x] #10 Done\n- [x] #11 Also done\n"
	mockClient.OpenIssueCount = 0

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.NextAction != ActionAllComplete {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionAllComplete)
	}
}

func TestDecide_EpicMode_CreateTask(t *testing.T) {
	tmpDir := t.TempDir()
	mockClient := NewMockGitHubClient()
	config := &Config{
		Specs: SpecsConfig{
			Active:   []string{"my-project"},
			Tracking: TrackingConfig{Mode: TrackingModeGitHubEpic, EpicIssues: map[string]int{"my-project": 100}},
		},
		GitHub: GitHubConfig{Labels: DefaultLabels()},
	}
	a := newTestAnalyzer(tmpDir, config, mockClient)

	mockClient.IssueBodies[100] = "- [x] #10 Done\n- [ ] Implement new feature\n"
	mockClient.OpenIssueCount = 0

	decision, err := a.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decision.NextAction != ActionCreateTask {
		t.Errorf("NextAction = %q, want %q", decision.NextAction, ActionCreateTask)
	}
	if decision.TaskText != "Implement new feature" {
		t.Errorf("TaskText = %q, want %q", decision.TaskText, "Implement new feature")
	}
	if decision.EpicIssue != 100 {
		t.Errorf("EpicIssue = %d, want 100", decision.EpicIssue)
	}
}

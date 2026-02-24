package epicaudit

import (
	"testing"
)

func TestExtractDesignSections(t *testing.T) {
	content := `# Main Title

## Overview
Some text

### Architecture
More text

## Requirements
- R1: WebSocket server
- R2: Game logic

### Sub-requirement
Details

## Testing
Test plan
`

	sections := extractDesignSections(content)
	expected := []string{"Overview", "Architecture", "Requirements", "Sub-requirement", "Testing"}

	if len(sections) != len(expected) {
		t.Fatalf("expected %d sections, got %d: %v", len(expected), len(sections), sections)
	}
	for i, s := range sections {
		if s != expected[i] {
			t.Errorf("section[%d]: expected %q, got %q", i, expected[i], s)
		}
	}
}

func TestExtractDesignSections_Empty(t *testing.T) {
	sections := extractDesignSections("")
	if len(sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(sections))
	}
}

func TestExtractRequirementLabels(t *testing.T) {
	content := `
## Requirements
- R1: WebSocket server
- R2: Game loop
- R3: Matchmaking
- FR-1: Frontend rendering
- REQ-01: Authentication
- R1 is also mentioned here again
`

	labels := extractRequirementLabels(content)
	expected := map[string]bool{
		"R1": true, "R2": true, "R3": true,
		"FR-1": true, "REQ-01": true,
	}

	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %v", len(expected), len(labels), labels)
	}
	for _, l := range labels {
		if !expected[l] {
			t.Errorf("unexpected label: %q", l)
		}
	}
}

func TestExtractRequirementLabels_None(t *testing.T) {
	labels := extractRequirementLabels("No requirements here, just prose.")
	if len(labels) != 0 {
		t.Errorf("expected 0 labels, got %d: %v", len(labels), labels)
	}
}

func TestExtractRepoMentions(t *testing.T) {
	text := "Implement WebSocket in backend, render UI in frontend"
	repos := extractRepoMentions(text, []string{"backend", "frontend"})

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(repos), repos)
	}
}

func TestExtractRepoMentions_CaseInsensitive(t *testing.T) {
	text := "BACKEND should handle this"
	repos := extractRepoMentions(text, []string{"backend"})

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
}

func TestExtractRepoMentions_None(t *testing.T) {
	text := "Some generic text without repo names"
	repos := extractRepoMentions(text, []string{"backend", "frontend"})

	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestGenerateGapHints_RepoUncovered(t *testing.T) {
	report := &AuditReport{
		ReposInDesign: []string{"backend", "frontend"},
		ReposInTasks:  []string{"backend"},
		Tasks:         []TaskStatus{{Text: "Implement backend API"}},
		TotalTasks:    1,
	}

	hints := generateGapHints(report)
	found := false
	for _, h := range hints {
		if h == "REPO_UNCOVERED:frontend" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected REPO_UNCOVERED:frontend hint, got %v", hints)
	}
}

func TestGenerateGapHints_ReqUncovered(t *testing.T) {
	report := &AuditReport{
		DesignRequirements: []string{"R1", "R2", "R3"},
		Tasks: []TaskStatus{
			{Text: "Implement R1 WebSocket"},
			{Text: "Implement R3 matchmaking"},
		},
		TotalTasks: 2,
	}

	hints := generateGapHints(report)
	found := false
	for _, h := range hints {
		if h == "REQ_UNCOVERED:R2" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected REQ_UNCOVERED:R2 hint, got %v", hints)
	}
}

func TestGenerateGapHints_LowTaskCount(t *testing.T) {
	report := &AuditReport{
		DesignSections: []string{"Overview", "Architecture", "Requirements", "Testing"},
		Tasks:          []TaskStatus{{Text: "Do everything"}},
		TotalTasks:     1,
	}

	hints := generateGapHints(report)
	found := false
	for _, h := range hints {
		if h == "LOW_TASK_COUNT" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected LOW_TASK_COUNT hint, got %v", hints)
	}
}

func TestGenerateGapHints_MissingIntegration(t *testing.T) {
	report := &AuditReport{
		ReposInDesign: []string{"backend", "frontend"},
		ReposInTasks:  []string{"backend", "frontend"},
		Tasks: []TaskStatus{
			{Text: "Implement backend API"},
			{Text: "Implement frontend UI"},
		},
		TotalTasks: 2,
	}

	hints := generateGapHints(report)
	found := false
	for _, h := range hints {
		if h == "MISSING_INTEGRATION_TASK" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected MISSING_INTEGRATION_TASK hint, got %v", hints)
	}
}

func TestGenerateGapHints_IntegrationPresent(t *testing.T) {
	report := &AuditReport{
		ReposInDesign: []string{"backend", "frontend"},
		ReposInTasks:  []string{"backend", "frontend"},
		Tasks: []TaskStatus{
			{Text: "Implement backend API"},
			{Text: "Implement frontend UI"},
			{Text: "Integrate frontend with backend WebSocket"},
		},
		TotalTasks: 3,
	}

	hints := generateGapHints(report)
	for _, h := range hints {
		if h == "MISSING_INTEGRATION_TASK" {
			t.Errorf("should NOT have MISSING_INTEGRATION_TASK when integration task exists, got %v", hints)
		}
	}
}

func TestGenerateGapHints_AllGood(t *testing.T) {
	report := &AuditReport{
		DesignSections:     []string{"Overview", "Architecture"},
		DesignRequirements: []string{"R1"},
		ReposInDesign:      []string{"backend"},
		ReposInTasks:       []string{"backend"},
		Tasks: []TaskStatus{
			{Text: "Implement R1 backend logic"},
			{Text: "Write tests for backend"},
		},
		TotalTasks: 2,
	}

	hints := generateGapHints(report)
	if len(hints) != 0 {
		t.Errorf("expected 0 hints for well-covered epic, got %v", hints)
	}
}

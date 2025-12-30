package worker

import (
	"strings"
	"testing"
)

func TestBuildCommitMessage(t *testing.T) {
	tests := []struct {
		title  string
		expect string
	}{
		{"[feat] Add API", "[feat] add api"},
		{"[fix]   Bug #123!", "[fix] bug 123"},
		{"Improve logging", "[chore] improve logging"},
		{"", "[chore] issue"},
	}

	for _, tt := range tests {
		if got := BuildCommitMessage(tt.title); got != tt.expect {
			t.Errorf("BuildCommitMessage(%q) = %q, want %q", tt.title, got, tt.expect)
		}
	}
}

func TestExtractTitleLine(t *testing.T) {
	body := "Intro\n# Title Line\r\nMore text\n# Another"
	if got := extractTitleLine(body); got != "Title Line" {
		t.Errorf("extractTitleLine() = %q, want %q", got, "Title Line")
	}
}

func TestBuildWorkDirInstruction(t *testing.T) {
	instruction := buildWorkDirInstruction("directory", "backend/", "/tmp/repo/backend", "backend")
	if instruction == "" {
		t.Fatal("expected directory instruction")
	}
	if !strings.Contains(instruction, "MONOREPO") {
		t.Errorf("instruction missing monorepo hint: %s", instruction)
	}

	instruction = buildWorkDirInstruction("submodule", "engine/", "/tmp/repo/engine", "engine")
	if instruction == "" {
		t.Fatal("expected submodule instruction")
	}
	if !strings.Contains(instruction, "SUBMODULE") {
		t.Errorf("instruction missing submodule hint: %s", instruction)
	}
}

func TestFindProtectedChanges(t *testing.T) {
	files := []string{
		"README.md",
		".ai/scripts/cleanup.sh",
		".ai/commands/run.md",
	}
	violations := findProtectedChanges(files, "")
	if len(violations) != 2 {
		t.Errorf("expected 2 violations, got %d", len(violations))
	}

	violations = findProtectedChanges(files, ".ai/scripts/cleanup.sh")
	if len(violations) != 1 {
		t.Errorf("expected 1 violation with whitelist, got %d", len(violations))
	}
}

func TestFindSensitiveMatches(t *testing.T) {
	diff := "+ password = \"secret\"\n+ API_KEY = \"value\"\n"
	matches := findSensitiveMatches(diff, []string{})
	if len(matches) == 0 {
		t.Fatal("expected sensitive pattern matches")
	}

	custom := findSensitiveMatches("token=abc", []string{`token=\w+`})
	if len(custom) == 0 {
		t.Fatal("expected custom pattern match")
	}
}

func TestExtractTicketValue(t *testing.T) {
	body := "- allow_script_changes: true\n**Release**: false\n"
	if got := extractTicketValue(body, "allow_script_changes"); got != "true" {
		t.Errorf("extractTicketValue allow_script_changes = %q", got)
	}
	if got := extractTicketValue(body, "Release"); got != "false" {
		t.Errorf("extractTicketValue Release = %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(45); got != "45s" {
		t.Errorf("formatDuration(45) = %q", got)
	}
	if got := formatDuration(90); got != "1m 30s" {
		t.Errorf("formatDuration(90) = %q", got)
	}
	if got := formatDuration(3660); got != "1h 1m" {
		t.Errorf("formatDuration(3660) = %q", got)
	}
}

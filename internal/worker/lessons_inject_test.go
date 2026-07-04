package worker

import (
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

func seedLessonStore(t *testing.T, root string) {
	t.Helper()
	s := &lessons.Store{Version: 1}
	s.Lessons = append(s.Lessons, lessons.Lesson{
		ID: "L-001", Title: "sync schema when config struct changes",
		Content: "- check workflow.schema.json on config edits",
		Kind:    lessons.KindPitfall, Categories: []string{"config"},
		Scope:       []string{"internal/analyzer/"},
		Fingerprint: "abcd1234", Status: lessons.StatusActive,
		CreatedAt: "2026-06-01T00:00:00Z",
	})
	if err := lessons.Save(root, s); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLessonsSection_InjectsAndRecordsAttribution(t *testing.T) {
	root := t.TempDir()
	seedLessonStore(t, root)

	ticket := "# [fix] config drift\n\n## Scope\n- `internal/analyzer/config.go` — add lessons config\n"
	section := loadLessonsSection(root, 42, ticket, nil)

	if !strings.Contains(section, "[L-001]") {
		t.Fatalf("expected lesson injected, got:\n%s", section)
	}
	rec := lessons.LoadInjection(root, 42)
	if rec == nil || len(rec.LessonIDs) != 1 || rec.LessonIDs[0] != "L-001" {
		t.Errorf("attribution record missing or wrong: %+v", rec)
	}
}

func TestLoadLessonsSection_IrrelevantTicketInjectsNothing(t *testing.T) {
	root := t.TempDir()
	seedLessonStore(t, root)

	ticket := "# [feat] frontend button\n\n## Scope\n- `frontend/src/ui.tsx` — styling only\n"
	if section := loadLessonsSection(root, 7, ticket, nil); section != "" {
		t.Errorf("expected no injection for irrelevant ticket, got:\n%s", section)
	}
	if rec := lessons.LoadInjection(root, 7); rec != nil {
		t.Errorf("no attribution should be recorded when nothing injected: %+v", rec)
	}
}

func TestLoadLessonsSection_DisabledByConfig(t *testing.T) {
	root := t.TempDir()
	seedLessonStore(t, root)
	off := false
	cfg := &workflowLessons{Enabled: &off}

	ticket := "## Scope\n- `internal/analyzer/config.go`"
	if section := loadLessonsSection(root, 7, ticket, cfg); section != "" {
		t.Errorf("expected empty section when disabled, got:\n%s", section)
	}
}

func TestLoadLessonsSection_ClearsStaleAttribution(t *testing.T) {
	root := t.TempDir()
	seedLessonStore(t, root)

	// First attempt injects and records.
	relevant := "## Scope\n- `internal/analyzer/config.go`"
	if s := loadLessonsSection(root, 9, relevant, nil); s == "" {
		t.Fatal("expected injection on first attempt")
	}
	// Retry with an unrelated ticket must clear the stale record.
	if s := loadLessonsSection(root, 9, "## Scope\n- `frontend/src/ui.tsx`", nil); s != "" {
		t.Fatal("expected no injection on retry")
	}
	if rec := lessons.LoadInjection(root, 9); rec != nil {
		t.Errorf("stale attribution should be cleared: %+v", rec)
	}
}

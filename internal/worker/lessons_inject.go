package worker

import (
	"fmt"
	"os"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// workflowLessons mirrors the lessons section of workflow.yaml for the
// worker's standalone config parsing.
type workflowLessons struct {
	Enabled        *bool `yaml:"enabled"`
	InjectTopK     int   `yaml:"inject_top_k"`
	InjectMaxChars int   `yaml:"inject_max_chars"`
}

func (l *workflowLessons) isEnabled() bool {
	if l == nil || l.Enabled == nil {
		return true
	}
	return *l.Enabled
}

// loadLessonsSection selects ticket-relevant lessons, records the injection
// for later hit/miss settlement, and returns the prompt section body.
// Best-effort: any failure returns "" and never blocks the dispatch.
func loadLessonsSection(stateRoot string, issueID int, ticket string, cfg *workflowLessons) string {
	if !cfg.isEnabled() {
		return ""
	}
	store, err := lessons.Load(stateRoot)
	if err != nil || len(store.Lessons) == 0 {
		return ""
	}

	// Reuse the knowledge-graph ticket tokenizer for scope signals; the
	// ticket text itself provides category signals.
	q := lessons.Query{
		PathTokens: ticketPathTokens(ticket),
		Categories: lessons.KeywordCategories(ticket),
	}

	topK, maxChars := 0, 0
	if cfg != nil {
		topK = cfg.InjectTopK
		maxChars = cfg.InjectMaxChars
	}
	selected := lessons.Select(store, q, topK, maxChars, time.Now().UTC())
	if len(selected) == 0 {
		// Clear any stale attribution from a previous attempt.
		_ = lessons.RecordInjection(stateRoot, issueID, nil)
		return ""
	}

	if err := lessons.RecordInjection(stateRoot, issueID, lessons.IDs(selected)); err != nil {
		fmt.Fprintf(os.Stderr, "[WORKER] warning: failed to record lesson injection: %v\n", err)
	}
	return lessons.FormatForPrompt(selected)
}

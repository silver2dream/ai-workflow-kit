package worker

import (
	"fmt"
	"os"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// loadLessonsSection selects ticket-relevant lessons, records the injection
// for later hit/miss settlement, and returns the prompt section body.
// Best-effort: any failure returns "" and never blocks the dispatch.
func loadLessonsSection(stateRoot string, issueID int, ticket string, cfg *workflowLessons) string {
	// No configuration at all means the default: the loop is on.
	if cfg != nil && !cfg.IsEnabled() {
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

package lessons

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InjectionRecord captures which lessons were injected into a worker run —
// the attribution basis for hit/miss settlement.
type InjectionRecord struct {
	IssueID    int      `json:"issue_id"`
	LessonIDs  []string `json:"lesson_ids"`
	InjectedAt string   `json:"injected_at"`
}

func injectionPath(stateRoot string, issueID int) string {
	return filepath.Join(stateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", issueID), "injected_lessons.json")
}

// RecordInjection persists the injected lesson IDs for an issue run.
// Overwrites any previous record for the issue (last dispatch wins — that is
// the run the upcoming review outcome belongs to).
func RecordInjection(stateRoot string, issueID int, lessonIDs []string) error {
	if len(lessonIDs) == 0 {
		// No lessons injected: remove any stale record so settlement does
		// not credit lessons from an earlier attempt.
		_ = os.Remove(injectionPath(stateRoot, issueID))
		return nil
	}
	rec := InjectionRecord{
		IssueID:    issueID,
		LessonIDs:  lessonIDs,
		InjectedAt: time.Now().UTC().Format(time.RFC3339),
	}
	path := injectionPath(stateRoot, issueID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadInjection reads the injection record for an issue; nil when absent.
func LoadInjection(stateRoot string, issueID int) *InjectionRecord {
	data, err := os.ReadFile(injectionPath(stateRoot, issueID))
	if err != nil {
		return nil
	}
	var rec InjectionRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil
	}
	return &rec
}

// Outcome values for settlement.
const (
	OutcomeMerged           = "merged"
	OutcomeChangesRequested = "changes_requested"
	OutcomeReviewBlocked    = "review_blocked"
)

// State-machine thresholds (design doc §2 step 4).
const (
	promoteToActiveHits  = 2
	promoteToProvenHits  = 5
	retireMisses         = 3
	provenMissRateLimit  = 0.2
)

// Settle closes the loop for one review outcome: every lesson that was
// injected into this issue's run gets a hit (outcome clean or unrelated) or
// a miss (the same failure pattern recurred despite the lesson), then the
// lesson's status advances through the candidate -> active -> proven state
// machine. Counts drive status only — promotion to hard gates stays a human
// decision (awkit lessons promote).
//
// rejectionText is the review body / reason for non-merged outcomes; it is
// matched against each injected lesson's categories to decide hit vs miss.
func Settle(stateRoot string, issueID int, outcome, rejectionText string) error {
	rec := LoadInjection(stateRoot, issueID)
	if rec == nil || len(rec.LessonIDs) == 0 {
		return nil // nothing was injected; nothing to settle
	}

	s, err := Load(stateRoot)
	if err != nil {
		return err
	}

	rejCats := map[string]bool{}
	if outcome != OutcomeMerged {
		for _, c := range KeywordCategories(rejectionText) {
			rejCats[c] = true
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	changed := false
	for _, id := range rec.LessonIDs {
		l := s.FindByID(id)
		if l == nil {
			continue
		}
		changed = true

		miss := false
		if outcome != OutcomeMerged {
			// Miss only when the recurrence matches what this lesson is
			// about; an unrelated rejection neither confirms nor refutes it.
			for _, c := range l.Categories {
				if rejCats[c] {
					miss = true
					break
				}
			}
		}

		switch {
		case miss:
			l.Misses++
		case outcome == OutcomeMerged:
			l.Hits++
			l.LastHitAt = now
		default:
			// Rejected for an unrelated reason: no signal either way.
			continue
		}
		advanceStatus(l)
	}

	if !changed {
		return nil
	}
	// Settlement consumed this record; remove it so a later re-dispatch
	// starts a fresh attribution window.
	_ = os.Remove(injectionPath(stateRoot, issueID))
	return Save(stateRoot, s)
}

// advanceStatus applies the lesson state machine after a count change.
func advanceStatus(l *Lesson) {
	if l.Misses >= retireMisses {
		l.Status = StatusRetired
		return
	}
	switch l.Status {
	case StatusCandidate:
		if l.Hits >= promoteToActiveHits && l.Misses == 0 {
			l.Status = StatusActive
		}
	case StatusActive:
		total := l.Hits + l.Misses
		if l.Hits >= promoteToProvenHits && total > 0 &&
			float64(l.Misses)/float64(total) < provenMissRateLimit {
			l.Status = StatusProven
		}
	}
}

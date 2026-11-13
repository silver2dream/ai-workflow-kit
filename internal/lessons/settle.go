package lessons

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InjectionRecord captures which lessons were injected into a worker run —
// the attribution basis for hit/miss settlement.
type InjectionRecord struct {
	IssueID    int      `json:"issue_id"`
	LessonIDs  []string `json:"lesson_ids"`
	InjectedAt string   `json:"injected_at"`
	// Missed lists the lessons whose failure pattern recurred in a rejection
	// of this issue. It outlives re-dispatches: a lesson is charged at most
	// one miss per issue, and the eventual merge is not a hit for it — the
	// retry was steered by the review feedback, not by the lesson.
	Missed []string `json:"missed,omitempty"`
}

func injectionPath(stateRoot string, issueID int) string {
	return filepath.Join(stateRoot, ".ai", "runs", fmt.Sprintf("issue-%d", issueID), "injected_lessons.json")
}

// RecordInjection persists the injected lesson IDs for an issue run.
// It replaces the lesson list of any previous record for the issue (the last
// dispatch is the run the upcoming review outcome belongs to) and keeps the
// record's Missed list, which belongs to the issue rather than to one run.
func RecordInjection(stateRoot string, issueID int, lessonIDs []string) error {
	var missed []string
	if prev := LoadInjection(stateRoot, issueID); prev != nil {
		missed = prev.Missed
	}
	if len(lessonIDs) == 0 && len(missed) == 0 {
		// Nothing injected and nothing to remember: remove any stale record
		// so settlement does not credit lessons from an earlier attempt.
		_ = os.Remove(injectionPath(stateRoot, issueID))
		return nil
	}
	return writeInjection(stateRoot, InjectionRecord{
		IssueID:    issueID,
		LessonIDs:  lessonIDs,
		InjectedAt: time.Now().UTC().Format(time.RFC3339),
		Missed:     missed,
	})
}

func writeInjection(stateRoot string, rec InjectionRecord) error {
	path := injectionPath(stateRoot, rec.IssueID)
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
	promoteToActiveHits = 2
	promoteToProvenHits = 5
	retireMisses        = 3
	provenMissRateLimit = 0.2
)

// SettleInput describes one terminal review outcome for an issue.
type SettleInput struct {
	IssueID int
	Outcome string // OutcomeMerged, OutcomeChangesRequested or OutcomeReviewBlocked
	// RejectionText is what the reviewer objected to (non-merged outcomes).
	// Pass the findings, not the whole review: a review's boilerplate
	// headings mention tests and errors whether or not they were the problem.
	RejectionText string
	// ChangedPaths are the repository-relative paths the pull request changed.
	ChangedPaths []string
	// PathsKnown is false when the pull request's file list could not be
	// read. Scoped lessons then get no count: their applicability is unknown.
	PathsKnown bool
}

// SettleReport lists, by lesson ID, what one settlement decided.
type SettleReport struct {
	Hits     []string
	Misses   []string
	NoSignal []string // injected, but this outcome says nothing about the lesson
}

// Settle closes the loop for one review outcome. A lesson that was injected
// into the issue's run is counted only when the outcome is evidence about it:
//
//   - it must apply to the pull request: it has no scope, or the pull request
//     changed a file under its scope;
//   - a miss is a rejection whose findings fall in the lesson's categories,
//     charged at most once per issue;
//   - a hit is a merge of an issue on which the lesson never missed, and
//     which is not the lesson's own source (its evidence).
//
// Anything else is no signal. Counts drive the candidate -> active -> proven
// state machine only — promotion to hard gates stays a human decision
// (awkit lessons promote).
//
// The injection record is kept until the merge: a rejection that is followed
// by a re-review instead of a re-dispatch still has its attribution.
func Settle(stateRoot string, in SettleInput) (*SettleReport, error) {
	report := &SettleReport{}
	rec := LoadInjection(stateRoot, in.IssueID)
	if rec == nil {
		return report, nil // nothing was injected; nothing to settle
	}
	merged := in.Outcome == OutcomeMerged

	if len(rec.LessonIDs) > 0 {
		s, err := Load(stateRoot)
		if err != nil {
			return report, err
		}

		rejCats := map[string]bool{}
		if !merged {
			for _, c := range KeywordCategories(in.RejectionText) {
				rejCats[c] = true
			}
		}
		missed := make(map[string]bool, len(rec.Missed))
		for _, id := range rec.Missed {
			missed[id] = true
		}

		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range rec.LessonIDs {
			l := s.FindByID(id)
			if l == nil {
				continue
			}
			switch {
			case missed[id], !lessonApplies(*l, in.ChangedPaths, in.PathsKnown):
				report.NoSignal = append(report.NoSignal, id)
			case merged:
				if hasEvidenceFor(*l, in.IssueID) {
					report.NoSignal = append(report.NoSignal, id)
					continue
				}
				l.Hits++
				l.LastHitAt = now
				advanceStatus(l)
				report.Hits = append(report.Hits, id)
			case inCategories(l.Categories, rejCats):
				l.Misses++
				advanceStatus(l)
				missed[id] = true
				rec.Missed = append(rec.Missed, id)
				report.Misses = append(report.Misses, id)
			default:
				// Rejected for an unrelated reason: no signal either way.
				report.NoSignal = append(report.NoSignal, id)
			}
		}

		if len(report.Hits)+len(report.Misses) > 0 {
			if err := Save(stateRoot, s); err != nil {
				return report, err
			}
		}
	}

	if merged {
		// The merge ends the issue's attribution window.
		_ = os.Remove(injectionPath(stateRoot, in.IssueID))
		return report, nil
	}
	if len(report.Misses) > 0 {
		if err := writeInjection(stateRoot, *rec); err != nil {
			return report, err
		}
	}
	return report, nil
}

// lessonApplies reports whether a lesson had a chance to matter for a pull
// request. A lesson without scope applies repository-wide. A scoped lesson
// applies when the pull request changed a path under one of its prefixes;
// when the file list is unknown, that cannot be told and the answer is no.
func lessonApplies(l Lesson, changedPaths []string, pathsKnown bool) bool {
	if len(l.Scope) == 0 {
		return true
	}
	if !pathsKnown {
		return false
	}
	for _, sc := range l.Scope {
		prefix := scopeKey(sc)
		if prefix == "" {
			return true // the repository root
		}
		for _, p := range changedPaths {
			path := scopeKey(p)
			if path == "" {
				continue
			}
			// The second test covers a changed entry that is a directory
			// (a submodule pointer): something under the scope may have
			// changed, and the file list cannot say more.
			if strings.HasPrefix(path, prefix) || strings.HasPrefix(prefix, path+"/") {
				return true
			}
		}
	}
	return false
}

// scopeKey normalizes a path or scope prefix for comparison.
func scopeKey(p string) string {
	p = strings.ToLower(normPath(strings.TrimSpace(p)))
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	if p == "." || p == "/" {
		return ""
	}
	return p
}

// hasEvidenceFor reports whether the lesson was derived from this issue.
// The lesson's own source merging later is not independent confirmation.
func hasEvidenceFor(l Lesson, issueID int) bool {
	for _, ev := range l.Evidence {
		if ev.Issue == issueID {
			return true
		}
	}
	return false
}

func inCategories(categories []string, set map[string]bool) bool {
	for _, c := range categories {
		if set[c] {
			return true
		}
	}
	return false
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

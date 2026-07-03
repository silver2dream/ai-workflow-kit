// Package lessons implements AWK's learning loop: review failures are
// recorded (by the reviewer), distilled into compact lessons, injected into
// future Worker/Reviewer prompts, and verified via hit/miss settlement so
// ineffective lessons are retired and effective ones can be promoted into
// hard gates.
//
// Design: .ai/specs/learning-loop/design.md
// The store is a committable JSON artifact (like the Understand-Anything
// knowledge graph): team-shareable, diffable, and survives state resets.
package lessons

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Lesson lifecycle states.
const (
	StatusCandidate = "candidate" // newly distilled, not yet validated
	StatusActive    = "active"    // validated by at least one clean outcome
	StatusProven    = "proven"    // repeatedly effective; promotion candidate
	StatusRetired   = "retired"   // evicted, decayed, or repeatedly missed
)

// Lesson kinds.
const (
	KindPitfall  = "pitfall"  // distilled from a failure
	KindStrategy = "strategy" // distilled from a success (Phase C)
)

// storeRelPath is the lessons store location relative to the state root.
const storeRelPath = ".ai/state/lessons.json"

// Evidence links a lesson to the concrete review outcome it came from.
// Evidence is ALWAYS written by Go from feedback records — the distiller
// LLM has no authority to fabricate it.
type Evidence struct {
	Issue int    `json:"issue"`
	PR    int    `json:"pr,omitempty"`
	Type  string `json:"type,omitempty"` // changes_requested | review_blocked | ...
}

// Lesson is one distilled, actionable lesson. Content follows the
// ReasoningBank three-part shape: title, one-line description, and
// actionable checks/constraints.
type Lesson struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Content     string     `json:"content"`
	Kind        string     `json:"kind"`
	Categories  []string   `json:"categories,omitempty"`
	Scope       []string   `json:"scope,omitempty"` // path prefixes this lesson anchors to
	Fingerprint string     `json:"fingerprint"`
	Status      string     `json:"status"`
	Hits        int        `json:"hits"`
	Misses      int        `json:"misses"`
	Evidence    []Evidence `json:"evidence,omitempty"`
	CreatedAt   string     `json:"created_at"`
	LastHitAt   string     `json:"last_hit_at,omitempty"`
	Source      string     `json:"source,omitempty"` // distiller | human | post-mortem
}

// Watermark tracks how much of the feedback log has been distilled.
type Watermark struct {
	FeedbackLine int    `json:"feedback_line"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// Store is the on-disk lessons artifact.
type Store struct {
	Version   int       `json:"version"`
	Watermark Watermark `json:"watermark"`
	Lessons   []Lesson  `json:"lessons"`
}

// StorePath returns the lessons store path under a state root.
func StorePath(stateRoot string) string {
	return filepath.Join(stateRoot, storeRelPath)
}

// Load reads the store; a missing file yields an empty version-1 store.
func Load(stateRoot string) (*Store, error) {
	data, err := os.ReadFile(StorePath(stateRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{Version: 1}, nil
		}
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("lessons store is corrupt: %w", err)
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return &s, nil
}

// Save writes the store atomically (tmp + rename).
func Save(stateRoot string, s *Store) error {
	path := StorePath(stateRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// FindByID returns the lesson with the given ID, or nil.
func (s *Store) FindByID(id string) *Lesson {
	for i := range s.Lessons {
		if s.Lessons[i].ID == id {
			return &s.Lessons[i]
		}
	}
	return nil
}

// NextID returns the next available lesson ID (L-001, L-002, ...).
func (s *Store) NextID() string {
	max := 0
	for _, l := range s.Lessons {
		var n int
		if _, err := fmt.Sscanf(l.ID, "L-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("L-%03d", max+1)
}

// nonWordRe strips non-alphanumeric runes for fingerprint normalization.
var nonWordRe = regexp.MustCompile(`[^a-z0-9]+`)

// normPath converts backslash separators to forward slashes on EVERY
// platform. filepath.ToSlash is a no-op on Unix, but scope strings may be
// authored on Windows (backslash paths in feedback/tickets) and consumed on
// Linux — fingerprints and matching must agree across platforms.
func normPath(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}

// FingerprintOf computes a stable fingerprint from categories, scope, and
// title keywords. Used for dedup at distill time and recurrence matching at
// settlement time.
func FingerprintOf(title string, categories, scope []string) string {
	words := nonWordRe.Split(strings.ToLower(title), -1)
	tokens := make(map[string]bool)
	for _, w := range words {
		if len(w) >= 3 {
			tokens[w] = true
		}
	}
	for _, c := range categories {
		tokens["c:"+strings.ToLower(strings.TrimSpace(c))] = true
	}
	for _, p := range scope {
		tokens["s:"+strings.ToLower(normPath(strings.TrimSpace(p)))] = true
	}
	sorted := make([]string, 0, len(tokens))
	for t := range tokens {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)
	sum := sha1.Sum([]byte(strings.Join(sorted, "|")))
	return hex.EncodeToString(sum[:])[:8]
}

// evictionHalfLifeDays controls the recency decay of the eviction score.
const evictionHalfLifeDays = 90.0

// EvictionScore ranks lessons for retention: hits weighted by recency of the
// last hit, minus misses (Generative Agents-style recency x importance).
func EvictionScore(l Lesson, now time.Time) float64 {
	ref := l.LastHitAt
	if ref == "" {
		ref = l.CreatedAt
	}
	ageDays := 0.0
	if t, err := time.Parse(time.RFC3339, ref); err == nil {
		ageDays = now.Sub(t).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
	}
	decay := math.Exp(-ageDays * math.Ln2 / evictionHalfLifeDays)
	return float64(l.Hits)*decay - float64(l.Misses)
}

// counts returns the number of lessons in the given statuses.
func (s *Store) counts(statuses ...string) int {
	n := 0
	for _, l := range s.Lessons {
		for _, st := range statuses {
			if l.Status == st {
				n++
				break
			}
		}
	}
	return n
}

// EnforceCaps retires the lowest-scoring lessons when the store exceeds its
// limits: maxActive for active+proven, maxCandidates for candidates.
func (s *Store) EnforceCaps(maxActive, maxCandidates int, now time.Time) {
	if maxActive <= 0 {
		maxActive = 30
	}
	if maxCandidates <= 0 {
		maxCandidates = 10
	}
	retireLowest := func(statuses []string, over int) {
		if over <= 0 {
			return
		}
		type scored struct {
			idx   int
			score float64
		}
		var pool []scored
		for i, l := range s.Lessons {
			for _, st := range statuses {
				if l.Status == st {
					pool = append(pool, scored{i, EvictionScore(l, now)})
					break
				}
			}
		}
		sort.Slice(pool, func(a, b int) bool { return pool[a].score < pool[b].score })
		for i := 0; i < over && i < len(pool); i++ {
			s.Lessons[pool[i].idx].Status = StatusRetired
		}
	}
	retireLowest([]string{StatusActive, StatusProven}, s.counts(StatusActive, StatusProven)-maxActive)
	retireLowest([]string{StatusCandidate}, s.counts(StatusCandidate)-maxCandidates)
}

// categoryKeywords maps lesson/feedback categories to trigger keywords.
// Mirrors the reviewer package's rejection categories (kept local to avoid
// an import cycle: reviewer imports lessons for settlement).
var categoryKeywords = map[string][]string{
	"test":                 {"test", "testing", "unit test", "coverage"},
	"error-handling":       {"error handling", "error", "panic", "recover"},
	"naming":               {"naming", "rename", "variable name"},
	"architecture":         {"architecture", "structure", "layer", "dependency"},
	"security":             {"security", "vulnerability", "injection", "auth", "credential", "secret"},
	"performance":          {"performance", "slow", "optimize", "memory", "latency"},
	"scope":                {"scope", "out of scope", "non-goal"},
	"style":                {"style", "formatting", "lint", "convention"},
	"config":               {"config", "workflow.yaml", "schema"},
	"ci-failure":           {"ci", "pipeline", "workflow run"},
	"severity-consistency": {"severity", "critical:", "important:"},
	"criteria-mapping":     {"criteria", "acceptance"},
	"assertion":            {"assertion", "assert"},
}

// KeywordCategories extracts known categories mentioned in free text.
func KeywordCategories(text string) []string {
	lower := strings.ToLower(text)
	var found []string
	seen := make(map[string]bool)
	for cat, kws := range categoryKeywords {
		for _, kw := range kws {
			if strings.Contains(lower, kw) {
				if !seen[cat] {
					seen[cat] = true
					found = append(found, cat)
				}
				break
			}
		}
	}
	sort.Strings(found)
	return found
}

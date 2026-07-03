package lessons

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Query carries the relevance signals extracted from a ticket (or review
// context) by the caller. PathTokens are lowercase, slash-normalized path
// fragments (the worker's ticket Scope/Repo tokens, optionally expanded via
// the knowledge graph); Categories are feedback categories.
type Query struct {
	PathTokens []string
	Categories []string
}

// QueryFromText builds a Query from free text using keyword categories and
// naive path extraction. Callers with better signals (worker ticket parsing,
// knowledge-graph expansion) should build the Query themselves.
func QueryFromText(text string) Query {
	var paths []string
	seen := make(map[string]bool)
	for _, f := range strings.Fields(text) {
		f = strings.Trim(strings.ToLower(filepath.ToSlash(f)), "`\"'().,;: ")
		if len(f) >= 4 && strings.ContainsAny(f, "/.") && !strings.HasPrefix(f, "http") {
			if !seen[f] {
				seen[f] = true
				paths = append(paths, f)
			}
		}
	}
	return Query{PathTokens: paths, Categories: KeywordCategories(text)}
}

// selection weights (design doc §2 step 3).
const (
	weightScope    = 3.0
	weightCategory = 2.0
	weightHits     = 1.0
	weightRecency  = 1.0
	recencyHalfLifeDays = 30.0
)

// Select returns the top-k most relevant lessons within a character budget,
// following the ReasoningBank finding that small k beats large k: defaults
// are k=3 and 800 chars, and at most one candidate lesson is included
// (controlled exploration slot).
func Select(s *Store, q Query, topK, maxChars int, now time.Time) []Lesson {
	if topK <= 0 {
		topK = 3
	}
	if maxChars <= 0 {
		maxChars = 800
	}

	type scored struct {
		l     Lesson
		score float64
	}
	var pool []scored
	for _, l := range s.Lessons {
		if l.Status != StatusActive && l.Status != StatusProven && l.Status != StatusCandidate {
			continue
		}
		score := relevance(l, q, now)
		if score <= 0 {
			continue
		}
		pool = append(pool, scored{l, score})
	}

	sort.SliceStable(pool, func(a, b int) bool {
		if pool[a].score != pool[b].score {
			return pool[a].score > pool[b].score
		}
		return pool[a].l.ID < pool[b].l.ID
	})

	var out []Lesson
	usedChars := 0
	candidateUsed := false
	for _, sc := range pool {
		if len(out) >= topK {
			break
		}
		if sc.l.Status == StatusCandidate {
			if candidateUsed {
				continue
			}
		}
		cost := len(formatLessonLine(len(out)+1, sc.l))
		if usedChars+cost > maxChars {
			continue
		}
		if sc.l.Status == StatusCandidate {
			candidateUsed = true
		}
		usedChars += cost
		out = append(out, sc.l)
	}
	return out
}

// relevance scores a lesson against a query. Zero means "not relevant" and
// the lesson is never injected — silence beats noise.
func relevance(l Lesson, q Query, now time.Time) float64 {
	score := 0.0

	scopeHit := false
	for _, s := range l.Scope {
		sNorm := strings.ToLower(filepath.ToSlash(s))
		for _, tok := range q.PathTokens {
			if strings.Contains(tok, sNorm) || strings.Contains(sNorm, tok) {
				scopeHit = true
				break
			}
		}
		if scopeHit {
			break
		}
	}
	if scopeHit {
		score += weightScope
	}

	catHit := false
	qCats := make(map[string]bool, len(q.Categories))
	for _, c := range q.Categories {
		qCats[strings.ToLower(c)] = true
	}
	for _, c := range l.Categories {
		if qCats[strings.ToLower(c)] {
			catHit = true
			break
		}
	}
	if catHit {
		score += weightCategory
	}

	if score == 0 {
		// Neither scope nor category matched: irrelevant regardless of
		// hits/recency.
		return 0
	}

	// Normalized hit weight (saturates at 5 hits).
	h := float64(l.Hits)
	if h > 5 {
		h = 5
	}
	score += weightHits * h / 5

	// Recency of last hit.
	if l.LastHitAt != "" {
		if t, err := time.Parse(time.RFC3339, l.LastHitAt); err == nil {
			ageDays := now.Sub(t).Hours() / 24
			if ageDays < 0 {
				ageDays = 0
			}
			score += weightRecency * halfLifeDecay(ageDays, recencyHalfLifeDays)
		}
	}
	return score
}

func halfLifeDecay(ageDays, halfLife float64) float64 {
	return math.Exp2(-ageDays / halfLife)
}

// formatLessonLine renders one lesson as a numbered prompt line.
func formatLessonLine(n int, l Lesson) string {
	content := strings.TrimSpace(l.Content)
	// Collapse the content bullets into one line for the prompt.
	content = strings.Join(strings.Fields(content), " ")
	line := fmt.Sprintf("%d. [%s] %s — %s\n", n, l.ID, strings.TrimSpace(l.Title), content)
	return line
}

// FormatForPrompt renders selected lessons as a prompt section body.
// Returns "" when no lessons are given.
func FormatForPrompt(selected []Lesson) string {
	if len(selected) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("LESSONS FROM THIS PROJECT'S REVIEW HISTORY (follow these checks):\n")
	for i, l := range selected {
		b.WriteString(formatLessonLine(i+1, l))
	}
	return b.String()
}

// IDs extracts the lesson IDs from a slice.
func IDs(selected []Lesson) []string {
	ids := make([]string, len(selected))
	for i, l := range selected {
		ids[i] = l.ID
	}
	return ids
}

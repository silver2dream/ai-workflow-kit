package workbench

import (
	"net/http"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

// The lessons of a project are what its reviews taught the AI (awkit lessons):
// the page shows them all, and the ones the screening held back first, for
// the person to read and decide on, as awkit lessons approve and reject do.

// LessonView is a lesson as the page shows it.
type LessonView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Content is the lesson's text with the characters a person cannot see
	// made visible (<U+200B>): what the AI would read, all of it.
	Content    string   `json:"content"`
	Status     string   `json:"status"`
	Hits       int      `json:"hits"`
	Misses     int      `json:"misses"`
	Categories []string `json:"categories"`
	Scope      []string `json:"scope"`
	CreatedAt  string   `json:"created_at"`
	LastHitAt  string   `json:"last_hit_at,omitempty"`
	// Source is who wrote it: the distiller, a person's correction it was
	// distilled from, or a person.
	Source string `json:"source"`
	// HeldBack is why the screening held it back; kept after a person turned
	// it down.
	HeldBack string `json:"held_back,omitempty"`
	// Hidden says the text has characters a person cannot see.
	Hidden   bool           `json:"hidden,omitempty"`
	Evidence []EvidenceView `json:"evidence"`
}

// EvidenceView is a review case a lesson came from.
type EvidenceView struct {
	Issue int `json:"issue"`
	PR    int `json:"pr"`
}

// LessonsView is the lessons of a project: the quarantined first, then the
// ones in use by how much they helped, then the retired.
type LessonsView struct {
	Lessons []LessonView `json:"lessons"`
}

func lessonView(l lessons.Lesson) LessonView {
	text := strings.Join([]string{l.Title, l.Description, l.Content}, "\n")
	v := LessonView{
		ID: l.ID, Title: guard.Reveal(l.Title), Description: guard.Reveal(l.Description),
		Content: guard.Reveal(l.Content), Status: l.Status, Hits: l.Hits, Misses: l.Misses,
		Categories: l.Categories, Scope: l.Scope, CreatedAt: l.CreatedAt, LastHitAt: l.LastHitAt,
		Source: l.Source, HeldBack: l.Quarantine, Hidden: guard.Reveal(text) != text,
		Evidence: []EvidenceView{},
	}
	if v.Categories == nil {
		v.Categories = []string{}
	}
	if v.Scope == nil {
		v.Scope = []string{}
	}
	for _, e := range l.Evidence {
		v.Evidence = append(v.Evidence, EvidenceView{Issue: e.Issue, PR: e.PR})
	}
	return v
}

// lessonOrder puts what needs the person first and what is gone last.
var lessonOrder = map[string]int{
	lessons.StatusQuarantined: 0,
	lessons.StatusProven:      1,
	lessons.StatusActive:      2,
	lessons.StatusCandidate:   3,
	lessons.StatusRetired:     4,
}

func (s *Server) handleLessons(w http.ResponseWriter, r *http.Request, root string) {
	store, err := lessons.Load(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view := LessonsView{Lessons: []LessonView{}}
	for _, l := range store.Lessons {
		view.Lessons = append(view.Lessons, lessonView(l))
	}
	sort.SliceStable(view.Lessons, func(i, j int) bool {
		a, b := view.Lessons[i], view.Lessons[j]
		if lessonOrder[a.Status] != lessonOrder[b.Status] {
			return lessonOrder[a.Status] < lessonOrder[b.Status]
		}
		if a.Hits != b.Hits {
			return a.Hits > b.Hits
		}
		return a.ID < b.ID
	})
	writeJSON(w, http.StatusOK, view)
}

// handleLessonDecision approves or rejects a quarantined lesson: the
// decision is the person's, recorded as theirs.
func (s *Server) handleLessonDecision(decision string) func(w http.ResponseWriter, r *http.Request, root string) {
	return func(w http.ResponseWriter, r *http.Request, root string) {
		id := r.PathValue("lesson")
		store, err := lessons.Load(root)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		l := store.FindByID(id)
		switch {
		case l == nil:
			writeError(w, http.StatusNotFound, "this project has no lesson "+id)
			return
		case l.Status != lessons.StatusQuarantined:
			// Decided already, here or in a terminal: the page shows it
			// again as it is.
			writeError(w, http.StatusConflict, "lesson "+id+" is no longer held back: it is "+l.Status)
			return
		}
		decided, err := lessons.Decide(root, id, decision, "workbench")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, lessonView(*decided))
	}
}

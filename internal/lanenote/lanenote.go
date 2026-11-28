// Package lanenote keeps what the owner said of a ticket for its
// implementer: why they returned a change that waited for their approval
// (design.md §10, task 2.5), a note they added (task 2.6). Each dispatch of
// the ticket gives the notes to the Worker as the owner's word, beside the
// ticket. The engine writes them; the Worker reads them.
package lanenote

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// Kinds of note.
const (
	// Returned is the owner's reason for returning a change that waited for
	// their approval.
	Returned = "returned"
)

// MaxText bounds a note's text: what is longer is cut.
const MaxText = 4000

// Note is one thing the owner said of a ticket.
type Note struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	// By says who: a person's login, "the workbench", "awkit lane".
	By   string `json:"by,omitempty"`
	Text string `json:"text"`
}

var mu sync.Mutex

func file(root string, issue int) string {
	return filepath.Join(root, filepath.FromSlash(layout.StateDir), "engine", "notes", fmt.Sprintf("%d.jsonl", issue))
}

// Add keeps a note of a ticket, after the ones before it. A note with no
// text is not kept.
func Add(root string, issue int, n Note) error {
	n.Text = strings.TrimSpace(n.Text)
	if n.Text == "" {
		return nil
	}
	if r := []rune(n.Text); len(r) > MaxText {
		n.Text = string(r[:MaxText]) + "…"
	}
	if n.At.IsZero() {
		n.At = time.Now().UTC()
	}
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	path := file(root, issue)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Read is the notes of a ticket, the oldest first; none when it has none. A
// line that cannot be read is passed over.
func Read(root string, issue int) ([]Note, error) {
	f, err := os.Open(file(root, issue))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var notes []Note
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var n Note
		if json.Unmarshal(scanner.Bytes(), &n) == nil && n.Text != "" {
			notes = append(notes, n)
		}
	}
	return notes, scanner.Err()
}

// Text is a ticket's notes as the Worker is given them: each with when and
// why it was said; "" when there are none.
func Text(root string, issue int) string {
	notes, err := Read(root, issue)
	if err != nil || len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	for _, n := range notes {
		what := "A note"
		if n.Kind == Returned {
			what = "Why the owner returned the change they were to approve"
		}
		fmt.Fprintf(&b, "%s (%s):\n%s\n\n", what, n.At.UTC().Format("2006-01-02 15:04 UTC"), n.Text)
	}
	return strings.TrimSpace(b.String())
}

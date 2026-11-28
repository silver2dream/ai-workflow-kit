package lanenote

import (
	"strings"
	"testing"
	"time"
)

// Notes are kept in order, by ticket; one with no text is not, one too long
// is cut; the Worker is given them with when and why.
func TestNotes(t *testing.T) {
	root := t.TempDir()
	if notes, err := Read(root, 7); err != nil || notes != nil {
		t.Fatalf("none yet: %v, %v", notes, err)
	}
	if Text(root, 7) != "" {
		t.Error("text of no notes")
	}
	at := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	for _, n := range []Note{
		{At: at, Kind: Returned, By: "awkit lane", Text: "  Keep the old name as an alias.  "},
		{At: at.Add(time.Hour), Kind: "note", Text: "Use the shared clock."},
		{Kind: Returned, Text: "   "},
	} {
		if err := Add(root, 7, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := Add(root, 8, Note{Kind: Returned, Text: strings.Repeat("é", MaxText+10)}); err != nil {
		t.Fatal(err)
	}
	notes, err := Read(root, 7)
	if err != nil || len(notes) != 2 || notes[0].Text != "Keep the old name as an alias." || notes[0].By != "awkit lane" || notes[1].Kind != "note" {
		t.Fatalf("notes: %+v, %v", notes, err)
	}
	if long, _ := Read(root, 8); len([]rune(long[0].Text)) != MaxText+1 || long[0].At.IsZero() {
		t.Errorf("a long note: %d characters, at %v", len([]rune(long[0].Text)), long[0].At)
	}
	want := "Why the owner returned the change they were to approve (2026-11-28 09:00 UTC):\nKeep the old name as an alias.\n\nA note (2026-11-28 10:00 UTC):\nUse the shared clock."
	if got := Text(root, 7); got != want {
		t.Errorf("text:\n%s\nwant:\n%s", got, want)
	}
}

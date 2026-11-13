package council

import (
	"reflect"
	"strings"
	"testing"
)

// The rosters the kit ships can hold a meeting, and their roles disagree by
// design: one decides, one plans, every role owns something.
func TestDefaultRosters(t *testing.T) {
	for _, preset := range []string{"generic", "godot-go"} {
		roster, err := ParseRoster(DefaultRoster(preset))
		if err != nil {
			t.Errorf("%s: %v", preset, err)
			continue
		}
		if roster.Decider() == nil || roster.Planner() == nil || roster.Decider().ID == roster.Planner().ID {
			t.Errorf("%s: decides %v, plans %v; want two different roles", preset, roster.Decider(), roster.Planner())
		}
		if len(roster.Roles) < 4 {
			t.Errorf("%s: %d roles", preset, len(roster.Roles))
		}
		for _, role := range roster.Roles {
			if len(role.Owns) == 0 {
				t.Errorf("%s: %s owns no document", preset, role.ID)
			}
			if !strings.Contains(strings.Join(strings.Fields(role.Stance), " "), "push back") {
				t.Errorf("%s: the stance of %s does not say what it pushes back on:\n%s", preset, role.ID, role.Stance)
			}
			if role.Model != "sonnet" {
				t.Errorf("%s: %s is played by %q, want the roster's model", preset, role.ID, role.Model)
			}
		}
		if roster.Language != "" {
			t.Errorf("%s: the language is fixed to %q; a shipped roster follows the brief", preset, roster.Language)
		}
	}
	// A preset without a roster of its own gets the generic one.
	if string(DefaultRoster("unknown-preset")) != string(DefaultRoster("generic")) {
		t.Error("an unknown preset did not get the generic roster")
	}
	if string(DefaultRoster("godot-go")) == string(DefaultRoster("generic")) {
		t.Error("godot-go has no roster of its own")
	}
}

func TestParseRoster_Defaults(t *testing.T) {
	roster, err := ParseRoster([]byte(`
model: opus
roles:
  - id: a
    decides: true
    stance: stands for a
  - id: b
    title: The B
    plans: true
    model: haiku
    stance: stands for b
`))
	if err != nil {
		t.Fatalf("ParseRoster: %v", err)
	}
	if want := (Limits{Disputes: 5, DisputeRounds: 2, Calls: 80, Parallel: 3, TimeoutSeconds: 600}); roster.Limits != want {
		t.Errorf("limits = %+v, want %+v", roster.Limits, want)
	}
	if a := roster.Role("a"); a.Model != "opus" || a.Title != "a" {
		t.Errorf("role a = %+v; want the roster's model and its id for a title", a)
	}
	if b := roster.Role("b"); b.Model != "haiku" || b.Title != "The B" {
		t.Errorf("role b = %+v", b)
	}
	if !reflect.DeepEqual(roster.IDs(), []string{"a", "b"}) || roster.Role("c") != nil {
		t.Errorf("IDs = %v, Role(c) = %v", roster.IDs(), roster.Role("c"))
	}
}

func TestParseRoster_RefusesWhatCannotHoldAMeeting(t *testing.T) {
	const two = "roles:\n  - {id: a, decides: true, stance: s}\n  - {id: b, plans: true, stance: s}\n"
	tests := []struct {
		name, roster, wantInError string
	}{
		{"one role", "roles:\n  - {id: a, decides: true, plans: true, stance: s}\n", "at least two"},
		{"nobody decides", "roles:\n  - {id: a, stance: s}\n  - {id: b, plans: true, stance: s}\n", "0 roles decide"},
		{"two decide", "roles:\n  - {id: a, decides: true, stance: s}\n  - {id: b, decides: true, plans: true, stance: s}\n", "2 roles decide (a, b)"},
		{"nobody plans", "roles:\n  - {id: a, decides: true, stance: s}\n  - {id: b, stance: s}\n", "0 roles plan"},
		{"a role twice", "roles:\n  - {id: a, decides: true, stance: s}\n  - {id: a, plans: true, stance: s}\n", `role "a" is in the roster twice`},
		{"an id with a space", "roles:\n  - {id: Tech Lead, decides: true, stance: s}\n  - {id: b, plans: true, stance: s}\n", `role id "Tech Lead"`},
		{"no stance", "roles:\n  - {id: a, decides: true}\n  - {id: b, plans: true, stance: s}\n", `role "a" has no stance`},
		{"a misspelt field", strings.Replace(two, "decides", "decide", 1), "cannot be read"},
		{"a document outside docs", strings.Replace(two, "stance: s}", "stance: s, owns: [src/main.go]}", 1), "is outside docs/"},
		{"a document that leaves docs", strings.Replace(two, "stance: s}", "stance: s, owns: [docs/../README.md]}", 1), "write the path plainly"},
		{"a document that is not markdown", strings.Replace(two, "stance: s}", "stance: s, owns: [docs/plan.txt]}", 1), "is not a markdown file"},
		{"the decision log", strings.Replace(two, "stance: s}", "stance: s, owns: [docs/decisions.md]}", 1), "kept by the council itself"},
		{"the index", strings.Replace(two, "stance: s}", "stance: s, owns: [docs/README.md]}", 1), "kept by the council itself"},
		{"a plan's folder", strings.Replace(two, "stance: s}", "stance: s, owns: [docs/plans/x/minutes.md]}", 1), "where the council keeps its plans"},
		{"a path with backslashes", strings.Replace(two, "stance: s}", `stance: s, owns: ['docs\a.md']}`, 1), "write the path plainly"},
		{"a document with two owners", strings.ReplaceAll(two, "stance: s}", "stance: s, owns: [docs/a.md]}"), `docs/a.md has two owners, "a" and "b"`},
	}
	for _, tt := range tests {
		roster, err := ParseRoster([]byte(tt.roster))
		if err == nil {
			t.Errorf("%s: accepted: %+v", tt.name, roster)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: error = %q, want it to mention %q", tt.name, err, tt.wantInError)
		}
	}
	if _, err := ParseRoster([]byte(two)); err != nil {
		t.Errorf("the roster the cases are made from was refused: %v", err)
	}
}

func TestDecodeJSON(t *testing.T) {
	type answer struct {
		Summary string `json:"summary"`
	}
	for name, text := range map[string]string{
		"a json block":             "Here.\n```json\n{\"summary\": \"ok\"}\n```\n",
		"a block without the word": "```\n{\"summary\": \"ok\"}\n```",
		"the object alone":         "{\"summary\": \"ok\"}",
		"the object in a sentence": "My answer is {\"summary\": \"ok\"} as asked.",
		"CRLF":                     "```json\r\n{\"summary\": \"ok\"}\r\n```\r\n",
		"a field it does not know": "```json\n{\"summary\": \"ok\", \"confidence\": 0.9}\n```",
		// The first block is not JSON: the object is found all the same.
		"another block first": "```go\nfunc main() {}\n```\n```json\n{\"summary\": \"ok\"}\n```",
	} {
		var got answer
		if err := decodeJSON(text, &got); err != nil || got.Summary != "ok" {
			t.Errorf("%s: got %+v, err %v", name, got, err)
		}
	}
	for name, text := range map[string]string{
		"no object":      "I would rather not say.",
		"a broken one":   "```json\n{\"summary\": \n```",
		"the wrong type": "```json\n{\"summary\": 3}\n```",
	} {
		var got answer
		if err := decodeJSON(text, &got); err == nil {
			t.Errorf("%s: read as %+v", name, got)
		}
	}
}

func TestParseDocuments(t *testing.T) {
	answer := "Here are my documents.\r\n<<<DOCUMENT docs/a.md>>>\r\n\r\n# A\r\n\r\n```go\nfunc main() {}\n```\r\n\r\n<<<END>>>\r\n\nand\n<<<DOCUMENT  docs/b.md >>>\n# B\n<<<END>>>\ntrailing words\n"

	got, err := parseDocuments(answer)

	if err != nil {
		t.Fatalf("parseDocuments: %v", err)
	}
	want := []document{
		{Path: "docs/a.md", Content: "# A\n\n```go\nfunc main() {}\n```\n"},
		{Path: "docs/b.md", Content: "# B\n"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("documents = %#v\nwant %#v", got, want)
	}

	// A document the role leaves as it is takes one line. Inside a document
	// the same line is text.
	got, err = parseDocuments("<<<UNCHANGED docs/a.md>>>\r\n<<<DOCUMENT docs/b.md>>>\n# B\n<<<UNCHANGED docs/c.md>>>\n<<<END>>>\n<<<UNCHANGED  docs/d.md >>>\n")
	if err != nil {
		t.Fatalf("parseDocuments: %v", err)
	}
	want = []document{
		{Path: "docs/a.md", Unchanged: true},
		{Path: "docs/b.md", Content: "# B\n<<<UNCHANGED docs/c.md>>>\n"},
		{Path: "docs/d.md", Unchanged: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("documents = %#v\nwant %#v", got, want)
	}

	for name, text := range map[string]string{
		"nothing":   "I have no documents.",
		"not ended": "<<<DOCUMENT docs/a.md>>>\n# A\n",
	} {
		if docs, err := parseDocuments(text); err == nil {
			t.Errorf("%s: read as %+v", name, docs)
		}
	}
}

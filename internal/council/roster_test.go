package council

import (
	"reflect"
	"strings"
	"testing"
)

// The rosters the kit ships can hold a meeting, and their roles disagree by
// design: one decides, one plans, every role owns something.
// The repos of the two presets, as a project's configuration names them.
var (
	oneRepo  = []Repo{{Name: "root", Language: "generic", Path: "./"}}
	gameRepo = []Repo{{Name: "backend", Language: "go", Path: "backend/"}, {Name: "frontend", Language: "csharp", Path: "frontend/"}}
)

func defaultRoster(t *testing.T, preset string, repos []Repo) string {
	t.Helper()
	data, err := DefaultRoster(preset, repos)
	if err != nil {
		t.Fatalf("DefaultRoster(%s): %v", preset, err)
	}
	return string(data)
}

func TestDefaultRosters(t *testing.T) {
	for preset, repos := range map[string][]Repo{"generic": oneRepo, "godot-go": gameRepo} {
		roster, err := ParseRoster([]byte(defaultRoster(t, preset, repos)))
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
	if defaultRoster(t, "unknown-preset", oneRepo) != defaultRoster(t, "generic", oneRepo) {
		t.Error("an unknown preset did not get the generic roster")
	}
	if defaultRoster(t, "godot-go", gameRepo) == defaultRoster(t, "generic", gameRepo) {
		t.Error("godot-go has no roster of its own")
	}
}

// The engineering documents of a first roster are kept with the project's
// repos, where the rules a project starts with say they are
// (layout.RepoDocs): the folder docs/ of each repo, or docs/engineering/ when
// the repo is the whole repository. Product and design documents stay under
// docs/. Every role owns folders, so that a topic can open the document it
// needs.
func TestDefaultRosters_EngineeringDocumentsAreKeptWithTheRepos(t *testing.T) {
	owns := func(preset string, repos []Repo, role string) []string {
		t.Helper()
		roster, err := ParseRoster([]byte(defaultRoster(t, preset, repos)))
		if err != nil {
			t.Fatalf("%s: %v", preset, err)
		}
		if roster.Role(role) == nil {
			t.Fatalf("%s has no role %s: %v", preset, role, roster.IDs())
		}
		return roster.Role(role).Owns
	}
	tests := []struct {
		name, preset, role string
		repos              []Repo
		want               []string
	}{
		{"the game studio", "godot-go", "tech-lead", gameRepo, []string{"backend/docs/", "frontend/docs/"}},
		{"the game studio, with the client first and other directories", "godot-go", "tech-lead",
			[]Repo{{Name: "client", Language: "godot", Path: "client"}, {Name: "server", Language: "golang", Path: "./apps/server/"}},
			[]string{"client/docs/", "apps/server/docs/"}},
		{"one repo that is the whole repository", "generic", "architect", oneRepo, []string{"docs/engineering/"}},
		{"several repos", "generic", "architect",
			[]Repo{{Name: "api", Language: "python", Path: "api/"}, {Name: "web", Language: "react", Path: "web/"}},
			[]string{"api/docs/", "web/docs/"}},
		{"no repo named: the whole repository", "generic", "architect", nil, []string{"docs/engineering/"}},
		{"two repos in one directory have one folder", "generic", "architect",
			[]Repo{{Name: "a", Path: "./"}, {Name: "b", Path: "."}}, []string{"docs/engineering/"}},
		{"a directory YAML would not read as it is written", "generic", "architect",
			[]Repo{{Name: "api", Path: "my api/"}, {Name: "n", Path: "#1/"}}, []string{"my api/docs/", "#1/docs/"}},
	}
	for _, tt := range tests {
		if got := owns(tt.preset, tt.repos, tt.role); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %s owns %v, want %v", tt.name, tt.role, got, tt.want)
		}
	}
	// The others own folders under docs/: nothing of theirs moves with the repos.
	for preset, want := range map[string]map[string][]string{
		"godot-go": {"producer": {"docs/product/"}, "game-designer": {"docs/design/"}, "numerical-designer": {"docs/balance/"}, "qa": {"docs/qa/"}},
		"generic":  {"product-owner": {"docs/product/"}, "user-advocate": {"docs/ux/"}, "skeptic": {"docs/qa/"}},
	} {
		for role, folders := range want {
			if got := owns(preset, gameRepo, role); !reflect.DeepEqual(got, folders) {
				t.Errorf("%s: %s owns %v, want %v", preset, role, got, folders)
			}
		}
	}

	// A roster is not written for repos it could not hold a meeting with: a
	// repo outside the project has no place for its documents in it.
	if data, err := DefaultRoster("generic", []Repo{{Name: "elsewhere", Path: "../elsewhere"}}); err == nil || !strings.Contains(err.Error(), "write the path plainly") {
		t.Errorf("a repo outside the project: %q, %v", data, err)
	}
	// What is written is the template filled in: nothing of it is left.
	for _, preset := range []string{"generic", "godot-go"} {
		if roster := defaultRoster(t, preset, gameRepo); strings.Contains(roster, "{{") || strings.Contains(roster, "}}") {
			t.Errorf("%s: the roster still holds a piece of its template:\n%s", preset, roster)
		}
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
	if want := (Limits{Topics: 8, Disputes: 2, DisputeRounds: 2, Calls: 200, Parallel: 3, TimeoutSeconds: 600}); roster.Limits != want {
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
	// owning is the roster of two whose first role owns the documents.
	owning := func(docs string) string {
		return strings.Replace(two, "stance: s}", "stance: s, owns: ["+docs+"]}", 1)
	}
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
		{"a document that is not markdown", owning("src/main.go"), "is not a markdown file"},
		{"a text file under docs", owning("docs/plan.txt"), "is not a markdown file"},
		{"a document that leaves docs", owning("docs/../README.md"), "write the path plainly"},
		{"a document outside the project", owning("../other/docs/a.md"), "write the path plainly"},
		{"the directory above", owning(".."), "write the path plainly"},
		{"a path from the root of the disk", owning("/etc/a.md"), "write the path plainly"},
		{"a path with a drive", owning("'C:/docs/a.md'"), "write the path plainly"},
		{"a path with a colon", owning("'docs/a.md:stream.md'"), "write the path plainly"},
		{"a path with backslashes", owning(`'docs\a.md'`), "write the path plainly"},
		{"a path with a dot in front", owning("./docs/a.md"), "write the path plainly"},
		{"a folder written with backslashes", owning(`'docs\design/'`), "write the path plainly, from the project's root, with forward slashes and a slash at its end"},
		{"a folder written with backslashes only", owning(`'docs\design\'`), "write the path plainly, from the project's root and with forward slashes"},
		{"a folder above the project", owning("../docs/"), "folder \"../docs/\": write the path plainly"},
		{"the root as a folder", owning("./"), "folder \"./\": write the path plainly"},
		{"a folder written twice over", owning("docs//design/"), "folder \"docs//design/\": write the path plainly"},
		{"a folder a tool keeps", owning(".claude/rules/"), "folder \".claude/rules/\" is in .claude, which a tool keeps for itself"},
		{"the folder of the council's own files", owning("docs/"), "holds the council's own files (docs/decisions.md, docs/README.md, docs/plans/): own a folder inside it"},
		{"the folder of the council's own files, in other letters", owning("Docs/"), "holds the council's own files"},
		{"a folder of the plans", owning("docs/plans/"), "is under docs/plans/, where the council keeps its plans"},
		{"a folder in a plan", owning("docs/plans/x/"), "is under docs/plans/, where the council keeps its plans"},
		{"a folder with two owners", strings.ReplaceAll(two, "stance: s}", "stance: s, owns: [docs/design/]}"), `docs/design/ has two owners, "a" and "b"`},
		{"a document in another role's folder", strings.Replace(owning("docs/design/"), "plans: true, stance: s}", "plans: true, stance: s, owns: [docs/design/economy.md]}", 1), `docs/design/economy.md is in docs/design/, which "a" owns; a document has one owner`},
		{"a folder around another role's document", strings.Replace(owning("docs/design/economy.md"), "plans: true, stance: s}", "plans: true, stance: s, owns: [docs/]}", 1), "holds the council's own files"},
		{"a folder in another role's folder", strings.Replace(owning("docs/design/"), "plans: true, stance: s}", "plans: true, stance: s, owns: [Docs/Design/numbers/]}", 1), `Docs/Design/numbers/ is in docs/design/, which "a" owns`},
		{"a folder around another role's folder", strings.Replace(owning("backend/docs/api/"), "plans: true, stance: s}", "plans: true, stance: s, owns: [backend/]}", 1), `backend/docs/api/ is in backend/, which "b" owns`},
		{"a document in a role's own folder", owning("docs/design/, docs/design/game.md"), `role "a" owns docs/design/game.md twice: it is in docs/design/`},
		{"a folder in a role's own folder", owning("docs/design/game.md, docs/"), "holds the council's own files"},
		{"no path", owning("''"), "write the path plainly"},
		{"the decision log", owning("docs/decisions.md"), "kept by the council itself"},
		{"the decision log, in other letters", owning("Docs/Decisions.MD"), "kept by the council itself"},
		{"the index", owning("docs/README.md"), "kept by the council itself"},
		{"the index, in other letters", owning("docs/readme.md"), "kept by the council itself"},
		{"a plan's folder", owning("docs/plans/x/minutes.md"), "where the council keeps its plans"},
		{"a plan's folder, in other letters", owning("docs/Plans/x/minutes.md"), "where the council keeps its plans"},
		{"a rule", owning(".claude/rules/backend.md"), "is in .claude, which a tool keeps for itself"},
		{"a skill", owning(".claude/skills/x/SKILL.md"), "is in .claude, which a tool keeps for itself"},
		{"what awkit keeps", owning(".ai/notes.md"), "is in .ai, which a tool keeps for itself"},
		{"what GitHub reads", owning(".github/pull_request_template.md"), "is in .github, which a tool keeps for itself"},
		{"a hidden directory inside a repo", owning("backend/.notes/a.md"), "is in .notes, which a tool keeps for itself"},
		{"a hidden file", owning("docs/.draft.md"), "is in .draft.md, which a tool keeps for itself"},
		{"what Claude Code is instructed by", owning("CLAUDE.md"), "what an agent is instructed by"},
		{"what Claude Code is instructed by, in a repo", owning("backend/CLAUDE.md"), "what an agent is instructed by"},
		{"what Claude Code is instructed by, in other letters", owning("backend/Claude.MD"), "what an agent is instructed by"},
		{"the local instructions", owning("CLAUDE.local.md"), "what an agent is instructed by"},
		{"what a Worker is instructed by", owning("AGENTS.md"), "what an agent is instructed by"},
		{"what a Worker is instructed by, in a repo", owning("frontend/src/AGENTS.md"), "what an agent is instructed by"},
		{"a document with two owners", strings.ReplaceAll(two, "stance: s}", "stance: s, owns: [docs/a.md]}"), `docs/a.md has two owners, "a" and "b"`},
		{"a document with two owners who write it in other letters", strings.Replace(owning("docs/a.md"), "plans: true, stance: s}", "plans: true, stance: s, owns: [Docs/A.md]}", 1), `Docs/A.md has two owners, "a" and "b"`},
		{"a document one role owns twice", owning("docs/a.md, docs/a.md"), `role "a" owns docs/a.md twice`},
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

	// A document is a Markdown file of the project, wherever the project keeps
	// it: with the product documents, with a repo, or in the root; and a
	// folder is one of such files.
	for _, doc := range []string{"docs/design/game-design.md", "docs/engineering/architecture.md", "backend/docs/architecture.md",
		"apps/server/docs/research/storage.md", "backend/internal/match/README.md", "README.md", "docs/Design/UPPER.MD",
		"docs/claude.md.notes.md", "docs/my agents.md", "'docs/設計/遊戲.md'", "'my api/docs/a b.md'",
		"docs/design/", "backend/docs/", "backend/", "docs/a.md/", "'docs/設計/'", "docs/design/, docs/qa/", "docs/plansx/", "documents/"} {
		if _, err := ParseRoster([]byte(owning(doc))); err != nil {
			t.Errorf("a role cannot own %s: %v", doc, err)
		}
	}
}

// A role owns a document the roster names, or one in a folder it names, at
// any depth; nobody owns what cannot be a document.
func TestRoleOwnsDocument(t *testing.T) {
	role := &Role{ID: "a", Owns: []string{"docs/design/", "docs/qa/risks.md", "backend/docs/"}}
	for doc, want := range map[string]bool{
		"docs/design/game.md":               true,
		"docs/design/systems/combat/hit.md": true,
		"docs/qa/risks.md":                  true,
		"backend/docs/architecture.md":      true,
		"docs/qa/tests.md":                  false,
		"docs/designs/game.md":              false,
		"docs/design":                       false,
		"docs/design/":                      false,
		"docs/design/game.txt":              false,
		"docs/design/CLAUDE.md":             false,
		"docs/design/.draft.md":             false,
		"backend/docs/../main.md":           false,
		"Docs/Design/game.md":               false,
		"frontend/docs/architecture.md":     false,
	} {
		if got := role.OwnsDocument(doc); got != want {
			t.Errorf("OwnsDocument(%q) = %v, want %v", doc, got, want)
		}
	}
	if got, want := role.Files(), []string{"docs/qa/risks.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Files = %v, want %v", got, want)
	}
	if got, want := role.Folders(), []string{"docs/design/", "backend/docs/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Folders = %v, want %v", got, want)
	}
	roster := mustRoster(t, "roles:\n  - {id: a, decides: true, stance: s, owns: [docs/design/]}\n  - {id: b, plans: true, stance: s, owns: [docs/qa/risks.md]}\n")
	for doc, want := range map[string]string{"docs/design/x.md": "a", "docs/qa/risks.md": "b", "docs/qa/x.md": "", "docs/decisions.md": ""} {
		got := ""
		if owner := roster.Owner(doc); owner != nil {
			got = owner.ID
		}
		if got != want {
			t.Errorf("Owner(%q) = %q, want %q", doc, got, want)
		}
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

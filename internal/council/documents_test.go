package council

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// A document that is written already is its owner's to leave as it is; one
// the owner changes is written whole. What a person changed by hand since is
// not written over when the plan is gone on with.
func TestRun_DocumentsThatAreAlreadyThere(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	const risks = "# Risks\n\nThe database can be down.\n"
	putFile(t, root, "docs/qa/risks.md", risks)
	putFile(t, root, "docs/tech/architecture.md", "# Architecture\n\nOne Go service.\n")
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "documents" && c.Role == "skeptic" {
			return "Nothing changes for me.\n<<<UNCHANGED docs/qa/risks.md>>>\n", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, root, "docs/qa/risks.md"); got != risks {
		t.Errorf("the document its owner left as it was has changed:\n%s", got)
	}
	if got := readFile(t, root, "docs/tech/architecture.md"); !strings.Contains(got, "Written by builder.") || strings.Contains(got, "One Go service.") {
		t.Errorf("the document its owner rewrote is not the one handed in:\n%s", got)
	}
	state := map[string]WrittenDocument{}
	for _, doc := range plan.Documents {
		state[doc.Path] = doc
	}
	if !state["docs/qa/risks.md"].Unchanged || state["docs/tech/architecture.md"].Unchanged || state["docs/tech/architecture.md"].Refused != "" {
		t.Errorf("documents = %+v", plan.Documents)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"- `docs/qa/risks.md` (skeptic): left as it was\n", "- `docs/tech/architecture.md` (builder)\n"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes should say %q:\n%s", want, minutes)
		}
	}

	// A role is shown its document whole before it writes, and is told it may
	// leave it; a role with nothing written yet is not offered that.
	skeptic := model.promptOf("skeptic", "documents")
	for _, want := range []string{"Your documents as they stand:\n===== docs/qa/risks.md\n# Risks\n\nThe database can be down.\n", "keep the rest as it is, word for word", "<<<UNCHANGED docs/the/path.md>>>"} {
		if !strings.Contains(skeptic, want) {
			t.Errorf("the skeptic's question should say %q:\n%s", want, skeptic)
		}
	}
	owner := model.promptOf("owner", "documents")
	if !strings.Contains(owner, "- docs/product/requirements.md: not written yet\n") || strings.Contains(owner, "<<<UNCHANGED") {
		t.Errorf("a role with nothing written is offered to leave it unchanged:\n%s", owner)
	}
	// The planning role reads every document as it stands after the meeting.
	worklist := model.promptOf("builder", "worklist")
	for _, want := range []string{"===== docs/qa/risks.md (owner: skeptic; left unchanged by this meeting)\n# Risks\n\nThe database can be down.\n",
		"===== docs/tech/architecture.md (owner: builder; written in this meeting)\n", "Written by builder."} {
		if !strings.Contains(worklist, want) {
			t.Errorf("the planning role should be shown %q:\n%s", want, worklist)
		}
	}

	// A person corrects a document by hand. Going on with the plan asks
	// nothing and leaves the correction where it is.
	const corrected = "# Architecture\n\nCorrected by hand.\n"
	putFile(t, root, "docs/tech/architecture.md", corrected)
	before := len(model.steps())
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err != nil {
		t.Fatalf("going on: %v", err)
	}
	if asked := model.steps()[before:]; len(asked) != 0 {
		t.Errorf("going on with a finished plan asked: %v", asked)
	}
	if got := readFile(t, root, "docs/tech/architecture.md"); got != corrected {
		t.Errorf("going on with the plan wrote over what a person corrected:\n%s", got)
	}
}

// A document longer than a role is shown is not rewritten: the role would
// drop what it did not see. It stays, the minutes say why, and the role still
// writes the others it owns.
func TestRun_ADocumentTooLongToRewriteStays(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, strings.Replace(testRoster, "owns: [docs/qa/risks.md]", "owns: [docs/qa/risks.md, docs/qa/tests.md]", 1))
	long := "# 風險\n\n" + strings.Repeat("資料庫可能連不上，健康檢查要能分辨。\n", maxDocument/40)
	if len(long) <= maxDocument {
		t.Fatalf("the document of the test is %d bytes, not longer than %d", len(long), maxDocument)
	}
	putFile(t, root, "docs/qa/risks.md", long)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "documents" && c.Role == "skeptic" {
			// The role hands the long one in all the same.
			return docs("skeptic", "docs/qa/risks.md", "docs/qa/tests.md"), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, root, "docs/qa/risks.md"); got != long {
		t.Errorf("the long document was written over (%d bytes now)", len(got))
	}
	if !strings.Contains(readFile(t, root, "docs/qa/tests.md"), "Written by skeptic.") {
		t.Error("the role's other document was not written")
	}
	var left WrittenDocument
	for _, doc := range plan.Documents {
		if doc.Path == "docs/qa/risks.md" {
			left = doc
		}
	}
	if !strings.Contains(left.Refused, "bytes long, and the council writes a document whole, up to 40000 bytes: split it") {
		t.Errorf("the long document is on record as %+v", left)
	}
	if minutes := readFile(t, root, plan.Dir()+"/minutes.md"); !strings.Contains(minutes, "- `docs/qa/risks.md` (skeptic): **not written**, it is ") {
		t.Errorf("the minutes do not say that a document was left:\n%s", minutes)
	}

	// The role is told so, is not asked for it, and sees its beginning, cut at
	// a line; the planning role sees it cut too.
	asked := model.promptOf("skeptic", "documents")
	for _, want := range []string{"write the documents you own, as they read after this meeting:\n- docs/qa/tests.md\n(docs/qa/risks.md is too long to be rewritten here. It stays as it is: do not hand it in.)\n",
		"===== docs/qa/risks.md (only its beginning: it is too long to be shown whole)\n# 風險\n"} {
		if !strings.Contains(asked, want) {
			t.Errorf("the skeptic's question should say %q:\n%s", want, asked[:min(len(asked), 3000)])
		}
	}
	worklist := model.promptOf("builder", "worklist")
	if !strings.Contains(worklist, "健康檢查要能分辨。\n(cut here; the file has the rest)\n") {
		t.Error("the planning role is not shown the long document cut at a line")
	}
	// A document that was left is not said to have been written.
	if !strings.Contains(worklist, "===== docs/qa/risks.md (owner: skeptic; not written by this meeting)\n") || !strings.Contains(worklist, "===== docs/qa/tests.md (owner: skeptic; written in this meeting)\n") {
		t.Error("the planning role is not told which document the meeting wrote and which it left")
	}
	if review := model.promptOf("designer", "review"); !strings.Contains(review, "- docs/qa/risks.md (owner: skeptic; not written by this meeting)\n- docs/qa/tests.md (owner: skeptic; written in this meeting)\n") {
		t.Errorf("those who look at the work list are not told which document was left:\n%s", review)
	}
	for name, prompt := range map[string]string{"documents": asked, "position": model.promptOf("skeptic", "position"), "work list": worklist} {
		if !utf8.ValidString(prompt) {
			t.Errorf("the %s question is not valid UTF-8: a document was cut inside a character", name)
		}
	}

	// A role all of whose documents are too long is not asked at all.
	root = t.TempDir()
	roster = mustRoster(t, testRoster)
	putFile(t, root, "docs/qa/risks.md", long)
	model = &table{answer: agreeable(roster)}
	if _, err := runPlan(t, root, roster, model, "Add a health endpoint."); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := model.count("skeptic", "documents"); got != 0 {
		t.Errorf("the skeptic was asked %d times for a document it cannot write", got)
	}
	if got := readFile(t, root, "docs/qa/risks.md"); got != long {
		t.Error("the long document was written over")
	}
}

// A document that cannot be written stops the plan. The role is not asked
// again: nothing was wrong with its answer, and what it answered is asked for
// anew when the plan is gone on with, not taken as written.
func TestRun_AFileThatCannotBeWrittenIsNotTheRolesFault(t *testing.T) {
	root := t.TempDir()
	// A file where the folder of the builder's document should be.
	putFile(t, root, "docs/tech", "in the way\n")
	roster := mustRoster(t, testRoster)
	model := &table{answer: agreeable(roster)}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err == nil || !strings.Contains(err.Error(), "the meeting stopped at the documents: documents builder: ") {
		t.Fatalf("err = %v, want the meeting stopped at the builder's documents", err)
	}
	if strings.Contains(err.Error(), "could not be used") {
		t.Errorf("the failure is blamed on the answer: %v", err)
	}
	if got := model.count("builder", "documents"); got != 1 {
		t.Errorf("the builder was asked %d times, want once", got)
	}
	// What the others handed in is written.
	if !strings.Contains(readFile(t, root, "docs/qa/risks.md"), "Written by skeptic.") {
		t.Error("the other roles' documents were not written")
	}

	// With the way clear, going on asks the builder again and writes.
	if err := os.Remove(filepath.Join(root, "docs", "tech")); err != nil {
		t.Fatal(err)
	}
	before := len(model.steps())
	if _, err := Run(context.Background(), Options{Root: root, ID: plan.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow}); err != nil {
		t.Fatalf("going on: %v", err)
	}
	if asked := model.steps()[before:]; len(asked) == 0 || asked[0] != "builder documents" {
		t.Errorf("going on asked %v, want the builder's documents first", asked)
	}
	if !strings.Contains(readFile(t, root, "docs/tech/architecture.md"), "Written by builder.") {
		t.Error("the builder's document was not written when the plan was gone on with")
	}
}

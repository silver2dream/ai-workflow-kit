package council

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// deciding is the agreeable table whose deciding role rules twice and names
// two things only the owner can answer.
func deciding(roster *Roster) func(c call) (string, error) {
	base := agreeable(roster)
	return func(c call) (string, error) {
		if c.Step == "decide" {
			return block(`{"decisions": [
				{"title": "scope", "decision": "The endpoint only.", "rationale": "It is small.", "dissent": ""},
				{"title": "format", "decision": "JSON.", "rationale": "A monitor reads it.", "dissent": "designer wanted text"}],
			  "assumptions": [
				{"question": "Who calls the endpoint?", "assumption": "A readiness probe, every ten seconds.", "if_wrong": "The timeout is set again."},
				{"question": "", "assumption": "The database is PostgreSQL.", "if_wrong": ""}]}`), nil
		}
		return base(c)
	}
}

func names(plan *Plan) string {
	var out []string
	for _, decision := range plan.Decisions {
		out = append(out, decision.ID)
	}
	for _, assumption := range plan.Assumptions {
		out = append(out, assumption.ID)
	}
	return strings.Join(out, " ")
}

// Decisions and assumptions are named by the council, not by the role, and
// each counts on from the last one on record: D2 means one decision for as
// long as the log lasts. What only the owner can answer is kept apart from
// what was decided.
func TestRun_DecisionsAndAssumptionsAreNamedForGood(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	model := &table{answer: deciding(roster)}

	first, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := names(first); got != "D1 D2 A1 A2" {
		t.Errorf("the first plan's names = %q, want D1 D2 A1 A2", got)
	}
	entry := "- **D1 scope**: The endpoint only.\n  - Why: It is small.\n" +
		"- **D2 format**: JSON.\n  - Why: A monitor reads it.\n  - Held otherwise: designer wanted text\n" +
		"\nAssumed until the owner of the project says otherwise:\n\n" +
		"- **A1** Who calls the endpoint?\n  - Assumed: A readiness probe, every ten seconds.\n  - If that is wrong: The timeout is set again.\n" +
		"- **A2** The database is PostgreSQL.\n"
	if log := readFile(t, root, DecisionsPath); !strings.HasSuffix(log, entry) {
		t.Errorf("the decision log should end with\n%s\nit is\n%s", entry, log)
	}
	if minutes := readFile(t, root, first.Dir()+"/minutes.md"); !strings.Contains(minutes, entry) {
		t.Errorf("the minutes do not hold the decisions as the log does:\n%s", minutes)
	}
	// Whoever writes after the ruling is told the names, to refer to them.
	told := "D1. scope: The endpoint only.\n   Why: It is small.\n" +
		"D2. format: JSON.\n   Why: A monitor reads it.\n   Held otherwise: designer wanted text\n" +
		"\nAssumed until the owner of the project says otherwise:\n" +
		"A1. Who calls the endpoint?\n   Assumed: A readiness probe, every ten seconds.\n   If that is wrong: The timeout is set again.\n" +
		"A2. The database is PostgreSQL.\n"
	for _, c := range []struct{ role, step string }{{"skeptic", "documents"}, {"builder", "worklist"}, {"designer", "review"}} {
		if prompt := model.promptOf(c.role, c.step); !strings.Contains(prompt, told) {
			t.Errorf("%s %s is not told the decisions by name:\n%s", c.role, c.step, prompt)
		}
	}

	// The next plan counts on, and its table reads what the first decided.
	next := &table{answer: deciding(roster)}
	second, err := runPlan(t, root, roster, next, "Add a version endpoint.")
	if err != nil {
		t.Fatalf("the second plan: %v", err)
	}
	if got := names(second); got != "D3 D4 A3 A4" {
		t.Errorf("the second plan's names = %q, want D3 D4 A3 A4", got)
	}
	log := readFile(t, root, DecisionsPath)
	if !strings.Contains(log, "- **D3 scope**: The endpoint only.") || strings.Index(log, "- **D2 format**") > strings.Index(log, "- **D3 scope**") {
		t.Errorf("the second plan's decisions do not follow the first's:\n%s", log)
	}
	if position := next.promptOf("skeptic", "position"); !strings.Contains(position, "- **D2 format**: JSON.") || !strings.Contains(position, "- **A1** Who calls the endpoint?") {
		t.Errorf("the second table is not shown what is on record:\n%s", position)
	}

	// Going on with the first plan gives its decisions the names they have,
	// and records nothing twice.
	again, err := Run(context.Background(), Options{Root: root, ID: first.ID, Roster: roster, Model: model, Repos: testRepos, Now: fixedNow})
	if err != nil {
		t.Fatalf("going on with the first plan: %v", err)
	}
	if got := names(again); got != "D1 D2 A1 A2" {
		t.Errorf("gone on with, the first plan's names = %q, want D1 D2 A1 A2", got)
	}
	if after := readFile(t, root, DecisionsPath); after != log {
		t.Errorf("going on with a plan changed the decision log:\n%s", after)
	}
}

// The names count on from the highest one in the whole log, also when a role
// is shown only its end; and what a role is shown of a long log starts at a
// line, in whatever script the log is written.
func TestRun_ALongDecisionLog(t *testing.T) {
	root := t.TempDir()
	var log strings.Builder
	log.WriteString("# Decisions\n\n## 2026-01-05, plan 20260105-first\n\n<!-- plan: 20260105-first -->\n\n- **D900 起點**: 這一條在日誌的最前面。\n")
	for i := 1; i <= 400; i++ {
		fmt.Fprintf(&log, "- **D%d 決策**: 伺服器是唯一的權威，客戶端只負責顯示。\n", i)
	}
	log.WriteString("\nAssumed until the owner of the project says otherwise:\n\n- **A7** 誰會呼叫這個端點？\n  - Assumed: 只有內部的監控。\n")
	putFile(t, root, DecisionsPath, log.String())
	roster := mustRoster(t, testRoster)
	model := &table{answer: deciding(roster)}

	plan, err := runPlan(t, root, roster, model, "加入健康檢查端點。")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := names(plan); got != "D901 D902 A8 A9" {
		t.Errorf("names = %q, want them to count on from D900 and A7", got)
	}
	position := model.promptOf("skeptic", "position")
	if !utf8.ValidString(position) {
		t.Error("the prompt is not valid UTF-8: the log was cut inside a character")
	}
	shown := position[strings.Index(position, "Decisions already on record"):]
	shown = shown[:strings.Index(shown, "\nThe brief:")]
	if !strings.Contains(shown, "(earlier entries left out)\n- **D") || strings.Contains(shown, "D900") {
		t.Errorf("the role should be shown the end of the log, from a whole line:\n%s", shown[:min(400, len(shown))])
	}
	if !strings.HasSuffix(strings.TrimSpace(shown), "- Assumed: 只有內部的監控。") {
		t.Errorf("what is shown does not reach the end of the log:\n%s", shown[max(0, len(shown)-300):])
	}
	if len(shown) > 8200 {
		t.Errorf("the role is shown %d bytes of the log, want about 8000", len(shown))
	}
}

// A role asked for an empty string often says "none" instead. The log does
// not record that as somebody holding otherwise.
func TestRun_NobodyHeldOtherwise(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, testRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		if c.Step == "decide" {
			return block(`{"decisions": [
				{"title": "a", "decision": "A.", "rationale": "r", "dissent": "None."},
				{"title": "b", "decision": "B.", "rationale": "r", "dissent": " 無。"},
				{"title": "c", "decision": "C.", "rationale": "r", "dissent": "N/A"},
				{"title": "d", "decision": "D.", "rationale": "r", "dissent": "nobody but the skeptic"}]}`), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "Add a health endpoint.")

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for i, want := range []string{"", "", "", "nobody but the skeptic"} {
		if got := plan.Decisions[i].Dissent; got != want {
			t.Errorf("decision %d: dissent = %q, want %q", i+1, got, want)
		}
	}
	if log := readFile(t, root, DecisionsPath); strings.Count(log, "Held otherwise:") != 1 || !strings.Contains(log, "- **D4 d**: D.\n  - Why: r\n  - Held otherwise: nobody but the skeptic\n") {
		t.Errorf("the log should record one dissent:\n%s", log)
	}
	for text, want := range map[string]bool{"": true, " - ": true, "No one.": true, "沒有": true, "none of the above applies": false, "architect": false} {
		if got := saysNobody(text); got != want {
			t.Errorf("saysNobody(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestHeadOfAndTailOf(t *testing.T) {
	text := "第一行\n第二行\n第三行\n" // 10 bytes a line
	tests := []struct {
		max                int
		wantHead, wantTail string
		wantCut            bool
	}{
		{100, text, text, false},
		{30, text, text, false},
		{25, "第一行\n第二行\n", "第二行\n第三行\n", true},
		{12, "第一行\n", "第三行\n", true},
		// Shorter than a line: the head is cut at a character, never inside
		// one; no whole line is left of the tail.
		{5, "第", "", true},
	}
	for _, tt := range tests {
		head, cut := headOf(text, tt.max)
		if head != tt.wantHead || cut != tt.wantCut {
			t.Errorf("headOf(%d) = %q, %v; want %q, %v", tt.max, head, cut, tt.wantHead, tt.wantCut)
		}
		tail, cut := tailOf(text, tt.max)
		if tail != tt.wantTail || cut != tt.wantCut {
			t.Errorf("tailOf(%d) = %q, %v; want %q, %v", tt.max, tail, cut, tt.wantTail, tt.wantCut)
		}
		if !utf8.ValidString(head) || !utf8.ValidString(tail) {
			t.Errorf("max %d: head %q or tail %q is cut inside a character", tt.max, head, tail)
		}
	}
	// One long line and no newline to cut at.
	if head, cut := headOf("一二三四五", 7); head != "一二" || !cut {
		t.Errorf("headOf of one long line = %q, %v", head, cut)
	}
	if tail, cut := tailOf("一二三四五", 7); tail != "四五" || !cut {
		t.Errorf("tailOf of one long line = %q, %v", tail, cut)
	}
}

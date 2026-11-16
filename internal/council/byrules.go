package council

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/rules"
)

// The documents of a plan are written by the project's rules: the rule files
// under .claude/rules that hold for the files a role writes, the ones Claude
// Code would load for somebody working on those files. A role is given them
// when it writes. What it wrote is then read against them by a reader who
// takes no side, and the role puts right what that reader found, once. What
// still stands after that is on record, for the person who approves the plan.
//
// The rules are the project's own. The ones AWK ships (layout.KitRulesDir)
// say how an agent works with git, which no role does: they are not passed
// on, and a project that has no other rule has its documents written without.

// maxRules is how much rule text, in bytes, one question carries. Rules that
// do not fit are named and not shown.
const maxRules = 60000

// maxFindings is how many findings of one check are kept.
const maxFindings = 8

// RuleFinding is a point where a document is at odds with a rule of the
// project.
type RuleFinding struct {
	// Owner is the role that owns the document.
	Owner string `json:"-"`
	// Document is the document's path, and Rule the rule's file.
	Document string `json:"document"`
	Rule     string `json:"rule"`
	// Point says what is at odds, and Fix what would put it right.
	Point string `json:"point"`
	Fix   string `json:"fix"`
}

// rulebook is the rules that hold for a set of files: the ones a question
// carries, and the ones that hold too and did not fit.
type rulebook struct {
	shown, left []rules.Rule
}

// rulesFor picks the project's rules that hold for files. When they are
// together longer than a question carries, a rule that names these files
// comes before one that is about everything.
func (p *planner) rulesFor(files []string) rulebook {
	var applying []rules.Rule
	for _, rule := range rules.For(p.rules, files...) {
		if strings.TrimSpace(rule.Text) != "" && !OfTheKit(rule) {
			applying = append(applying, rule)
		}
	}
	fits := make([]bool, len(applying))
	room := maxRules
	for _, everywhere := range []bool{false, true} {
		for i, rule := range applying {
			if rule.Everywhere() == everywhere && len(rule.Text) <= room {
				fits[i] = true
				room -= len(rule.Text)
			}
		}
	}
	var book rulebook
	for i, rule := range applying {
		if fits[i] {
			book.shown = append(book.shown, rule)
		} else {
			book.left = append(book.left, rule)
		}
	}
	return book
}

// OfTheKit reports whether a rule is one AWK ships, not one of the project's
// own: the council does not write by it.
func OfTheKit(rule rules.Rule) bool {
	return strings.HasPrefix(rule.File, layout.KitRulesDir+"/")
}

// files lists the rule files a question carries.
func (book rulebook) files() []string {
	names := make([]string, len(book.shown))
	for i, rule := range book.shown {
		names[i] = rule.File
	}
	return names
}

// named returns the rule a finding names, by what a reader would call it: its
// file, the end of the file's path, the name of the file with or without .md,
// or its title. It is found when that is one rule.
func (book rulebook) named(name string) (rules.Rule, bool) {
	name = strings.TrimPrefix(strings.Trim(strings.TrimSpace(name), "`\"'"), "./")
	var found []rules.Rule
	for _, rule := range book.shown {
		base := path.Base(rule.File)
		if rule.File == name || strings.HasSuffix(rule.File, "/"+name) || strings.EqualFold(base, name) ||
			strings.EqualFold(strings.TrimSuffix(base, ".md"), name) || strings.EqualFold(rule.Title, name) {
			found = append(found, rule)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return rules.Rule{}, false
}

// text shows the rules to whoever writes or reads by them.
func (book rulebook) text() string {
	var b strings.Builder
	for _, rule := range book.shown {
		about := "every file of the project"
		if !rule.Everywhere() {
			about = "the files that match " + strings.Join(rule.Paths, ", ")
		}
		fmt.Fprintf(&b, "<rule file=%q about=%q>\n%s\n</rule>\n\n", rule.File, about, strings.TrimSpace(rule.Text))
	}
	if len(book.left) > 0 {
		b.WriteString("These rules hold too. They are not shown, because the rules together are longer than one question carries:\n")
		for _, rule := range book.left {
			fmt.Fprintf(&b, "- %s (%s)\n", rule.File, rule.Title)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// facts says what a rule may ask a document to state, and what nobody at the
// table can vouch for.
func (p *planner) facts(role *Role, plan *Plan) string {
	return fmt.Sprintf(`What holds for these documents, wherever a rule asks a document to say it:
- their owner: %s (%s)
- today's date: %s
- the plan they are written in: %s
- how far things are: this is a planning meeting. Nothing this plan decides is built yet, and nobody at the table read the code or ran anything, so nothing was verified against an implementation today. What a document said was built before this meeting, it may go on saying.
`, role.ID, role.Title, p.today(), plan.ID)
}

// byTheRules tells a role that writes documents which rules hold for them.
func (p *planner) byTheRules(role *Role, plan *Plan, book rulebook) string {
	if len(book.shown) == 0 {
		return fmt.Sprintf("\nStart every document you write with its title, and under it this line:\nOwner: %s. Written: %s, in plan %s.\n", role.ID, p.today(), plan.ID)
	}
	return "\nThe project has rules (" + layout.RulesDir + "/). These hold for the files you write, and your documents follow them: where a rule says where a document is kept, how it starts, which state it may claim, what it has to answer, what does not belong in it or how long it may get, write it so. A rule about code, commits or tools is the frame the work is done in: design nothing against it, and do not copy it into a document. A document does not quote the rules and does not say that it follows them.\n\n" +
		book.text() + p.facts(role, plan) +
		fmt.Sprintf("\nWhere no rule says how a document starts, start it with its title, and under it this line:\nOwner: %s. Written: %s, in plan %s.\n", role.ID, p.today(), plan.ID)
}

// checkerFrame opens the question to the reader who checks documents against
// the rules. Like the moderator it is played with the roster's model and has
// no stance.
const checkerFrame = "You check documents against the rules of a software project. You have no opinion on their subject, and whether the design is good is not yours to say: a table of roles argued that out and decided. You say where a document does not do what a rule asks of it, and nothing else."

// documentsAsTheyStand shows documents whole, as they are on disk.
func (p *planner) documentsAsTheyStand(docs []string) string {
	var b strings.Builder
	for _, doc := range docs {
		shown, cut := headOf(p.read(doc), maxDocument)
		fmt.Fprintf(&b, "===== %s\n%s\n", doc, strings.TrimRight(shown, "\n"))
		if cut {
			b.WriteString("(cut here; the file has the rest)\n")
		}
	}
	return b.String()
}

// ruleCheckPrompt asks the reader where a role's documents are at odds with
// the rules.
func (p *planner) ruleCheckPrompt(role *Role, plan *Plan, docs []string, book rulebook) string {
	var b strings.Builder
	b.WriteString(checkerFrame + "\n\n" + p.languageLine() + "\n\n")
	fmt.Fprintf(&b, "The documents below were written at a planning meeting by their owner, the %s (%s). The owner writes these files and no other:\n", role.Title, role.ID)
	for _, doc := range role.Owns {
		fmt.Fprintf(&b, "- %s\n", doc)
	}
	b.WriteString("\n" + p.facts(role, plan))
	b.WriteString("\nThe project's rules that hold for these files:\n\n" + book.text())
	b.WriteString("The documents, as they stand:\n\n" + p.documentsAsTheyStand(docs))
	fmt.Fprintf(&b, `
Your task: name each point where one of these documents is at odds with one of these rules. A document is at odds with a rule when it is kept where the rule says such a document is not kept, starts otherwise than the rule says, claims a state that what holds for it (above) does not carry, leaves out what the rule says such a document has to answer, holds what the rule keeps out of it, is longer than the rule allows, or prescribes what a rule about code forbids. Go by the words of the rule and quote them. A rule asks nothing of a document it is not about: a rule about commits says nothing on a design document. What cannot be put right in these documents (the rule asks for a file the owner does not write, say) is a point too; say so in fix.

Do not look for points: when the documents do what the rules ask, the list is empty.

%s
{"findings": [{"document": "the document's path", "rule": "the rule's file", "point": "what is at odds, with the words of the rule", "fix": "what to change in the document"}]}
At most %d findings, the ones that matter most first.
`, jsonOnly, maxFindings)
	return b.String()
}

// checkRules has a role's documents read against the rules and returns what
// the reader found.
func (p *planner) checkRules(ctx context.Context, step string, role *Role, plan *Plan, docs []string, book rulebook) ([]RuleFinding, error) {
	type answer struct {
		Findings []RuleFinding `json:"findings"`
	}
	got, err := settleJSON(ctx, p.session, step, p.roster.Model, p.ruleCheckPrompt(role, plan, docs, book), func(got *answer) error {
		for i := range got.Findings {
			finding := &got.Findings[i]
			finding.Document = strings.Trim(strings.TrimSpace(finding.Document), "`")
			if !contains(docs, finding.Document) {
				return fmt.Errorf("a finding is about %q, which is not one of the documents (%s)", finding.Document, strings.Join(docs, ", "))
			}
			rule, ok := book.named(finding.Rule)
			if !ok {
				return fmt.Errorf("a finding names the rule %q, which is not one of the rules shown (%s)", finding.Rule, strings.Join(book.files(), ", "))
			}
			finding.Rule = rule.File
			if strings.TrimSpace(finding.Point) == "" {
				return errors.New("a finding has no point")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	findings := got.Findings
	if len(findings) == 0 {
		return nil, nil
	}
	if len(findings) > maxFindings {
		findings = findings[:maxFindings]
	}
	for i := range findings {
		findings[i].Owner = role.ID
	}
	return findings, nil
}

// findingsText lists findings for the role that puts them right.
func findingsText(findings []RuleFinding) string {
	var b strings.Builder
	for i, finding := range findings {
		fmt.Fprintf(&b, "%d. %s, against %s: %s\n", i+1, finding.Document, finding.Rule, oneLine(finding.Point))
		if strings.TrimSpace(finding.Fix) != "" {
			fmt.Fprintf(&b, "   To put right: %s\n", oneLine(finding.Fix))
		}
	}
	return b.String()
}

// revisionPrompt asks a role to put right what the check of its documents
// found. named are the documents the findings are about: the role answers for
// each of them.
func (p *planner) revisionPrompt(role *Role, plan *Plan, book rulebook, findings []RuleFinding) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(p.standing(role))
	b.WriteString("\nThe meeting is over. What was decided:\n")
	b.WriteString(decisionsText(plan))
	b.WriteString("\nThe project's rules that hold for the files you write (" + layout.RulesDir + "/):\n\n" + book.text() + p.facts(role, plan))
	b.WriteString("\nYour documents were read against these rules. What the check found:\n" + findingsText(findings))
	b.WriteString(`
Your task now: put right what the check found. A finding that reads the rule the way it is written, you take in. One that misreads the rule, or asks for what is not yours to do (a file you do not own), you leave: change nothing for it. Change nothing else in a document: what the meeting decided stands, and so does every word the findings are not about.

Hand in each document you change, whole, between these two lines:
<<<DOCUMENT path/of/the/document.md>>>
(the whole document)
<<<END>>>
For a document a finding names and you leave as it is, answer with this single line instead:
<<<UNCHANGED path/of/the/document.md>>>
Answer with nothing else.
`)
	return b.String()
}

// documentsOf lists the documents findings are about, each once, in the order
// they are named.
func documentsOf(findings []RuleFinding) []string {
	var docs []string
	for _, finding := range findings {
		if !contains(docs, finding.Document) {
			docs = append(docs, finding.Document)
		}
	}
	return docs
}

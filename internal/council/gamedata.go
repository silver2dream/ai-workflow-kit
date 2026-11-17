package council

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/gamedata"
)

// The game data: the numbers and the text the game reads, kept as the tables
// Luban turns into the client's and the server's data (internal/gamedata).
// A topic that settles numbers or text ends in them, written by the roles
// that own them. The tables are written first, then the text of the keys
// they hold; what the check of the game data finds goes back to the owners,
// once, and what still stands is on record. A person reads what was written
// on two pages of the plan: the copy, every language side by side, and the
// tables.

// CopyFile and TablesFile are the pages of a plan that show what it wrote to
// the game data, for a person to read and review.
const (
	CopyFile   = "copy.md"
	TablesFile = "tables.md"
)

// DataCheck is what the check of the game data came to for what a topic
// wrote.
type DataCheck struct {
	// Found is what the check found before the owners put anything right.
	Found []gamedata.Finding
	// Open is what it still finds after their turn.
	Open []gamedata.Finding
	// Settled says how it ended is known: nothing was found, or the owners
	// had their turn and the data was checked again. It is false when the
	// meeting stopped in between.
	Settled bool
}

// findGameData looks for the project's game data. A project without any has
// nothing checked; one whose luban.conf cannot be read is told so and has
// nothing checked either: the meeting goes on.
func (p *planner) findGameData() {
	found, err := gamedata.Find(os.DirFS(p.root))
	if err != nil {
		p.session.say("  game data: %v; what the council writes there is not checked", err)
		return
	}
	p.data = found
}

// gameDataNote tells the table where the game data is kept.
func (p *planner) gameDataNote() string {
	if p.data == nil {
		return ""
	}
	schemas := strings.Join(p.data.Schemas, ", ")
	return fmt.Sprintf("\nThe game data is kept in %s, as Luban reads it (%s): the schema of the tables in %s, one table per CSV file in %s, and the text the player reads in %s, by key. The numbers and the text the game reads are written there, not in a document: a document says why and names the table. A topic that settles numbers or text ends in those files.\n",
		p.data.Dir, p.data.Conf(), schemas, p.data.Data, p.data.Text())
}

// gameDataTopics tells the moderator which topics end in the game data: the
// numbers the game reads are written in tables, not in prose, and the words
// the player reads in text tables. Empty when the project keeps none.
func (p *planner) gameDataTopics() string {
	if p.data == nil {
		return ""
	}
	schema := p.data.Dir + "Defines/"
	for _, s := range p.data.Schemas {
		if strings.HasSuffix(s, "/") {
			schema = s
			break
		}
	}
	return fmt.Sprintf("- A topic that settles numbers the game reads (stats, costs, odds, timings, curves) ends, besides the document that says why, in the tables that hold them: a schema in %s and a table in %s, named for what they hold (%smatch.xml and %smatch_rule.csv, say). A topic that settles words the player reads (names, the words on a screen, messages) ends in a text table in %s. Each is written by the role that owns its folder, which then leads or speaks to the topic.\n",
		schema, p.data.Data, schema, p.data.Data, p.data.Text())
}

// isGameData reports whether a file is part of the project's game data.
func (p *planner) isGameData(file string) bool {
	return p.data != nil && p.data.Holds(file)
}

// textOwner is the role that writes the text tables; nil when none does.
func (p *planner) textOwner() *Role {
	if p.data == nil {
		return nil
	}
	return p.roster.Owner(p.data.Text() + "text.csv")
}

// waves orders the seats of a topic: the ones that write the tables, then the
// ones that write the text of the keys the tables hold. Without game data, or
// when only one kind is written, all write at once.
func (p *planner) waves(seats []*seat) [][]*seat {
	if p.data == nil {
		return [][]*seat{seats}
	}
	var tables, texts []*seat
	for _, at := range seats {
		writesText := false
		for _, doc := range at.asked {
			if p.data.IsText(doc) {
				writesText = true
			}
		}
		if writesText {
			texts = append(texts, at)
		} else {
			tables = append(tables, at)
		}
	}
	if len(tables) == 0 || len(texts) == 0 {
		return [][]*seat{seats}
	}
	return [][]*seat{tables, texts}
}

// writtenData lists the files of the game data among what was written.
func (p *planner) writtenData(written []WrittenDocument) []string {
	var files []string
	for _, doc := range writtenBy(written) {
		if p.isGameData(doc) {
			files = append(files, doc)
		}
	}
	sort.Strings(files)
	return files
}

// dataFindings checks the game data and keeps what the files given are
// answerable for: what is found in them, in the other files of a table one of
// them is part of, and, where a text table is among them, every key a table
// holds that the text tables do not have.
func (p *planner) dataFindings(files []string) (*gamedata.Data, []gamedata.Finding, error) {
	d, findings, err := gamedata.Check(os.DirFS(p.root), p.data)
	if err != nil {
		return nil, nil, err
	}
	touched := map[string]bool{}
	text := false
	for _, file := range files {
		touched[file] = true
		text = text || p.data.IsText(file)
	}
	for _, t := range d.Tables {
		group := append([]string{t.File}, t.Inputs...)
		if t.Bean != nil {
			group = append(group, t.Bean.File)
		}
		hit := false
		for _, file := range group {
			hit = hit || contains(files, file)
		}
		if hit {
			for _, file := range group {
				touched[file] = true
			}
		}
	}
	var kept []gamedata.Finding
	for _, f := range findings {
		if touched[f.File] || text && f.IsMissingKey() {
			kept = append(kept, f)
		}
	}
	return d, kept, nil
}

// missingKeys lists the findings that are keys the text tables lack.
func missingKeys(findings []gamedata.Finding) []gamedata.Finding {
	var keys []gamedata.Finding
	for _, f := range findings {
		if f.IsMissingKey() {
			keys = append(keys, f)
		}
	}
	return keys
}

// keysFor tells the seats that write the text which keys the tables written
// so far hold and the text tables lack.
func (p *planner) keysFor(wave []*seat, written []string) error {
	if len(written) == 0 {
		return nil
	}
	_, findings, err := p.dataFindings(written)
	if err != nil {
		return err
	}
	keys := missingKeys(findings)
	owner := p.textOwner()
	for _, at := range wave {
		if owner != nil && at.role.ID == owner.ID {
			at.keys, at.tables = keys, written
		}
	}
	return nil
}

// checkData checks the game data a topic wrote. What is found goes back to
// the owners of the files, once: a key the text tables lack to the role that
// writes the text. What the check still finds after their turn is on record.
func (p *planner) checkData(ctx context.Context, plan *Plan, r *TopicRecord) error {
	if p.data == nil {
		return nil
	}
	written := p.writtenData(r.Documents)
	if len(written) == 0 {
		r.DataCheck = nil
		return nil
	}
	_, found, err := p.dataFindings(written)
	if err != nil {
		return err
	}
	r.DataCheck = &DataCheck{Found: found}
	if len(found) == 0 {
		r.DataCheck.Settled = true
		return nil
	}

	// Each finding goes to the role that owns its file, a missing key to the
	// role that writes the text. What nobody owns stays as it was found.
	owners := map[string][]gamedata.Finding{}
	var order []string
	text := p.textOwner()
	for _, f := range found {
		owner := p.roster.Owner(f.File)
		if f.IsMissingKey() && text != nil {
			owner = text
		}
		if owner == nil {
			continue
		}
		if _, ok := owners[owner.ID]; !ok {
			order = append(order, owner.ID)
		}
		owners[owner.ID] = append(owners[owner.ID], f)
	}
	p.session.say("  game data: %d point(s) the check found; the owners put them right", len(found))
	handedIn := make([][]WrittenDocument, len(order))
	err = p.session.each(len(order), func(i int) error {
		role := p.roster.Role(order[i])
		var err error
		handedIn[i], err = p.putDataRight(ctx, plan, r, role, owners[role.ID])
		return err
	})
	// What was handed in is on record, also when a turn failed.
	for _, docs := range handedIn {
		r.Documents = mergeWritten(r.Documents, docs)
		for _, file := range writtenBy(docs) {
			if !contains(written, file) {
				written = append(written, file)
			}
		}
	}
	if err != nil {
		return err
	}
	_, open, err := p.dataFindings(written)
	if err != nil {
		return err
	}
	r.DataCheck.Open, r.DataCheck.Settled = open, true
	if len(open) > 0 {
		p.session.say("  game data: %d point(s) still found", len(open))
	}
	return nil
}

// mergeWritten adds to what became of a topic's documents what became of the
// ones handed in again: one written now that was left as it was is written,
// one written for the first time is added, one turned away is on record, and
// one left as it was again changes nothing.
func mergeWritten(documents, again []WrittenDocument) []WrittenDocument {
	for _, doc := range again {
		switch {
		case doc.Refused != "":
			documents = append(documents, doc)
		case doc.Unchanged:
		default:
			known := false
			for i := range documents {
				if documents[i].Path == doc.Path && documents[i].Refused == "" {
					documents[i].Unchanged, known = false, true
				}
			}
			if !known {
				documents = append(documents, doc)
			}
		}
	}
	return documents
}

// putDataRight has a role put right what the check of the game data found in
// its files, and returns what became of the files it handed in.
func (p *planner) putDataRight(ctx context.Context, plan *Plan, r *TopicRecord, role *Role, findings []gamedata.Finding) ([]WrittenDocument, error) {
	var asked, must []string
	add := func(doc string, answer bool) {
		if contains(asked, doc) || !role.OwnsDocument(doc) || p.tooLong(doc) != "" {
			return
		}
		asked = append(asked, doc)
		if answer {
			must = append(must, doc)
		}
	}
	for _, f := range findings {
		// A key the text tables lack is put right in them, or, by the role
		// that writes the table, in the table: it need not answer for the
		// table then.
		add(f.File, !f.IsMissingKey())
	}
	if keys := missingKeys(findings); len(keys) > 0 {
		// The text of a key goes into a text table of the role's, or into a
		// new one named after the table that holds the key.
		for _, doc := range p.documentsOf(role) {
			if p.data.IsText(doc) {
				add(doc, false)
			}
		}
		for _, f := range keys {
			add(p.data.Text()+path.Base(f.File), false)
		}
	}
	if len(asked) == 0 {
		return nil, nil
	}
	book := p.rulesFor(asked)
	return p.handIn(ctx, r.step("game data revised "+role.ID), role, p.dataRevisionPrompt(role, plan, r, asked, book, findings), asked,
		must, func(doc string) error {
			return fmt.Errorf("%s was not answered for, and the check names it: hand it in, or leave it as it is with <<<UNCHANGED %s>>>", doc, doc)
		})
}

// dataFindingsText lists what the check of the game data found.
func dataFindingsText(findings []gamedata.Finding) string {
	var b strings.Builder
	for i, f := range findings {
		fmt.Fprintf(&b, "%d. %s\n", i+1, f)
	}
	return b.String()
}

// keysText lists the keys the tables hold and the text tables lack, with the
// row each is in.
func keysText(findings []gamedata.Finding) string {
	var b strings.Builder
	for _, f := range findings {
		at := f.File
		if f.Line > 0 {
			at = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(&b, "- %s (%s)\n", f.Key, at)
	}
	return b.String()
}

// gameDataForm tells a role that writes files of the game data what they are.
const gameDataForm = `
A table of the game data (.csv) and its schema (.xml) are not documents: they are what the game reads, written in the form the project's rules give for them, and in no other. They hold no title, no prose and no note outside what that form has room for. Write each one whole, every row of it, also the rows this topic does not change. After the topic the game data is checked; what the check finds comes back to you.
`

// dataRevisionPrompt asks a role to put right what the check of the game data
// found in its files.
func (p *planner) dataRevisionPrompt(role *Role, plan *Plan, r *TopicRecord, asked []string, book rulebook, findings []gamedata.Finding) string {
	var b strings.Builder
	b.WriteString(p.frame(role))
	b.WriteString(topicHead(plan, r))
	b.WriteString("\nWhat this meeting has decided, up to and with this topic:\n")
	b.WriteString(throughTopic(plan, r))
	b.WriteString("\nYour files of the game data as they stand:\n" + p.asTheyStand(asked))
	if tables := p.tablesHolding(findings); len(tables) > 0 {
		b.WriteString("\nThe tables that hold the keys:\n" + p.reading(tables))
	}
	b.WriteString(gameDataForm)
	if len(book.shown) > 0 {
		b.WriteString("\nThe project's rules that hold for these files:\n\n" + book.text())
	}
	b.WriteString("\nThe game data was checked. What the check found:\n" + dataFindingsText(findings))
	b.WriteString(`
Your task now: put right what the check of the game data found, in your files. A key the text tables lack: write its text, in the language the text is written in and in each language it is translated into, in one of the text tables above. Change nothing else: what the meeting decided stands, and so does every row the findings are not about.

Hand in each file you change, whole, between these two lines:
<<<DOCUMENT path/of/the/file>>>
(the whole file)
<<<END>>>
For a file a finding names and you leave as it is, answer with this single line instead:
<<<UNCHANGED path/of/the/file>>>
Answer with nothing else.
`)
	return b.String()
}

// tablesHolding lists the tables that hold the keys among findings.
func (p *planner) tablesHolding(findings []gamedata.Finding) []string {
	var tables []string
	for _, f := range missingKeys(findings) {
		if !contains(tables, f.File) {
			tables = append(tables, f.File)
		}
	}
	return tables
}

// reviewGameData writes the pages that show what the plan wrote to the game
// data, for a person to read and review, and keeps what the check finds in
// it now: a later topic may have put right what an earlier one left.
func (p *planner) reviewGameData(plan *Plan) error {
	plan.GameData, plan.DataOpen, plan.Reviews = nil, nil, nil
	if p.data == nil {
		return nil
	}
	written := p.writtenData(plan.Documents)
	if len(written) == 0 {
		return nil
	}
	plan.GameData = written
	d, open, err := p.dataFindings(written)
	if err != nil {
		return err
	}
	plan.DataOpen = open
	if texts := p.data.TextsOf(written); len(texts) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "# Plan %s: the copy\n\n", plan.ID)
		b.WriteString("> The text this plan wrote, as the player reads it, for a person to review: every language side by side, where each key is used, and what to look at. Written by awkit from the text tables; change the text there, not here.\n\n")
		b.WriteString(d.CopyReview(texts, open))
		if err := p.writeFile(plan.Dir()+"/"+CopyFile, b.String()); err != nil {
			return err
		}
		plan.Reviews = append(plan.Reviews, plan.Dir()+"/"+CopyFile)
	}
	if tables := p.data.TablesOf(written); len(tables) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "# Plan %s: the tables\n\n", plan.ID)
		b.WriteString("> The tables of the game data this plan wrote, for a person to review: what each column holds, each text key with its text, and what to look at. Written by awkit from the tables; change them there, not here.\n\n")
		b.WriteString(d.DataReview(tables, open))
		if err := p.writeFile(plan.Dir()+"/"+TablesFile, b.String()); err != nil {
			return err
		}
		plan.Reviews = append(plan.Reviews, plan.Dir()+"/"+TablesFile)
	}
	return nil
}

// writeDataCheck puts on record what the check of the game data came to.
func writeDataCheck(b *strings.Builder, check *DataCheck) {
	if check == nil {
		return
	}
	b.WriteString("\n### The check of the game data\n\n")
	switch {
	case len(check.Found) == 0 && check.Settled:
		b.WriteString("Nothing found.\n")
		return
	case !check.Settled:
		fmt.Fprintf(b, "%d point(s) found; **the meeting stopped before they were put right and checked again**.\n", len(check.Found))
	case len(check.Open) == 0:
		fmt.Fprintf(b, "%d point(s) found and put right.\n", len(check.Found))
	default:
		fmt.Fprintf(b, "%d point(s) found; **%d still found** after the owners' turn.\n", len(check.Found), len(check.Open))
	}
	b.WriteString("\n")
	for _, f := range check.Found {
		fmt.Fprintf(b, "- Found: %s\n", f)
	}
	for _, f := range check.Open {
		fmt.Fprintf(b, "- **Still**: %s\n", f)
	}
}

// writeGameData puts the game data the plan wrote on record in its minutes:
// the pages to review it on, and what the check finds in it now.
func writeGameData(b *strings.Builder, plan *Plan) {
	if len(plan.GameData) == 0 {
		return
	}
	b.WriteString("\n## Game data\n\n")
	fmt.Fprintf(b, "Written in this plan: %s.\n", strings.Join(quoted(plan.GameData), ", "))
	if len(plan.Reviews) > 0 {
		var links []string
		for _, page := range plan.Reviews {
			links = append(links, "["+path.Base(page)+"]("+path.Base(page)+")")
		}
		fmt.Fprintf(b, "\nTo review it: %s.\n", strings.Join(links, ", "))
	}
	if len(plan.DataOpen) == 0 {
		b.WriteString("\nThe check of the game data finds nothing in it.\n")
		return
	}
	fmt.Fprintf(b, "\nThe check of the game data still finds %d point(s) in it:\n\n", len(plan.DataOpen))
	for _, f := range plan.DataOpen {
		fmt.Fprintf(b, "- %s\n", f)
	}
}

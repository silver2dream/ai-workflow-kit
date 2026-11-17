package council

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// seat is one role's turn at the documents of a topic: what it is asked for,
// the rules that hold for those files, and what came of it.
type seat struct {
	role *Role
	// asked are the documents of the topic the role owns and the council can
	// write.
	asked []string
	book  rulebook
	// written is what became of each document.
	written []WrittenDocument
	// check is what reading them against the rules came to; nil when nothing
	// was read.
	check *RuleCheck
}

// writeDown has the owners write the documents a topic ends in, by the
// project's rules for those files. What a role wrote is read against the
// rules, and the role puts right what was found, once. Only this function
// writes the documents, and only to the paths the role owns.
func (p *planner) writeDown(ctx context.Context, plan *Plan, r *TopicRecord) error {
	// What each role is asked for: its documents of the topic, without the
	// ones that are too long for the council to write.
	var seats []*seat
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		at := &seat{role: role}
		for _, doc := range r.Topic.Writes {
			if !role.OwnsDocument(doc) {
				continue
			}
			if reason := p.tooLong(doc); reason != "" {
				at.written = append(at.written, WrittenDocument{Path: doc, Owner: role.ID, Refused: reason})
				continue
			}
			at.asked = append(at.asked, doc)
		}
		if len(at.asked)+len(at.written) == 0 {
			continue
		}
		at.book = p.rulesFor(at.asked)
		seats = append(seats, at)
	}

	r.Rules, r.RulesLeft = nil, nil
	for _, at := range seats {
		for _, rule := range at.book.shown {
			if !contains(r.Rules, rule.File) {
				r.Rules = append(r.Rules, rule.File)
			}
		}
		for _, rule := range at.book.left {
			if !contains(r.RulesLeft, rule.File) {
				r.RulesLeft = append(r.RulesLeft, rule.File)
			}
		}
	}
	sort.Strings(r.Rules)
	sort.Strings(r.RulesLeft)

	if len(r.Rules) == 0 {
		p.session.say("  documents: %s", strings.Join(r.Topic.Writes, ", "))
	} else {
		p.session.say("  documents: %s, by %d rule(s) of the project", strings.Join(r.Topic.Writes, ", "), len(r.Rules))
	}
	if len(r.RulesLeft) > 0 {
		p.session.say("  not passed on, the rules together being longer than one question carries: %s", strings.Join(r.RulesLeft, ", "))
	}

	err := p.session.each(len(seats), func(i int) error {
		return p.write(ctx, plan, r, seats[i])
	})

	// What was written is on record, also when a role's turn failed.
	r.Documents, r.RuleChecks = nil, nil
	for _, at := range seats {
		r.Documents = append(r.Documents, at.written...)
		if at.check != nil {
			r.RuleChecks = append(r.RuleChecks, *at.check)
		}
	}
	return err
}

// write is one role's turn at the documents of a topic: it writes them, they
// are read against the rules, it puts right what was found, and the reader
// says which of the findings still stand.
func (p *planner) write(ctx context.Context, plan *Plan, r *TopicRecord, at *seat) error {
	role := at.role
	if len(at.asked) == 0 {
		return nil
	}
	written, err := p.handIn(ctx, r.step("documents "+role.ID), role, p.documentsPrompt(role, plan, r, at.asked, at.book), at.asked,
		at.asked, func(doc string) error {
			return fmt.Errorf("%s was not handed in; this topic ends in it, and it is yours", doc)
		})
	if err != nil {
		return err
	}
	at.written = append(at.written, written...)

	// Only what this meeting wrote is read against the rules: a document its
	// owner left as it was is not this meeting's.
	read := writtenBy(written)
	if len(at.book.shown) == 0 || len(read) == 0 {
		return nil
	}
	at.check = &RuleCheck{Owner: role.ID}
	found, err := p.checkRules(ctx, r.step("rules check "+role.ID), role, plan, read, at.book)
	if err != nil {
		return err
	}
	at.check.Found = found
	if len(found) == 0 {
		at.check.Settled = true
		return nil
	}

	p.session.say("  %s: %d point(s) at odds with the rules; %s puts them right", role.ID, len(found), role.ID)
	revised, err := p.handIn(ctx, r.step("documents revised "+role.ID), role, p.revisionPrompt(role, plan, r, at.asked, at.book, found), at.asked,
		documentsOf(found), func(doc string) error {
			return fmt.Errorf("%s was not answered for, and a finding names it: hand it in, or leave it as it is with <<<UNCHANGED %s>>>", doc, doc)
		})
	if err != nil {
		return err
	}
	changed := false
	for _, doc := range revised {
		switch {
		case doc.Refused != "":
			at.written = append(at.written, doc)
		case !doc.Unchanged:
			changed = true
			for i := range at.written {
				if at.written[i].Path == doc.Path && at.written[i].Refused == "" {
					at.written[i].Unchanged = false
				}
			}
		}
	}
	if !changed {
		// The owner left every document as it was: what was found stands.
		at.check.Open, at.check.Settled = found, true
		return nil
	}
	open, err := p.recheckRules(ctx, r.step("rules recheck "+role.ID), role, plan, writtenBy(at.written), at.book, found)
	if err != nil {
		return err
	}
	at.check.Open, at.check.Settled = open, true
	if len(open) > 0 {
		p.session.say("  %s: %d point(s) still at odds with the rules", role.ID, len(open))
	}
	return nil
}

// writtenBy lists the documents a meeting wrote: not the ones left as they
// were, and not the ones that were turned away.
func writtenBy(written []WrittenDocument) []string {
	var docs []string
	for _, doc := range written {
		if doc.Refused == "" && !doc.Unchanged && !contains(docs, doc.Path) {
			docs = append(docs, doc.Path)
		}
	}
	return docs
}

// handIn settles an answer that holds a role's documents: it reads them,
// turns away what the role does not own, and writes the rest. asked are the
// documents the role may hand in. must are the ones it has to answer for,
// with the document or with the line that leaves it as it is; missing says
// what is wrong when it does not.
func (p *planner) handIn(ctx context.Context, step string, role *Role, prompt string, asked, must []string, missing func(doc string) error) ([]WrittenDocument, error) {
	var written []WrittenDocument
	err := p.session.settle(ctx, step, role.Model, prompt, func(answer string, replay bool) error {
		docs, err := parseDocuments(answer)
		if err != nil {
			return err
		}
		var accepted []document
		written = nil
		handedIn := map[string]bool{}
		for _, doc := range docs {
			switch {
			case !role.OwnsDocument(doc.Path):
				// Not an error of form: the document is left out and the
				// minutes say so. A role does not get to write what
				// another role owns, however it asks.
				written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID,
					Refused: fmt.Sprintf("%s does not own this document", role.ID)})
				continue
			case !contains(asked, doc.Path) && p.tooLong(doc.Path) != "":
				continue // too long to be written: it stays, and is on record as left
			case !contains(asked, doc.Path):
				// Its own, and not what this topic ends in: it stays as it is.
				written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID,
					Refused: "this topic does not end in it"})
				continue
			case handedIn[doc.Path]:
				return fmt.Errorf("%s was handed in twice", doc.Path)
			}
			handedIn[doc.Path] = true
			switch {
			case doc.Unchanged && strings.TrimSpace(p.read(doc.Path)) == "":
				return fmt.Errorf("%s is not written yet, so it cannot stay unchanged: write it", doc.Path)
			case doc.Unchanged:
				written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID, Unchanged: true})
			case strings.TrimSpace(doc.Content) == "":
				return fmt.Errorf("%s is empty", doc.Path)
			case len(doc.Content) > maxDocument:
				return fmt.Errorf("%s is %d bytes long; a document stays under %d bytes, so that it can be rewritten whole: shorten it, or end the topic in two documents", doc.Path, len(doc.Content), maxDocument)
			default:
				accepted = append(accepted, doc)
				written = append(written, WrittenDocument{Path: doc.Path, Owner: role.ID})
			}
		}
		for _, owned := range must {
			if !handedIn[owned] {
				return missing(owned)
			}
		}
		// An answer from an earlier run was written then. What is on disk
		// now may have been changed by hand since, and stays.
		if replay {
			return nil
		}
		for _, doc := range accepted {
			if err := p.writeFile(doc.Path, doc.Content); err != nil {
				return broken{err}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

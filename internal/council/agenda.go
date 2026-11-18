package council

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// Topic is one point of the agenda: one question, argued by the roles it
// concerns in the order they speak, decided, and written down in the
// documents it ends in.
type Topic struct {
	// ID names the topic in the minutes and in what the meeting remembers.
	ID string `yaml:"id" json:"id"`
	// Title is what the topic is called.
	Title string `yaml:"title" json:"title"`
	// Question is what the topic settles.
	Question string `yaml:"question" json:"question"`
	// Lead is the role that proposes; Speakers speak after it, in order.
	Lead     string   `yaml:"lead" json:"lead"`
	Speakers []string `yaml:"speakers" json:"speakers"`
	// Reads are the documents the roles read before they speak: written
	// already, or written by an earlier topic.
	Reads []string `yaml:"reads,omitempty" json:"reads"`
	// Writes are the documents the topic ends in, each written by its owner.
	Writes []string `yaml:"writes" json:"writes"`
}

// Agenda is what a meeting takes up, in order, and what only the owner of the
// project can answer.
type Agenda struct {
	Plan      string   `yaml:"plan"`
	Questions []string `yaml:"questions,omitempty"`
	Topics    []Topic  `yaml:"topics"`
}

// AgendaFile is the name of the agenda in a plan's folder.
const AgendaFile = "agenda.yaml"

var topicID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// checkAgenda checks that an agenda can be held by the table: every topic has
// a question, a lead and at least one role to speak after it, all of them at
// the table; every document it ends in is a document of a role that speaks to
// it; and every document it reads is there by then.
func checkAgenda(agenda *Agenda, roster *Roster, maxTopics int, exists func(doc string) bool) error {
	if len(agenda.Topics) == 0 {
		return errors.New("the agenda has no topic")
	}
	if len(agenda.Topics) > maxTopics {
		return fmt.Errorf("the agenda has %d topics; at most %d (limits.topics in the roster): merge the smaller ones, and leave what can wait for a later plan", len(agenda.Topics), maxTopics)
	}
	if questions := nonEmpty(agenda.Questions); len(questions) > maxQuestions {
		return fmt.Errorf("the agenda has %d questions for the owner; at most %d, the ones the plan turns on", len(questions), maxQuestions)
	}
	roles := strings.Join(roster.IDs(), ", ")
	ids := map[string]bool{}
	written := map[string]bool{}
	for i, topic := range agenda.Topics {
		name := fmt.Sprintf("topic %d (%s)", i+1, topic.ID)
		switch {
		case !topicID.MatchString(topic.ID):
			return fmt.Errorf("topic %d: the id %q is not lower-case letters, digits and hyphens", i+1, topic.ID)
		case ids[topic.ID]:
			return fmt.Errorf("%s: two topics have this id", name)
		case strings.TrimSpace(topic.Title) == "":
			return fmt.Errorf("%s has no title", name)
		case strings.TrimSpace(topic.Question) == "":
			return fmt.Errorf("%s does not say what it settles: it has no question", name)
		case roster.Role(topic.Lead) == nil:
			return fmt.Errorf("%s: the lead %q is not at the table (the roles are: %s)", name, topic.Lead, roles)
		case len(topic.Speakers) == 0:
			return fmt.Errorf("%s: nobody speaks after %s; a topic one role holds alone is not argued", name, topic.Lead)
		case len(topic.Writes) == 0:
			return fmt.Errorf("%s ends in no document; a topic ends in the documents it settles", name)
		}
		ids[topic.ID] = true
		at := map[string]bool{topic.Lead: true}
		for _, speaker := range topic.Speakers {
			switch {
			case roster.Role(speaker) == nil:
				return fmt.Errorf("%s: the speaker %q is not at the table (the roles are: %s)", name, speaker, roles)
			case at[speaker]:
				return fmt.Errorf("%s: %s speaks twice; a role speaks once to a topic, the lead first", name, speaker)
			}
			at[speaker] = true
		}
		for j, doc := range topic.Writes {
			owner := roster.Owner(doc)
			switch {
			case checkOwnedPath(doc) != nil:
				return fmt.Errorf("%s: %w", name, checkOwnedPath(doc))
			case owner == nil:
				return fmt.Errorf("%s ends in %s, which no role owns: a document is a file a role's owns names, or a Markdown file in a folder it names", name, doc)
			case !at[owner.ID]:
				return fmt.Errorf("%s ends in %s, which %s writes, and %s does not speak to the topic: add %s to the speakers", name, doc, owner.ID, owner.ID, owner.ID)
			case contains(topic.Writes[:j], doc):
				return fmt.Errorf("%s ends in %s twice", name, doc)
			}
		}
		for j, doc := range topic.Reads {
			switch {
			case roster.Owner(doc) == nil && checkReadPath(doc) != nil:
				return fmt.Errorf("%s reads %s: %w", name, doc, checkReadPath(doc))
			case roster.Owner(doc) == nil && !exists(doc):
				return fmt.Errorf("%s reads %s, which is neither a document of the table nor a file of the project that is there", name, doc)
			case !written[doc] && !exists(doc):
				return fmt.Errorf("%s reads %s, which is not written yet and no earlier topic writes", name, doc)
			case contains(topic.Reads[:j], doc):
				return fmt.Errorf("%s reads %s twice", name, doc)
			}
		}
		for _, doc := range topic.Writes {
			written[doc] = true
		}
	}
	return nil
}

func agendaPath(root, id string) string {
	return filepath.Join(root, filepath.FromSlash(PlansDir), id, AgendaFile)
}

const agendaHeader = `# The agenda of plan %s: the topics the council takes up, in order.
#
# Change it before the meeting goes on: the order of the topics, what one
# settles, who speaks and in which order, the documents it reads and the ones
# it ends in. Then go on with:
#
#   awkit plan --resume %s
#
# lead and speakers are role ids of %s. writes are documents of a role
# that speaks to the topic: a file its owns names, or a new Markdown file in
# a folder it names. reads are documents written already, or by an earlier
# topic. A topic that was held stays as it was held; to hold it again, give it
# a new id.
`

// SaveAgenda writes a plan's agenda.
func SaveAgenda(root string, agenda *Agenda) error {
	body, err := yaml.Marshal(agenda)
	if err != nil {
		return err
	}
	file := agendaPath(root, agenda.Plan)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(fmt.Sprintf(agendaHeader, agenda.Plan, agenda.Plan, layout.RosterFile)+"\n"+string(body)), 0o644)
}

// LoadAgenda reads a plan's agenda as it stands: the person going on with the
// plan may have changed it.
func LoadAgenda(root, id string) (*Agenda, error) {
	data, err := os.ReadFile(agendaPath(root, id))
	if err != nil {
		return nil, err
	}
	var agenda Agenda
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&agenda); err != nil {
		return nil, fmt.Errorf("%s/%s/%s cannot be read: %w", PlansDir, id, AgendaFile, err)
	}
	agenda.Plan = id
	return &agenda, nil
}

// setAgenda gives the meeting its agenda: the one in the plan's folder, which
// a person may have changed, or a new one from the moderator.
func (p *planner) setAgenda(ctx context.Context, plan *Plan) error {
	agenda, err := LoadAgenda(p.root, plan.ID)
	switch {
	case err == nil:
		if err := checkAgenda(agenda, p.roster, p.roster.Limits.Topics, p.exists); err != nil {
			return fmt.Errorf("%s/%s: %w", plan.Dir(), AgendaFile, err)
		}
		p.session.say("Agenda: %d topics, from %s/%s", len(agenda.Topics), plan.Dir(), AgendaFile)
	case os.IsNotExist(err):
		p.session.say("Agenda: the moderator sets it")
		type answer struct {
			Topics    []Topic  `json:"topics"`
			Questions []string `json:"questions"`
		}
		got, err := settleJSON(ctx, p.session, "moderator sets the agenda", p.roster.Voice(), p.agendaPrompt(), func(got *answer) error {
			for i := range got.Topics {
				topic := &got.Topics[i]
				topic.ID = strings.TrimSpace(topic.ID)
				topic.Lead = strings.TrimSpace(topic.Lead)
				topic.Speakers, topic.Reads, topic.Writes = nonEmpty(topic.Speakers), nonEmpty(topic.Reads), nonEmpty(topic.Writes)
			}
			return checkAgenda(&Agenda{Topics: got.Topics, Questions: got.Questions}, p.roster, p.roster.Limits.Topics, p.exists)
		})
		if err != nil {
			return err
		}
		agenda = &Agenda{Plan: plan.ID, Questions: nonEmpty(got.Questions), Topics: got.Topics}
		if err := SaveAgenda(p.root, agenda); err != nil {
			return broken{err}
		}
	default:
		return err
	}
	plan.Agenda = agenda
	plan.Topics = make([]TopicRecord, len(agenda.Topics))
	for i, topic := range agenda.Topics {
		plan.Topics[i] = TopicRecord{Number: i + 1, Topic: topic}
		p.session.say("  %d. %s (%s leads; %s speak)", i+1, oneLine(topic.Title), topic.Lead, strings.Join(topic.Speakers, ", "))
	}
	return nil
}

// agendaPrompt asks the moderator for the agenda.
func (p *planner) agendaPrompt() string {
	var b strings.Builder
	b.WriteString(moderatorFrame)
	b.WriteString("Before the meeting you set its agenda: the topics the table takes up one after another, and for each one who speaks, in which order, and which documents it ends in.\n\n")
	b.WriteString(p.languageLine())
	b.WriteString("\n\nThe brief:\n<brief>\n" + strings.TrimSpace(p.brief) + "\n</brief>\n\nThe table:\n")
	for _, role := range p.roster.Roles {
		note := ""
		switch {
		case role.Decides && role.Plans:
			note = " - decides what stays disputed, and draws up the work list"
		case role.Decides:
			note = " - decides what stays disputed"
		case role.Plans:
			note = " - draws up the work list"
		}
		fmt.Fprintf(&b, "- %s (%s)%s\n  Stands for: %s\n", role.ID, role.Title, note, oneLine(role.Stance))
		if len(role.Owns) > 0 {
			fmt.Fprintf(&b, "  Writes: %s\n", strings.Join(role.Owns, ", "))
		} else {
			b.WriteString("  Writes: nothing\n")
		}
	}
	b.WriteString("\nThe project's repos:\n")
	for _, repo := range p.repos {
		fmt.Fprintf(&b, "- %s%s\n", repo.Name, repoNote(repo))
	}
	b.WriteString("\nThe project's documents as they stand:\n")
	listed := false
	for i := range p.roster.Roles {
		role := &p.roster.Roles[i]
		for _, doc := range p.documentsOf(role) {
			if p.exists(doc) {
				fmt.Fprintf(&b, "- %s (%s)\n", doc, role.ID)
				listed = true
			}
		}
	}
	if !listed {
		b.WriteString("  (none written yet)\n")
	}
	b.WriteString(p.gameDataNote())
	if log := p.decisionLog(); log != "" {
		b.WriteString("\nDecisions already on record (" + DecisionsPath + "):\n" + log)
	}
	newFile := "a new Markdown file in a folder of that role"
	if p.data != nil {
		newFile = "a new file in a folder of that role (a Markdown document; in the folders of the game data, a schema (.xml) or a table (.csv))"
	}
	fmt.Fprintf(&b, `
Your task: set the agenda.
- Cut the brief into topics, each one question the table can settle at one sitting. Take them in the order one builds on another: what is made and for whom, and what is left out, first; then what the person using it does; then the numbers; then how it is built; then how it is tested and what can go wrong. A later topic starts from the documents of the earlier ones.
- At most %d topics. Cover what has to be settled before work on the brief can start; what can wait for a later plan is left out.
- lead: the role whose documents the topic is mostly about; it proposes. speakers: the other roles whose stance bears on the topic, in the order they speak, each building on the one before: who shapes the thing, then who builds it, then who weighs what it costs, and who looks for how it fails last. Not every role speaks to every topic.
- writes: the documents the topic ends in. Each is a document of a role that leads or speaks to the topic: a file the table lists as that role's, or %s, named for what it holds. A subject the table splits into several topics can end in several documents.
%s- reads: the documents the roles read before they speak: ones written already (listed above), or written by an earlier topic; or a text file of the project that is there, such as a configuration or a contract.
- questions: what only the owner of the project can answer and the plan turns on, at most %d, each answerable in a sentence or two. The meeting does not wait for the answers: where they are missing, the deciding role says what it assumes.

%s
{
  "topics": [
    {"id": "a-short-id", "title": "what the topic is about", "question": "what it settles", "lead": "role id", "speakers": ["role id, in the order they speak"], "reads": ["path of a document"], "writes": ["path of a document"]}
  ],
  "questions": ["a question for the owner"]
}
`, p.roster.Limits.Topics, newFile, p.gameDataTopics(), maxQuestions, jsonOnly)
	return b.String()
}

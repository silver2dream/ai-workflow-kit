package council

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Record is what a plan came to, as data: the minutes, for whoever reads them
// from outside the meeting (the workbench). It is written with the minutes,
// in the plan's state folder, and read back by nobody in the meeting.
type Record struct {
	ID    string `json:"id"`
	Brief string `json:"brief"`
	// Status is LiveFinished, LiveFailed, or the state the meeting waits in
	// (LivePaused, LiveAgenda, LiveQuestions); Stopped says why it stopped
	// when it failed.
	Status  string `json:"status"`
	Stopped string `json:"stopped,omitempty"`

	Agenda  *Agenda       `json:"agenda,omitempty"`
	Answers []OwnerAnswer `json:"answers"`
	Words   []OwnerWord   `json:"words"`
	Topics  []TopicView   `json:"topics"`
	// Revisions are what the deciding role made of the owner's words.
	Revisions []Review `json:"revisions"`
	// Decisions and Assumptions are all of them, each with what became of it.
	Decisions   []DecisionState   `json:"decisions"`
	Assumptions []AssumptionState `json:"assumptions"`
	Documents   []WrittenDocument `json:"documents"`
	// Findings are the points where a document is still at odds with a rule
	// of the project; DataOpen what the check of the game data still finds.
	Findings []RuleFinding `json:"findings"`
	DataOpen []string      `json:"data_open"`
	// Pages are the pages that show the game data for a person to review.
	Pages []string `json:"pages"`

	Milestones     []Milestone     `json:"milestones"`
	Tickets        []Ticket        `json:"tickets"`
	ListObjections []ListObjection `json:"list_objections"`
	Later          []string        `json:"later"`

	// Calls is how many model calls the last run made.
	Calls   int       `json:"calls"`
	Updated time.Time `json:"updated"`
}

// TopicView is a topic as it was taken up.
type TopicView struct {
	Number      int               `json:"number"`
	Topic       Topic             `json:"topic"`
	Started     bool              `json:"started"`
	Held        bool              `json:"held"`
	Proposal    *Speech           `json:"proposal,omitempty"`
	Speeches    []Speech          `json:"speeches"`
	Disputes    []DisputeView     `json:"disputes"`
	Summary     Summary           `json:"summary"`
	Decisions   []Decision        `json:"decisions"`
	Assumptions []Assumption      `json:"assumptions"`
	Documents   []WrittenDocument `json:"documents"`
	RuleChecks  []RuleCheck       `json:"rule_checks"`
}

// DisputeView is a dispute as it was argued out.
type DisputeView struct {
	Dispute
	Exchanges []Exchange `json:"exchanges"`
	Settled   bool       `json:"settled"`
	Outcome   string     `json:"outcome"`
}

// DecisionState is a decision of the plan and what became of it.
type DecisionState struct {
	Decision
	// Topic is the topic that made it; empty for one the owner's word made.
	Topic string `json:"topic,omitempty"`
	// ReplacedBy names the decision that took its place; Withdrawn says why
	// it was taken back.
	ReplacedBy string `json:"replaced_by,omitempty"`
	Withdrawn  string `json:"withdrawn,omitempty"`
}

// AssumptionState is an assumption of the plan and what the owner answered.
type AssumptionState struct {
	Assumption
	Topic  string `json:"topic,omitempty"`
	Answer string `json:"answer,omitempty"`
}

// RecordFile is the name of the record in the plan's state folder.
const RecordFile = "record.json"

// statusOf says how a meeting ended, as a Record and Live say it.
func statusOf(stopped error) (status, note string) {
	var waiting *Waiting
	switch {
	case stopped == nil:
		return LiveFinished, ""
	case errors.As(stopped, &waiting):
		return waiting.State, ""
	default:
		return LiveFailed, stopped.Error()
	}
}

// writeRecord writes what the plan came to, as data.
func (p *planner) writeRecord(plan *Plan, stopped error) error {
	record := Record{ID: plan.ID, Brief: plan.Brief, Agenda: plan.Agenda, Answers: nonNil(plan.Answers), Words: nonNil(plan.Words),
		Revisions: nonNil(plan.Revisions), Documents: nonNil(plan.Documents), Findings: nonNil(plan.OpenFindings()),
		Milestones: nonNil(plan.Milestones), Tickets: nonNil(plan.Tickets), ListObjections: nonNil(plan.ListObjections),
		Later: nonNil(plan.Later), Pages: nonNil(plan.Reviews), Calls: plan.Calls, Updated: p.now(),
		Topics: []TopicView{}, Decisions: []DecisionState{}, Assumptions: []AssumptionState{}, DataOpen: []string{}}
	record.Status, record.Stopped = statusOf(stopped)
	for _, finding := range plan.DataOpen {
		record.DataOpen = append(record.DataOpen, fmt.Sprint(finding))
	}
	for _, r := range plan.Topics {
		view := TopicView{Number: r.Number, Topic: r.Topic, Started: r.started(), Held: r.Held, Proposal: r.Proposal,
			Speeches: nonNil(r.Speeches), Summary: r.Summary, Decisions: nonNil(r.Decisions), Assumptions: nonNil(r.Assumptions),
			Documents: nonNil(r.Documents), RuleChecks: nonNil(r.RuleChecks), Disputes: []DisputeView{}}
		for _, dispute := range r.Disputes {
			view.Disputes = append(view.Disputes, DisputeView{Dispute: dispute, Exchanges: nonNil(dispute.Exchanges),
				Settled: dispute.Settled, Outcome: dispute.Outcome})
		}
		record.Topics = append(record.Topics, view)
		for _, decision := range r.Decisions {
			record.Decisions = append(record.Decisions, p.decisionState(plan, decision, r.Topic.ID))
		}
		for _, assumption := range r.Assumptions {
			answer, _ := plan.answered(assumption.ID)
			record.Assumptions = append(record.Assumptions, AssumptionState{Assumption: assumption, Topic: r.Topic.ID, Answer: answer})
		}
	}
	for _, review := range plan.Revisions {
		for _, decision := range review.Decisions {
			record.Decisions = append(record.Decisions, p.decisionState(plan, decision, ""))
		}
		record.Documents = append(record.Documents, review.Documents...)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	dir := p.session.stateDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, RecordFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, RecordFile)); err != nil {
		_ = os.Remove(tmp)
		return os.WriteFile(filepath.Join(dir, RecordFile), data, 0o644)
	}
	return nil
}

func (p *planner) decisionState(plan *Plan, decision Decision, topic string) DecisionState {
	state := DecisionState{Decision: decision, Topic: topic}
	by, withdrawn := plan.replaced(decision.ID)
	state.ReplacedBy = by
	if withdrawn != nil {
		state.Withdrawn = withdrawn.Why
	}
	return state
}

// nonNil turns a nil list into an empty one, so that it reads as [] and
// not as null.
func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// ReadRecord reads what a plan came to, as its last meeting wrote it. A plan
// held before records were written has none: os.ErrNotExist.
func ReadRecord(root, id string) (*Record, error) {
	if !planID.MatchString(id) {
		return nil, fmt.Errorf("%q is not the name of a plan", id)
	}
	data, err := os.ReadFile(filepath.Join(StateDir(root, id), RecordFile))
	if err != nil {
		return nil, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("%s cannot be read: %w", RecordFile, err)
	}
	return &record, nil
}

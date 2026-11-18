package council

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Speech is what a role said to a topic: the lead's proposal, or what a role
// that speaks after it says.
type Speech struct {
	Role      string   `json:"-"`
	Says      string   `json:"says"`
	Proposals []string `json:"proposals"`
	Concerns  []string `json:"concerns"`
	// Objections are what the role cannot accept in what was said before it.
	Objections []Objection `json:"objections"`
}

// TopicRecord is what became of one topic of the agenda.
type TopicRecord struct {
	// Number is the topic's place on the agenda, from 1.
	Number int
	Topic  Topic

	// Proposal is the lead's; Speeches are the others', in the order spoken.
	Proposal *Speech
	Speeches []Speech
	// Objections are those raised in the speeches; Disputes the ones argued out.
	Objections []Objection
	Disputes   []Dispute
	Summary    Summary

	Decisions   []Decision
	Assumptions []Assumption

	// Documents is what became of the documents the topic ends in; Rules and
	// RulesLeft the project's rules they were written by and the ones that
	// did not fit; RuleChecks what reading them against those rules came to.
	Documents        []WrittenDocument
	Rules, RulesLeft []string
	RuleChecks       []RuleCheck
	// DataCheck is what the check of the game data the topic wrote came to;
	// nil when it wrote none.
	DataCheck *DataCheck

	// Held says the topic was taken up to its end: its documents written.
	Held bool
}

// step names a step of the topic in what the meeting remembers.
func (r *TopicRecord) step(what string) string {
	return "topic " + r.Topic.ID + " " + what
}

// at lists who speaks to the topic: the lead, then the speakers in order.
func (r *TopicRecord) at() []string {
	return append([]string{r.Topic.Lead}, r.Topic.Speakers...)
}

// undisputed lists the objections of the topic that did not become a dispute.
func (r *TopicRecord) undisputed() []string {
	var out []string
	for _, objection := range r.Objections {
		argued := false
		for _, dispute := range r.Disputes {
			if dispute.Challenger == objection.From && dispute.Defender == objection.To {
				argued = true
			}
		}
		if !argued {
			out = append(out, fmt.Sprintf("%s to %s: %s", objection.From, objection.To, oneLine(objection.Point)))
		}
	}
	return out
}

// hold takes one topic up: the lead proposes, the others speak in turn, what
// they cannot agree on is argued out, the moderator says where it stands, the
// deciding role rules, and the owners write the documents it ends in.
func (p *planner) hold(ctx context.Context, plan *Plan, i int) error {
	r := &plan.Topics[i]
	p.session.say("Topic %d/%d: %s", r.Number, len(plan.Topics), oneLine(r.Topic.Title))
	steps := []func(context.Context, *Plan, *TopicRecord) error{p.propose, p.speak, p.argueOut, p.sumUp, p.decide, p.writeDown}
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := step(ctx, plan, r); err != nil {
			return err
		}
	}
	r.Held = true
	return nil
}

// propose asks the lead how the topic is settled.
func (p *planner) propose(ctx context.Context, plan *Plan, r *TopicRecord) error {
	lead := p.roster.Role(r.Topic.Lead)
	got, err := settleJSON(ctx, p.session, r.step(lead.ID+" proposes"), lead.Voice(), p.proposalPrompt(lead, plan, r), func(got *Speech) error {
		if strings.TrimSpace(got.Says) == "" {
			return errors.New("the proposal says nothing: says is empty")
		}
		return nil
	})
	if err != nil {
		return err
	}
	got.Role, got.Objections = lead.ID, nil
	r.Proposal = got
	return nil
}

// speak asks every role that speaks to the topic, in turn: each is shown what
// was said before it, and objects to it where it cannot accept it.
func (p *planner) speak(ctx context.Context, plan *Plan, r *TopicRecord) error {
	r.Speeches, r.Objections = nil, nil
	spoken := []string{r.Topic.Lead}
	for _, id := range r.Topic.Speakers {
		role := p.roster.Role(id)
		got, err := settleJSON(ctx, p.session, r.step(id+" speaks"), role.Voice(), p.speechPrompt(role, plan, r), func(got *Speech) error {
			if strings.TrimSpace(got.Says) == "" {
				return errors.New("says is empty: say where you stand on the topic")
			}
			for i := range got.Objections {
				objection := &got.Objections[i]
				objection.To = strings.TrimSpace(objection.To)
				switch {
				case objection.To == id:
					return errors.New("an objection is addressed to yourself")
				case !contains(spoken, objection.To):
					return fmt.Errorf("an objection is addressed to %q, who has not spoken to this topic (so far: %s)", objection.To, strings.Join(spoken, ", "))
				case strings.TrimSpace(objection.Point) == "":
					return errors.New("an objection has no point")
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		got.Role = id
		for i := range got.Objections {
			got.Objections[i].From = id
		}
		r.Speeches = append(r.Speeches, *got)
		r.Objections = append(r.Objections, got.Objections...)
		spoken = append(spoken, id)
	}
	if len(r.Objections) > 0 {
		p.session.say("  %d objection(s) raised", len(r.Objections))
	}
	return nil
}

// argueOut has the objections of a topic argued out between the two roles
// concerned: all of them when they are few, the ones the moderator picks
// when they are more than the roster argues out.
func (p *planner) argueOut(ctx context.Context, plan *Plan, r *TopicRecord) error {
	r.Disputes = nil
	limit := p.roster.Limits.Disputes
	switch {
	case len(r.Objections) == 0:
		return nil
	case len(r.Objections) <= limit:
		for _, objection := range r.Objections {
			r.Disputes = append(r.Disputes, Dispute{Topic: topicOf(objection.Point), Challenger: objection.From,
				Defender: objection.To, Point: objection.Point, Wants: objection.Wants})
		}
	default:
		type answer struct {
			Disputes []Dispute `json:"disputes"`
		}
		at := r.at()
		got, err := settleJSON(ctx, p.session, r.step("moderator picks the disputes"), p.roster.Voice(), p.disputesPrompt(r), func(got *answer) error {
			if len(got.Disputes) == 0 {
				return errors.New("no dispute was picked, and there are objections")
			}
			for _, dispute := range got.Disputes {
				if !contains(at, dispute.Challenger) || !contains(at, dispute.Defender) || dispute.Challenger == dispute.Defender {
					return fmt.Errorf("the dispute %q is not between two roles of this topic (they are: %s)", dispute.Topic, strings.Join(at, ", "))
				}
				if strings.TrimSpace(dispute.Point) == "" {
					return fmt.Errorf("the dispute %q has no point", dispute.Topic)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		r.Disputes = got.Disputes
		if len(r.Disputes) > limit {
			r.Disputes = r.Disputes[:limit]
		}
	}
	p.session.say("  %d point(s) argued out", len(r.Disputes))
	// Each dispute is argued on its own: what a role concedes in one is not
	// known to it in another, and need not be.
	return p.session.each(len(r.Disputes), func(i int) error {
		return p.argue(ctx, plan, r, i, &r.Disputes[i])
	})
}

// topicOf names a dispute after the first words of its point.
func topicOf(point string) string {
	words := strings.Fields(point)
	if len(words) > 8 {
		return strings.Join(words[:8], " ") + "..."
	}
	return strings.Join(words, " ")
}

// argue lets the two roles of a dispute go back and forth: the defender
// answers, the challenger says whether that settles it. It ends when the
// defender concedes, the challenger accepts, or the rounds are used up.
func (p *planner) argue(ctx context.Context, plan *Plan, r *TopicRecord, index int, dispute *Dispute) error {
	defender, challenger := p.roster.Role(dispute.Defender), p.roster.Role(dispute.Challenger)
	type defence struct {
		Stance   string `json:"stance"`
		Reply    string `json:"reply"`
		Position string `json:"position"`
	}
	type reply struct {
		Accepts bool   `json:"accepts"`
		Reply   string `json:"reply"`
		Wants   string `json:"wants"`
	}

	for round := 1; round <= p.roster.Limits.DisputeRounds; round++ {
		step := r.step(fmt.Sprintf("dispute %d round %d", index+1, round))
		answer, err := settleJSON(ctx, p.session, step+" "+defender.ID+" answers", defender.Voice(), p.defencePrompt(defender, plan, r, dispute), func(got *defence) error {
			got.Stance = strings.ToLower(strings.TrimSpace(got.Stance))
			switch got.Stance {
			case "concede", "revise", "hold":
			default:
				return fmt.Errorf("stance is %q; it is one of concede, revise, hold", got.Stance)
			}
			if strings.TrimSpace(got.Reply) == "" {
				return errors.New("the answer has no reply")
			}
			return nil
		})
		if err != nil {
			return err
		}
		exchange := Exchange{Stance: answer.Stance, Reply: answer.Reply, Position: answer.Position}
		dispute.Exchanges = append(dispute.Exchanges, exchange)
		if exchange.Stance == "concede" {
			dispute.Settled = true
			dispute.Outcome = fmt.Sprintf("%s conceded in round %d", defender.ID, round)
			break
		}

		verdict, err := settleJSON(ctx, p.session, step+" "+challenger.ID+" replies", challenger.Voice(), p.replyPrompt(challenger, plan, r, dispute), func(got *reply) error {
			if strings.TrimSpace(got.Reply) == "" {
				return errors.New("the answer has no reply")
			}
			return nil
		})
		if err != nil {
			return err
		}
		last := &dispute.Exchanges[len(dispute.Exchanges)-1]
		last.Answered, last.Accepts, last.Answer, last.StillWants = true, verdict.Accepts, verdict.Reply, verdict.Wants
		if verdict.Accepts {
			dispute.Settled = true
			dispute.Outcome = fmt.Sprintf("%s accepted %s's answer in round %d", challenger.ID, defender.ID, round)
			break
		}
	}
	if !dispute.Settled {
		dispute.Outcome = fmt.Sprintf("not settled after %d round(s); it goes to %s", len(dispute.Exchanges), p.roster.Decider().ID)
	}
	state := "not settled"
	if dispute.Settled {
		state = "settled"
	}
	p.session.say("  %s against %s: %s", dispute.Challenger, dispute.Defender, state)
	return nil
}

// sumUp asks the moderator where the topic stands.
func (p *planner) sumUp(ctx context.Context, plan *Plan, r *TopicRecord) error {
	got, err := settleJSON[Summary](ctx, p.session, r.step("moderator sums up"), p.roster.Voice(), p.summaryPrompt(plan, r), nil)
	if err != nil {
		return err
	}
	r.Summary = *got
	return nil
}

// decide asks the deciding role to rule on the topic, and puts the rulings on
// record at once: the next topic is argued from them.
func (p *planner) decide(ctx context.Context, plan *Plan, r *TopicRecord) error {
	decider := p.roster.Decider()
	type answer struct {
		Decisions   []Decision   `json:"decisions"`
		Assumptions []Assumption `json:"assumptions"`
	}
	got, err := settleJSON(ctx, p.session, r.step("decision"), decider.Voice(), p.decisionPrompt(decider, plan, r), func(got *answer) error {
		if len(got.Decisions) == 0 {
			return errors.New("nothing was decided; a topic ends in at least one decision")
		}
		for _, decision := range got.Decisions {
			if strings.TrimSpace(decision.Decision) == "" {
				return errors.New("a decision does not say what is decided")
			}
		}
		for _, assumption := range got.Assumptions {
			if strings.TrimSpace(assumption.Assumption) == "" {
				return errors.New("an assumption does not say what is assumed")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.Decisions, r.Assumptions = got.Decisions, got.Assumptions
	for i := range r.Decisions {
		if saysNobody(r.Decisions[i].Dissent) {
			r.Decisions[i].Dissent = ""
		}
	}
	p.number(plan, r)
	p.session.say("  %s rules: %d decided, %d assumed until the owner answers", decider.ID, len(r.Decisions), len(r.Assumptions))
	return p.recordDecisions(plan, r)
}

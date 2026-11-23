package lessons

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// A lesson is shown to every later Worker whose ticket it fits: text that got
// into one is text a Worker reads again and again. So a lesson an agent's
// case gave rise to is screened before it is given to anyone, and again each
// time it is about to be: by the guard's injection rules, and by what a
// lesson has no business saying. One that fails is quarantined.

// lessonPolicy is what a lesson must not say, whatever the wording of an
// attempt: it is about how code is written, not about the workflow's
// permissions, checks, network or files.
var lessonPolicy = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`(?i)(--dangerously|bypass\s*permissions|--no-verify|allow_script_changes|allow_secrets|permission[_-]?mode)`), "it speaks of the workflow's permissions and checks"},
	{regexp.MustCompile(`(?i)\b(curl|wget|invoke-webrequest|invoke-restmethod|netcat)\b|https?://`), "it reaches for another host"},
	{regexp.MustCompile(`(?i)(\.ai/state|settings\.local\.json|\.claude/|kickoff\.lock|awkit\s+(submit-review|stop-workflow|dispatch-worker))`), "it speaks of the workflow's own files and commands"},
}

// Screen says why a lesson must not be given to a Worker, "" when nothing
// does.
func Screen(l Lesson) string {
	text := strings.Join([]string{l.Title, l.Description, l.Content}, "\n")
	for _, v := range guard.Screen(guard.Piece{ID: l.ID, Source: "lesson " + l.ID, Trust: guard.Agent, Text: text}) {
		if v.Result != guard.Pass {
			return v.Reason + " (" + v.Rule + ")"
		}
	}
	for _, rule := range lessonPolicy {
		if rule.pattern.MatchString(text) {
			return rule.reason
		}
	}
	return ""
}

// Quarantine holds a lesson back, saying why.
func Quarantine(l *Lesson, reason string) {
	l.Status = StatusQuarantined
	l.Quarantine = reason
}

// ScreenVerdict is the verdict of a quarantine, for the record.
func ScreenVerdict(l Lesson) guard.Verdict {
	return guard.Verdict{
		Result: guard.Block, Class: guard.Security, Stage: guard.StageMemory,
		Rule: "lesson-screen", Piece: l.ID, Source: "lesson " + l.ID, Reason: l.Quarantine,
	}
}

// SourceOwner is the Source of a lesson a person added by hand
// (awkit lessons add): the owner's own words, which are not screened.
const SourceOwner = "human"

// ScreenAll quarantines every lesson that is about to be given to a Worker
// and fails the screening: a lesson from before the screening existed, or
// one the rules have come to catch. It returns the lessons it quarantined.
func (s *Store) ScreenAll() []Lesson {
	var held []Lesson
	for i := range s.Lessons {
		l := &s.Lessons[i]
		switch l.Status {
		case StatusCandidate, StatusActive, StatusProven:
		default:
			continue
		}
		if l.Source == SourceOwner {
			continue
		}
		if reason := Screen(*l); reason != "" {
			Quarantine(l, reason)
			held = append(held, *l)
		}
	}
	return held
}

// LoadForPrompt loads the store for a prompt: every lesson that is about to
// be given to an agent and fails the screening is quarantined first, the
// quarantine recorded against the issue (0 for none) and saved. A failure to
// save is no reason to give the lesson out: it stays held back in the store
// returned.
func LoadForPrompt(stateRoot string, issue int) (*Store, error) {
	s, err := Load(stateRoot)
	if err != nil {
		return nil, err
	}
	held := s.ScreenAll()
	if len(held) == 0 {
		return s, nil
	}
	verdicts := make([]guard.Verdict, len(held))
	for i, l := range held {
		verdicts[i] = ScreenVerdict(l)
	}
	guard.Record(stateRoot, verdicts, issue)
	if err := Save(stateRoot, s); err != nil {
		fmt.Fprintf(os.Stderr, "[lessons] warning: failed to save quarantined lessons: %v\n", err)
	}
	return s, nil
}

// Approve gives a quarantined lesson its trial: a person read it and vouches
// for it. It goes on as a candidate, to be settled like any other.
func (s *Store) Approve(id string) error {
	l := s.FindByID(id)
	if l == nil {
		return fmt.Errorf("no lesson %s", id)
	}
	if l.Status != StatusQuarantined {
		return fmt.Errorf("lesson %s is %s, not quarantined", id, l.Status)
	}
	l.Status = StatusCandidate
	l.Quarantine = ""
	l.Source = SourceOwner
	return nil
}

// Reject turns a quarantined lesson down: a person read it and will not have
// it. It is retired, never given to anyone, and keeps the reason it was held
// back, for the record.
func (s *Store) Reject(id string) error {
	l := s.FindByID(id)
	if l == nil {
		return fmt.Errorf("no lesson %s", id)
	}
	if l.Status != StatusQuarantined {
		return fmt.Errorf("lesson %s is %s, not quarantined", id, l.Status)
	}
	l.Status = StatusRetired
	return nil
}

// Decide applies a person's decision on a quarantined lesson (Approve or
// Reject), saves the store and records the decision. by says where the
// person decided ("cli", "workbench").
//
// The store has no lock: a distillation that loaded it before and saves
// after can put the lesson back as it was. That fails safe, a quarantined
// lesson stays held back, and the person decides again.
func Decide(stateRoot, id, decision, by string) (*Lesson, error) {
	s, err := Load(stateRoot)
	if err != nil {
		return nil, err
	}
	l := s.FindByID(id)
	if l == nil {
		return nil, fmt.Errorf("no lesson %s", id)
	}
	said := l.Quarantine
	switch decision {
	case guard.ApproveDecision:
		err = s.Approve(id)
	case guard.RejectDecision:
		err = s.Reject(id)
	default:
		err = fmt.Errorf("no such decision %q (approve or reject)", decision)
	}
	if err != nil {
		return nil, err
	}
	if err := Save(stateRoot, s); err != nil {
		return nil, err
	}
	guard.RecordDecision(stateRoot, guard.Decision{
		Stage: guard.StageMemory, Rule: "lesson-screen", Piece: id, Said: said, Decision: decision, By: by,
	})
	decided := *s.FindByID(id)
	return &decided, nil
}

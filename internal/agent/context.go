package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// The context builder makes what an agent is told (design.md §5.6, task
// 1.3). Each role gets the kinds of context its work needs and no other;
// each piece's trust follows from what kind it is and who wrote it, never
// from what it says; credentials and personal data are masked; the pieces go
// through the input checks (guard.Prepare); the untrusted ones are fenced as
// data; and what the agent was given is kept, masked, beside its run.

// Kind is what a piece of context is.
type Kind string

const (
	// KindInstructions are the role's own instructions, which AWK writes.
	KindInstructions Kind = "instructions"
	// KindTicket is the ticket an approved plan or a person made.
	KindTicket Kind = "ticket"
	// KindTicketSummary is a ticket in short, for a resolver.
	KindTicketSummary Kind = "ticket_summary"
	// KindOwnerNote is what the owner added to a lane (an instruction).
	KindOwnerNote Kind = "owner_note"
	// KindRequest is a person's words to be turned into an instruction.
	KindRequest Kind = "request"
	// KindRules are AGENTS.md and the project's rule files.
	KindRules Kind = "rules"
	// KindCodeMap is what the code is and where.
	KindCodeMap Kind = "code_map"
	// KindReview is a review the workflow wrote.
	KindReview Kind = "review"
	// KindPRComment is a comment on the pull request: its trust is its
	// author's.
	KindPRComment Kind = "pr_comment"
	// KindLesson is a lesson of the learning loop.
	KindLesson Kind = "lesson"
	// KindDiff is the change under review, as the implementer made it.
	KindDiff Kind = "diff"
	// KindTestResult is what the engine's own run of the checks said.
	KindTestResult Kind = "test_result"
	// KindConflict is a conflicting file and its two sides.
	KindConflict Kind = "conflict"
	// KindLog is an error and the log around it.
	KindLog Kind = "log"
	// KindActions are what the engine can do, for a role to choose from.
	KindActions Kind = "actions"
	// KindStatistics are what the workflow counted.
	KindStatistics Kind = "statistics"
	// KindReviewCase is a review case for the distiller.
	KindReviewCase Kind = "review_case"
	// KindJudged is a text a judge gives a second opinion on.
	KindJudged Kind = "judged"
	// KindSelfReport is the implementer's own account of its work. No role
	// is given it: a reviewer judges the change, not what was said of it.
	KindSelfReport Kind = "self_report"
)

// kindTrust is how far each kind is trusted, by who writes it.
var kindTrust = map[Kind]string{
	KindInstructions:  guard.System,
	KindTestResult:    guard.System,
	KindActions:       guard.System,
	KindStatistics:    guard.System,
	KindTicket:        guard.Owner,
	KindTicketSummary: guard.Owner,
	KindOwnerNote:     guard.Owner,
	KindRequest:       guard.Owner,
	KindRules:         guard.Repo,
	KindCodeMap:       guard.Repo,
	KindReview:        guard.Agent,
	KindLesson:        guard.Agent,
	KindDiff:          guard.Agent,
	KindConflict:      guard.Agent,
	KindReviewCase:    guard.Agent,
	KindSelfReport:    guard.Agent,
	KindLog:           guard.External,
	KindJudged:        guard.External,
	KindPRComment:     guard.External, // and higher, by its author (trustOf)
}

// roleKinds are the kinds of context each role is given: the least its work
// needs (design.md §5.6).
var roleKinds = map[Role][]Kind{
	Implementer: {KindInstructions, KindTicket, KindOwnerNote, KindRules, KindCodeMap, KindReview, KindPRComment, KindLesson, KindStatistics},
	Reviewer:    {KindInstructions, KindTicket, KindRules, KindDiff, KindTestResult, KindPRComment, KindLesson},
	Resolver:    {KindInstructions, KindConflict, KindTicketSummary, KindRules},
	Triage:      {KindInstructions, KindLog, KindActions},
	Interpreter: {KindInstructions, KindRequest, KindActions},
	Judge:       {KindInstructions, KindJudged},
	Distiller:   {KindInstructions, KindReviewCase, KindLesson},
}

// Input is a piece of context as the engine has it.
type Input struct {
	Kind Kind
	// Source says where it came from, for a person: "issue #12", "PR #30
	// comment 4411", "lessons".
	Source string
	// Author is who wrote it, when that is known.
	Author string
	// For a comment on the pull request: whether its author can write to
	// the repository, and whether the engine posted it (the engine keeps
	// the ids of its comments: it and the owner post as one account).
	WriteAccess bool
	ByEngine    bool
	// Quotes are the trust levels of what the piece quotes: it is trusted
	// no more than the least of them.
	Quotes []string
	Title  string
	Text   string
	// MaxChars is how long it may be; 0 is guard.MaxPieceChars.
	MaxChars int
}

// awkFormat marks a comment the workflow wrote, from before the engine kept
// its comments' ids: it is agent text whoever's name is on it.
var awkFormat = []string{"AWK Review", "<!-- AWK:", "AWK 沒有", "## AWK"}

// trustOf is how far an input is trusted.
func trustOf(in Input) string {
	trust := kindTrust[in.Kind]
	if in.Kind == KindPRComment {
		switch {
		case in.ByEngine || slices.ContainsFunc(awkFormat, func(m string) bool { return strings.Contains(in.Text, m) }):
			trust = guard.Agent
		case in.WriteAccess:
			trust = guard.Owner
		default:
			trust = guard.External
		}
	}
	for _, q := range in.Quotes {
		trust = guard.Lower(trust, q)
	}
	return trust
}

// PieceRecord is a piece as the agent was given it: masked and cleaned.
type PieceRecord struct {
	ID     string `json:"id"`
	Kind   Kind   `json:"kind"`
	Source string `json:"source"`
	Author string `json:"author,omitempty"`
	Trust  string `json:"trust"`
	Title  string `json:"title,omitempty"`
	Text   string `json:"text"`
	// Redacted are the kinds of credential and personal data masked in it.
	Redacted []string `json:"redacted,omitempty"`
}

// Context is what an agent is told, built and checked.
type Context struct {
	Role   Role
	Pieces []PieceRecord
	// Verdicts are what the input checks said, the passes left out.
	Verdicts []guard.Verdict
	// Untrusted says some piece is fenced as data: the agent is to report
	// the suspicious ones.
	Untrusted bool
	// Prompt is what the agent is told: the trusted pieces, the rules of the
	// data blocks, then the data blocks.
	Prompt string
}

// Blocked says the input checks stopped the context: it is not given to the
// agent, and the lane needs a person.
func (c *Context) Blocked() bool { return guard.Blocks(c.Verdicts) }

// Build makes a role's context from inputs. withSchema says the answer has a
// schema, which then carries the suspicious blocks (SuspiciousSchema); a
// free answer reports them on its last line. A kind the role is not given is
// the caller's mistake, and an error: it never reaches the agent.
func Build(role Role, inputs []Input, withSchema bool) (*Context, error) {
	allowed, ok := roleKinds[role]
	if !ok {
		return nil, fmt.Errorf("no agent role %q", role)
	}
	pieces := make([]guard.Piece, len(inputs))
	redacted := make([][]string, len(inputs))
	var verdicts []guard.Verdict
	for i, in := range inputs {
		if !slices.Contains(allowed, in.Kind) {
			return nil, fmt.Errorf("the %s is not given %s context (%s): it gets %s", role, in.Kind, in.Source, kindList(allowed))
		}
		text, kinds := guard.Redact(in.Text)
		title, titleKinds := guard.Redact(in.Title)
		redacted[i] = append(kinds, titleKinds...)
		pieces[i] = guard.Piece{Source: in.Source, Author: in.Author, Trust: trustOf(in), Title: title, Text: text, Max: in.MaxChars}
		if len(redacted[i]) > 0 {
			verdicts = append(verdicts, guard.Verdict{
				Result: guard.Flag, Class: guard.Info, Stage: guard.StageInput,
				Rule: "redact", Piece: "c" + strconv.Itoa(i+1), Source: in.Source,
				Reason: "masked before the agent saw it: " + strings.Join(redacted[i], ", "),
			})
		}
	}
	prepared := guard.Prepare(pieces)
	ctx := &Context{Role: role, Verdicts: append(verdicts, prepared.Verdicts...)}

	byID := map[string]int{}
	for i := range inputs {
		byID["c"+strconv.Itoa(i+1)] = i
	}
	var trusted, data []string
	for _, p := range prepared.Pieces {
		i := byID[p.ID]
		ctx.Pieces = append(ctx.Pieces, PieceRecord{
			ID: p.ID, Kind: inputs[i].Kind, Source: p.Source, Author: p.Author, Trust: p.Trust, Title: p.Title, Text: p.Text, Redacted: redacted[i],
		})
		if guard.Untrusted(p.Trust) {
			data = append(data, guard.Render(p))
		} else {
			trusted = append(trusted, guard.Render(p))
		}
	}
	ctx.Untrusted = len(data) > 0
	parts := trusted
	if ctx.Untrusted {
		rules := guard.DataRules
		if withSchema {
			rules = guard.DataRulesSchema
		}
		parts = append(parts, rules+"\n")
		parts = append(parts, data...)
	}
	ctx.Prompt = strings.Join(parts, "\n")
	return ctx, nil
}

func kindList(kinds []Kind) string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}

// SuspiciousSchema adds to an answer's schema the field the agent reports
// suspicious data blocks in: "suspicious", a list of block ids. The schema
// must describe an object.
func SuspiciousSchema(schema json.RawMessage) (json.RawMessage, error) {
	var s map[string]any
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil, fmt.Errorf("the answer's schema is not a JSON object: %w", err)
	}
	if s["type"] != "object" {
		return nil, fmt.Errorf("the answer's schema describes %v, not an object: it has no field for the suspicious blocks", s["type"])
	}
	props, _ := s["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	if _, taken := props["suspicious"]; taken {
		return nil, fmt.Errorf("the answer's schema has a field named suspicious of its own")
	}
	props["suspicious"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	s["properties"] = props
	required, _ := s["required"].([]any)
	s["required"] = append(required, "suspicious")
	return json.Marshal(s)
}

// Suspicious reads the blocks an agent reported, from its structured answer
// or, for a free answer, from its last report line. An id that is no data
// block of the context is left out: an agent cannot make one up.
func (c *Context) Suspicious(output json.RawMessage, text string) []string {
	var ids []string
	if len(output) > 0 {
		var answer struct {
			Suspicious []string `json:"suspicious"`
		}
		if json.Unmarshal(output, &answer) == nil {
			ids = answer.Suspicious
		}
	} else {
		ids = guard.Suspicious(text)
	}
	var out []string
	for _, id := range ids {
		for _, p := range c.Pieces {
			if p.ID == strings.TrimSpace(id) && guard.Untrusted(p.Trust) && !slices.Contains(out, p.ID) {
				out = append(out, p.ID)
			}
		}
	}
	return out
}

// SuspiciousVerdicts are the verdicts of what an agent reported.
func (c *Context) SuspiciousVerdicts(ids []string) []guard.Verdict {
	var out []guard.Verdict
	for _, id := range ids {
		v := guard.Verdict{
			Result: guard.Flag, Class: guard.Security, Stage: guard.StageOutput, Rule: "agent-reported", Piece: id,
			Reason: "the agent reported that this piece asked it to do something its instructions do not allow",
		}
		for _, p := range c.Pieces {
			if p.ID == id {
				v.Source = p.Source
			}
		}
		out = append(out, v)
	}
	return out
}

// RunsDir is where the engine keeps each run's files, under the project.
const RunsDir = layout.StateDir + "/engine/runs"

// RunDir is the folder of one attempt of a lane's work.
func RunDir(root string, issue, attempt int) string {
	return filepath.Join(root, filepath.FromSlash(RunsDir), strconv.Itoa(issue), strconv.Itoa(attempt))
}

// envelopeFile is what is kept of what an agent was given.
type envelopeFile struct {
	Role     Role            `json:"role"`
	Profile  Profile         `json:"profile"`
	Issue    int             `json:"issue,omitempty"`
	Schema   json.RawMessage `json:"schema,omitempty"`
	Pieces   []PieceRecord   `json:"pieces"`
	Verdicts []guard.Verdict `json:"verdicts,omitempty"`
	At       time.Time       `json:"at"`
}

// SaveEnvelope keeps, in a run's folder, what its agent was given: the
// pieces as they went in (masked and cleaned), the checks' verdicts, the
// settings and the answer's schema. It is written whole or not at all.
func SaveEnvelope(dir string, env Envelope, c *Context, at time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(envelopeFile{
		Role: c.Role, Profile: env.Profile, Issue: env.Issue, Schema: env.Schema, Pieces: c.Pieces, Verdicts: c.Verdicts, At: at,
	}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".envelope-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, "envelope.json")); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

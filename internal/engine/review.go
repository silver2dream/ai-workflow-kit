package engine

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/agent"
	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/hooks"
	"github.com/silver2dream/ai-workflow-kit/internal/reviewer"
	"github.com/silver2dream/ai-workflow-kit/internal/rules"
)

// The engine's review (design.md §5.5, task 1.4). The engine gathers what
// the review needs and runs the tests itself. A reviewer that can only read
// judges the change, and answers in the review's schema. What follows from
// its answer — the evidence gate on the engine's run of the tests, the
// score, a maintainer's word, the merge — is decided as the Principal's
// workflow decides it (reviewer.SubmitReview), and what that does on GitHub
// goes through the engine's publisher, which leaves the labels to the lane.
// The step says where the lane goes; the engine's loop records it.

// AgentRunner runs an agent: an *agent.Runner, or a stand-in in tests.
type AgentRunner interface {
	Run(ctx context.Context, env agent.Envelope) agent.Result
}

// EnginePublisher is what the engine's review does on GitHub: what the
// Principal's does, without the labels. The engine keeps a ticket's state in
// its lane; the labels show that state (labels.go), and are written as the
// lane moves, not by the review.
type EnginePublisher struct{ reviewer.Publisher }

// NewEnginePublisher is the engine's publisher, on the gh CLI.
func NewEnginePublisher(timeout time.Duration) EnginePublisher {
	return EnginePublisher{reviewer.GHPublisher(timeout)}
}

// Labels changes no label: the lane's projection does.
func (EnginePublisher) Labels(context.Context, int, []string, []string) error { return nil }

// ReviewStep reviews the pull request of a lane.
type ReviewStep struct {
	// Root is the project.
	Root string
	// Agents runs the reviewer, with Profile as its settings; a Profile
	// without a role is the reviewer's defaults.
	Agents  AgentRunner
	Profile agent.Profile
	// Publisher does what the review does on GitHub; nil is the engine's.
	Publisher reviewer.Publisher
	// GHTimeout bounds each call to GitHub; 0 is a minute.
	GHTimeout time.Duration

	// Replaced in tests.
	now     func() time.Time
	prepare func(context.Context, reviewer.PrepareReviewOptions) (*reviewer.ReviewContext, error)
	diff    func(ctx context.Context, pr int, timeout time.Duration) (string, error)
	test    func(ctx context.Context, dir, command, language string, timeout time.Duration) reviewer.TestRun
	submit  func(context.Context, reviewer.SubmitReviewOptions) (*reviewer.SubmitReviewResult, error)
}

// How much of the change and of what the tests printed the reviewer is
// given. It reads the files themselves for the rest.
const (
	maxDiffChars       = 80000
	maxTestOutputChars = 20000
)

//go:embed review_instructions.md
var reviewInstructions string

// reviewSchema is the review's answer: reviewer.StructuredReview. Every
// field is required, and an empty string or false stands for one left out,
// so that a backend that holds an answer to its schema strictly (Codex)
// takes it as it is; what a schema cannot say (an implementation of 20
// characters, the evidence of a criterion that is not meta) is checked by
// reviewer.ParseStructuredReview, and an answer that fails it is asked for
// again.
const reviewSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["score", "summary", "criteria", "improvements", "risks"],
  "properties": {
    "score": {"type": "integer", "description": "from 1 to 10"},
    "summary": {"type": "string", "description": "why this score"},
    "criteria": {
      "type": "array",
      "description": "one entry per acceptance criterion of the ticket",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["criterion", "implementation", "test_name", "assertion", "meta"],
        "properties": {
          "criterion": {"type": "string", "description": "the acceptance criterion, copied verbatim from the ticket"},
          "implementation": {"type": "string", "description": "the function, its file:line and its key logic, in at least 20 characters; empty for a meta criterion"},
          "test_name": {"type": "string", "description": "the exact name of the test that verifies it; empty for a meta criterion"},
          "assertion": {"type": "string", "description": "a key assertion line of that test, copied verbatim from the test file; empty for a meta criterion"},
          "meta": {"type": "boolean", "description": "true only for a criterion about the process, which the engine's run of the tests verifies"}
        }
      }
    },
    "improvements": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["severity", "location", "text"],
        "properties": {
          "severity": {"type": "string", "enum": ["critical", "important", "nit", "optional", "fyi"]},
          "location": {"type": "string", "description": "file:line, or empty"},
          "text": {"type": "string"}
        }
      }
    },
    "risks": {"type": "string", "description": "what could go wrong after the merge; empty when nothing"}
  }
}`

func (s *ReviewStep) defaults() {
	if s.GHTimeout == 0 {
		s.GHTimeout = time.Minute
	}
	if s.Profile.Role == "" {
		s.Profile = agent.Defaults(agent.Reviewer, agent.Profile{})
	}
	if s.Publisher == nil {
		s.Publisher = NewEnginePublisher(s.GHTimeout)
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.prepare == nil {
		s.prepare = reviewer.PrepareReview
	}
	if s.diff == nil {
		s.diff = reviewer.PRDiff
	}
	if s.test == nil {
		s.test = reviewer.RunTests
	}
	if s.submit == nil {
		s.submit = reviewer.SubmitReview
	}
}

// Review reviews the pull request of a lane that is in Reviewing, and says
// where the lane goes: the transition for the engine to record. What goes
// wrong in a review is where the lane goes too (ReviewRetry, NeedsYou). An
// error is either the caller's mistake (a lane that is not being reviewed)
// or the engine stopping (ctx), and leaves the lane where it is.
//
// The reviewer's files are kept under the lane's run folder for this
// review, named by the sequence number of the transition that started it
// (agent.RunDir(root, issue, lane.Seq)): one folder per review, as unique as
// the event that began it.
func (s *ReviewStep) Review(ctx context.Context, lane Lane) (Transition, error) {
	if lane.State != Reviewing {
		return Transition{}, fmt.Errorf("lane #%d is %s: only a lane in %s is reviewed", lane.Issue, show(lane.State), Reviewing)
	}
	if lane.PR == 0 {
		return Transition{}, fmt.Errorf("lane #%d has no pull request to review", lane.Issue)
	}
	s.defaults()
	move := func(to State, cause Cause, note string) Transition {
		return Transition{Issue: lane.Issue, From: Reviewing, To: to, Cause: cause, Note: note, At: s.now().UTC()}
	}
	retry := func(note string) Transition { return move(ReviewRetry, CauseReviewFailed, note) }
	stopped := func() error { return ctx.Err() }

	rc, err := s.prepare(ctx, reviewer.PrepareReviewOptions{PRNumber: lane.PR, IssueNumber: lane.Issue, StateRoot: s.Root, GHTimeout: s.GHTimeout})
	if stopped() != nil {
		return Transition{}, stopped()
	}
	if err != nil {
		return retry("the review could not be prepared: " + err.Error()), nil
	}
	if strings.TrimSpace(rc.Ticket) == "" {
		return retry(fmt.Sprintf("the ticket of issue #%d could not be read", lane.Issue)), nil
	}
	diff, err := s.diff(ctx, lane.PR, s.GHTimeout)
	if stopped() != nil {
		return Transition{}, stopped()
	}
	if err != nil {
		return retry("the pull request's diff could not be read: " + err.Error()), nil
	}
	settings := reviewer.GetReviewSettings(s.Root)

	// The engine runs the tests; the reviewer is told what they said, and
	// the evidence gate checks the review against the same run.
	var run *reviewer.TestRun
	if rc.TestCommand != "" {
		r := s.test(ctx, rc.WorktreePath, rc.TestCommand, rc.Language, settings.TestTimeout)
		if stopped() != nil {
			return Transition{}, stopped()
		}
		run = &r
	}

	c, err := agent.Build(agent.Reviewer, s.inputs(lane, rc, run, diff, settings), true)
	if err != nil {
		return Transition{}, err
	}
	schema := json.RawMessage(reviewSchema)
	if c.Untrusted {
		if schema, err = agent.SuspiciousSchema(schema); err != nil {
			return Transition{}, err
		}
	}
	dir := agent.RunDir(s.Root, lane.Issue, lane.Seq)
	env := agent.Envelope{Profile: s.Profile, Dir: rc.WorktreePath, Prompt: c.Prompt, Schema: schema, StateRoot: s.Root, Issue: lane.Issue}
	if err := agent.SaveEnvelope(dir, env, c, s.now().UTC()); err != nil {
		return retry("what the reviewer is given could not be kept: " + err.Error()), nil
	}
	guard.Record(s.Root, c.Verdicts, lane.Issue)
	if c.Blocked() {
		t := move(NeedsYou, CauseEscalated, "the guard stopped what the reviewer was to read: "+guard.Summary(guard.Blocking(c.Verdicts)))
		t.Why = WhyGuardBlocked
		return t, nil
	}

	review, t, err := s.ask(ctx, lane, c, env, dir, move, retry)
	if err != nil || review == nil {
		return t, err
	}

	opts := reviewer.SubmitReviewOptions{
		PRNumber: lane.PR, IssueNumber: lane.Issue,
		Score: review.Score, CIStatus: rc.CIStatus,
		ReviewBody: review.RenderMarkdown(), Structured: review,
		StateRoot: s.Root, WorktreePath: rc.WorktreePath,
		TestCommand: rc.TestCommand, Language: rc.Language, Ticket: rc.Ticket,
		ScoreThreshold: settings.ScoreThreshold, MergeStrategy: settings.MergeStrategy,
		GHTimeout: s.GHTimeout, TestTimeout: settings.TestTimeout,
		Publisher: s.Publisher, TestRun: run, Diff: diff,
	}
	// The project's review hooks fire as they do for the Principal's.
	if cfg, err := analyzer.LoadConfig(config.Path(s.Root)); err == nil {
		opts.HookRunner = hooks.NewHookRunner(cfg.Hooks, s.Root, os.Stderr)
	}
	res, err := s.submit(ctx, opts)
	if err != nil {
		if stopped() != nil {
			return Transition{}, stopped()
		}
		return retry("the review could not be submitted: " + err.Error()), nil
	}
	return s.after(lane, res), nil
}

// ask runs the reviewer until it gives a review the workflow can take, as
// often as its settings allow. It returns the review, or, when there is
// none, where the lane goes instead.
func (s *ReviewStep) ask(ctx context.Context, lane Lane, c *agent.Context, env agent.Envelope, dir string,
	move func(State, Cause, string) Transition, retry func(string) Transition) (*reviewer.StructuredReview, Transition, error) {
	prompt := c.Prompt
	for try := 1; ; try++ {
		env.Prompt = prompt
		env.RunDir = filepath.Join(dir, "reviewer-"+strconv.Itoa(try))
		res := s.Agents.Run(ctx, env)
		if ctx.Err() != nil {
			return nil, Transition{}, ctx.Err()
		}
		switch res.Status {
		case agent.StatusOK:
		case agent.StatusRefused:
			t := move(NeedsYou, CauseEscalated, "the reviewer was not run: "+res.Error)
			t.Why = WhyAgentRefused
			return nil, t, nil
		default:
			return nil, retry(fmt.Sprintf("the reviewer ended %s: %s", res.Status, res.Error)), nil
		}
		// What the reviewer reported as an attempt to steer it is kept as
		// a verdict; the review goes on (design.md §9.2).
		guard.Record(s.Root, c.SuspiciousVerdicts(c.Suspicious(res.Output, res.Text)), lane.Issue)

		review, problems := parseReview(res.Output)
		if problems == "" {
			return review, Transition{}, nil
		}
		if try > s.Profile.SchemaRetries {
			return nil, retry("the reviewer's answer is not a review the workflow can take: " + problems), nil
		}
		prompt = c.Prompt + "\n\nYour last answer was not accepted: " + problems + ". Answer again, with each field as its description and the instructions say."
	}
}

// parseReview reads a reviewer's answer as a review: the schema's fields,
// without the suspicious blocks, held to what reviewer.ParseStructuredReview
// asks of a submission. problems is what is wrong with it, or "".
func parseReview(output json.RawMessage) (review *reviewer.StructuredReview, problems string) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output, &fields); err != nil {
		return nil, "the answer is not a JSON object"
	}
	delete(fields, "suspicious")
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err.Error()
	}
	review, errs := reviewer.ParseStructuredReview(data)
	if len(errs) > 0 {
		said := make([]string, len(errs))
		for i, e := range errs {
			said[i] = e.String()
		}
		return nil, strings.Join(said, "; ")
	}
	return review, ""
}

// needsYouWhy is why a lane needs a person, for each reason a review gives.
var needsYouWhy = map[string]string{
	reviewer.NeedsYouMergeBlocked: WhyMergeBlocked,
	reviewer.NeedsYouMaintainer:   WhyNeedsHumanReview,
	reviewer.NeedsYouGuardOutput:  WhyGuardOutput,
}

// after is where a lane goes after its review was submitted. What it does
// not know needs a person: a lane is never moved on by a guess.
func (s *ReviewStep) after(lane Lane, res *reviewer.SubmitReviewResult) Transition {
	t := Transition{Issue: lane.Issue, From: Reviewing, Note: res.Reason, At: s.now().UTC()}
	switch res.Next {
	case reviewer.NextMerged:
		t.To, t.Cause = Merged, CauseMerged
	case reviewer.NextRework:
		t.To, t.Cause = Rework, CauseChangesWanted
	case reviewer.NextReviewRetry:
		t.To, t.Cause = ReviewRetry, CauseReviewFailed
	case reviewer.NextResolveConflict:
		t.To, t.Cause, t.Resolve = Resolving, CauseConflicting, ResolveConflict
	case reviewer.NextResolveRebase:
		t.To, t.Cause, t.Resolve = Resolving, CauseConflicting, ResolveRebase
	default:
		t.To, t.Cause = NeedsYou, CauseEscalated
		t.Why = needsYouWhy[res.NeedsYou]
		if t.Why == "" {
			t.Why = WhyNeedsHumanReview
		}
		if t.Note == "" {
			t.Note = fmt.Sprintf("the review ended %q, with nothing said of what comes next", res.Result)
		}
	}
	return t
}

// inputs are what the reviewer is given (design.md §5.6): its instructions,
// the ticket, the project's rules that apply to the files the change
// touches, the change, the engine's run of the tests, a maintainer's review
// comments, and the review lessons.
func (s *ReviewStep) inputs(lane Lane, rc *reviewer.ReviewContext, run *reviewer.TestRun, diff string, settings reviewer.ReviewSettings) []agent.Input {
	in := []agent.Input{
		{Kind: agent.KindInstructions, Source: "awkit", Title: "Your task", Text: instructionsFor(lane, rc.Language, settings.ScoreThreshold)},
		{Kind: agent.KindTicket, Source: fmt.Sprintf("issue #%d", lane.Issue), Title: "The ticket", Text: rc.Ticket},
	}
	in = append(in, s.rulesFor(reviewer.ChangedPaths(diff))...)

	change := diff
	if commits := strings.TrimSpace(rc.CommitsJSON); commits != "" && !strings.HasPrefix(commits, "ERROR:") {
		change = "Commits:\n" + commits + "\n\nDiff:\n" + diff
	}
	in = append(in, agent.Input{Kind: agent.KindDiff, Source: fmt.Sprintf("PR #%d", lane.PR), Title: "The change", Text: change, MaxChars: maxDiffChars})
	in = append(in, testInputs(run)...)

	if rc.MaintainerFeedback != "" {
		// Only the comments of people who can write to the repository are
		// kept (reviewer.selectHumanFeedback): the owner's word.
		in = append(in, agent.Input{
			Kind: agent.KindPRComment, Source: fmt.Sprintf("PR #%d review comments", lane.PR), Author: "maintainers", WriteAccess: true,
			Title: "A maintainer's review comments", Text: rc.MaintainerFeedback,
		})
	}
	if rc.LessonsSection != "" {
		in = append(in, agent.Input{Kind: agent.KindLesson, Source: "lessons", Title: "Lessons of past reviews", Text: rc.LessonsSection})
	}
	return in
}

// instructionsFor are the reviewer's instructions for a lane.
func instructionsFor(lane Lane, language string, threshold int) string {
	return strings.NewReplacer(
		"{{pr}}", strconv.Itoa(lane.PR),
		"{{issue}}", strconv.Itoa(lane.Issue),
		"{{threshold}}", strconv.Itoa(threshold),
		"{{test_names}}", testNames(language),
	).Replace(strings.TrimSpace(reviewInstructions))
}

// testNames says what a test's name is, in the project's language.
func testNames(language string) string {
	switch strings.ToLower(language) {
	case "go", "golang":
		return "in Go, the name after `func` (`func TestMovement(t *testing.T)` is `TestMovement`), and a subtest is `TestParent/SubTest`"
	case "node", "typescript", "javascript", "ts", "js":
		return "in JavaScript and TypeScript, the description of its `it()` or `test()`, copied verbatim"
	case "python":
		return "in Python, the test function's name (`test_collision`), or `TestClass::test_method`"
	default:
		return "the name the test runner prints for it"
	}
}

// testInputs are what the reviewer is told of the engine's run of the
// tests: the engine's own account, and what the tests printed. What they
// printed comes from the code under review, and is trusted no more than it.
func testInputs(run *reviewer.TestRun) []agent.Input {
	if run == nil {
		return []agent.Input{{Kind: agent.KindTestResult, Source: "engine", Title: "The tests",
			Text: "The project has no test command for this repository: the engine ran no tests."}}
	}
	verdict := "passed"
	if run.Err != nil {
		verdict = "failed: " + run.Err.Error()
	}
	output := run.Output
	if len(output) > maxTestOutputChars {
		output = fmt.Sprintf("(the first %d characters are left out)\n", len(output)-maxTestOutputChars) + output[len(output)-maxTestOutputChars:]
	}
	return []agent.Input{
		{Kind: agent.KindTestResult, Source: "engine", Title: "The tests",
			Text: fmt.Sprintf("The engine ran `%s` in the pull request's worktree. It %s.", run.Command, verdict)},
		{Kind: agent.KindTestResult, Source: "test output", Title: "What the tests printed", Quotes: []string{guard.Agent},
			Text: output, MaxChars: maxTestOutputChars + 100},
	}
}

// rulesFor are the project's rules that apply to files. They are read from
// the project, not from the worktree: a rule the change itself edits is not
// the rule it is held to.
func (s *ReviewStep) rulesFor(files []string) []agent.Input {
	all, err := rules.LoadAll(s.Root)
	if err != nil {
		return nil
	}
	var in []agent.Input
	for _, r := range rules.For(all, files...) {
		in = append(in, agent.Input{Kind: agent.KindRules, Source: r.File, Title: r.Title, Text: r.Text})
	}
	return in
}

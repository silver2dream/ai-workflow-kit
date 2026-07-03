package lessons

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FeedbackRecord mirrors the reviewer package's feedback JSONL entries.
// Defined locally so the JSONL file — not a Go import — is the contract
// between the reviewer (writer) and the distiller (reader); reviewer imports
// lessons for settlement, so importing back would cycle.
type FeedbackRecord struct {
	Timestamp  string   `json:"timestamp"`
	IssueID    int      `json:"issue_id"`
	PRNumber   int      `json:"pr_number"`
	Score      int      `json:"score"`
	Categories []string `json:"categories"`
	Summary    string   `json:"summary"`
	Paths      []string `json:"paths,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
}

// feedbackRelPath matches reviewer.feedbackFile.
const feedbackRelPath = ".ai/state/review_feedback.jsonl"

// numberedRecord pairs a feedback record with its 1-based line number so
// partial distillation runs can advance the watermark precisely.
type numberedRecord struct {
	Line int
	Rec  FeedbackRecord
}

// readFeedbackSince returns entries after the given line watermark and the
// total line count. Malformed lines are skipped but still occupy a line
// number (they advance the watermark when passed).
func readFeedbackSince(stateRoot string, sinceLine int) ([]numberedRecord, int, error) {
	f, err := os.Open(filepath.Join(stateRoot, feedbackRelPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, sinceLine, nil
		}
		return nil, sinceLine, err
	}
	defer f.Close()

	var records []numberedRecord
	line := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line++
		if line <= sinceLine {
			continue
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var rec FeedbackRecord
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			continue
		}
		records = append(records, numberedRecord{Line: line, Rec: rec})
	}
	if err := scanner.Err(); err != nil {
		return records, line, err
	}
	return records, line, nil
}

// DistillOptions configures a distillation run.
type DistillOptions struct {
	Model         string        // default "sonnet"
	Timeout       time.Duration // per-entry LLM timeout, default 60s
	MaxActive     int           // active+proven cap, default 30
	MaxCandidates int           // candidate cap, default 10
	MaxEntries    int           // max feedback entries per run, default 5
}

// DistillReport summarizes what a distillation run did.
type DistillReport struct {
	Processed    int
	Created      []string // new lesson IDs
	Matched      []string // upvoted lesson IDs
	Skipped      int      // NOOP or unparseable
	RunnerErrors int      // LLM invocation failures (CLI missing, timeout)
}

// distillRunnerFunc invokes the distiller LLM; replaced in tests.
var distillRunnerFunc = runDistillCLI

func runDistillCLI(ctx context.Context, prompt, model string) (string, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return "", fmt.Errorf("claude CLI not found in PATH")
	}
	cmd := exec.CommandContext(ctx, "claude", "--print", "--model", model, "--max-turns", "1")
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("distiller timed out")
		}
		return "", fmt.Errorf("claude exited with error: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), nil
}

// Distill processes new feedback entries since the watermark: one LLM call
// per entry produces a MATCH / NEW / NOOP decision that the Go curator
// validates and applies. LLM or parse failures skip the entry (the watermark
// still advances — a garbled distillation is dropped, not retried forever).
func Distill(ctx context.Context, stateRoot string, opts DistillOptions) (*DistillReport, error) {
	if opts.Model == "" {
		opts.Model = "sonnet"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 5
	}

	s, err := Load(stateRoot)
	if err != nil {
		return nil, err
	}
	records, newWatermark, err := readFeedbackSince(stateRoot, s.Watermark.FeedbackLine)
	if err != nil {
		return nil, err
	}

	report := &DistillReport{}
	consumedThrough := s.Watermark.FeedbackLine
	processedAll := true
	for _, nr := range records {
		if report.Processed >= opts.MaxEntries {
			processedAll = false
			break
		}
		rec := nr.Rec
		consumedThrough = nr.Line
		// Only failures are distilled into pitfalls; approved entries feed
		// settlement, not distillation (strategy lessons are Phase C).
		if rec.Outcome == "approved" || strings.TrimSpace(rec.Summary) == "" {
			continue
		}
		report.Processed++

		prompt := buildDistillPrompt(rec, s)
		callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		output, err := distillRunnerFunc(callCtx, prompt, opts.Model)
		cancel()
		if err != nil {
			report.Skipped++
			report.RunnerErrors++
			continue
		}

		decision, err := parseDistillOutput(output)
		if err != nil {
			report.Skipped++
			continue
		}
		applyDecision(s, decision, rec, stateRoot, report)
	}

	// If every LLM call failed (claude missing, backend down), nothing was
	// actually judged — keep the watermark so the entries are retried on the
	// next run instead of being silently consumed. Parse failures DO advance
	// (the model ran; garbled output is dropped by design, not retried).
	allRunsFailed := report.Processed > 0 && report.RunnerErrors == report.Processed
	if allRunsFailed {
		// leave watermark unchanged
	} else if processedAll {
		s.Watermark.FeedbackLine = newWatermark
	} else {
		s.Watermark.FeedbackLine = consumedThrough
	}
	s.Watermark.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.EnforceCaps(opts.MaxActive, opts.MaxCandidates, time.Now().UTC())

	if err := Save(stateRoot, s); err != nil {
		return report, err
	}
	return report, nil
}

// buildDistillPrompt constructs the strict-format distillation prompt.
// Existing lessons are passed as an id+title digest to keep tokens low.
func buildDistillPrompt(rec FeedbackRecord, s *Store) string {
	var digest strings.Builder
	for _, l := range s.Lessons {
		if l.Status == StatusRetired {
			continue
		}
		digest.WriteString(fmt.Sprintf("- %s: %s [categories: %s]\n",
			l.ID, l.Title, strings.Join(l.Categories, ", ")))
	}
	if digest.Len() == 0 {
		digest.WriteString("(none)\n")
	}

	return fmt.Sprintf(`You are a lessons curator for an automated code-review workflow.
A pull request was rejected. Decide whether this rejection matches an existing lesson, reveals a NEW generalizable lesson, or is too specific to keep (NOOP).

Rules:
- A lesson must be a durable, generalizable check — not a restatement of this one rejection.
- Prefer MATCH over NEW when an existing lesson covers the same root cause.
- CONTENT must be actionable checks/constraints (imperative bullets), not narrative.
- SCOPE must be repository path prefixes taken ONLY from the rejection's paths below; never invent paths.
- If the rejection is task-specific noise (flaky CI, one-off typo), answer NOOP.

Existing lessons:
%s
Rejection:
- Issue: #%d  PR: #%d  Score: %d
- Categories: %s
- Paths: %s
- Reason: %s

Answer EXACTLY in this format (omit TITLE and later lines for MATCH/NOOP):
DECISION: MATCH <lesson-id> | NEW | NOOP
TITLE: <short imperative title>
DESCRIPTION: <one line: what went wrong>
CONTENT:
- <check or constraint>
- <check or constraint>
CATEGORIES: <comma-separated from: test, error-handling, naming, architecture, security, performance, scope, style, config, ci-failure, severity-consistency, criteria-mapping, assertion>
SCOPE: <comma-separated path prefixes from the rejection's paths, or empty>`,
		digest.String(), rec.IssueID, rec.PRNumber, rec.Score,
		strings.Join(rec.Categories, ", "), strings.Join(rec.Paths, ", "), rec.Summary)
}

// DistillDecision is the parsed distiller output.
type DistillDecision struct {
	Action      string // MATCH | NEW | NOOP
	MatchID     string
	Title       string
	Description string
	Content     string
	Categories  []string
	Scope       []string
}

var decisionRe = regexp.MustCompile(`(?im)^DECISION:\s*(MATCH\s+(L-\d+)|NEW|NOOP)\s*$`)

// parseDistillOutput parses the strict distiller format. Any deviation is an
// error — a garbled distillation must be dropped, never guessed at.
func parseDistillOutput(output string) (*DistillDecision, error) {
	m := decisionRe.FindStringSubmatch(output)
	if m == nil {
		return nil, fmt.Errorf("no DECISION line in distiller output")
	}
	d := &DistillDecision{}
	switch {
	case strings.HasPrefix(strings.ToUpper(m[1]), "MATCH"):
		d.Action = "MATCH"
		d.MatchID = m[2]
		return d, nil
	case strings.EqualFold(m[1], "NOOP"):
		d.Action = "NOOP"
		return d, nil
	}
	d.Action = "NEW"

	d.Title = captureLine(output, "TITLE")
	d.Description = captureLine(output, "DESCRIPTION")
	d.Content = captureContent(output)
	for _, c := range strings.Split(captureLine(output, "CATEGORIES"), ",") {
		if c = strings.TrimSpace(strings.ToLower(c)); c != "" {
			d.Categories = append(d.Categories, c)
		}
	}
	for _, p := range strings.Split(captureLine(output, "SCOPE"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			d.Scope = append(d.Scope, normPath(p))
		}
	}

	if d.Title == "" || d.Content == "" {
		return nil, fmt.Errorf("NEW decision missing TITLE or CONTENT")
	}
	if len(d.Categories) == 0 {
		return nil, fmt.Errorf("NEW decision missing CATEGORIES")
	}
	return d, nil
}

func captureLine(output, key string) string {
	re := regexp.MustCompile(`(?im)^` + key + `:\s*(.*)$`)
	if m := re.FindStringSubmatch(output); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// captureContent collects the bullet lines under CONTENT: until the next
// KEY: line.
func captureContent(output string) string {
	lines := strings.Split(output, "\n")
	var b strings.Builder
	in := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "CONTENT:") {
			in = true
			continue
		}
		if in {
			if strings.HasPrefix(upper, "CATEGORIES:") || strings.HasPrefix(upper, "SCOPE:") ||
				strings.HasPrefix(upper, "DECISION:") || strings.HasPrefix(upper, "TITLE:") {
				break
			}
			if trimmed != "" {
				b.WriteString(trimmed)
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// applyDecision is the deterministic curator: it validates and applies one
// distiller decision to the store.
func applyDecision(s *Store, d *DistillDecision, rec FeedbackRecord, repoRoot string, report *DistillReport) {
	ev := Evidence{Issue: rec.IssueID, PR: rec.PRNumber, Type: firstCategory(rec.Categories)}
	now := time.Now().UTC().Format(time.RFC3339)

	switch d.Action {
	case "NOOP":
		report.Skipped++
		return

	case "MATCH":
		l := s.FindByID(d.MatchID)
		if l == nil || l.Status == StatusRetired {
			report.Skipped++
			return
		}
		l.Hits++
		l.LastHitAt = now
		l.Evidence = appendEvidence(l.Evidence, ev)
		advanceStatus(l)
		report.Matched = append(report.Matched, l.ID)
		return

	case "NEW":
		// Scope hallucination guard: keep only prefixes that exist in the
		// repo OR appear in the rejection's own paths.
		d.Scope = validateScope(repoRoot, d.Scope, rec.Paths)

		fp := FingerprintOf(d.Title, d.Categories, d.Scope)
		for i := range s.Lessons {
			if s.Lessons[i].Fingerprint == fp && s.Lessons[i].Status != StatusRetired {
				// Duplicate of an existing lesson: treat as MATCH.
				s.Lessons[i].Hits++
				s.Lessons[i].LastHitAt = now
				s.Lessons[i].Evidence = appendEvidence(s.Lessons[i].Evidence, ev)
				advanceStatus(&s.Lessons[i])
				report.Matched = append(report.Matched, s.Lessons[i].ID)
				return
			}
		}

		l := Lesson{
			ID:          s.NextID(),
			Title:       d.Title,
			Description: d.Description,
			Content:     d.Content,
			Kind:        KindPitfall,
			Categories:  d.Categories,
			Scope:       d.Scope,
			Fingerprint: fp,
			Status:      StatusCandidate,
			Evidence:    []Evidence{ev},
			CreatedAt:   now,
			Source:      "distiller",
		}
		s.Lessons = append(s.Lessons, l)
		report.Created = append(report.Created, l.ID)
	}
}

// validateScope keeps only scope prefixes that exist on disk or are backed
// by the rejection's own paths — the distiller cannot invent locations.
func validateScope(repoRoot string, scope, evidencePaths []string) []string {
	var out []string
	for _, p := range scope {
		p = strings.TrimSpace(normPath(p))
		if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
			continue
		}
		ok := false
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(strings.TrimSuffix(p, "/")))); err == nil {
			ok = true
		} else {
			for _, ep := range evidencePaths {
				if strings.HasPrefix(strings.ToLower(normPath(ep)), strings.ToLower(p)) {
					ok = true
					break
				}
			}
		}
		if ok {
			out = append(out, p)
		}
	}
	return out
}

func appendEvidence(evs []Evidence, ev Evidence) []Evidence {
	for _, e := range evs {
		if e.Issue == ev.Issue && e.PR == ev.PR {
			return evs
		}
	}
	// Keep evidence bounded.
	if len(evs) >= 10 {
		evs = evs[1:]
	}
	return append(evs, ev)
}

func firstCategory(cats []string) string {
	if len(cats) > 0 {
		return cats[0]
	}
	return ""
}

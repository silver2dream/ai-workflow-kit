package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/lessons"
)

func usageLessons() {
	fmt.Fprint(os.Stderr, `Usage: awkit lessons <subcommand> [options]

Manage the learning loop's lesson store (.ai/state/lessons.json).
See .ai/specs/learning-loop/design.md for the full design.

Subcommands:
  list [--all]           List lessons (--all includes retired)
  stats                  Show status counts and per-lesson hit/miss rates
  add                    Add a lesson manually (post-mortem entry point)
      --title <t>        Required: short imperative title
      --content <c>      Required: actionable checks (use \n for bullets)
      --categories <a,b> Comma-separated categories
      --scope <p/,q/>    Comma-separated repo path prefixes
      --issue <N>        Evidence issue number
      --pr <N>           Evidence PR number
  distill [--max N]      Distill new review feedback into lessons via LLM
                         (processes at most N entries, default 5)
  promote <L-xxx>        Open a GitHub issue proposing to harden a proven
                         lesson into a rule/audit check (human-gated)

Options:
  --state-root <path>    Project root (default: current directory)
  --help, -h             Show this help
`)
}

func cmdLessons(args []string) int {
	if len(args) == 0 {
		usageLessons()
		return 2
	}
	sub := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet("lessons "+sub, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usageLessons

	stateRoot := fs.String("state-root", ".", "")
	showAll := fs.Bool("all", false, "")
	title := fs.String("title", "", "")
	content := fs.String("content", "", "")
	categories := fs.String("categories", "", "")
	scope := fs.String("scope", "", "")
	issue := fs.Int("issue", 0, "")
	pr := fs.Int("pr", 0, "")
	maxEntries := fs.Int("max", 5, "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")

	// promote takes a positional lesson ID before flags.
	var promoteID string
	if sub == "promote" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		promoteID = rest[0]
		rest = rest[1:]
	}

	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usageLessons()
		return 0
	}

	switch sub {
	case "list":
		return lessonsList(*stateRoot, *showAll)
	case "stats":
		return lessonsStats(*stateRoot)
	case "add":
		return lessonsAdd(*stateRoot, *title, *content, *categories, *scope, *issue, *pr)
	case "distill":
		return lessonsDistill(*stateRoot, *maxEntries)
	case "promote":
		return lessonsPromote(*stateRoot, promoteID)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", sub)
		usageLessons()
		return 2
	}
}

func lessonsList(stateRoot string, all bool) int {
	s, err := lessons.Load(stateRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(s.Lessons) == 0 {
		fmt.Println("No lessons yet. They accumulate as reviews reject PRs (or add one with 'awkit lessons add').")
		return 0
	}
	fmt.Printf("%-7s %-10s %4s %4s  %s\n", "ID", "STATUS", "HIT", "MISS", "TITLE")
	for _, l := range s.Lessons {
		if !all && l.Status == lessons.StatusRetired {
			continue
		}
		fmt.Printf("%-7s %-10s %4d %4d  %s\n", l.ID, l.Status, l.Hits, l.Misses, l.Title)
	}
	return 0
}

func lessonsStats(stateRoot string) int {
	s, err := lessons.Load(stateRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	counts := map[string]int{}
	for _, l := range s.Lessons {
		counts[l.Status]++
	}
	fmt.Printf("Lessons: %d total — candidate: %d, active: %d, proven: %d, retired: %d\n",
		len(s.Lessons), counts[lessons.StatusCandidate], counts[lessons.StatusActive],
		counts[lessons.StatusProven], counts[lessons.StatusRetired])
	fmt.Printf("Feedback watermark: line %d (updated %s)\n\n", s.Watermark.FeedbackLine, s.Watermark.UpdatedAt)

	sorted := make([]lessons.Lesson, len(s.Lessons))
	copy(sorted, s.Lessons)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Hits > sorted[b].Hits })

	fmt.Printf("%-7s %-10s %4s %4s %8s  %s\n", "ID", "STATUS", "HIT", "MISS", "MISSRATE", "TITLE")
	for _, l := range sorted {
		total := l.Hits + l.Misses
		rate := "-"
		if total > 0 {
			rate = fmt.Sprintf("%.0f%%", float64(l.Misses)/float64(total)*100)
		}
		fmt.Printf("%-7s %-10s %4d %4d %8s  %s\n", l.ID, l.Status, l.Hits, l.Misses, rate, l.Title)
	}
	return 0
}

func lessonsAdd(stateRoot, title, content, categories, scope string, issue, pr int) int {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" {
		fmt.Fprintln(os.Stderr, "error: --title and --content are required")
		return 2
	}
	s, err := lessons.Load(stateRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	var cats, scopes []string
	for _, c := range strings.Split(categories, ",") {
		if c = strings.TrimSpace(strings.ToLower(c)); c != "" {
			cats = append(cats, c)
		}
	}
	for _, p := range strings.Split(scope, ",") {
		if p = strings.TrimSpace(p); p != "" {
			scopes = append(scopes, filepath.ToSlash(p))
		}
	}

	l := lessons.Lesson{
		ID:          s.NextID(),
		Title:       strings.TrimSpace(title),
		Content:     strings.ReplaceAll(content, `\n`, "\n"),
		Kind:        lessons.KindPitfall,
		Categories:  cats,
		Scope:       scopes,
		Fingerprint: lessons.FingerprintOf(title, cats, scopes),
		Status:      lessons.StatusCandidate,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Source:      "human",
	}
	if issue > 0 {
		l.Evidence = []lessons.Evidence{{Issue: issue, PR: pr}}
	}
	s.Lessons = append(s.Lessons, l)

	cfg := loadLessonsConfig(stateRoot)
	s.EnforceCaps(cfg.MaxActive, 10, time.Now().UTC())
	if err := lessons.Save(stateRoot, s); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("Added %s (candidate): %s\n", l.ID, l.Title)
	return 0
}

func lessonsDistill(stateRoot string, maxEntries int) int {
	cfg := loadLessonsConfig(stateRoot)
	if !cfg.IsEnabled() {
		fmt.Println("lessons: disabled in workflow.yaml (lessons.enabled: false)")
		return 0
	}
	timeout := 60 * time.Second
	if cfg.Distiller.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.Distiller.TimeoutSeconds) * time.Second
	}

	report, err := lessons.Distill(context.Background(), stateRoot, lessons.DistillOptions{
		Model:      cfg.Distiller.Model,
		Timeout:    timeout,
		MaxActive:  cfg.MaxActive,
		MaxEntries: maxEntries,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("Distilled %d entr(ies): %d new, %d matched, %d skipped\n",
		report.Processed, len(report.Created), len(report.Matched), report.Skipped)
	for _, id := range report.Created {
		fmt.Printf("  new: %s\n", id)
	}
	for _, id := range report.Matched {
		fmt.Printf("  upvoted: %s\n", id)
	}
	return 0
}

func lessonsPromote(stateRoot, id string) int {
	if id == "" {
		fmt.Fprintln(os.Stderr, "error: usage: awkit lessons promote <L-xxx>")
		return 2
	}
	s, err := lessons.Load(stateRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	l := s.FindByID(id)
	if l == nil {
		fmt.Fprintf(os.Stderr, "error: lesson %s not found\n", id)
		return 1
	}
	if l.Status != lessons.StatusProven {
		fmt.Fprintf(os.Stderr, "error: only proven lessons can be promoted (%s is %s)\n", id, l.Status)
		return 1
	}

	var evidence strings.Builder
	for _, ev := range l.Evidence {
		evidence.WriteString(fmt.Sprintf("- issue #%d", ev.Issue))
		if ev.PR > 0 {
			evidence.WriteString(fmt.Sprintf(" / PR #%d", ev.PR))
		}
		evidence.WriteString("\n")
	}

	body := fmt.Sprintf(`## Objective
Harden proven lesson **%s** into an enforced gate so this failure pattern can no longer recur through prompt drift.

## Lesson
**%s**

%s

- Categories: %s
- Scope: %s
- Track record: %d hits / %d misses

## Evidence
%s
## Proposed enforcement (pick one)
- [ ] Rule file under .ai/rules/ (worker-facing constraint)
- [ ] audit.custom check in workflow.yaml (deterministic gate)
- [ ] escalation trigger (pause/approval pattern)
- [ ] lifecycle hook (pre_dispatch / pre_review command)

## Constraints
- This promotion was proposed automatically by the learning loop; the enforcement change itself MUST be reviewed by a human before merge.

_Source: awkit lessons promote %s_`,
		l.ID, l.Title, l.Content,
		strings.Join(l.Categories, ", "), strings.Join(l.Scope, ", "),
		l.Hits, l.Misses, evidence.String(), l.ID)

	titleArg := fmt.Sprintf("[feat] promote lesson %s to enforced gate: %s", l.ID, l.Title)
	cmd := exec.Command("gh", "issue", "create", "--title", titleArg, "--body", body, "--label", "enhancement")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: gh issue create failed: %v\n", err)
		return 1
	}
	return 0
}

// loadLessonsConfig reads the lessons section of workflow.yaml with defaults.
func loadLessonsConfig(stateRoot string) analyzer.LessonsConfig {
	cfg, err := analyzer.LoadConfig(filepath.Join(stateRoot, ".ai", "config", "workflow.yaml"))
	if err != nil {
		return analyzer.LessonsConfig{}
	}
	return cfg.Lessons
}

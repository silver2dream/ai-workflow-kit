package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/kickoff"
	"github.com/silver2dream/ai-workflow-kit/internal/repo"
	"github.com/silver2dream/ai-workflow-kit/internal/task"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

func usagePlan() {
	fmt.Fprint(os.Stderr, `Plan work with the council: several roles argue a brief out

Usage:
  awkit plan "<what you want built>" [options]
  awkit plan --brief-file <path> [options]
  awkit plan --resume <plan>
  awkit plan approve <plan> [--dry-run]

What a plan does:
  Every role of .ai/roster.yaml states where it stands on the brief. The roles
  object to each other, and the objections that matter most are argued out
  between the two roles concerned. A moderator who takes no side says where
  the meeting stands, the deciding role rules on what is still disputed, every
  role writes the documents it owns under docs/, and the planning role draws
  up the tickets, which the table gets to object to once.

  What only you can answer, the council assumes, and says so: a plan never
  waits for a person. With --ask it puts its questions to you first, a minute
  or so into the meeting, and argues from your answers.

  A plan ends with a proposal: docs/plans/<plan>/tickets.yaml. Read it, change
  what you want changed, then create the issues:

    awkit plan approve <plan>

  Nothing is created on GitHub before that.

Options:
  --ask          Answer the council's questions before it argues (it waits for you)
  --brief-file   Read the brief from a file
  --resume       Go on with a plan that stopped; its name is its folder in docs/plans/
  --roster       The roster to use (default: .ai/roster.yaml)
  --state-root   Project root (default: git root)

Options of approve:
  --dry-run      Say what would be created, and create nothing

Examples:
  awkit plan "A daily login reward: seven days, the reward grows, a missed day starts over"
  awkit plan --ask --brief-file notes/reward.md
  awkit plan --resume 20261114-a-daily-login-reward
  awkit plan approve 20261114-a-daily-login-reward
`)
}

// planModel answers for the roles of a plan. Replaced in tests.
var planModel council.Model = council.ClaudeCLI{}

// planInput is where the answers to the council's questions are read from.
// Replaced in tests.
var planInput io.Reader = os.Stdin

// ownerAtTerminal puts the council's questions to the person at the terminal,
// one at a time. A line is an answer; an empty line leaves the question to
// the council. When the input ends, the questions that are left stay open.
func ownerAtTerminal(in io.Reader, out io.Writer) func(context.Context, []string) ([]string, error) {
	reader := bufio.NewReader(in)
	return func(ctx context.Context, questions []string) ([]string, error) {
		fmt.Fprintf(out, "\nThe council has %d question(s) only you can answer.\nAnswer in a line, or press Enter to let the council assume.\n", len(questions))
		answers := make([]string, len(questions))
		for i, question := range questions {
			fmt.Fprintf(out, "\n%d/%d  %s\n> ", i+1, len(questions), strings.Join(strings.Fields(question), " "))
			line, err := reader.ReadString('\n')
			answers[i] = strings.TrimSpace(line)
			if err != nil {
				break
			}
		}
		fmt.Fprintln(out)
		return answers, nil
	}
}

// planCreate makes the issue of one ticket. Replaced in tests.
var planCreate = func(ctx context.Context, stateRoot, title, bodyFile string) (int, error) {
	result, err := task.CreateTask(ctx, task.CreateTaskOptions{Title: title, BodyFile: bodyFile, StateRoot: stateRoot})
	if err != nil {
		return 0, err
	}
	return result.IssueNumber, nil
}

func cmdPlan(args []string) int {
	if len(args) > 0 && args[0] == "approve" {
		return cmdPlanApprove(args[1:])
	}

	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usagePlan
	briefFile := fs.String("brief-file", "", "")
	askOwner := fs.Bool("ask", false, "")
	resume := fs.String("resume", "", "")
	rosterFile := fs.String("roster", "", "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")
	// The brief may come before or after the options.
	flags, words := splitFlags(args, map[string]bool{"--brief-file": true, "--resume": true, "--roster": true, "--state-root": true,
		"-brief-file": true, "-resume": true, "-roster": true, "-state-root": true})
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usagePlan()
		return 0
	}

	brief := strings.TrimSpace(strings.Join(words, " "))
	switch {
	case *resume != "" && (brief != "" || *briefFile != ""):
		errorf("--resume goes on with a plan that has its brief; give no other\n")
		return 2
	case brief != "" && *briefFile != "":
		errorf("give the brief as words or with --brief-file, not both\n")
		return 2
	case *briefFile != "":
		data, err := os.ReadFile(*briefFile)
		if err != nil {
			errorf("the brief cannot be read: %v\n", err)
			return 1
		}
		brief = strings.TrimSpace(string(data))
	}
	if brief == "" && *resume == "" {
		errorf("a plan needs a brief: say what you want built\n\n")
		usagePlan()
		return 2
	}

	root, code := planRoot(*stateRoot)
	if code != 0 {
		return code
	}
	repos, err := planRepos(root)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	roster, err := planRoster(root, *rosterFile, repos)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}

	fmt.Println("")
	fmt.Printf("AWK Plan: %d roles at the table (%s decides, %s plans), at most %d model calls\n", len(roster.Roles),
		roster.Decider().ID, roster.Planner().ID, roster.Limits.Calls)
	fmt.Println("")
	var ask func(context.Context, []string) ([]string, error)
	if *askOwner {
		ask = ownerAtTerminal(planInput, os.Stdout)
	}
	plan, err := council.Run(context.Background(), council.Options{
		Root: root, Brief: brief, Roster: roster, Model: planModel, Repos: repos, ID: *resume, Ask: ask,
		Progress: func(line string) { fmt.Println(line) },
	})
	fmt.Println("")
	if err != nil {
		errorf("%v\n", err)
		if plan != nil && plan.ID != "" {
			fmt.Fprintf(os.Stderr, "\nWhat was reached is in %s/minutes.md. Go on with:\n  awkit plan --resume %s\n", plan.Dir(), plan.ID)
		}
		return 1
	}

	success("Plan %s: %d decisions, %d documents written, %d tickets proposed (%d model calls)\n", plan.ID, len(plan.Decisions),
		writtenDocuments(plan), len(plan.Tickets), plan.Calls)
	fmt.Println("")
	fmt.Printf("  Minutes:    %s/minutes.md\n", plan.Dir())
	fmt.Printf("  Decisions:  %s\n", council.DecisionsPath)
	fmt.Printf("  Work list:  %s/%s\n", plan.Dir(), council.WorkListFile)
	fmt.Println("")
	for _, doc := range plan.Documents {
		// A document its owner could not write, not one a role had no right to.
		if owner := roster.Role(doc.Owner); doc.Refused != "" && owner != nil && owner.OwnsDocument(doc.Path) {
			warn("%s was not written: %s\n", doc.Path, doc.Refused)
		}
	}
	if len(plan.Assumptions) > 0 {
		fmt.Printf("The plan assumes %d thing(s) only you can answer (%s):\n", len(plan.Assumptions), council.DecisionsPath)
		for _, assumption := range plan.Assumptions {
			question, assumed := oneLine(assumption.Question), oneLine(assumption.Assumption)
			if question == "" {
				fmt.Printf("  %-4s %s\n", assumption.ID, assumed)
				continue
			}
			fmt.Printf("  %-4s %s\n       assumed: %s\n", assumption.ID, question, assumed)
		}
		if *askOwner {
			fmt.Println("Where one is wrong, correct the documents and the work list before you approve it.")
		} else {
			fmt.Printf("Where one is wrong, correct the documents and the work list before you approve it. With %s the council asks you first.\n", cyan("awkit plan --ask"))
		}
		fmt.Println("")
	}
	fmt.Println("Nothing is on GitHub yet. Read the work list, change what you want changed, then:")
	fmt.Printf("  %s\n", cyan("awkit plan approve "+plan.ID))
	fmt.Println("")
	fmt.Printf("A Worker reads the project from %s: commit and push docs/ there before %s.\n", integrationBranch(root), cyan("awkit run"))
	return 0
}

// writtenDocuments counts the documents the plan wrote: not the ones left as
// they were, and not the ones that were refused.
func writtenDocuments(plan *council.Plan) int {
	n := 0
	for _, doc := range plan.Documents {
		if doc.Refused == "" && !doc.Unchanged {
			n++
		}
	}
	return n
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func cmdPlanApprove(args []string) int {
	fs := flag.NewFlagSet("plan approve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = usagePlan
	dryRun := fs.Bool("dry-run", false, "")
	rosterFile := fs.String("roster", "", "")
	stateRoot := fs.String("state-root", "", "")
	showHelp := fs.Bool("help", false, "")
	showHelpShort := fs.Bool("h", false, "")
	flags, words := splitFlags(args, map[string]bool{"--roster": true, "--state-root": true, "-roster": true, "-state-root": true})
	if err := fs.Parse(flags); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usagePlan()
		return 0
	}
	if len(words) != 1 {
		errorf("name the plan to approve: its folder in %s/\n", council.PlansDir)
		// Say which ones there are, where the project can be found.
		project := *stateRoot
		if project == "" {
			project, _ = resolveGitRoot()
		}
		if open := council.Open(project); project != "" && len(open) > 0 {
			fmt.Fprintln(os.Stderr, "\nPlans with tickets that are not on GitHub yet:")
			for _, plan := range open {
				fmt.Fprintf(os.Stderr, "  %s\n", plan)
			}
		}
		return 2
	}
	id := words[0]

	root, code := planRoot(*stateRoot)
	if code != 0 {
		return code
	}
	repos, err := planRepos(root)
	if err != nil {
		errorf("%v\n", err)
		return 1
	}
	// The roster only serves the document index here: an approval does not
	// fail for want of one.
	var roster *council.Roster
	if file := rosterPath(root, *rosterFile); fileExists(file) {
		roster, _ = council.LoadRoster(file)
	}

	if !*dryRun {
		if dirty := uncommittedDocs(root); dirty != "" {
			warn("docs/ has changes that are not committed. A Worker reads the project from %s and will not see them:\n%s\n", integrationBranch(root), dirty)
		}
	}
	approval, err := council.Approve(context.Background(), council.ApproveOptions{
		Root: root, ID: id, Repos: repos, Roster: roster, DryRun: *dryRun,
		Create: func(ctx context.Context, title, bodyFile string) (int, error) {
			return planCreate(ctx, root, title, bodyFile)
		},
		Progress: func(line string) { fmt.Println(line) },
	})
	if err != nil {
		errorf("%v\n", err)
		if approval != nil && len(approval.Created) > 0 {
			fmt.Fprintf(os.Stderr, "\n%d issue(s) were created before that and are on record in the work list. Run the command again to go on.\n", len(approval.Created))
		}
		return 1
	}
	fmt.Println("")
	if *dryRun {
		fmt.Printf("DRY_RUN: %d issue(s) would be created, %d exist already\n", len(approval.Created), len(approval.Existing))
		return 0
	}
	success("%d issue(s) created, %d existed already\n", len(approval.Created), len(approval.Existing))
	if len(approval.Created)+len(approval.Existing) > 0 {
		fmt.Println("")
		fmt.Printf("Start the work with %s\n", cyan("awkit run"))
	}
	return 0
}

// splitFlags tells the options of a command line from its other words, so
// that the words may come first. takesValue names the options that are
// followed by a value.
func splitFlags(args []string, takesValue map[string]bool) (flags, words []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			words = append(words, args[i+1:]...)
			return flags, words
		case strings.HasPrefix(arg, "-") && arg != "-":
			flags = append(flags, arg)
			if takesValue[arg] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			words = append(words, arg)
		}
	}
	return flags, words
}

func planRoot(flagValue string) (string, int) {
	if flagValue != "" {
		return flagValue, 0
	}
	root, err := resolveGitRoot()
	if err != nil {
		errorf("failed to resolve git root: %v\n", err)
		return "", 1
	}
	return root, 0
}

// planRepos lists the repos a ticket can be for: the ones of the config, and
// the repository root.
func planRepos(root string) ([]council.Repo, error) {
	cfg, err := analyzer.LoadConfig(filepath.Join(root, ".ai", "config", "workflow.yaml"))
	if err != nil {
		return nil, fmt.Errorf("the project's config cannot be read (.ai/config/workflow.yaml): %w\nRun `awkit init` first", err)
	}
	var repos []council.Repo
	for _, name := range cfg.TicketRepos() {
		entry := council.Repo{Name: name}
		if configured := cfg.GetRepoByName(name); configured != nil {
			entry.Language, entry.Path = configured.Language, configured.Path
		} else if name == ticket.RootRepo {
			entry.Path = "./"
		}
		repos = append(repos, entry)
	}
	return repos, nil
}

func rosterPath(root, flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return filepath.Join(root, filepath.FromSlash(council.RosterPath))
}

// planRoster reads the project's roster. A project that has none gets the one
// that fits its repos, written to .ai/roster.yaml so that it can be changed.
func planRoster(root, flagValue string, repos []council.Repo) (*council.Roster, error) {
	file := rosterPath(root, flagValue)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		if flagValue != "" {
			return nil, fmt.Errorf("the roster %s does not exist", file)
		}
		preset := rosterPresetFor(repos)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(file, council.DefaultRoster(preset), 0o644); err != nil {
			return nil, err
		}
		info("This project had no roster: wrote %s (the %s one). Change it to fit the project; this plan uses it as it is.\n", council.RosterPath, preset)
	}
	roster, err := council.LoadRoster(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return roster, nil
}

// rosterPresetFor picks the roster a project starts with from its repos: the
// game studio for a Go server with a C# (Godot) client, the generic one for
// everything else.
func rosterPresetFor(repos []council.Repo) string {
	var hasGo, hasDotnet bool
	for _, entry := range repos {
		switch repo.ToolchainOf(entry.Language) {
		case repo.ToolchainGo:
			hasGo = true
		case repo.ToolchainDotnet:
			hasDotnet = true
		}
	}
	if hasGo && hasDotnet {
		return "godot-go"
	}
	return "generic"
}

// integrationBranch names the branch pull requests go to, for a message.
func integrationBranch(root string) string {
	if cfg, err := kickoff.LoadConfig(filepath.Join(root, ".ai", "config", "workflow.yaml")); err == nil && cfg.Git.IntegrationBranch != "" {
		return "the integration branch (" + cfg.Git.IntegrationBranch + ")"
	}
	return "the integration branch"
}

// uncommittedDocs lists what git says is not committed under docs/.
func uncommittedDocs(root string) string {
	out, err := exec.Command("git", "-C", root, "status", "--porcelain", "--", "docs").Output()
	if err != nil {
		return ""
	}
	// Nothing to report is one empty line, and comes back as nothing.
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	const show = 8
	if len(lines) > show {
		lines = append(lines[:show], fmt.Sprintf("  ... and %d more", len(lines)-show))
	}
	return strings.Join(lines, "\n")
}

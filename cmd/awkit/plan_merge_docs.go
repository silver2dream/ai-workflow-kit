package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/council"
	"github.com/silver2dream/ai-workflow-kit/internal/landing"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	status "github.com/silver2dream/ai-workflow-kit/internal/status"
	"github.com/silver2dream/ai-workflow-kit/internal/task"
)

// What an approval that merges the documents asks of git, GitHub and the
// project. Replaced in tests.
var (
	planCreateUnlabeled = func(ctx context.Context, stateRoot, title, bodyFile string) (int, error) {
		result, err := task.CreateTask(ctx, task.CreateTaskOptions{Title: title, BodyFile: bodyFile, StateRoot: stateRoot, Unlabeled: true})
		if err != nil {
			return 0, err
		}
		return result.IssueNumber, nil
	}
	planLabel                      = task.Label
	planGit         landing.Runner = landing.Exec
	planGitHub      landing.GitHub // gh in the project when nil
	planLandTimings = func(*landing.Options) {}
	planRunGoing    = func(root string) bool {
		report, err := status.Collect(root, status.Options{})
		return err == nil && report.Run.State == "running"
	}
)

// docPaths are where a meeting writes: docs/, the documents the roster names
// elsewhere, and the roster, which a project's first plan writes.
func docPaths(roster *council.Roster) []string {
	paths := []string{"docs", layout.RosterFile}
	if roster != nil {
		for _, role := range roster.Roles {
			for _, doc := range role.Owns {
				if !strings.HasPrefix(doc, "docs/") {
					paths = append(paths, doc)
				}
			}
		}
	}
	return paths
}

// approveMergingDocs approves a plan's work list so that no Worker starts
// before the plan's documents are on the integration branch: the issues are
// created without the task label; the documents, the work list with its
// issues among them, go to the integration branch through a pull request
// merged once its checks pass; then the issues get the label, and the
// working tree is brought up to the branch. Where it is, it writes to the
// plan's approval record as it goes, for the workbench. Approving again goes
// on where it stopped.
func approveMergingDocs(ctx context.Context, root, id string, repos []council.Repo, roster *council.Roster, dryRun bool) int {
	record := &council.ApprovalRecord{PID: os.Getpid(), State: council.ApprovalWorking, Step: "check", Started: time.Now()}
	save := func() {
		if !dryRun {
			_ = council.WriteApproval(root, id, record)
		}
	}
	say := func(line string) {
		fmt.Println(line)
		record.Say(line)
		save()
	}
	step := func(name string) {
		record.Step = name
		save()
	}
	fail := func(why string, err error) int {
		record.State, record.Why, record.Error = council.ApprovalFailed, why, err.Error()
		save()
		errorf("%v\n", err)
		return 1
	}
	save()

	cfg, err := config.Load(root)
	if err != nil {
		return fail("error", fmt.Errorf("the project's config cannot be read: %w", err))
	}
	integration := cfg.Git.IntegrationBranch
	if planRunGoing(root) {
		return fail("run", errors.New("a run is going in this project: approve once it has stopped (awkit stop), so that it does not take up the tickets half way"))
	}
	if _, err := planGit(ctx, root, "git", "ls-remote", "--exit-code", "--heads", "origin", integration); err != nil {
		return fail("branch", fmt.Errorf("the integration branch %s is not on origin, or origin cannot be reached: the documents cannot be merged into it (%v)", integration, err))
	}

	step("issues")
	approval, err := council.Approve(ctx, council.ApproveOptions{
		Root: root, ID: id, Repos: repos, DryRun: dryRun,
		Create: func(ctx context.Context, title, bodyFile string) (int, error) {
			return planCreateUnlabeled(ctx, root, title, bodyFile)
		},
		Progress: say,
	})
	if approval != nil && !dryRun {
		record.Created = len(approval.Created)
	}
	if err != nil {
		return fail("error", err)
	}
	if list, err := council.LoadWorkList(root, id); err == nil {
		record.Issues = record.Issues[:0]
		for _, ticket := range list.Tickets {
			if ticket.Issue != 0 {
				record.Issues = append(record.Issues, ticket.Issue)
			}
		}
	}

	changes, err := landing.Pending(ctx, planGit, root, docPaths(roster))
	if err != nil {
		return fail("error", fmt.Errorf("git cannot say what the meeting changed: %w", err))
	}
	if dryRun {
		fmt.Println("")
		fmt.Printf("DRY_RUN: %d issue(s) would be created without the task label, %d exist already\n", len(approval.Created), len(approval.Existing))
		fmt.Printf("DRY_RUN: %d document(s) would go to %s through a pull request, merged once its checks pass:\n", len(changes), integration)
		for _, change := range changes {
			fmt.Printf("  %s\n", change.Path)
		}
		fmt.Println("DRY_RUN: then the issues would get the task label, and the working tree would be brought up to " + integration)
		return 0
	}

	step("documents")
	options := landing.Options{
		Root: root, Integration: integration, Branch: "feat/plan-" + id,
		Title:    fmt.Sprintf("[docs] plan %s: what the council wrote", id),
		Body:     landingBody(id, integration, changes),
		Strategy: cfg.Review.Strategy(), Changes: changes,
		Run: planGit, GitHub: planGitHub, Progress: say,
		OnPR: func(pr landing.PR) {
			record.PR, record.URL = pr.Number, pr.URL
			step("checks")
		},
	}
	planLandTimings(&options)
	if _, err := landing.Land(ctx, options); err != nil {
		var failed *landing.ChecksFailed
		var absent *landing.ChecksAbsent
		var slow *landing.ChecksTooLong
		var refused *landing.MergeRefused
		switch {
		case errors.As(err, &failed):
			return fail("checks", err)
		case errors.As(err, &absent):
			return fail("noci", err)
		case errors.As(err, &slow):
			return fail("slow", err)
		case errors.As(err, &refused):
			return fail("refused", err)
		}
		return fail("error", err)
	}

	step("label")
	if err := planLabel(ctx, root, record.Issues); err != nil {
		return fail("error", err)
	}
	say(fmt.Sprintf("%d ticket(s) are ready for a run", len(record.Issues)))

	if len(changes) > 0 {
		step("sync")
		synced, err := landing.Sync(ctx, planGit, root, integration, changes)
		switch {
		case err != nil:
			record.Left = err.Error()
		case !synced.Updated:
			record.Left = synced.Why
		default:
			say(fmt.Sprintf("The working tree is up to %s, and clean", integration))
		}
		if record.Left != "" {
			warn("The documents are on %s, but the working tree was not brought up to it: %s. A run starts only from a clean working tree.\n", integration, record.Left)
		}
	}
	record.State, record.Step = council.ApprovalDone, "done"
	save()
	fmt.Println("")
	success("%d issue(s) created, %d existed already; the documents are on %s\n", len(approval.Created), len(approval.Existing), integration)
	fmt.Printf("Start the work with %s\n", cyan("awkit run"))
	return 0
}

// landingBody says what the pull request of a plan's documents is.
func landingBody(id, integration string, changes []landing.Change) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The documents the council wrote for plan `%s`, and its work list with the issues of its tickets.\n\n", id)
	fmt.Fprintf(&b, "Approving the plan brings them to `%s` before any of its tickets is taken up: the issues are labelled for a run once this is merged.\n\n", integration)
	for _, change := range changes {
		if change.Deleted {
			fmt.Fprintf(&b, "- `%s` (removed)\n", change.Path)
		} else {
			fmt.Fprintf(&b, "- `%s`\n", change.Path)
		}
	}
	return b.String()
}

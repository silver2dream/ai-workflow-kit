package workflow

import (
	"fmt"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// Ending is how a run ended, said for a person: at the end of `awkit run`,
// and in `awkit status` for as long as that run is the last one.
type Ending struct {
	// Done is true when the run ended because nothing was left to do, or
	// because a person asked it to stop. False means it stopped short: work
	// is left, and something has to be looked at before it can go on.
	Done    bool
	Summary string // what happened
	Next    string // what to do about it
}

// EndingFor says how a run ended from the reason it stopped with: the reason
// in the STOP marker, which is the one `awkit stop-workflow` was given.
func EndingFor(reason string) Ending {
	switch reason {
	case "all_tasks_complete":
		return Ending{Done: true, Summary: "Every ticket is done.", Next: "Plan more work (awkit plan), or add a ticket (awkit create-task)."}
	case "user_stopped", "interrupted", "":
		return Ending{Done: true, Summary: "The run was stopped, as asked.", Next: "awkit run goes on from where it stopped."}
	case "needs_human_review", "escalation_triggered":
		return Ending{Summary: "A ticket needs you.", Next: "awkit status shows which one and why. Deal with it, take its needs-human-review label off, then awkit run."}
	case "review_max_retries":
		return Ending{Summary: "A pull request was turned back by the review as often as a review is tried.", Next: "awkit status shows which one. Read the review comments on it, then awkit run."}
	case "worker_failed", "max_failures":
		return Ending{Summary: "A ticket's Worker failed on every attempt.", Next: "awkit status shows which one; the issue has a comment with the last error. Fix the cause or the ticket, then awkit reset --labels and awkit run."}
	case "invalid_ticket":
		return Ending{Summary: "A ticket cannot be read.", Next: "The issue has a comment that says what is wrong with its awk-ticket block. Correct it, take the needs-human-review label off, then awkit run."}
	case "blocked_by_dependencies":
		return Ending{Summary: "Every open ticket waits for an issue that is not done.", Next: "awkit status shows what each one waits for. Finish or reopen that issue, or correct depends_on, then awkit run."}
	case "no_actionable_tasks":
		return Ending{Summary: "Tickets are open, and none of them can be worked on.", Next: "awkit status shows the state each one is in."}
	case "max_loop_reached":
		return Ending{Summary: "The run reached the limit of its loop.", Next: "That many iterations mean a ticket goes round in circles: awkit events --level decision shows which. Then awkit reset and awkit run."}
	case "max_consecutive_failures":
		return Ending{Summary: "Too many attempts failed in a row.", Next: "awkit events --level error shows them. Fix what they have in common, then awkit reset and awkit run."}
	case "config_not_found":
		return Ending{Summary: "The project has no configuration.", Next: "awkit init writes " + layout.ConfigFile + "."}
	case "config_invalid":
		return Ending{Summary: "The configuration cannot be used.", Next: "awkit validate says what is wrong with " + layout.ConfigFile + "."}
	case "github_api_error", "loop_count_error", "error_exit", "contract_violation":
		return Ending{Summary: "The run could not go on (" + reason + ").", Next: "awkit doctor checks the tools, the remote and the state the run keeps."}
	default:
		return Ending{Summary: fmt.Sprintf("The run stopped (%s).", reason), Next: "awkit status and awkit events show what happened."}
	}
}

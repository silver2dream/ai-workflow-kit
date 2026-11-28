package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/guard"
	"github.com/silver2dream/ai-workflow-kit/internal/lanenote"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
	"github.com/silver2dream/ai-workflow-kit/internal/worker"
)

// The owner's gate (design.md §10, task 2.5). A change the review approved
// that is plainly high risk waits for its owner (awaiting_approval) before
// it goes to the merge queue: its ticket says so, or the change touches what
// the high-risk rules name (guard.HighRisk). The owner approves it, and it
// is queued, or returns it, and their reason goes to the next dispatch as
// their word.

// ApprovalRequest is what the owner is shown of a change that waits for
// them: why it is high risk, and what its review said.
type ApprovalRequest struct {
	Issue int `json:"issue"`
	PR    int `json:"pr"`
	// Head is the commit the review approved.
	Head    string             `json:"head,omitempty"`
	Reasons []guard.RiskReason `json:"reasons"`
	// Score and Summary are the review's.
	Score   int       `json:"score,omitempty"`
	Summary string    `json:"summary,omitempty"`
	At      time.Time `json:"at"`
}

func approvalFile(root string, issue int) string {
	return filepath.Join(root, filepath.FromSlash(layout.StateDir), "engine", "approvals", strconv.Itoa(issue)+".json")
}

// SaveApprovalRequest keeps what the owner is shown of a lane, in place of
// what was kept before.
func SaveApprovalRequest(root string, r ApprovalRequest) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Dir(approvalFile(root, r.Issue)), strconv.Itoa(r.Issue)+".json", append(data, '\n'))
}

// ReadApprovalRequest is what the owner is shown of a lane; ok is false
// when nothing can be read.
func ReadApprovalRequest(root string, issue int) (r ApprovalRequest, ok bool) {
	data, err := os.ReadFile(approvalFile(root, issue))
	if err != nil || json.Unmarshal(data, &r) != nil || r.Issue != issue {
		return ApprovalRequest{}, false
	}
	return r, true
}

// highRisk says why a change, the pull request's diff, is high risk, as its
// ticket and the project's configuration have it; none when it is not.
func highRisk(root, ticketText, diff string) []guard.RiskReason {
	in := guard.RiskInput{}
	in.Files, in.Deleted = guard.ChangeOf(diff)
	if block, found, err := ticket.Parse(ticketText); err == nil && found {
		in.TicketRisk = block.Risk
	}
	if meta, err := worker.ReadTicketMetadata(ticketText); err == nil {
		in.Release = meta.Release
	}
	if cfg, err := config.Load(root); err == nil {
		in.Paths, in.DeleteThreshold = cfg.Guardrails.HighRisk.Paths, cfg.Guardrails.HighRisk.DeleteThreshold
	}
	return guard.HighRisk(in)
}

// riskNote says the reasons in a line, for the lane.
func riskNote(reasons []guard.RiskReason) string {
	said := make([]string, len(reasons))
	for i, r := range reasons {
		said[i] = r.Reason
		if len(r.Files) > 0 {
			files := r.Files
			more := ""
			if len(files) > 3 {
				files, more = files[:3], fmt.Sprintf(" and %d more", len(r.Files)-3)
			}
			said[i] += " (" + strings.Join(files, ", ") + more + ")"
		}
	}
	return "high risk: " + strings.Join(said, "; ")
}

// approvalComment is what the engine says on the issue of a lane that waits
// for its owner's approval.
func approvalComment(l Lane, names config.Labels) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## AWK：這張工單要你核可才會合併\n\nPR #%d 審查通過了，但它是高風險的改動：\n\n", l.PR)
	for _, line := range strings.Split(strings.TrimPrefix(l.Note, "high risk: "), "; ") {
		if line != "" {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	fmt.Fprintf(&b, "\n核可：在工作台的「需要你處理」，或在終端機執行 `awkit lane approve %d`，它就進合併佇列。\n", l.Issue)
	fmt.Fprintf(&b, "退回：`awkit lane reject %d --note \"要改什麼\"`，你寫的說明會交給下一次的執行者。\n\n", l.Issue)
	fmt.Fprintf(&b, "拿掉 `%s` 標籤不算核可。細節在這台電腦的 `awkit events --issue %d`。", names.NeedsHumanReview, l.Issue)
	return b.String()
}

// Command kinds of the owner's gate.
const (
	// CommandApprove approves the change of a lane that waits for it.
	CommandApprove = "approve"
	// CommandReject returns it to its implementer, with the owner's reason
	// (Args["note"]).
	CommandReject = "reject"
)

// approval answers an approval or a return of a lane: what became of it, and
// why. A lane that does not wait for approval is not moved.
func (e *Engine) approval(ctx context.Context, c Command) (result, reason string) {
	l := e.Journal.Lane(c.Issue)
	if c.Issue == 0 || l == nil {
		return Rejected, fmt.Sprintf("#%d has no lane", c.Issue)
	}
	if l.State != AwaitingApproval {
		return Rejected, fmt.Sprintf("#%d is %s: only a lane that waits for approval is approved or returned", c.Issue, show(l.State))
	}
	by := c.By
	if by == "" {
		by = "the owner"
	}
	t := Transition{Issue: l.Issue, From: AwaitingApproval, At: e.clock()}
	if c.Kind == CommandApprove {
		t.To, t.Cause, t.Note = Queued, CauseOwnerApproved, "approved by "+by
		reason = "it goes to the merge queue"
	} else {
		note := strings.TrimSpace(c.Args["note"])
		if err := lanenote.Add(e.Root, l.Issue, lanenote.Note{At: e.clock(), Kind: lanenote.Returned, By: by, Text: note}); err != nil {
			return Rejected, "the reason could not be kept: " + err.Error()
		}
		t.To, t.Cause, t.Note = Rework, CauseOwnerReturned, "returned by "+by
		reason = "it goes back to its implementer"
		if note != "" {
			reason += ", with the reason"
		}
	}
	if _, err := e.record(ctx, t, true); err != nil {
		return Rejected, err.Error()
	}
	e.nudge()
	return Applied, reason
}

// nudge has the run do a round now, when it waits for its steps: a lane
// moved outside them may start one.
func (e *Engine) nudge() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

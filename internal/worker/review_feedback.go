package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"
)

// Review feedback is pasted into the Worker prompt as instructions, and the
// Worker runs unattended with write access to the worktree. On a public
// repository anyone can comment on an issue or a pull request, so feedback is
// taken only from the account running the workflow or from accounts with
// write access. The text of a comment proves nothing about who wrote it.

// issueComment is the part of `gh issue view --json comments` used here.
type issueComment struct {
	Body              string `json:"body"`
	CreatedAt         string `json:"createdAt"`
	AuthorAssociation string `json:"authorAssociation"`
	ViewerDidAuthor   bool   `json:"viewerDidAuthor"`
}

// prReviewComment is the part of the REST pull request review comment used here.
type prReviewComment struct {
	Path              string `json:"path"`
	Line              int    `json:"line"`
	OriginalLine      int    `json:"original_line"`
	Body              string `json:"body"`
	AuthorAssociation string `json:"author_association"`
	User              struct {
		Login string `json:"login"`
	} `json:"user"`
}

// trustedAuthorAssociation reports whether a GitHub author association implies
// write access to the repository.
func trustedAuthorAssociation(association string) bool {
	switch strings.ToUpper(strings.TrimSpace(association)) {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	}
	return false
}

// selectReviewFeedback returns the "AWK Review" comments that may enter the
// Worker prompt, and how many were left out because of who wrote them.
func selectReviewFeedback(comments []issueComment) (feedback string, ignored int) {
	var b strings.Builder
	for _, c := range comments {
		if !strings.Contains(c.Body, "AWK Review") {
			continue
		}
		if !c.ViewerDidAuthor && !trustedAuthorAssociation(c.AuthorAssociation) {
			ignored++
			continue
		}
		b.WriteString("---\n")
		b.WriteString(c.CreatedAt)
		b.WriteString(":\n")
		b.WriteString(c.Body)
		b.WriteString("\n")
	}
	return b.String(), ignored
}

// trustedPRReviewLines renders the file-level review comments that may enter
// the Worker prompt as the "path<TAB>line<TAB>body" lines
// formatPRReviewComments reads, and reports how many were left out. viewerLogin
// is the account running the workflow ("" when it could not be determined).
func trustedPRReviewLines(comments []prReviewComment, viewerLogin string) (lines string, ignored int) {
	flatten := strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")
	var b strings.Builder
	for _, c := range comments {
		byViewer := viewerLogin != "" && strings.EqualFold(c.User.Login, viewerLogin)
		if !byViewer && !trustedAuthorAssociation(c.AuthorAssociation) {
			ignored++
			continue
		}
		line := c.OriginalLine
		if line == 0 {
			line = c.Line
		}
		fmt.Fprintf(&b, "%s\t%d\t%s\n", c.Path, line, flatten.Replace(c.Body))
	}
	return b.String(), ignored
}

// ghViewerLogin returns the login of the account gh is authenticated as, or ""
// when it cannot be determined. GraphQL's viewer also answers for app tokens,
// which the REST /user endpoint refuses.
func ghViewerLogin(ctx context.Context) string {
	output, err := ghutil.RunWithRetry(ctx, ghutil.DefaultRetryConfig(), "gh", "api", "graphql",
		"-f", "query=query{viewer{login}}", "--jq", ".data.viewer.login")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

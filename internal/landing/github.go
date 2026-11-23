package landing

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

// GH is GitHub through the gh CLI, run in the project's repository: the
// repository is the one its origin names, unless Repo names another.
type GH struct {
	Run  Runner
	Repo string // owner/name; origin's when empty
}

func (g GH) gh(ctx context.Context, root string, args ...string) (string, error) {
	if g.Repo != "" {
		args = append(args, "--repo", g.Repo)
	}
	// What would be written to GitHub is scanned first: a credential in it
	// stops the call.
	if err := guard.GHGate(guard.WithScope(ctx, guard.Scope{StateRoot: root}), args); err != nil {
		return "", err
	}
	run := g.Run
	if run == nil {
		run = Exec
	}
	return run(ctx, root, "gh", args...)
}

func (g GH) OpenPR(ctx context.Context, root, branch string) (*PR, error) {
	out, err := g.gh(ctx, root, "pr", "list", "--head", branch, "--state", "open", "--json", "number,url", "--limit", "1")
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &prs); err != nil {
		return nil, fmt.Errorf("gh pr list said what cannot be read: %q", strings.TrimSpace(out))
	}
	if len(prs) == 0 {
		return nil, nil
	}
	return &prs[0], nil
}

var pullURL = regexp.MustCompile(`https://\S+/pull/(\d+)`)

func (g GH) CreatePR(ctx context.Context, root, base, branch, title, body string) (*PR, error) {
	out, err := g.gh(ctx, root, "pr", "create", "--base", base, "--head", branch, "--title", title, "--body", body)
	if err != nil {
		return nil, err
	}
	match := pullURL.FindStringSubmatch(out)
	if match == nil {
		return nil, fmt.Errorf("gh pr create did not say which pull request it opened: %q", strings.TrimSpace(out))
	}
	number, _ := strconv.Atoi(match[1])
	return &PR{Number: number, URL: match[0]}, nil
}

func (g GH) Checks(ctx context.Context, root string, pr int) ([]Check, error) {
	out, err := g.gh(ctx, root, "pr", "checks", strconv.Itoa(pr), "--json", "name,bucket")
	// gh says pending and failing checks by its exit code too: what it
	// printed is read whatever the code.
	var checks []Check
	if text := strings.TrimSpace(out); strings.HasPrefix(text, "[") {
		if jsonErr := json.Unmarshal([]byte(text), &checks); jsonErr == nil {
			return checks, nil
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "no checks reported") {
			return nil, nil
		}
		return nil, err
	}
	return nil, fmt.Errorf("gh pr checks said what cannot be read: %q", strings.TrimSpace(out))
}

func (g GH) Merge(ctx context.Context, root string, pr int, strategy string) error {
	flag := "--squash"
	switch strategy {
	case "merge":
		flag = "--merge"
	case "rebase":
		flag = "--rebase"
	}
	_, err := g.gh(ctx, root, "pr", "merge", strconv.Itoa(pr), flag, "--delete-branch")
	return err
}

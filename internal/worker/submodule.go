package worker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrNoSubmoduleChanges = errors.New("no changes in submodule")

// SubmoduleState tracks submodule consistency details.
type SubmoduleState struct {
	SubmoduleSHA      string
	ParentSHA         string
	ConsistencyStatus string
}

// CheckSubmoduleBoundary ensures staged files stay within the submodule path.
func CheckSubmoduleBoundary(ctx context.Context, wtDir, submodulePath string, allowParent bool) ([]string, error) {
	output, err := gitOutput(ctx, wtDir, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}

	files := splitLines(output)
	if len(files) == 0 {
		return nil, nil
	}

	submodulePath = normalizePath(submodulePath)
	var outside []string
	for _, file := range files {
		normalized := normalizePath(file)
		if normalized == submodulePath {
			continue
		}
		if !strings.HasPrefix(normalized, submodulePath+"/") {
			outside = append(outside, file)
		}
	}

	if len(outside) > 0 && !allowParent {
		return outside, fmt.Errorf("changes detected outside submodule boundary")
	}

	return outside, nil
}

// CommitSubmodule commits changes in the submodule and updates parent reference.
func CommitSubmodule(ctx context.Context, wtDir, submodulePath, commitMsg string, timeout time.Duration) (*SubmoduleState, error) {
	state := &SubmoduleState{ConsistencyStatus: "consistent"}
	submoduleDir := filepath.Join(wtDir, filepath.Clean(submodulePath))

	if err := runGit(ctx, submoduleDir, timeout, "add", "-A"); err != nil {
		return state, err
	}

	if err := runGit(ctx, submoduleDir, timeout, "diff", "--cached", "--quiet"); err == nil {
		return state, ErrNoSubmoduleChanges
	}

	if err := runGit(ctx, submoduleDir, timeout, "commit", "-m", commitMsg); err != nil {
		return state, err
	}

	subSHA, err := gitOutput(ctx, submoduleDir, "rev-parse", "HEAD")
	if err != nil {
		return state, err
	}
	state.SubmoduleSHA = strings.TrimSpace(subSHA)

	if err := runGit(ctx, wtDir, timeout, "add", submodulePath); err != nil {
		return state, err
	}

	if err := runGit(ctx, wtDir, timeout, "commit", "-m", commitMsg); err != nil {
		state.ConsistencyStatus = "submodule_committed_parent_failed"
		return state, err
	}

	parentSHA, err := gitOutput(ctx, wtDir, "rev-parse", "HEAD")
	if err != nil {
		return state, err
	}
	state.ParentSHA = strings.TrimSpace(parentSHA)

	return state, nil
}

// PushSubmodule pushes the submodule branch first, then the parent branch.
func PushSubmodule(ctx context.Context, wtDir, submodulePath, branch string, timeout time.Duration) (*SubmoduleState, error) {
	state := &SubmoduleState{ConsistencyStatus: "consistent"}
	submoduleDir := filepath.Join(wtDir, filepath.Clean(submodulePath))

	submoduleBranch := branch
	if head, err := gitOutput(ctx, submoduleDir, "symbolic-ref", "--short", "HEAD"); err == nil {
		head = strings.TrimSpace(head)
		if head != "" && head != "HEAD" {
			submoduleBranch = head
		}
	}

	if err := ensureBranch(ctx, submoduleDir, submoduleBranch, timeout); err != nil {
		return state, err
	}

	if err := runGit(ctx, submoduleDir, timeout, "push", "-u", "origin", submoduleBranch); err != nil {
		state.ConsistencyStatus = "submodule_push_failed"
		return state, err
	}

	if err := runGit(ctx, wtDir, timeout, "push", "-u", "origin", branch); err != nil {
		state.ConsistencyStatus = "parent_push_failed_submodule_pushed"
		return state, err
	}

	return state, nil
}

func ensureBranch(ctx context.Context, dir, branch string, timeout time.Duration) error {
	if branch == "" {
		return nil
	}
	if err := runGit(ctx, dir, timeout, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return nil
	}
	return runGit(ctx, dir, timeout, "checkout", "-b", branch)
}

func runGit(ctx context.Context, dir string, timeout time.Duration, args ...string) error {
	ctx, cancel := withOptionalTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	return cmd.Run()
}

package guard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ProjectRoot finds the project a directory belongs to: the folder that
// holds the repository's own .git, also from a worktree of it. "" when the
// directory is in no repository.
func ProjectRoot(dir string) string {
	if dir == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	common := strings.TrimSpace(string(out))
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	return filepath.Dir(filepath.Clean(common))
}

// WorkingProject is the project of the current directory.
func WorkingProject() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return ProjectRoot(dir)
}

var worktreeIssue = regexp.MustCompile(`[/\\]issue-(\d+)([/\\]|$)`)

// IssueOfDir reads the issue from a Worker's worktree (.worktrees/issue-N);
// 0 for any other directory.
func IssueOfDir(dir string) int {
	if m := worktreeIssue.FindStringSubmatch(dir); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

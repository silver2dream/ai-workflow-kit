package install

import (
	"os/exec"
	"strings"
)

// Branches are the two branches a project's configuration names: the one
// pull requests are merged into, and the one releases are cut from. A project
// with a single trunk has the same branch for both.
type Branches struct {
	Integration string
	Release     string
}

// DefaultBranches is what a project gets when nothing can be learned from its
// repository: a single trunk named main.
func DefaultBranches() Branches {
	return Branches{Integration: "main", Release: "main"}
}

// orDefault fills in what was not given: a single trunk when only one branch
// is named, the default when neither is.
func (b Branches) orDefault() Branches {
	switch {
	case b.Integration == "" && b.Release == "":
		return DefaultBranches()
	case b.Integration == "":
		b.Integration = b.Release
	case b.Release == "":
		b.Release = b.Integration
	}
	return b
}

// DetectBranches names the branches a new project's configuration starts
// with, from the repository in dir. The branches it names exist, so the first
// run has something to branch from: a configuration that named a branch
// nobody had created failed at the first ticket.
//
// The release branch is the remote's default branch; without one, main or
// master if there is such a branch, and else the branch that is checked out
// (which a repository without a commit has too). The integration branch is
// develop when the repository has one, and the release branch otherwise.
func DetectBranches(dir string) Branches {
	git := func(args ...string) (string, bool) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
		return strings.TrimSpace(string(out)), err == nil
	}
	has := func(branch string) bool {
		for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
			if _, ok := git("show-ref", "--verify", "--quiet", ref); ok {
				return true
			}
		}
		return false
	}

	release := ""
	if head, ok := git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); ok {
		release = strings.TrimPrefix(head, "origin/")
	}
	for _, name := range []string{"main", "master"} {
		if release == "" && has(name) {
			release = name
		}
	}
	if release == "" {
		if current, ok := git("symbolic-ref", "--quiet", "--short", "HEAD"); ok {
			release = current
		}
	}
	if release == "" {
		return DefaultBranches()
	}

	integration := release
	if has("develop") {
		integration = "develop"
	}
	return Branches{Integration: integration, Release: release}
}

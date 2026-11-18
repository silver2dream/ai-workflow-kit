package landing

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExpectsChecks reports whether a pull request into the integration branch
// is to have checks: a GitHub Actions workflow of the repository in dir, as
// the branch has it, runs on pull requests into it. A pull request that
// shows none for a while then is not one of a project without CI: GitHub did
// not run its CI, and it is not merged as if it had passed.
//
// A workflow that runs only when some paths change is not counted: a pull
// request of documents may well not start it.
func ExpectsChecks(dir, integration string) bool {
	var files []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		found, _ := filepath.Glob(filepath.Join(dir, ".github", "workflows", pattern))
		files = append(files, found...)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var workflow map[string]any
		if yaml.Unmarshal(data, &workflow) != nil {
			continue
		}
		if runsOnPullRequests(workflow["on"], integration) {
			return true
		}
	}
	return false
}

// pullRequestEvents are the events of a pull request a workflow can run on.
var pullRequestEvents = []string{"pull_request", "pull_request_target"}

func isPullRequestEvent(name string) bool { return slices.Contains(pullRequestEvents, name) }

// runsOnPullRequests reads a workflow's `on`: one event, a list of them, or
// events with their filters.
func runsOnPullRequests(on any, branch string) bool {
	switch events := on.(type) {
	case string:
		return isPullRequestEvent(events)
	case []any:
		for _, event := range events {
			if name, ok := event.(string); ok && isPullRequestEvent(name) {
				return true
			}
		}
	case map[string]any:
		for _, event := range pullRequestEvents {
			if filter, ok := events[event]; ok && filterTakes(filter, branch) {
				return true
			}
		}
	}
	return false
}

// filterTakes reports whether the filter of a pull request event lets a
// pull request of documents into branch start the workflow.
func filterTakes(filter any, branch string) bool {
	rules, ok := filter.(map[string]any)
	if !ok {
		return true
	}
	if _, ok := rules["paths"]; ok {
		return false
	}
	if _, ok := rules["paths-ignore"]; ok {
		return false
	}
	if types, ok := rules["types"]; ok && !anyOf(types, "opened", "synchronize", "reopened") {
		return false
	}
	if branches, ok := rules["branches"]; ok {
		return matchesAny(branches, branch)
	}
	if ignored, ok := rules["branches-ignore"]; ok {
		return !matchesAny(ignored, branch)
	}
	return true
}

func stringList(list any) []string {
	var out []string
	switch items := list.(type) {
	case string:
		out = append(out, items)
	case []any:
		for _, item := range items {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

func anyOf(list any, wanted ...string) bool {
	for _, item := range stringList(list) {
		if slices.Contains(wanted, item) {
			return true
		}
	}
	return false
}

// matchesAny matches a branch against GitHub's branch patterns: * and **,
// which a pattern of a workflow may use. A pattern it cannot read counts as
// a match: better to wait for checks that do not come than to merge without
// the ones that do.
func matchesAny(patterns any, branch string) bool {
	for _, pattern := range stringList(patterns) {
		if strings.HasPrefix(pattern, "!") {
			continue
		}
		if strings.Contains(pattern, "**") {
			return true
		}
		if ok, err := path.Match(pattern, branch); err != nil || ok {
			return true
		}
	}
	return false
}

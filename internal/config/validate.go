package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ValidationError says what is wrong with one field of the configuration.
type ValidationError struct {
	Field    string
	Message  string
	Expected string
}

func (e ValidationError) Error() string {
	if e.Expected != "" {
		return fmt.Sprintf("%s: %s (expected: %s)", e.Field, e.Message, e.Expected)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

var agentName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// AgentModels are the models a custom agent may name.
var AgentModels = []string{"haiku", "sonnet", "opus"}

// AgentTriggers are the points of the workflow a custom agent may be tied to.
var AgentTriggers = []string{"review_pr", "check_result", "dispatch_worker"}

func oneOf(value string, known []string) bool {
	for _, candidate := range known {
		if value == candidate {
			return true
		}
	}
	return false
}

// Validate checks that the configuration says what a run needs: what the
// project is, where work goes, and repos and custom agents that are well
// formed. It returns every problem it finds, in the order of the file.
func (c *Config) Validate() []ValidationError {
	var errors []ValidationError
	missing := func(field string) {
		errors = append(errors, ValidationError{Field: field, Message: "required field is missing"})
	}

	if c.Project.Name == "" {
		missing("project.name")
	}
	switch c.Project.Type {
	case "monorepo", "single-repo":
	case "":
		errors = append(errors, ValidationError{Field: "project.type", Message: "required field is missing", Expected: "monorepo or single-repo"})
	default:
		errors = append(errors, ValidationError{Field: "project.type", Message: fmt.Sprintf("invalid value: %s", c.Project.Type), Expected: "monorepo or single-repo"})
	}
	if c.Git.IntegrationBranch == "" {
		missing("git.integration_branch")
	}

	builtin := map[string]bool{}
	for _, name := range c.Agents.Builtin {
		builtin[name] = true
	}
	seen := map[string]bool{}
	for i, agent := range c.Agents.Custom {
		field := func(name string) string { return fmt.Sprintf("agents.custom[%d].%s", i, name) }
		if agent.Name == "" {
			missing(field("name"))
		} else {
			if !agentName.MatchString(agent.Name) {
				errors = append(errors, ValidationError{Field: field("name"), Message: fmt.Sprintf("invalid name: %s", agent.Name),
					Expected: "lowercase letters, digits, hyphens (must start with letter)"})
			}
			if builtin[agent.Name] {
				errors = append(errors, ValidationError{Field: field("name"), Message: fmt.Sprintf("name '%s' collides with built-in agent", agent.Name)})
			}
			if seen[agent.Name] {
				errors = append(errors, ValidationError{Field: field("name"), Message: fmt.Sprintf("duplicate agent name: %s", agent.Name)})
			}
			seen[agent.Name] = true
		}
		if agent.Description == "" {
			missing(field("description"))
		}
		if agent.Model != "" && !oneOf(agent.Model, AgentModels) {
			errors = append(errors, ValidationError{Field: field("model"), Message: fmt.Sprintf("invalid model: %s", agent.Model), Expected: "haiku, sonnet, or opus"})
		}
		if agent.Trigger != "" && !oneOf(agent.Trigger, AgentTriggers) {
			errors = append(errors, ValidationError{Field: field("trigger"), Message: fmt.Sprintf("invalid trigger: %s", agent.Trigger),
				Expected: "review_pr, check_result, or dispatch_worker"})
		}
	}

	for i, repo := range c.Repos {
		if repo.Name == "" {
			missing(fmt.Sprintf("repos[%d].name", i))
		}
		if repo.Path == "" {
			missing(fmt.Sprintf("repos[%d].path", i))
		}
		if repo.Type != "" && !oneOf(repo.Type, []string{"root", "directory", "submodule"}) {
			errors = append(errors, ValidationError{Field: fmt.Sprintf("repos[%d].type", i), Message: fmt.Sprintf("invalid value: %s", repo.Type),
				Expected: "root, directory, or submodule"})
		}
	}
	return errors
}

// ValidatePaths checks that the path of every repo exists under baseDir.
func (c *Config) ValidatePaths(baseDir string) []ValidationError {
	var errors []ValidationError
	for i, repo := range c.Repos {
		if _, err := os.Stat(filepath.Join(baseDir, repo.Path)); os.IsNotExist(err) {
			errors = append(errors, ValidationError{Field: fmt.Sprintf("repos[%d].path", i), Message: fmt.Sprintf("path does not exist: %s", repo.Path)})
		}
	}
	return errors
}

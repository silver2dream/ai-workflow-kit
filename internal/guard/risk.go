package guard

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// A change that is plainly high risk waits for its owner's approval before
// it is merged (design.md §10, task 2.5): the ticket says so, or the change
// touches what a person wants to look at before it lands. The rules only
// raise a change's risk; a project adds rules of its own, and cannot remove
// these. A rule that would flag ordinary work cannot be turned off, so each
// is kept to what is plainly a risk.

// RiskVersion names the rules below in every verdict they give.
const RiskVersion = "high-risk/1"

// DefaultDeleteThreshold is how many deleted files make a change high risk
// when the project sets no number (guardrails.high_risk.delete_threshold).
const DefaultDeleteThreshold = 20

// RiskInput is a change, and what its ticket and its project say of it.
type RiskInput struct {
	// Files are the files the change touches, from the project's root;
	// Deleted is how many of them it deletes.
	Files   []string
	Deleted int
	// TicketRisk is the ticket's risk (low, medium, high); Release says it
	// goes to the release branch.
	TicketRisk string
	Release    bool
	// Paths are the project's own high-risk globs; DeleteThreshold the
	// deleted files that make a change high risk (0 is the default).
	Paths           []string
	DeleteThreshold int
}

// RiskReason is one reason a change is high risk: a rule, what it means,
// and the files it found.
type RiskReason struct {
	Rule   string   `json:"rule"`
	Reason string   `json:"reason"`
	Files  []string `json:"files,omitempty"`
}

// The rules, by the id after RiskVersion.
const (
	RiskTicket     = "ticket"
	RiskRelease    = "release"
	RiskMigration  = "migration"
	RiskCIBuild    = "ci-build"
	RiskDependency = "dependency"
	RiskAuth       = "auth"
	RiskDeletion   = "deletion"
	RiskProject    = "project"
)

// HighRisk says why a change is high risk; none when it is not.
func HighRisk(in RiskInput) []RiskReason {
	var reasons []RiskReason
	if strings.EqualFold(in.TicketRisk, ticket.RiskHigh) {
		reasons = append(reasons, RiskReason{Rule: RiskVersion + "#" + RiskTicket, Reason: "its ticket says its risk is high"})
	}
	if in.Release {
		reasons = append(reasons, RiskReason{Rule: RiskVersion + "#" + RiskRelease, Reason: "it goes to the release branch"})
	}
	byFile := []struct {
		id, reason string
		match      func(string) bool
	}{
		{RiskMigration, "it changes a database migration", migrationPath},
		{RiskCIBuild, "it changes how the project is built, tested, released or run in a container", ciBuildPath},
		{RiskDependency, "it changes what the project depends on, or the versions it is locked to", dependencyPath},
		{RiskAuth, "it changes code of authentication, authorization or cryptography", authPath},
		{RiskProject, "it changes a path the project marks as high risk", func(f string) bool { return projectPath(in.Paths, f) }},
	}
	for _, rule := range byFile {
		var files []string
		for _, f := range in.Files {
			if rule.match(f) {
				files = append(files, f)
			}
		}
		if len(files) > 0 {
			sort.Strings(files)
			reasons = append(reasons, RiskReason{Rule: RiskVersion + "#" + rule.id, Reason: rule.reason, Files: files})
		}
	}
	threshold := in.DeleteThreshold
	if threshold <= 0 {
		threshold = DefaultDeleteThreshold
	}
	if in.Deleted > threshold {
		reasons = append(reasons, RiskReason{
			Rule:   RiskVersion + "#" + RiskDeletion,
			Reason: "it deletes " + strconv.Itoa(in.Deleted) + " files, more than " + strconv.Itoa(threshold),
		})
	}
	return reasons
}

// Verdict is a reason as a guardrail verdict: a flag, kept with the lane's
// ticket.
func (r RiskReason) Verdict() Verdict {
	evidence := strings.Join(r.Files, ", ")
	if len(r.Files) > 5 {
		evidence = strings.Join(r.Files[:5], ", ") + " and " + strconv.Itoa(len(r.Files)-5) + " more"
	}
	return Verdict{Result: Flag, Class: Info, Stage: StageOutput, Rule: r.Rule, Source: "the pull request", Reason: r.Reason, Evidence: evidence}
}

// ChangeOf reads a change from its diff (git's format, as GitHub gives a
// pull request's): the files it touches, and how many it deletes.
func ChangeOf(diff string) (files []string, deleted int) {
	seen := map[string]bool{}
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			// "diff --git a/x b/y": the file is y (x renamed, below).
			if i := strings.LastIndex(line, " b/"); i >= 0 {
				if f := line[i+3:]; f != "" && !seen[f] {
					seen[f] = true
					files = append(files, f)
				}
			}
		case strings.HasPrefix(line, "rename from "):
			if f := strings.TrimPrefix(line, "rename from "); !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		case strings.HasPrefix(line, "deleted file mode "):
			deleted++
		}
	}
	return files, deleted
}

// wordSplit splits a path into the words of its names: "internal/OAuth2-login/
// jwt_keys.go" is internal, oauth2, login, jwt, keys, go.
var wordSplit = regexp.MustCompile(`[^a-z0-9]+`)

func pathWords(file string) []string {
	return wordSplit.Split(strings.ToLower(file), -1)
}

var authWords = map[string]bool{
	"auth": true, "authn": true, "authz": true, "oauth": true, "oauth2": true, "login": true,
	"password": true, "passwords": true, "passwd": true, "credential": true, "credentials": true,
	"secret": true, "secrets": true, "crypto": true, "cryptography": true,
	"encrypt": true, "encryption": true, "decrypt": true, "decryption": true, "cipher": true,
	"jwt": true, "saml": true, "sso": true, "rbac": true, "acl": true, "permission": true, "permissions": true,
}

func authPath(file string) bool {
	for _, w := range pathWords(file) {
		if authWords[w] {
			return true
		}
	}
	return false
}

var flyway = regexp.MustCompile(`^v\d+(_\d+)*__.+\.sql$`)

func migrationPath(file string) bool {
	lower := strings.ToLower(file)
	dirs := strings.Split(path.Dir(lower), "/")
	for i, d := range dirs {
		switch {
		case d == "migrations", d == "migration", d == "alembic":
			return true
		case d == "migrate" && i > 0 && dirs[i-1] == "db":
			return true
		}
	}
	return flyway.MatchString(path.Base(lower))
}

func ciBuildPath(file string) bool {
	id, _ := pathSignal(file)
	return id == "ci-build"
}

var lockFiles = map[string]bool{
	"go.sum": true, "package-lock.json": true, "npm-shrinkwrap.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"poetry.lock": true, "pipfile.lock": true, "cargo.lock": true, "gemfile.lock": true, "composer.lock": true,
	"pubspec.lock": true, "mix.lock": true, "bun.lockb": true, "uv.lock": true,
}

func dependencyPath(file string) bool {
	if id, _ := pathSignal(file); id == "dependency" {
		return true
	}
	return lockFiles[path.Base(strings.ToLower(file))]
}

// projectPath says whether a file is one of the project's high-risk globs,
// without case.
func projectPath(globs []string, file string) bool {
	lower := strings.ToLower(file)
	for _, g := range globs {
		if g = strings.TrimSpace(g); g != "" && ticket.Overlap(strings.ToLower(g), lower) {
			return true
		}
	}
	return false
}

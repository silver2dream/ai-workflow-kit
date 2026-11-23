package guard

import (
	"fmt"
	"regexp"
	"strings"
)

// The roles a tool policy is written for. RolePrincipal is the session of
// `awkit run` today, with the reviewer and the conflict resolver in it as
// subagents: its policy allows what the three need and refuses the rest of
// what is dangerous. RoleImplementer is a Worker: it may only use what it
// needs to write code. See design.md §9.3.
const (
	RolePrincipal   = "principal"
	RoleImplementer = "implementer"
)

// PolicyVersion names the policies below in every verdict they give.
const PolicyVersion = "tool-policy/1"

// ToolCall is a call an agent is about to make, as a PreToolUse hook is told
// of it.
type ToolCall struct {
	Tool  string
	Input map[string]any
	// Cwd is the directory the agent works in.
	Cwd string
}

func (c ToolCall) str(key string) string {
	if v, ok := c.Input[key].(string); ok {
		return v
	}
	return ""
}

// Policy is what one role may do with its tools.
type Policy struct {
	Role string
	// Protected are the branches pull requests are merged into (the
	// project's integration and release branches): nothing is pushed to
	// them. main, master, develop and trunk always are.
	Protected []string
	// AllowControl lets the role write the workflow's own files (.claude,
	// .ai), as a ticket that sets allow_script_changes allows its Worker.
	// git's own files are never written.
	AllowControl bool
}

// Check decides whether the role may make a call. A role it does not know, a
// call it cannot read, a tool a role is not given: refused.
func (p Policy) Check(call ToolCall) Verdict {
	role := p.Role
	deny := func(rule, reason string) Verdict {
		return Verdict{Result: Block, Class: Security, Stage: StageTool, Rule: PolicyVersion + "#" + rule, Source: role + " " + call.Tool, Reason: reason}
	}
	if role != RolePrincipal && role != RoleImplementer {
		return deny("role", fmt.Sprintf("no tool policy for the role %q", role))
	}
	switch call.Tool {
	case "Bash", "PowerShell":
		return p.checkCommand(call.str("command"), deny)
	case "Read", "Glob", "Grep", "LS", "NotebookRead":
		for _, key := range []string{"file_path", "path", "pattern", "notebook_path"} {
			if path := call.str(key); credentialPath(path) {
				return deny("credentials", "reads where credentials are kept: "+path)
			}
		}
		return allow()
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		path := call.str("file_path")
		if path == "" {
			path = call.str("notebook_path")
		}
		if credentialPath(path) {
			return deny("credentials", "writes where credentials are kept: "+path)
		}
		if gitPath(path) {
			return deny("git-files", "writes git's own files: "+path)
		}
		if controlPath(path) && !p.AllowControl {
			return deny("control-files", "writes the workflow's own files, which the ticket does not allow (allow_script_changes): "+path)
		}
		return allow()
	case "WebFetch", "WebSearch":
		return deny("network", "this role does not reach the web")
	}
	if role == RoleImplementer {
		switch call.Tool {
		case "TodoWrite", "BashOutput", "KillShell", "KillBash", "Monitor":
			return allow()
		}
		return deny("tool", "a Worker is not given the tool "+call.Tool)
	}
	// The Principal's session also starts its subagents and skills, and may
	// have tools of the person's own setup.
	return allow()
}

func allow() Verdict { return Verdict{Result: Pass, Stage: StageTool, Rule: PolicyVersion} }

var (
	// credentialFile matches where tools keep credentials and keys.
	credentialFile = regexp.MustCompile(`(?i)(^|[/\\])(\.ssh|\.gnupg|\.aws|\.kube|\.docker[/\\]config\.json|\.git-credentials|\.netrc|_netrc|\.pgpass)([/\\]|$)|id_(rsa|dsa|ecdsa|ed25519)(\.pub)?$|[/\\]\.config[/\\]gh([/\\]|$)|github cli[/\\]hosts\.yml|[/\\]\.claude[/\\]\.credentials\.json|[/\\]\.codex[/\\]auth\.json|\.claude\.json$`)
	// homeNpmrc is a .npmrc in a home folder, which holds a token; a
	// project's own .npmrc names registries.
	homeNpmrc = regexp.MustCompile(`(?i)(~|\$home|%userprofile%|[/\\]users[/\\][^/\\]+|[/\\]home[/\\][^/\\]+)[/\\]\.npmrc$`)
	// controlFile matches the workflow's own files; gitFile git's.
	controlFile = regexp.MustCompile(`(?i)(^|[/\\])(\.claude|\.ai)([/\\]|$)`)
	gitFile     = regexp.MustCompile(`(?i)(^|[/\\])\.git([/\\]|$)`)
	// secretVariable is a use of a variable that holds a secret.
	secretVariable = regexp.MustCompile(`(?i)(\$\{?|\$env:|%)[a-z0-9_]*(token|secret|password|passwd|api_?key|access_?key|private_?key|credential)[a-z0-9_]*`)
	environFile    = regexp.MustCompile(`(?i)/proc/[^/\s]+/environ`)
)

func credentialPath(p string) bool {
	p = strings.TrimSpace(p)
	return p != "" && (credentialFile.MatchString(p) || homeNpmrc.MatchString(p))
}

func controlPath(p string) bool { return p != "" && controlFile.MatchString(p) }

func gitPath(p string) bool { return p != "" && gitFile.MatchString(p) }

// networkPrograms reach other hosts.
var networkPrograms = map[string]bool{
	"curl": true, "wget": true, "nc": true, "ncat": true, "netcat": true, "telnet": true, "ssh": true, "scp": true,
	"sftp": true, "ftp": true, "rsync": true, "invoke-webrequest": true, "iwr": true, "invoke-restmethod": true,
	"irm": true, "start-bitstransfer": true, "bitsadmin": true, "certutil": true, "socat": true, "aria2c": true,
}

// envDumpPrograms print every variable when given no command to run.
var envDumpPrograms = map[string]bool{"env": true, "printenv": true, "set": true, "export": true, "declare": true, "typeset": true}

func (p Policy) checkCommand(line string, deny func(rule, reason string) Verdict) Verdict {
	if strings.TrimSpace(line) == "" {
		return deny("command", "an empty command")
	}
	if m := secretVariable.FindString(line); m != "" {
		return deny("secret-variable", "uses a variable that holds a secret: "+m)
	}
	if environFile.MatchString(line) || strings.Contains(strings.ToLower(line), "getenvironmentvariables") {
		return deny("environment", "reads every environment variable")
	}
	// A path with a space, unquoted, reaches the check of each word in two
	// pieces: the line as a whole is looked at too.
	if m := credentialFile.FindString(line); m != "" {
		return deny("credentials", "names where credentials are kept: "+m)
	}
	cmds, ok := splitCommands(line)
	if !ok {
		return deny("unreadable", "the command cannot be read closely enough to check it")
	}
	for _, c := range cmds {
		if v := p.checkProgram(c, deny); v.Result != Pass {
			return v
		}
	}
	return allow()
}

func (p Policy) checkProgram(c command, deny func(rule, reason string) Verdict) Verdict {
	role := p.Role
	for _, a := range c.args {
		if credentialPath(a) {
			return deny("credentials", "reads where credentials are kept: "+a)
		}
	}
	if networkPrograms[c.program] {
		return deny("network", c.program+" reaches other hosts; this role does not")
	}
	if envDumpPrograms[c.program] && dumpsEnvironment(c) {
		return deny("environment", c.program+" prints every environment variable")
	}
	if isPowerShellEnvListing(c) {
		return deny("environment", "lists every environment variable")
	}
	switch c.program {
	case "gh":
		if role == RoleImplementer {
			return deny("github", "a Worker does not use GitHub: the runner does")
		}
		return checkGH(c, deny)
	case "git":
		return p.checkGit(c, deny)
	case "awkit":
		if role == RoleImplementer {
			return deny("awkit", "a Worker does not run awkit")
		}
	}
	return allow()
}

// dumpsEnvironment says whether env, printenv, set, export or declare is
// used to print the variables rather than to run or set something.
func dumpsEnvironment(c command) bool {
	switch c.program {
	case "env":
		rest := skipFlags(c.args)
		for len(rest) > 0 && assignment(rest[0]) {
			rest = rest[1:]
		}
		return len(rest) == 0
	case "printenv":
		return true
	case "set":
		return len(c.args) == 0
	case "export", "declare", "typeset":
		return len(c.args) == 0 || c.args[0] == "-p" || c.args[0] == "-x" && len(c.args) == 1
	}
	return false
}

func isPowerShellEnvListing(c command) bool {
	switch c.program {
	case "get-childitem", "gci", "ls", "dir", "get-item", "gi":
		for _, a := range c.args {
			if strings.HasPrefix(strings.ToLower(a), "env:") {
				return true
			}
		}
	}
	return false
}

// ghAllowed are what the Principal's session reads and writes on GitHub: the
// issues it labels and comments on, the pull requests it looks at.
var ghAllowed = map[string]map[string]bool{
	"issue": {"view": true, "list": true, "edit": true, "comment": true, "status": true},
	"pr":    {"view": true, "list": true, "diff": true, "checks": true, "status": true},
	"label": {"list": true},
	"run":   {"view": true, "list": true},
}

func checkGH(c command, deny func(rule, reason string) Verdict) Verdict {
	args := skipFlags(c.args)
	if len(args) == 0 {
		return allow()
	}
	group := strings.ToLower(args[0])
	if group == "api" {
		return checkGHAPI(args[1:], deny)
	}
	if len(args) < 2 {
		return deny("github", "gh "+group+" is not used by the workflow")
	}
	action := strings.ToLower(args[1])
	if ghAllowed[group][action] {
		return allow()
	}
	switch {
	case group == "pr" && action == "merge":
		return deny("merge", "a pull request is merged by awkit submit-review once the review passes, never directly")
	case group == "issue" && action == "create":
		return deny("issue-create", "issues are created by awkit plan approve and awkit create-task only")
	}
	return deny("github", fmt.Sprintf("gh %s %s is not used by the workflow", group, action))
}

// checkGHAPI allows reading the API, not writing to it.
func checkGHAPI(args []string, deny func(rule, reason string) Verdict) Verdict {
	for i, a := range args {
		la := strings.ToLower(a)
		switch {
		case la == "-x" || la == "--method":
			if i+1 < len(args) && !strings.EqualFold(args[i+1], "GET") {
				return deny("github-api", "gh api "+args[i+1]+" writes to GitHub")
			}
		case strings.HasPrefix(la, "-x") && len(la) > 2 && la[2:] != "get", strings.HasPrefix(la, "--method=") && la != "--method=get":
			return deny("github-api", "gh api "+a+" writes to GitHub")
		case la == "-f" || la == "--field" || la == "--raw-field" || la == "--input" || strings.HasPrefix(la, "--field=") || strings.HasPrefix(la, "--raw-field=") || strings.HasPrefix(la, "--input="):
			return deny("github-api", "gh api with fields or input writes to GitHub")
		}
	}
	return allow()
}

// alwaysProtected are pushed to by a merge, never by an agent, whatever the
// project calls its branches.
var alwaysProtected = []string{"main", "master", "develop", "trunk"}

func (p Policy) protected(branch string) bool {
	branch = strings.ToLower(strings.TrimPrefix(branch, "refs/heads/"))
	if strings.HasPrefix(branch, "release/") {
		return true
	}
	for _, b := range append(append([]string{}, alwaysProtected...), p.Protected...) {
		if b != "" && strings.EqualFold(branch, b) {
			return true
		}
	}
	return false
}

func (p Policy) checkGit(c command, deny func(rule, reason string) Verdict) Verdict {
	role := p.Role
	args := c.args
	// Options before the subcommand: -C dir, -c key=value, --git-dir=…
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if (args[0] == "-C" || args[0] == "-c") && len(args) > 1 {
			if args[0] == "-c" && strings.Contains(strings.ToLower(args[1]), "credential") {
				return deny("git-config", "sets git's credential helper")
			}
			args = args[2:]
			continue
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return allow()
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch sub {
	case "credential", "credential-store", "credential-cache", "credential-manager":
		return deny("git-credential", "git credential reads stored credentials")
	case "config":
		for _, a := range rest {
			if strings.Contains(strings.ToLower(a), "credential") {
				return deny("git-config", "git config of the credential helper")
			}
		}
		if readsConfig(rest) {
			return allow()
		}
		return deny("git-config", "changes git's configuration")
	case "remote":
		if len(rest) == 0 || rest[0] == "-v" || rest[0] == "show" || rest[0] == "get-url" {
			return allow()
		}
		return deny("git-remote", "changes where git pushes to")
	}
	if role == RoleImplementer {
		switch sub {
		case "push", "pull", "fetch", "clone", "commit", "tag", "merge", "rebase", "submodule", "worktree", "filter-branch", "gc", "prune", "reflog":
			return deny("git-"+sub, "git "+sub+" is the runner's to do, not a Worker's")
		}
		return allow()
	}
	if sub == "push" {
		return p.checkPush(rest, deny)
	}
	return allow()
}

func readsConfig(args []string) bool {
	for _, a := range args {
		switch a {
		case "--get", "--get-all", "--get-regexp", "--list", "-l":
			return true
		}
	}
	return false
}

// checkPush allows what the conflict resolver does, `git push
// --force-with-lease origin HEAD` and pushes of the workflow's own branches,
// and refuses a push to a branch that pull requests are merged into, a force
// without a lease, deletions and pushes of everything.
func (p Policy) checkPush(args []string, deny func(rule, reason string) Verdict) Verdict {
	var positional []string
	for _, a := range args {
		la := strings.ToLower(a)
		switch {
		case la == "--force" || la == "-f" || (strings.HasPrefix(la, "-") && !strings.HasPrefix(la, "--") && strings.Contains(la, "f")):
			return deny("push-force", "a force push without a lease")
		case la == "--all" || la == "--mirror" || la == "--tags" || la == "--delete" || la == "-d" || la == "--prune":
			return deny("push-wide", "git push "+a+" changes more than the branch of a ticket")
		case strings.HasPrefix(la, "-"):
			continue
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) <= 1 {
		// git push, git push origin: the branch that is checked out, which
		// the workflow's own push sets up.
		return allow()
	}
	for _, spec := range positional[1:] {
		spec = strings.TrimPrefix(spec, "+")
		if strings.HasPrefix(spec, ":") {
			return deny("push-delete", "deletes a branch on the remote: "+spec)
		}
		dst := spec
		if i := strings.LastIndex(spec, ":"); i >= 0 {
			dst = spec[i+1:]
		}
		if p.protected(dst) {
			return deny("push-protected", "pushes to "+dst+", which pull requests are merged into")
		}
	}
	return allow()
}

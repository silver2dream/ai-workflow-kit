package guard

import (
	"sort"
	"strings"
	"testing"
)

// An agent keeps what a shell and the toolchains need and its own backend's
// credential; every other secret, the gh token above all, stays behind.
func TestAgentEnv(t *testing.T) {
	environ := []string{
		"PATH=/usr/bin", "HOME=/home/me", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "TEMP=C:\\tmp",
		"GOPATH=/go", "GOFLAGS=-mod=mod", "NODE_OPTIONS=--max-old-space-size=4096", "DOTNET_ROOT=/dotnet",
		"HTTPS_PROXY=http://proxy:8080",
		"GH_TOKEN=ghp_x", "GITHUB_TOKEN=ghs_x", "NPM_CONFIG__AUTH_TOKEN=x", "NUGET_API_KEY=x",
		"AWS_SECRET_ACCESS_KEY=x", "DATABASE_PASSWORD=x", "GOOGLE_APPLICATION_CREDENTIALS=/k.json",
		"SOME_RANDOM_VAR=1", "=C:=C:\\work",
		"ANTHROPIC_API_KEY=sk-ant", "CLAUDE_CODE_OAUTH_TOKEN=oat", "OPENAI_API_KEY=sk-oa",
	}
	names := func(env []string) string {
		var out []string
		for _, kv := range env {
			out = append(out, kv[:strings.IndexByte(kv, '=')])
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	got := names(AgentEnv(environ, ClaudeCredentialEnv...))
	want := "ANTHROPIC_API_KEY,CLAUDE_CODE_OAUTH_TOKEN,DOTNET_ROOT,GOFLAGS,GOPATH,HOME,HTTPS_PROXY,LANG,LC_ALL,NODE_OPTIONS,PATH,TEMP"
	if got != want {
		t.Errorf("claude keeps %s\nwant        %s", got, want)
	}
	if got := names(AgentEnv(environ, CodexCredentialEnv...)); !strings.Contains(got, "OPENAI_API_KEY") || strings.Contains(got, "ANTHROPIC") || strings.Contains(got, "GH_TOKEN") {
		t.Errorf("codex keeps %s", got)
	}
}

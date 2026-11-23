package guard

import (
	"regexp"
	"strings"
)

// An agent starts with an environment of its own: what a shell and the
// project's toolchains need, and the credential of its own backend, not the
// rest. A token in the environment is a token an injected agent can send
// away; the gh token above all, which GitHub writes with.

// agentEnvNames are variables an agent keeps, by name.
var agentEnvNames = map[string]bool{
	"PATH": true, "PATHEXT": true, "HOME": true, "USER": true, "USERNAME": true, "LOGNAME": true, "SHELL": true,
	"TERM": true, "COLORTERM": true, "TZ": true, "LANG": true, "LANGUAGE": true, "TMPDIR": true, "TEMP": true, "TMP": true,
	"PWD": true, "HOSTNAME": true, "COMPUTERNAME": true, "USERPROFILE": true, "HOMEDRIVE": true, "HOMEPATH": true,
	"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true,
	"PROGRAMW6432": true, "COMMONPROGRAMFILES": true, "COMMONPROGRAMFILES(X86)": true, "SYSTEMROOT": true,
	"SYSTEMDRIVE": true, "WINDIR": true, "COMSPEC": true, "OS": true, "PROCESSOR_ARCHITECTURE": true,
	"NUMBER_OF_PROCESSORS": true, "PSMODULEPATH": true, "MSYSTEM": true, "CHERE_INVOKING": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
	"http_proxy": true, "https_proxy": true, "no_proxy": true, "all_proxy": true,
	"SSL_CERT_FILE": true, "SSL_CERT_DIR": true, "NODE_EXTRA_CA_CERTS": true, "REQUESTS_CA_BUNDLE": true,
	"JAVA_HOME": true, "ANDROID_HOME": true, "ANDROID_SDK_ROOT": true, "VIRTUAL_ENV": true, "CONDA_PREFIX": true,
	"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_DATA_HOME": true, "XDG_RUNTIME_DIR": true,
	"CI": true, "NO_COLOR": true, "FORCE_COLOR": true,
}

// agentEnvPrefixes are kinds of variables an agent keeps: locale, and the
// settings of the toolchains a project builds with.
var agentEnvPrefixes = []string{
	"LC_", "GO", "CGO_", "NODE_", "NPM_CONFIG_", "YARN_", "PNPM_", "DOTNET_", "NUGET_", "MSBUILD",
	"PYTHON", "PIP_", "POETRY_", "UV_", "CARGO_", "RUSTUP_", "RUSTC", "GRADLE_", "MAVEN_", "JDK_",
	"GODOT", "VCPKG_", "CMAKE_", "CC", "CXX",
}

// secretName is a variable whose name says it holds a secret. It is left
// out even when a prefix above would keep it (NPM_CONFIG__AUTH_TOKEN,
// NUGET_API_KEY).
var secretName = regexp.MustCompile(`(?i)(token|secret|password|passwd|api_?key|access_?key|private_?key|credential|_auth)`)

// AgentEnv returns, from an environment, the variables an agent keeps, and
// the variables of its own backend's credential (named in own), which the
// rule against secrets would otherwise leave out.
func AgentEnv(environ []string, own ...string) []string {
	ownSet := map[string]bool{}
	for _, name := range own {
		ownSet[strings.ToUpper(name)] = true
	}
	var out []string
	for _, kv := range environ {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			// Windows keeps per-drive directories as "=C:=C:\…".
			continue
		}
		name := kv[:eq]
		if ownSet[strings.ToUpper(name)] {
			out = append(out, kv)
			continue
		}
		if secretName.MatchString(name) {
			continue
		}
		if agentEnvNames[name] || agentEnvNames[strings.ToUpper(name)] {
			out = append(out, kv)
			continue
		}
		upper := strings.ToUpper(name)
		for _, prefix := range agentEnvPrefixes {
			if strings.HasPrefix(upper, prefix) {
				out = append(out, kv)
				break
			}
		}
	}
	return out
}

// The variables each backend authenticates with.
var (
	ClaudeCredentialEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "AWS_REGION", "AWS_PROFILE", "CLOUD_ML_REGION", "ANTHROPIC_VERTEX_PROJECT_ID"}
	CodexCredentialEnv  = []string{"OPENAI_API_KEY", "CODEX_API_KEY", "CODEX_HOME", "OPENAI_BASE_URL"}
)

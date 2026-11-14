package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func settingsProject() *Config {
	cfg := godotGoProject()
	cfg.Repos[0].Verify = VerifyConfig{Build: "go build ./...", Test: "CGO_ENABLED=0 go test ./..."}
	cfg.Repos[1].Verify = VerifyConfig{Build: "dotnet build", Test: "dotnet test"}
	cfg.Agents.Custom = []CustomAgentDef{{Name: "sec-check"}}
	return &cfg
}

func allowOf(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("the settings are not valid JSON: %v\n%s", err, data)
	}
	return settings.Permissions.Allow
}

// What a run is allowed without being asked: the skill, the subagents, the
// workflow's own tools, and the tools the project's build and test commands
// start with. The list named npm and python for every project, and not the
// tool a C# repo is tested with.
func TestNeededPermissions(t *testing.T) {
	got := neededPermissions(settingsProject())
	want := []string{
		"Skill(principal-workflow)",
		"Task(pr-reviewer)", "Task(conflict-resolver)", "Task(sec-check)",
		"Bash(gh:*)", "Bash(git:*)", "Bash(awkit:*)", "Bash(bash:*)", "Bash(codex:*)",
		"Bash(dotnet:*)", "Bash(go:*)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("neededPermissions =\n%v\nwant\n%v", got, want)
	}

	none := settingsProject()
	none.Repos[0].Verify, none.Repos[1].Verify = VerifyConfig{}, VerifyConfig{Test: "git diff --exit-code"}
	for _, permission := range neededPermissions(none) {
		if strings.Contains(permission, "npm") || strings.Contains(permission, "python") || permission == "Bash(go:*)" {
			t.Errorf("a project that runs none of them is allowed %s", permission)
		}
	}
	if count := strings.Count(strings.Join(neededPermissions(none), " "), "Bash(git:*)"); count != 1 {
		t.Errorf("Bash(git:*) is listed %d times", count)
	}
}

func TestCommandTool(t *testing.T) {
	for command, want := range map[string]string{
		"go test ./...":                "go",
		"dotnet build":                 "dotnet",
		"CGO_ENABLED=0 go build ./...": "go",
		"A=1 B=2 make test":            "make",
		"make build && make test":      "make",
		"golangci-lint run":            "golangci-lint",
		"":                             "",
		"   ":                          "",
		"./gradlew test":               "",
		"(cd tools && go run .)":       "",
		"$(which go) test":             "",
		"=x":                           "",
	} {
		if got := commandTool(command); got != want {
			t.Errorf("commandTool(%q) = %q, want %q", command, got, want)
		}
	}
}

// A project that has no local settings yet gets the file, with the list.
func TestGenerateClaudeSettings_NewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.local.json")
	changed, err := generateClaudeSettings(path, settingsProject())
	if err != nil || !changed {
		t.Fatalf("generateClaudeSettings: changed = %v, err = %v", changed, err)
	}
	if got, want := allowOf(t, path), neededPermissions(settingsProject()); !reflect.DeepEqual(got, want) {
		t.Errorf("allow = %v, want %v", got, want)
	}
}

// The local settings are the person's own: Claude Code adds to them every
// time a command is allowed for good. What a run needs is appended; nothing
// that is there is replaced, removed or reordered. The generator wrote the
// whole file, so every generate and every upgrade threw the person's away.
func TestGenerateClaudeSettings_AddsToWhatIsThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	own := `{
  "env": {"GOFLAGS": "-mod=mod"},
  "permissions": {
    "deny": ["Bash(rm -rf:*)"],
    "allow": [
      "Bash(make:*)",
      "Bash(gh:*)",
      "Bash(cd backend && go test ./... > out.txt:*)"
    ]
  }
}
`
	if err := os.WriteFile(path, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := generateClaudeSettings(path, settingsProject())
	if err != nil || !changed {
		t.Fatalf("generateClaudeSettings: changed = %v, err = %v", changed, err)
	}

	allow := allowOf(t, path)
	if len(allow) < 3 || allow[0] != "Bash(make:*)" || allow[1] != "Bash(gh:*)" || allow[2] != "Bash(cd backend && go test ./... > out.txt:*)" {
		t.Errorf("the person's entries are not first and in their order: %v", allow)
	}
	seen := map[string]int{}
	for _, permission := range allow {
		seen[permission]++
	}
	for _, needed := range neededPermissions(settingsProject()) {
		if seen[needed] != 1 {
			t.Errorf("%s is in the list %d times, want once", needed, seen[needed])
		}
	}

	data, _ := os.ReadFile(path)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if env, _ := settings["env"].(map[string]any); env["GOFLAGS"] != "-mod=mod" {
		t.Errorf("what else the file held is gone: %v", settings)
	}
	if deny, _ := settings["permissions"].(map[string]any)["deny"].([]any); len(deny) != 1 || deny[0] != "Bash(rm -rf:*)" {
		t.Errorf("the deny list is gone: %v", settings["permissions"])
	}
	// A command is written as it is, not with its & and > as escapes.
	if !strings.Contains(string(data), "cd backend && go test ./... > out.txt") {
		t.Errorf("a command was rewritten:\n%s", data)
	}

	// A second time there is nothing to add, and the file is not touched.
	before, _ := os.ReadFile(path)
	changed, err = generateClaudeSettings(path, settingsProject())
	after, _ := os.ReadFile(path)
	if err != nil || changed || string(before) != string(after) {
		t.Errorf("a second generate: changed = %v, err = %v, the file changed: %v", changed, err, string(before) != string(after))
	}
}

// A file with other things in it and no list gets one; a file awkit cannot
// read is left as it is and reported.
func TestGenerateClaudeSettings_OddFiles(t *testing.T) {
	dir := t.TempDir()

	noList := filepath.Join(dir, "no-list.json")
	if err := os.WriteFile(noList, []byte(`{"model": "sonnet"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := generateClaudeSettings(noList, settingsProject()); err != nil || !changed {
		t.Fatalf("a file without permissions: changed = %v, err = %v", changed, err)
	}
	data, _ := os.ReadFile(noList)
	if !strings.Contains(string(data), `"model": "sonnet"`) || len(allowOf(t, noList)) != len(neededPermissions(settingsProject())) {
		t.Errorf("a file without permissions:\n%s", data)
	}

	broken := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(broken), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := generateClaudeSettings(broken, settingsProject())
	if err == nil || changed || !strings.Contains(err.Error(), ".claude/settings.local.json is not valid JSON") || !strings.Contains(err.Error(), "left as it is") {
		t.Errorf("a file that is not JSON: changed = %v, err = %v", changed, err)
	}
	if data, _ := os.ReadFile(broken); string(data) != "{ not json" {
		t.Errorf("a file that is not JSON was written over: %q", data)
	}
}

// Through the generator: the person's entry survives a generate, and a file
// that cannot be read is a warning, not the end of the generate.
func TestGenerate_KeepsTheLocalSettings(t *testing.T) {
	dir := projectWith(t, strings.Replace(projectWithAMissingRule, "custom: [backend]", "custom: []", 1))
	settings := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"permissions": {"allow": ["Bash(make:*)"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Options{StateRoot: dir, Quiet: true}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	allow := allowOf(t, settings)
	if allow[0] != "Bash(make:*)" || !strings.Contains(strings.Join(allow, " "), "Task(pr-reviewer)") || !strings.Contains(strings.Join(allow, " "), "Bash(go:*)") {
		t.Errorf("allow = %v, want the person's entry kept and the run's added", allow)
	}

	if err := os.WriteFile(settings, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Generate(Options{StateRoot: dir, Quiet: true})
	if err != nil {
		t.Fatalf("Generate with settings that are not JSON: %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "not valid JSON") {
		t.Errorf("Warnings = %q, want the one about the settings", result.Warnings)
	}
}

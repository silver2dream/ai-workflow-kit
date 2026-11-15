package jittest

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
)

func enabledConfig() analyzer.JiTTestConfig {
	on := true
	return analyzer.JiTTestConfig{Enabled: &on, MaxTests: 3}
}

func TestSupported(t *testing.T) {
	for _, language := range []string{"go", "Golang", "typescript", "node", "python", "csharp", "C#", "godot", "dotnet"} {
		if !Supported(language) {
			t.Errorf("JiT tests should be available for %q", language)
		}
	}
	for _, language := range []string{"generic", "rust", "gdscript", "unknown", ""} {
		if Supported(language) {
			t.Errorf("JiT tests should not be offered for %q: the kit knows no test conventions for it", language)
		}
	}
}

// Every toolchain's example file name holds "jittest": the cleanup finds the
// generated files by that name.
func TestConventions_ExampleFileNamesAreFoundByTheCleanup(t *testing.T) {
	for toolchain, conv := range conventions {
		if !strings.Contains(filepath.Base(conv.FileExample), "jittest") {
			t.Errorf("%s: the example %q would not be cleaned up", toolchain, conv.FileExample)
		}
		if conv.Framework == "" || conv.Instructions == "" {
			t.Errorf("%s: the model is told nothing about the framework", toolchain)
		}
	}
}

func TestBuildPrompt_CSharp(t *testing.T) {
	prompt := buildPrompt(GenerateInput{
		Diff:         "+public int Increment() => ++Value;",
		Language:     "csharp",
		MaxTests:     3,
		TestProjects: []string{"tests/Game.Core.Tests", "tests/Game.Net.Tests"},
	})

	for _, want := range []string{
		"xUnit",
		"[Fact]",
		// Where the file may go, and that it may go nowhere else.
		"- tests/Game.Core.Tests\n",
		"- tests/Game.Net.Tests\n",
		"nowhere else",
		// How the file is named.
		".jittest.cs",
		`Put "jittest" in every file name`,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the C# prompt should contain %q:\n%s", want, prompt)
		}
	}
	for _, wrong := range []string{"_jittest_test.go", `Go's "testing" package`, "package declaration"} {
		if strings.Contains(prompt, wrong) {
			t.Errorf("the C# prompt talks about Go (%q):\n%s", wrong, prompt)
		}
	}
}

func TestBuildPrompt_ExampleFileFollowsTheLanguage(t *testing.T) {
	tests := map[string]string{
		"go":         "_jittest_test.go",
		"typescript": ".jittest.test.ts",
		"python":     "_jittest.py",
		"godot":      ".jittest.cs",
	}
	for language, want := range tests {
		prompt := buildPrompt(GenerateInput{Diff: "diff", Language: language, MaxTests: 1})
		if !strings.Contains(prompt, want) {
			t.Errorf("language %q: the prompt should show a file named like %q", language, want)
		}
		for other, suffix := range tests {
			if other != language && strings.Contains(prompt, suffix) {
				t.Errorf("language %q: the prompt shows the %s file name %q", language, other, suffix)
			}
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	testCsproj = `<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Microsoft.NET.Test.Sdk" Version="18.11.0" />
    <PackageReference Include="xunit" Version="2.9.3" />
  </ItemGroup>
</Project>
`
	libraryCsproj = `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>
`
)

func TestFindDotnetTestProjects(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "frontend/tests/Game.Core.Tests/Game.Core.Tests.csproj", testCsproj)
	writeFile(t, dir, "frontend/tests/Legacy/Legacy.csproj", `<Project><ItemGroup><PackageReference Include="NUnit" Version="4.0.0" /></ItemGroup></Project>`)
	writeFile(t, dir, "frontend/src/Game.Core/Game.Core.csproj", libraryCsproj)
	writeFile(t, dir, "frontend/game/Game.csproj", `<Project Sdk="Godot.NET.Sdk/4.7.1"></Project>`)
	// Copies of project files in build output and tool directories are not projects.
	writeFile(t, dir, "frontend/tests/Game.Core.Tests/obj/Copy.csproj", testCsproj)
	writeFile(t, dir, "frontend/game/.godot/mono/Temp.csproj", testCsproj)
	writeFile(t, dir, ".worktrees/issue-1/tests/X.Tests/X.Tests.csproj", testCsproj)

	got := findDotnetTestProjects(dir)

	want := []string{"frontend/tests/Game.Core.Tests", "frontend/tests/Legacy"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("test projects = %v, want %v", got, want)
	}
	if got := findDotnetTestProjects(t.TempDir()); len(got) != 0 {
		t.Errorf("an empty worktree has test projects %v", got)
	}
}

func TestInAnyDir(t *testing.T) {
	dirs := []string{"tests/Game.Core.Tests"}
	for name, want := range map[string]bool{
		"tests/Game.Core.Tests/Counter.jittest.cs":           true,
		"tests/Game.Core.Tests/Sub/Counter.jittest.cs":       true,
		"./tests/Game.Core.Tests/Counter.jittest.cs":         true,
		`tests\Game.Core.Tests\Counter.jittest.cs`:           filepath.Separator == '\\',
		"tests/Game.Core.Tests.Extra/Counter.jittest.cs":     false,
		"src/Game.Core/Counter.jittest.cs":                   false,
		"tests/Game.Core.Tests/../../src/Counter.jittest.cs": false,
		"tests/Game.Core.Tests":                              false,
		"":                                                   false,
	} {
		if got := inAnyDir(name, dirs); got != want {
			t.Errorf("inAnyDir(%q) = %v, want %v", name, got, want)
		}
	}
	// A test project at the root of the worktree takes any file.
	if !inAnyDir("Counter.jittest.cs", []string{"."}) {
		t.Error("a project at the worktree root should accept a file at the root")
	}
}

// A language the kit has no test conventions for is skipped, with a note: the
// model is not asked for tests in some other language's shape.
func TestRun_UnsupportedLanguageIsSkipped(t *testing.T) {
	origDiff, origClaude := getDiffFunc, claudeRunner
	defer func() { getDiffFunc, claudeRunner = origDiff, origClaude }()
	getDiffFunc = func(ctx context.Context, workDir, base, head string) (string, error) {
		t.Error("the diff was read for a language that is skipped")
		return "", nil
	}
	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		t.Error("the model was asked for tests in an unsupported language")
		return "", nil
	}

	result, err := Run(context.Background(), Input{WorkDir: t.TempDir(), Language: "rust", TestCommand: "cargo test"}, enabledConfig())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Generated != 0 || !strings.Contains(result.Error, `"rust"`) {
		t.Errorf("result = %+v, want a note that names the language", result)
	}
}

func TestRun_DotnetWithoutATestProjectIsSkipped(t *testing.T) {
	origClaude := claudeRunner
	defer func() { claudeRunner = origClaude }()
	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		t.Error("the model was asked for tests that have nowhere to go")
		return "", nil
	}
	dir := t.TempDir()
	writeFile(t, dir, "src/Game.Core/Game.Core.csproj", libraryCsproj)

	result, err := Run(context.Background(), Input{WorkDir: dir, Language: "csharp", TestCommand: "dotnet test"}, enabledConfig())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Generated != 0 || !strings.Contains(result.Error, "test project") {
		t.Errorf("result = %+v, want a note about the missing test project", result)
	}
}

// The whole run for a C# repo: the model is told where tests may go, a file
// inside a test project is written, run and removed, and a file it put into
// the product project is never written.
func TestRun_Dotnet(t *testing.T) {
	origDiff, origClaude, origExec := getDiffFunc, claudeRunner, executeTests
	defer func() { getDiffFunc, claudeRunner, executeTests = origDiff, origClaude, origExec }()

	dir := t.TempDir()
	writeFile(t, dir, "tests/Game.Core.Tests/Game.Core.Tests.csproj", testCsproj)
	writeFile(t, dir, "src/Game.Core/Game.Core.csproj", libraryCsproj)
	writeFile(t, dir, "src/Game.Core/Counter.cs", "namespace Game.Core;\npublic sealed class Counter { }\n")

	getDiffFunc = func(ctx context.Context, workDir, base, head string) (string, error) {
		return "+++ b/src/Game.Core/Counter.cs\n+public int Increment() => ++Value;\n", nil
	}
	var prompt string
	claudeRunner = func(ctx context.Context, p, model, workDir string) (string, error) {
		prompt = p
		return "```csharp filename: tests/Game.Core.Tests/CounterJitTests.jittest.cs\nnamespace Game.Core.Tests;\npublic class CounterJitTests { }\n```\n" +
			"```csharp filename: src/Game.Core/CounterJitTests.jittest.cs\nnamespace Game.Core;\npublic class Misplaced { }\n```\n", nil
	}
	inTestProject := filepath.Join(dir, "tests", "Game.Core.Tests", "CounterJitTests.jittest.cs")
	inProduct := filepath.Join(dir, "src", "Game.Core", "CounterJitTests.jittest.cs")
	executeTests = func(ctx context.Context, workDir, testCommand string, testFiles []string) testExecOutcome {
		if _, err := os.Stat(inTestProject); err != nil {
			t.Errorf("the test should be in the test project while the tests run: %v", err)
		}
		if _, err := os.Stat(inProduct); !os.IsNotExist(err) {
			t.Error("a test file was written into the product project")
		}
		return testExecOutcome{exitCode: 0}
	}

	result, err := Run(context.Background(), Input{WorkDir: dir, Language: "csharp", TestCommand: "dotnet test"}, enabledConfig())

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(prompt, "- tests/Game.Core.Tests\n") {
		t.Errorf("the model was not told where tests go:\n%s", prompt)
	}
	if strings.Contains(prompt, "- src/Game.Core\n") {
		t.Error("the product project was offered as a place for tests")
	}
	if result.Generated != 2 || result.Passed != 1 || result.Skipped != 1 || result.Failed != 0 {
		t.Errorf("generated %d, passed %d, skipped %d, failed %d; want 2, 1, 1, 0", result.Generated, result.Passed, result.Skipped, result.Failed)
	}
	var skippedWhy string
	for _, outcome := range result.Tests {
		if strings.HasPrefix(outcome.Test.Filename, "src/") {
			skippedWhy = outcome.Error
		}
	}
	if !strings.Contains(skippedWhy, "test project") {
		t.Errorf("the misplaced file should be reported with the reason, got %q", skippedWhy)
	}
	if _, err := os.Stat(inTestProject); !os.IsNotExist(err) {
		t.Error("the generated test was not removed after the run")
	}
}

func TestIsCompileError_CSharp(t *testing.T) {
	for output, want := range map[string]bool{
		`C:\work\tests\CounterJitTests.jittest.cs(7,13): error CS0103: The name 'counter' does not exist in the current context`: true,
		"Build FAILED.": true,
		// A failing test is not a compile error.
		"  Failed Game.Core.Tests.CounterJitTests.Adds [1 ms]\n  Error Message:\n   Assert.Equal() Failure": false,
		"error CSV file could not be read": false,
	} {
		if got := isCompileError(output); got != want {
			t.Errorf("isCompileError(%q) = %v, want %v", output, got, want)
		}
	}
}

// The generator itself refuses a language it has no conventions for, whoever
// calls it.
func TestGenerateTests_UnsupportedLanguage(t *testing.T) {
	origClaude := claudeRunner
	defer func() { claudeRunner = origClaude }()
	claudeRunner = func(ctx context.Context, prompt, model, workDir string) (string, error) {
		t.Error("the model was asked for tests in an unsupported language")
		return "", nil
	}

	tests, err := GenerateTests(context.Background(), GenerateInput{Diff: "+fn main() {}", Language: "rust", MaxTests: 3})

	if err == nil || !strings.Contains(err.Error(), `"rust"`) {
		t.Errorf("GenerateTests = %v, %v; want an error that names the language", tests, err)
	}
}

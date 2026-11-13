package reviewer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The fixtures are the output of a real `dotnet test` run over the godot-go
// skeleton with a probe test class added: a failing test, theories (one with a
// failing case, one with brackets in its arguments), a skipped test, a test
// with a display name and a test in a nested class.
//
//	dotnet_test_normal.txt   with --logger "console;verbosity=normal", as the review runs it
//	dotnet_test_minimal.txt  the plain command: passed tests are not listed at all
func dotnetFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	// The repository stores it with LF; a Windows checkout may hand out CRLF.
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestParseTestResults_Dotnet(t *testing.T) {
	passed, failed := ParseTestResults(dotnetFixture(t, "dotnet_test_normal.txt"))

	wantPassed := []string{
		// A test answers to its full name, to class and method, and to the method.
		"Game.Core.Tests.CounterTests.Increment_AddsOne",
		"CounterTests.Increment_AddsOne",
		"Increment_AddsOne",
		// A nested class keeps its "+".
		"Game.Core.Tests.ProbeTests+Nested.Inner_Passes",
		"ProbeTests+Nested.Inner_Passes",
		"Inner_Passes",
		// A theory: each case, and the method once every case passed.
		"Game.Core.Tests.ProbeTests.Increment_AddsMany(times: 1)",
		"ProbeTests.Increment_AddsMany(times: 2)",
		"Increment_AddsMany(times: 2)",
		"Game.Core.Tests.ProbeTests.Increment_AddsMany",
		"ProbeTests.Increment_AddsMany",
		"Increment_AddsMany",
		// Brackets inside the arguments are not the duration.
		`Game.Core.Tests.ProbeTests.Theory_WithOddArguments(label: "a [bracket]", weight: 2)`,
		`Theory_WithOddArguments(label: "two words", weight: 1.5)`,
		"Theory_WithOddArguments",
		// A display name is free text and answers only to itself.
		"counts up: from zero (display name)",
		// The case that passed, of a theory whose other case failed.
		"Game.Core.Tests.ProbeTests.Theory_OneCaseFails(value: 1, expected: True)",
		"Theory_OneCaseFails(value: 1, expected: True)",
	}
	for _, name := range wantPassed {
		if !passed[name] {
			t.Errorf("%q should have passed", name)
		}
	}

	wantFailed := []string{
		"Game.Core.Tests.ProbeTests.Broken_Fails",
		"ProbeTests.Broken_Fails",
		"Broken_Fails",
		"Game.Core.Tests.ProbeTests.Theory_OneCaseFails(value: 5, expected: True)",
		"ProbeTests.Theory_OneCaseFails(value: 5, expected: True)",
		"Theory_OneCaseFails(value: 5, expected: True)",
		// The method stands for all its cases: one failed, so it failed.
		"Game.Core.Tests.ProbeTests.Theory_OneCaseFails",
		"ProbeTests.Theory_OneCaseFails",
		"Theory_OneCaseFails",
	}
	for _, name := range wantFailed {
		if !failed[name] {
			t.Errorf("%q should have failed", name)
		}
		if passed[name] {
			t.Errorf("%q failed, so it must not count as passed", name)
		}
	}

	// A skipped test neither passed nor failed.
	for _, name := range []string{"Skipped_Test", "ProbeTests.Skipped_Test", "Game.Core.Tests.ProbeTests.Skipped_Test"} {
		if passed[name] || failed[name] {
			t.Errorf("the skipped test %q was counted (passed=%v failed=%v)", name, passed[name], failed[name])
		}
	}

	// Nothing but test names: no summary line, no runner diagnostics, no path.
	for _, name := range append(sortedNames(passed), sortedNames(failed)...) {
		for _, junk := range []string{"xUnit.net", "[FAIL]", "[SKIP]", "Total", "Test Run", "Error Message", ".dll", `C:\`, " ms"} {
			if strings.Contains(name, junk) {
				t.Errorf("%q is not a test name (contains %q)", name, junk)
			}
		}
	}
	if len(failed) != len(wantFailed) {
		t.Errorf("failed = %v, want exactly %v", sortedNames(failed), wantFailed)
	}
}

// The plain command lists the tests that failed and none that passed, which is
// why the review runs it with the console logger at normal verbosity.
func TestParseTestResults_DotnetPlainCommandListsOnlyFailures(t *testing.T) {
	passed, failed := ParseTestResults(dotnetFixture(t, "dotnet_test_minimal.txt"))

	if len(passed) != 0 {
		t.Errorf("passed = %v, want none: the plain output names no test that passed", sortedNames(passed))
	}
	for _, name := range []string{"Broken_Fails", "Theory_OneCaseFails", "Theory_OneCaseFails(value: 5, expected: True)"} {
		if !failed[name] {
			t.Errorf("%q should have failed; failed = %v", name, sortedNames(failed))
		}
	}
}

// On Windows the runner ends its lines with CRLF.
func TestParseTestResults_DotnetCRLF(t *testing.T) {
	lf := dotnetFixture(t, "dotnet_test_normal.txt")
	wantPassed, wantFailed := ParseTestResults(lf)

	passed, failed := ParseTestResults(strings.ReplaceAll(lf, "\n", "\r\n"))

	if !reflect.DeepEqual(sortedNames(passed), sortedNames(wantPassed)) {
		t.Errorf("with CRLF passed = %v\nwant %v", sortedNames(passed), sortedNames(wantPassed))
	}
	if !reflect.DeepEqual(sortedNames(failed), sortedNames(wantFailed)) {
		t.Errorf("with CRLF failed = %v\nwant %v", sortedNames(failed), sortedNames(wantFailed))
	}
	for name := range passed {
		if strings.ContainsAny(name, "\r\n") {
			t.Errorf("a line end is part of the name %q", name)
		}
	}
}

// A name that stands for several tests passed only if none of them failed:
// here the same method name in two classes, and the same test on two target
// frameworks.
func TestParseTestResults_DotnetSharedNames(t *testing.T) {
	output := strings.Join([]string{
		"  Passed App.Tests.OrderTests.Rejects_Empty [2 ms]",
		"  Failed App.Tests.CartTests.Rejects_Empty [1 ms]",
		"  Passed App.Tests.CartTests.Adds_Item [1 ms]",
		"  Passed App.Tests.ClockTests.Ticks [1 ms]",
		"  Failed App.Tests.ClockTests.Ticks [1 ms]",
		"",
	}, "\n")

	passed, failed := ParseTestResults(output)

	if !passed["OrderTests.Rejects_Empty"] || !failed["CartTests.Rejects_Empty"] {
		t.Errorf("each class's own test should keep its result: passed=%v failed=%v", sortedNames(passed), sortedNames(failed))
	}
	if passed["Rejects_Empty"] || !failed["Rejects_Empty"] {
		t.Error("Rejects_Empty names a failing test too, so it must count as failed")
	}
	if !passed["Adds_Item"] {
		t.Error("Adds_Item should have passed")
	}
	if passed["App.Tests.ClockTests.Ticks"] || !failed["App.Tests.ClockTests.Ticks"] {
		t.Error("a test that failed on one target framework failed")
	}
}

func TestDotnetTestNames(t *testing.T) {
	tests := []struct {
		full string
		want []string
	}{
		{"Game.Core.Tests.CounterTests.Increment_AddsOne",
			[]string{"Game.Core.Tests.CounterTests.Increment_AddsOne", "CounterTests.Increment_AddsOne", "Increment_AddsOne"}},
		{"Game.Core.Tests.CounterTests.Adds(times: 2)",
			[]string{
				"Game.Core.Tests.CounterTests.Adds", "Game.Core.Tests.CounterTests.Adds(times: 2)",
				"CounterTests.Adds", "CounterTests.Adds(times: 2)",
				"Adds", "Adds(times: 2)",
			}},
		// Dots inside the arguments belong to the arguments.
		{"A.B.Scales(weight: 1.5)", []string{"A.B.Scales", "A.B.Scales(weight: 1.5)", "B.Scales", "B.Scales(weight: 1.5)", "Scales", "Scales(weight: 1.5)"}},
		{"Game.Core.Tests.ProbeTests+Nested.Inner_Passes",
			[]string{"Game.Core.Tests.ProbeTests+Nested.Inner_Passes", "ProbeTests+Nested.Inner_Passes", "Inner_Passes"}},
		// A method outside any namespace and class path.
		{"Standalone", []string{"Standalone"}},
		// Display names are free text.
		{"counts up: from zero (display name)", []string{"counts up: from zero (display name)"}},
		{"adds 1.5 and 2.5", []string{"adds 1.5 and 2.5"}},
		{"(odd) name", []string{"(odd) name"}},
		{"Adds(1) and more", []string{"Adds(1) and more"}},
		{"名稱 帶空白", []string{"名稱 帶空白"}},
		// A non-ASCII identifier is still an identifier.
		{"遊戲.測試.計數器.加一", []string{"遊戲.測試.計數器.加一", "計數器.加一", "加一"}},
	}
	for _, tt := range tests {
		if got := dotnetTestNames(tt.full); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("dotnetTestNames(%q)\n got %q\nwant %q", tt.full, got, tt.want)
		}
	}
}

func TestHasIndividualTestNames_Dotnet(t *testing.T) {
	if !hasIndividualTestNames(dotnetFixture(t, "dotnet_test_normal.txt")) {
		t.Error("the normal-verbosity output names every test")
	}

	// What the plain command prints when everything passes: a summary, and at
	// most the tests that were skipped.
	allPassPlain := strings.Join([]string{
		"Starting test execution, please wait...",
		"A total of 1 test files matched the specified pattern.",
		"[xUnit.net 00:00:00.17]     Game.Core.Tests.ProbeTests.Skipped_Test [SKIP]",
		"  Skipped Game.Core.Tests.ProbeTests.Skipped_Test [1 ms]",
		"",
		"Passed!  - Failed:     0, Passed:     9, Skipped:     1, Total:    10, Duration: 31 ms - Game.Core.Tests.dll (net8.0)",
		"",
	}, "\n")
	if hasIndividualTestNames(allPassPlain) {
		t.Error("a summary and a skipped test name no test that ran")
	}
	if passed, failed := ParseTestResults(allPassPlain); len(passed) != 0 || len(failed) != 0 {
		t.Errorf("the summary line was read as a test: passed=%v failed=%v", sortedNames(passed), sortedNames(failed))
	}
}

func TestEnhanceTestCommandForVerbose_Dotnet(t *testing.T) {
	const logger = `--logger "console;verbosity=normal"`
	const english = "export DOTNET_CLI_UI_LANGUAGE=en; "
	tests := []struct {
		name, command, want string
	}{
		{"plain", "dotnet test", english + "dotnet test " + logger},
		{"in a directory repo", "cd frontend && dotnet test", english + "cd frontend && dotnet test " + logger},
		{"with its own arguments", "dotnet test Frontend.sln --no-build", english + "dotnet test " + logger + " Frontend.sln --no-build"},
		{"followed by another command", "dotnet test && echo done", english + "dotnet test " + logger + " && echo done"},
		{"two runs", "dotnet test A.sln && dotnet test B.sln", english + "dotnet test " + logger + " A.sln && dotnet test " + logger + " B.sln"},
		{"another logger only", "dotnet test --logger trx", english + "dotnet test " + logger + " --logger trx"},
		{"a console logger of its own", `dotnet test --logger "console;verbosity=detailed"`, english + `dotnet test --logger "console;verbosity=detailed"`},
		{"a console logger, short flag", "dotnet test -l console", english + "dotnet test -l console"},
		{"a console logger, colon form", "dotnet test --logger:console", english + "dotnet test --logger:console"},
		{"language already chosen", "DOTNET_CLI_UI_LANGUAGE=en dotnet test", "DOTNET_CLI_UI_LANGUAGE=en dotnet test " + logger},
		// Nothing can safely be added to a command that hides the test run.
		{"a script", "make test", "make test"},
		{"another dotnet command", "dotnet build", "dotnet build"},
		{"a tool whose name starts the same", "dotnet testing-tool run", "dotnet testing-tool run"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		if got := EnhanceTestCommandForVerbose(tt.command, "csharp"); got != tt.want {
			t.Errorf("%s: EnhanceTestCommandForVerbose(%q)\n got %q\nwant %q", tt.name, tt.command, got, tt.want)
		}
	}

	// Every name a config may give the language gets the same treatment.
	for _, language := range []string{"csharp", "C#", "cs", "dotnet", "godot", " CSharp "} {
		if got := EnhanceTestCommandForVerbose("dotnet test", language); got != english+"dotnet test "+logger {
			t.Errorf("language %q: got %q", language, got)
		}
	}
	// Other languages do not get a .NET logger.
	for _, language := range []string{"go", "typescript", "python", "generic", ""} {
		if got := EnhanceTestCommandForVerbose("dotnet test", language); strings.Contains(got, "--logger") || strings.Contains(got, "DOTNET_CLI_UI_LANGUAGE") {
			t.Errorf("language %q: got %q", language, got)
		}
	}
}

func TestDotnetTestValidator(t *testing.T) {
	validator := GetTestNameValidator("csharp")
	if _, ok := validator.(*DotnetTestValidator); !ok {
		t.Fatalf("GetTestNameValidator(csharp) = %T, want the .NET validator", validator)
	}
	for _, name := range []string{
		"Increment_AddsOne",
		"CounterTests.Increment_AddsOne",
		"Game.Core.Tests.CounterTests.Increment_AddsOne",
		"Increment_AddsMany(times: 2)",
		"ProbeTests+Nested.Inner_Passes",
		"counts up: from zero (display name)",
	} {
		if !validator.IsValid(name) {
			t.Errorf("%q should be a valid test name", name)
		}
	}
	for _, name := range []string{"", "N/A", "n/a", "All tests pass", "TODO", "SKIP"} {
		if validator.IsValid(name) {
			t.Errorf("%q should not be accepted as a test name", name)
		}
	}
	if hint := validator.FormatHint(); !strings.Contains(hint, "Increment_AddsOne") {
		t.Errorf("the hint should show what a name looks like, got %q", hint)
	}
	for _, language := range []string{"C#", "dotnet", "godot"} {
		if _, ok := GetTestNameValidator(language).(*DotnetTestValidator); !ok {
			t.Errorf("language %q should get the .NET validator", language)
		}
	}
}

func TestExtractAssertionFuncName_CSharp(t *testing.T) {
	tests := map[string]string{
		"Assert.Equal(1, counter.Increment())":                  "Assert.Equal",
		"Assert.Throws<ArgumentException>(() => parse(\"\"))":   "Assert.Throws",
		"`Assert.True(result.IsValid)`":                         "Assert.True",
		"the counter is 1 afterwards":                           "",
		"MyAssert.Equal(1, 2)":                                  "",
		"Debug.Assert(value > 0) then Assert.NotNull(instance)": "Assert.NotNull",
	}
	for assertion, want := range tests {
		if got := extractAssertionFuncName(assertion); got != want {
			t.Errorf("extractAssertionFuncName(%q) = %q, want %q", assertion, got, want)
		}
	}
}

// csharpWorktree lays out the part of a C# repo the verification looks at.
func csharpWorktree(t *testing.T) string {
	t.Helper()
	// The worktree itself sits in a directory called tests: what counts is a
	// file's place inside the worktree, not where the worktree is.
	dir := filepath.Join(t.TempDir(), "tests", "issue-7")
	files := map[string]string{
		"tests/Game.Core.Tests/CounterTests.cs": `using Xunit;

namespace Game.Core.Tests;

public sealed class CounterTests
{
    [Fact]
    public void Increment_AddsOne()
    {
        var counter = new Counter();

        Assert.Equal(1, counter.Increment());
    }
}
`,
		// Named like nothing in particular: it counts by the project it is in.
		"tests/Game.Core.Tests/Rules.cs": "// Assert.InRange(score, 0, 100);\n",
		// Product code and build output are not test files.
		"src/Game.Core/Counter.cs":                            "// Assert.Fail(\"product code\");\n",
		"tests/Game.Core.Tests/obj/Debug/net8.0/Generated.cs": "// Assert.Fail(\"build output\");\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFindTestFiles_CSharp(t *testing.T) {
	dir := csharpWorktree(t)

	files, err := findTestFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, file := range files {
		rel, _ := filepath.Rel(dir, file)
		got = append(got, filepath.ToSlash(rel))
	}
	sort.Strings(got)

	want := []string{"tests/Game.Core.Tests/CounterTests.cs", "tests/Game.Core.Tests/Rules.cs"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("test files = %v, want %v", got, want)
	}
}

func TestVerifyAssertions_CSharp(t *testing.T) {
	dir := csharpWorktree(t)

	found := []CriteriaVerification{
		{Criteria: "adds one", TestName: "Increment_AddsOne", Assertion: "Assert.Equal(1, counter.Increment())"},
		{Criteria: "score in range", TestName: "Score_InRange", Assertion: "`Assert.InRange(score, 0, 100);`"},
	}
	if err := VerifyAssertions(dir, found); err != nil {
		t.Errorf("assertions that are in the test project were not found: %v %v", err.Message, err.Details)
	}

	// Text that exists only in product code or build output is not evidence.
	for _, assertion := range []string{`Assert.Fail("product code")`, `Assert.Fail("build output")`} {
		missing := []CriteriaVerification{{Criteria: "c", TestName: "T", Assertion: assertion}}
		if err := VerifyAssertions(dir, missing); err == nil {
			t.Errorf("%q is not in any test file, yet it was accepted", assertion)
		}
	}
}

// The whole verification for a C# repo: the test command is made to name its
// tests, the names a review cites are found in its output, and the assertions
// are looked up in the test project.
func TestVerifyTestEvidence_CSharp(t *testing.T) {
	dir := csharpWorktree(t)
	if err := os.WriteFile(filepath.Join(dir, "output.txt"), []byte(dotnetFixture(t, "dotnet_test_normal.txt")), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stands in for the test run: prints what `dotnet test` printed.
	const testCommand = "cat output.txt"
	const ticket = "## Acceptance Criteria\n- [ ] Incrementing adds one\n"

	verify := func(testName string) (*VerifyReport, *EvidenceError) {
		return VerifyTestEvidenceWithReport(context.Background(), VerifyOptions{
			Ticket:       ticket,
			WorktreePath: dir,
			TestCommand:  testCommand,
			Language:     "csharp",
			Verifications: []CriteriaVerification{{
				Criteria:       "Incrementing adds one",
				Implementation: "Counter.Increment raises Value by one and returns it",
				TestName:       testName,
				Assertion:      "Assert.Equal(1, counter.Increment())",
			}},
		})
	}

	// However the review names the test, it is found among the tests that ran.
	for _, name := range []string{"Increment_AddsOne", "CounterTests.Increment_AddsOne", "Game.Core.Tests.CounterTests.Increment_AddsOne"} {
		report, err := verify(name)
		if err != nil {
			t.Errorf("test cited as %q: %s %v", name, err.Message, err.Details)
			continue
		}
		if report.FileLevelOnly {
			t.Errorf("test cited as %q: verification fell back to the file level although every test is named", name)
		}
	}

	// A test that failed is not evidence, and neither is a theory with a failing case.
	for _, name := range []string{"Broken_Fails", "Theory_OneCaseFails"} {
		_, err := verify(name)
		if err == nil {
			t.Errorf("%q failed, yet it was accepted as evidence", name)
			continue
		}
		if !strings.Contains(strings.Join(err.Details, "\n"), name+" (FAILED)") {
			t.Errorf("%q: the error should say the test failed, got %v", name, err.Details)
		}
	}

	// A test that is nowhere in the output is not evidence either.
	if _, err := verify("Scoreboard_RendersTotals"); err == nil {
		t.Error("a test that is not in the output was accepted as evidence")
	}
}

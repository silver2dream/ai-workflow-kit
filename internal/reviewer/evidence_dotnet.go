package reviewer

import (
	"regexp"
	"strings"
)

// dotnetResultLine matches a result line of `dotnet test` with the console
// logger at normal verbosity. The line is the same for xUnit, NUnit and MSTest:
//
//	Passed Game.Core.Tests.CounterTests.Increment_AddsOne [6 ms]
//	Failed Game.Core.Tests.CounterTests.Adds(label: "a [b]", times: 2) [< 1 ms]
//
// The name is everything up to the duration, and the duration is the last
// bracket of the line: a theory's arguments may hold brackets of their own.
// The summary ("Passed!  - Failed: 0, ...") and the runner's own diagnostics
// ("[xUnit.net 00:00:00.73]  ... [FAIL]") are different lines and do not match.
var dotnetResultLine = regexp.MustCompile(`(?m)^[ \t]*(Passed|Failed)[ \t]+(\S.*?)[ \t]+\[[^\[\]\r\n]*\][ \t]*\r?$`)

// dotnetQualifiedName is a test's default name: namespace, class and method,
// with "+" between nested classes. A name given with DisplayName is free text
// and does not look like this.
var dotnetQualifiedName = regexp.MustCompile("^[\\pL_][\\pL\\pN_+.<>,`]*$")

// parseDotnetResults reads the tests `dotnet test` reported.
//
// A test answers to several names (see dotnetTestNames), and one name can
// stand for several tests: a theory method for all its cases, a method name
// for the methods of that name in every class. Such a name passed only if
// nothing it stands for failed.
func parseDotnetResults(output string) (passed, failed map[string]bool) {
	passed, failed = map[string]bool{}, map[string]bool{}
	for _, m := range dotnetResultLine.FindAllStringSubmatch(output, -1) {
		into := passed
		if m[1] == "Failed" {
			into = failed
		}
		for _, name := range dotnetTestNames(strings.TrimSpace(m[2])) {
			into[name] = true
		}
	}
	for name := range failed {
		delete(passed, name)
	}
	return passed, failed
}

// dotnetTestNames returns the names a test answers to. A review may cite the
// method alone, the class with the method, or the full name, and for one case
// of a theory each of these with the arguments:
//
//	Game.Core.Tests.CounterTests.Adds(times: 2)
//	CounterTests.Adds(times: 2)    Adds(times: 2)
//	Game.Core.Tests.CounterTests.Adds    CounterTests.Adds    Adds
//
// A display name is free text and answers only to itself.
func dotnetTestNames(full string) []string {
	base, args := full, ""
	if open := strings.Index(full, "("); open > 0 && strings.HasSuffix(full, ")") {
		base, args = full[:open], full[open:]
	}
	if !dotnetQualifiedName.MatchString(base) {
		return []string{full}
	}

	forms := []string{base}
	if parts := strings.Split(base, "."); len(parts) > 1 {
		method := parts[len(parts)-1]
		forms = append(forms, parts[len(parts)-2]+"."+method, method)
	}

	names := make([]string, 0, 2*len(forms))
	for _, form := range forms {
		names = append(names, form)
		if args != "" {
			names = append(names, form+args)
		}
	}
	return names
}

// dotnetTestInvocation finds `dotnet test` in a command line.
var dotnetTestInvocation = regexp.MustCompile(`\bdotnet[ \t]+test\b`)

// dotnetConsoleLogger finds a console logger the command already configures.
var dotnetConsoleLogger = regexp.MustCompile(`(--logger|-l)[ \t:=]+["']?console`)

// enhanceDotnetTestCommand makes `dotnet test` name every test it ran. Left
// alone it lists only the tests that failed, and it words its results in the
// language of the machine, which the parser could not read.
//
// A command that does not call `dotnet test` itself (a script, a make target)
// is returned as it is: there is nothing that can safely be added to it.
func enhanceDotnetTestCommand(testCmd string) string {
	if !dotnetTestInvocation.MatchString(testCmd) {
		return testCmd
	}
	enhanced := testCmd
	if !dotnetConsoleLogger.MatchString(testCmd) {
		enhanced = dotnetTestInvocation.ReplaceAllStringFunc(testCmd, func(invocation string) string {
			return invocation + ` --logger "console;verbosity=normal"`
		})
	}
	if !strings.Contains(testCmd, "DOTNET_CLI_UI_LANGUAGE") {
		enhanced = "export DOTNET_CLI_UI_LANGUAGE=en; " + enhanced
	}
	return enhanced
}

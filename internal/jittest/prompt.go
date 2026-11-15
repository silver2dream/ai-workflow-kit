package jittest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/repo"
)

// testConventions is what the model is told about writing tests for a
// toolchain.
type testConventions struct {
	Framework string
	// FileExample shows where a generated file goes and how it is named. The
	// name holds "jittest": that is how the files are found and removed
	// after the run.
	FileExample  string
	Instructions string
}

// conventions holds the toolchains JiT tests can be generated for.
var conventions = map[string]testConventions{
	repo.ToolchainGo: {
		Framework:   "testing",
		FileExample: "path/to/file_jittest_test.go",
		Instructions: `- Use Go's "testing" package. Do NOT use testify or other external packages.
- Each test file must have the correct package declaration matching the directory.
- Use table-driven tests where appropriate.
- Test function names must start with "Test" and use descriptive names.`,
	},
	repo.ToolchainNode: {
		Framework:   "vitest or jest (whichever the project uses)",
		FileExample: "path/to/file.jittest.test.ts",
		Instructions: `- Use vitest or jest (whichever the project uses).
- Use describe/it blocks with clear descriptions.
- Import from relative paths matching the project structure.
- Give the file the extension the project's own tests have (.ts or .js).`,
	},
	repo.ToolchainPython: {
		Framework:   "pytest",
		FileExample: "path/to/test_file_jittest.py",
		Instructions: `- Use pytest conventions. Function names must start with "test_".
- Use assert statements, not unittest.TestCase.`,
	},
	repo.ToolchainDotnet: {
		Framework:   "xUnit (or the framework the test project already references)",
		FileExample: "path/to/Project.Tests/CounterJitTests.jittest.cs",
		Instructions: `- Use the test framework the test project already references. For xUnit that is [Fact], and [Theory] with [InlineData].
- A test file compiles only inside a test project. Put every file in one of the test projects listed below, and nowhere else.
- Use a file-scoped namespace and add the using directives the file needs.
- Name each class <Subject>JitTests, so that it cannot clash with a test class that already exists.`,
	},
}

// Supported reports whether JiT tests can be generated for a repo language.
func Supported(language string) bool {
	_, ok := conventions[repo.ToolchainOf(language)]
	return ok
}

// buildPrompt constructs the LLM prompt for JiT test generation. The language
// must be a supported one.
func buildPrompt(input GenerateInput) string {
	var b strings.Builder

	conv := conventions[repo.ToolchainOf(input.Language)]

	b.WriteString("You are a test engineer. Generate independent tests for the following code changes.\n\n")

	b.WriteString("## Rules\n")
	b.WriteString("1. Only test PUBLIC functions/methods that appear in the diff.\n")
	b.WriteString("2. Focus on: edge cases, error paths, boundary values, nil/empty inputs.\n")
	b.WriteString("3. Each test must be SELF-CONTAINED — no external services, no network calls.\n")
	b.WriteString(fmt.Sprintf("4. Generate at most %d test functions.\n", input.MaxTests))
	b.WriteString(fmt.Sprintf("5. Use the %s test framework.\n", conv.Framework))
	b.WriteString("6. Do NOT copy or reference any existing tests — these are independent verification tests.\n")
	b.WriteString("7. Output each test file as a fenced code block with the filename on the opening line.\n")
	b.WriteString("8. Tests must compile and run without modification.\n")
	b.WriteString("9. Put \"jittest\" in every file name, as in the example below: the files are removed by that name after the run.\n\n")

	if conv.Instructions != "" {
		b.WriteString("## Language-specific instructions\n")
		b.WriteString(conv.Instructions)
		b.WriteString("\n\n")
	}

	if len(input.TestProjects) > 0 {
		b.WriteString("## Test projects (the only directories a test file may go into)\n")
		for _, dir := range input.TestProjects {
			b.WriteString(fmt.Sprintf("- %s\n", dir))
		}
		b.WriteString("\n")
	}

	// Source files for context
	if len(input.SourceFiles) > 0 {
		b.WriteString("## Source files (for import paths, type definitions, function signatures)\n\n")
		// Sort keys for deterministic output
		keys := make([]string, 0, len(input.SourceFiles))
		for k := range input.SourceFiles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, path := range keys {
			b.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", path, input.SourceFiles[path]))
		}
	}

	b.WriteString("## PR Diff\n")
	b.WriteString("```diff\n")
	b.WriteString(input.Diff)
	b.WriteString("\n```\n\n")

	b.WriteString("## Output format\n")
	b.WriteString("For each test file, output a fenced code block like:\n\n")
	b.WriteString(fmt.Sprintf("```filename: %s\n", conv.FileExample))
	b.WriteString("(the complete test file)\n")
	b.WriteString("```\n")

	return b.String()
}

// dotnetTestProject recognizes a test project by what it references.
var dotnetTestProject = regexp.MustCompile(`(?i)Microsoft\.NET\.Test\.Sdk|Include="(xunit|nunit|MSTest)`)

// findDotnetTestProjects returns the directories of the worktree's .NET test
// projects, relative to it and with forward slashes. A generated test only
// compiles inside one of them.
func findDotnetTestProjects(workDir string) []string {
	var dirs []string
	_ = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".ai", ".worktrees", ".godot", "bin", "obj", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".csproj") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil || !dotnetTestProject.Match(content) {
			return nil
		}
		if rel, relErr := filepath.Rel(workDir, filepath.Dir(path)); relErr == nil {
			dirs = append(dirs, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(dirs)
	return dirs
}

// parseDiffFiles extracts file paths from a unified diff, excluding test files.
func parseDiffFiles(diff string) []string {
	var files []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+++ b/") {
			continue
		}
		path := strings.TrimRight(strings.TrimPrefix(line, "+++ b/"), "\r")
		if path == "/dev/null" {
			continue
		}
		// Skip test files — we want source only
		if repo.IsTestFile(path) {
			continue
		}
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	return files
}

// parseGeneratedTests extracts test files from LLM output.
// Expects fenced code blocks with "filename: path/to/file" on the opening line.
func parseGeneratedTests(output string) []GeneratedTest {
	var tests []GeneratedTest

	lines := strings.Split(output, "\n")
	var inBlock bool
	var currentFilename string
	var currentContent strings.Builder

	for _, line := range lines {
		if !inBlock {
			// Look for opening fence with filename
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") && strings.Contains(trimmed, "filename:") {
				// Extract filename after "filename:"
				idx := strings.Index(trimmed, "filename:")
				currentFilename = strings.TrimSpace(trimmed[idx+len("filename:"):])
				currentContent.Reset()
				inBlock = true
			}
			continue
		}

		// Check for closing fence
		trimmed := strings.TrimSpace(line)
		if trimmed == "```" {
			if currentFilename != "" && currentContent.Len() > 0 {
				tests = append(tests, GeneratedTest{
					Filename: currentFilename,
					Content:  currentContent.String(),
				})
			}
			inBlock = false
			currentFilename = ""
			continue
		}

		currentContent.WriteString(line)
		currentContent.WriteByte('\n')
	}

	return tests
}

package jittest

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// languageTestInfo maps language to test framework and file naming conventions.
type languageTestInfo struct {
	Framework    string // e.g. "testing", "vitest", "pytest"
	FileSuffix   string // e.g. "_jittest_test.go"
	FilePrefix   string // e.g. "test_" (python only)
	FileExt      string // e.g. ".go", ".ts", ".py"
	Instructions string // language-specific prompt instructions
}

var languageMap = map[string]languageTestInfo{
	"go": {
		Framework:  "testing",
		FileSuffix: "_jittest_test.go",
		FileExt:    ".go",
		Instructions: `- Use Go's "testing" package. Do NOT use testify or other external packages.
- Each test file must have the correct package declaration matching the directory.
- Use table-driven tests where appropriate.
- Test function names must start with "Test" and use descriptive names.`,
	},
	"typescript": {
		Framework:  "vitest",
		FileSuffix: ".jittest.test.ts",
		FileExt:    ".ts",
		Instructions: `- Use vitest or jest (whichever the project uses).
- Use describe/it blocks with clear descriptions.
- Import from relative paths matching the project structure.`,
	},
	"javascript": {
		Framework:  "vitest",
		FileSuffix: ".jittest.test.js",
		FileExt:    ".js",
		Instructions: `- Use vitest or jest (whichever the project uses).
- Use describe/it blocks with clear descriptions.`,
	},
	"python": {
		Framework:  "pytest",
		FilePrefix: "test_",
		FileSuffix: "_jittest.py",
		FileExt:    ".py",
		Instructions: `- Use pytest conventions. Function names must start with "test_".
- Use assert statements, not unittest.TestCase.`,
	},
	"node": {
		Framework:  "vitest",
		FileSuffix: ".jittest.test.ts",
		FileExt:    ".ts",
		Instructions: `- Use vitest or jest.
- Use describe/it blocks with clear descriptions.`,
	},
}

// buildPrompt constructs the LLM prompt for JiT test generation.
func buildPrompt(diff string, sourceFiles map[string]string, language string, maxTests int) string {
	var b strings.Builder

	langInfo, ok := languageMap[language]
	if !ok {
		langInfo = languageMap["go"] // fallback
	}

	b.WriteString("You are a test engineer. Generate independent tests for the following code changes.\n\n")

	b.WriteString("## Rules\n")
	b.WriteString("1. Only test PUBLIC functions/methods that appear in the diff.\n")
	b.WriteString("2. Focus on: edge cases, error paths, boundary values, nil/empty inputs.\n")
	b.WriteString("3. Each test must be SELF-CONTAINED — no external services, no network calls.\n")
	b.WriteString(fmt.Sprintf("4. Generate at most %d test functions.\n", maxTests))
	b.WriteString(fmt.Sprintf("5. Use the %s test framework.\n", langInfo.Framework))
	b.WriteString("6. Do NOT copy or reference any existing tests — these are independent verification tests.\n")
	b.WriteString("7. Output each test file as a fenced code block with the filename on the opening line.\n")
	b.WriteString("8. Tests must compile and run without modification.\n\n")

	if langInfo.Instructions != "" {
		b.WriteString("## Language-specific instructions\n")
		b.WriteString(langInfo.Instructions)
		b.WriteString("\n\n")
	}

	// Source files for context
	if len(sourceFiles) > 0 {
		b.WriteString("## Source files (for import paths, type definitions, function signatures)\n\n")
		// Sort keys for deterministic output
		keys := make([]string, 0, len(sourceFiles))
		for k := range sourceFiles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, path := range keys {
			b.WriteString(fmt.Sprintf("### %s\n```\n%s\n```\n\n", path, sourceFiles[path]))
		}
	}

	b.WriteString("## PR Diff\n")
	b.WriteString("```diff\n")
	b.WriteString(diff)
	b.WriteString("\n```\n\n")

	b.WriteString("## Output format\n")
	b.WriteString("For each test file, output a fenced code block like:\n\n")
	b.WriteString("```filename: path/to/file_jittest_test.go\n")
	b.WriteString("package ...\n\n")
	b.WriteString("// test code here\n")
	b.WriteString("```\n")

	return b.String()
}

// jitTestFilename generates a JiT test filename from a source file path.
func jitTestFilename(sourcePath, language string) string {
	langInfo, ok := languageMap[language]
	if !ok {
		langInfo = languageMap["go"]
	}

	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	var filename string
	if langInfo.FilePrefix != "" {
		filename = langInfo.FilePrefix + name + langInfo.FileSuffix
	} else {
		filename = name + langInfo.FileSuffix
	}

	return filepath.Join(dir, filename)
}

// parseDiffFiles extracts file paths from a unified diff, excluding test files.
func parseDiffFiles(diff string) []string {
	var files []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+++ b/") {
			continue
		}
		path := strings.TrimPrefix(line, "+++ b/")
		if path == "/dev/null" {
			continue
		}
		// Skip test files — we want source only
		if isTestFile(path) {
			continue
		}
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	return files
}

// isTestFile returns true if the file path looks like a test file.
func isTestFile(path string) bool {
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	if strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.js") {
		return true
	}
	if strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".spec.js") {
		return true
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	return false
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

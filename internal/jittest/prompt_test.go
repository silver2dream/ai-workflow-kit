package jittest

import (
	"strings"
	"testing"
)

func TestBuildPrompt_ContainsRules(t *testing.T) {
	prompt := buildPrompt("diff here", nil, "go", 5)

	checks := []string{
		"edge cases",
		"SELF-CONTAINED",
		"at most 5 test functions",
		"testing",
		"fenced code block",
	}
	for _, check := range checks {
		if !strings.Contains(prompt, check) {
			t.Errorf("prompt missing: %q", check)
		}
	}
}

func TestBuildPrompt_ContainsSourceFiles(t *testing.T) {
	sources := map[string]string{
		"pkg/handler.go": "package pkg\nfunc Handle() {}",
	}
	prompt := buildPrompt("diff", sources, "go", 3)

	if !strings.Contains(prompt, "pkg/handler.go") {
		t.Error("prompt should contain source file path")
	}
	if !strings.Contains(prompt, "func Handle()") {
		t.Error("prompt should contain source file content")
	}
}

func TestBuildPrompt_ContainsDiff(t *testing.T) {
	prompt := buildPrompt("+func NewFeature() {}", nil, "go", 3)
	if !strings.Contains(prompt, "+func NewFeature()") {
		t.Error("prompt should contain the diff")
	}
}

func TestBuildPrompt_TypeScriptInstructions(t *testing.T) {
	prompt := buildPrompt("diff", nil, "typescript", 3)
	if !strings.Contains(prompt, "vitest") {
		t.Error("TypeScript prompt should mention vitest")
	}
}

func TestBuildPrompt_PythonInstructions(t *testing.T) {
	prompt := buildPrompt("diff", nil, "python", 3)
	if !strings.Contains(prompt, "pytest") {
		t.Error("Python prompt should mention pytest")
	}
}

func TestBuildPrompt_UnknownLanguageFallsBackToGo(t *testing.T) {
	prompt := buildPrompt("diff", nil, "cobol", 3)
	if !strings.Contains(prompt, "testing") {
		t.Error("unknown language should fall back to Go testing framework")
	}
}

func TestJitTestFilename_Go(t *testing.T) {
	got := jitTestFilename("internal/config/config.go", "go")
	want := "internal/config/config_jittest_test.go"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJitTestFilename_TypeScript(t *testing.T) {
	got := jitTestFilename("src/utils/parser.ts", "typescript")
	want := "src/utils/parser.jittest.test.ts"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJitTestFilename_Python(t *testing.T) {
	got := jitTestFilename("app/handlers/auth.py", "python")
	want := "app/handlers/test_auth_jittest.py"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseDiffFiles_Basic(t *testing.T) {
	diff := `diff --git a/pkg/foo.go b/pkg/foo.go
--- a/pkg/foo.go
+++ b/pkg/foo.go
@@ -1 +1,2 @@
+func Foo() {}
diff --git a/pkg/foo_test.go b/pkg/foo_test.go
--- a/pkg/foo_test.go
+++ b/pkg/foo_test.go
@@ -1 +1 @@
+func TestFoo(t *testing.T) {}
`
	files := parseDiffFiles(diff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file (test excluded), got %d: %v", len(files), files)
	}
	if files[0] != "pkg/foo.go" {
		t.Errorf("expected pkg/foo.go, got %s", files[0])
	}
}

func TestParseDiffFiles_DevNull(t *testing.T) {
	diff := "+++ /dev/null\n"
	files := parseDiffFiles(diff)
	if len(files) != 0 {
		t.Errorf("expected 0 files for /dev/null, got %d", len(files))
	}
}

func TestParseDiffFiles_Dedup(t *testing.T) {
	diff := "+++ b/foo.go\n+++ b/foo.go\n"
	files := parseDiffFiles(diff)
	if len(files) != 1 {
		t.Errorf("expected 1 deduplicated file, got %d", len(files))
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"foo_test.go", true},
		{"foo.test.ts", true},
		{"foo.spec.js", true},
		{"test_handler.py", true},
		{"foo.go", false},
		{"handler.ts", false},
		{"main.py", false},
		{"testing/helper.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isTestFile(tt.path); got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseGeneratedTests_SingleBlock(t *testing.T) {
	output := "Here is the test:\n\n```go filename: pkg/foo_jittest_test.go\npackage pkg\n\nfunc TestFoo(t *testing.T) {}\n```\n"
	tests := parseGeneratedTests(output)
	if len(tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(tests))
	}
	if tests[0].Filename != "pkg/foo_jittest_test.go" {
		t.Errorf("unexpected filename: %s", tests[0].Filename)
	}
	if !strings.Contains(tests[0].Content, "TestFoo") {
		t.Error("content should contain test function")
	}
}

func TestParseGeneratedTests_MultipleBlocks(t *testing.T) {
	output := `Some explanation

` + "```go filename: a_jittest_test.go\npackage a\nfunc TestA(t *testing.T) {}\n```" + `

More text

` + "```go filename: b_jittest_test.go\npackage b\nfunc TestB(t *testing.T) {}\n```" + "\n"

	tests := parseGeneratedTests(output)
	if len(tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(tests))
	}
	if tests[0].Filename != "a_jittest_test.go" {
		t.Errorf("first test filename: %s", tests[0].Filename)
	}
	if tests[1].Filename != "b_jittest_test.go" {
		t.Errorf("second test filename: %s", tests[1].Filename)
	}
}

func TestParseGeneratedTests_NoFilenameSkipped(t *testing.T) {
	output := "```go\npackage x\nfunc Test(t *testing.T) {}\n```\n"
	tests := parseGeneratedTests(output)
	if len(tests) != 0 {
		t.Errorf("expected 0 tests (no filename), got %d", len(tests))
	}
}

func TestParseGeneratedTests_EmptyOutput(t *testing.T) {
	tests := parseGeneratedTests("")
	if len(tests) != 0 {
		t.Errorf("expected 0 tests for empty output, got %d", len(tests))
	}
}

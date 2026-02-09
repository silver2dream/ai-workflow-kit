package reviewer

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTestReviewTable_StripBackticks(t *testing.T) {
	// Simulate PR review table with backtick-wrapped test names
	reviewBody := `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves correctly | ` + "`TestAdvanceTickMovesSnake`" + ` | assert.Equal(t, expected, actual) |
| Wall collision ends game | ` + "`TestWallCollisionEndsGame`" + ` | assert.True(t, game.IsOver()) |
| Deterministic ticks | ` + "`TestDeterministicTicksWithSameSeed`" + ` | assert.Equal(t, state1, state2) |
`

	mappings, err := parseTestReviewTable(reviewBody, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mappings))
	}

	// Simulate test output parsing result (without backticks)
	passedTests := map[string]bool{
		"TestAdvanceTickMovesSnake":          true,
		"TestWallCollisionEndsGame":          true,
		"TestDeterministicTicksWithSameSeed": true,
	}

	// Verify all test names match (no backticks)
	for _, m := range mappings {
		if !passedTests[m.TestName] {
			t.Errorf("TestName %q not found in passedTests - backticks may not be stripped", m.TestName)
		}

		// Verify no backticks in test name
		if m.TestName[0] == '`' || m.TestName[len(m.TestName)-1] == '`' {
			t.Errorf("TestName %q still contains backticks", m.TestName)
		}
	}
}

func TestParseTestReviewTable_NoBackticks(t *testing.T) {
	// Test that it still works without backticks
	reviewBody := `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves | TestMovement | assert.Equal |
`

	mappings, err := parseTestReviewTable(reviewBody, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}

	if mappings[0].TestName != "TestMovement" {
		t.Errorf("expected TestName 'TestMovement', got %q", mappings[0].TestName)
	}
}

func TestIsValidTestName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid test names
		{"simple test", "TestFoo", true},
		{"with underscore", "TestBar_Integration", true},
		{"with numbers", "Test123", true},
		{"with subtest", "TestFoo/SubTest", true},
		{"complex valid", "TestFoo_Bar123", true},
		{"subtest with underscore", "TestFoo/Sub_Test", true},

		// Invalid test names
		{"all test functions", "All test functions", false},
		{"test in file", "test in file.go", false},
		{"NA", "N/A", false},
		{"empty string", "", false},
		{"lowercase test", "testFoo", false},
		{"no Test prefix", "FooTest", false},
		{"spaces in name", "Test Foo", false},
		{"special chars", "Test@Foo", false},
		{"multiple slashes", "TestFoo/Bar/Baz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidTestName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidTestName(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTestResults(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		expectedPassed []string
		expectedFailed []string
	}{
		{
			name: "simple pass",
			output: `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
PASS
ok  	example.com/pkg	0.001s`,
			expectedPassed: []string{"TestFoo"},
			expectedFailed: []string{},
		},
		{
			name: "subtest pass",
			output: `=== RUN   TestFoo
=== RUN   TestFoo/SubTest1
=== RUN   TestFoo/SubTest2
--- PASS: TestFoo/SubTest1 (0.00s)
--- PASS: TestFoo/SubTest2 (0.00s)
--- PASS: TestFoo (0.00s)
PASS`,
			expectedPassed: []string{"TestFoo", "TestFoo/SubTest1", "TestFoo/SubTest2"},
			expectedFailed: []string{},
		},
		{
			name: "subtest marks parent passed",
			output: `=== RUN   TestParent
=== RUN   TestParent/Child
--- PASS: TestParent/Child (0.00s)
--- PASS: TestParent (0.00s)`,
			expectedPassed: []string{"TestParent", "TestParent/Child"},
			expectedFailed: []string{},
		},
		{
			name: "mixed pass and fail",
			output: `--- PASS: TestGood (0.00s)
--- FAIL: TestBad (0.01s)`,
			expectedPassed: []string{"TestGood"},
			expectedFailed: []string{"TestBad"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed, failed := ParseTestResults(tt.output)

			// Check passed tests
			for _, name := range tt.expectedPassed {
				if !passed[name] {
					t.Errorf("expected %q to be in passed tests", name)
				}
			}

			// Check failed tests
			for _, name := range tt.expectedFailed {
				if !failed[name] {
					t.Errorf("expected %q to be in failed tests", name)
				}
			}
		})
	}
}

func TestParseTestReviewTable_InvalidTestName(t *testing.T) {
	tests := []struct {
		name        string
		reviewBody  string
		expectError bool
		errorSubstr string
	}{
		{
			name: "invalid: All test functions",
			reviewBody: `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves | All test functions | assert.Equal |
`,
			expectError: true,
			errorSubstr: "invalid test name in review table",
		},
		{
			name: "invalid: test in file.go",
			reviewBody: `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves | test in file.go | assert.Equal |
`,
			expectError: true,
			errorSubstr: "invalid test name in review table",
		},
		{
			name: "invalid: N/A",
			reviewBody: `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves | N/A | assert.Equal |
`,
			expectError: true,
			errorSubstr: "invalid test name in review table",
		},
		{
			name: "valid: TestFoo",
			reviewBody: `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves | TestFoo | assert.Equal |
`,
			expectError: false,
		},
		{
			name: "valid: TestFoo/SubTest",
			reviewBody: `
### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Snake moves | TestFoo/SubTest | assert.Equal |
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTestReviewTable(tt.reviewBody, "go")
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				} else if tt.errorSubstr != "" && !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("expected error containing %q, got %q", tt.errorSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestFuzzyMatchTestName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		// Exact match cases
		{"exact match", "TestFoo", "TestFoo", true},
		{"exact match with underscore", "TestFoo_Bar", "TestFoo_Bar", true},

		// Subtest parent matching
		{"subtest parent", "TestFoo", "TestFoo/SubTest", true},
		{"subtest parent multi", "TestFoo", "TestFoo/Sub/Test", true}, // nested subtest still matches parent
		{"expected is subtest", "TestFoo/SubTest", "TestFoo", true},

		// Version suffix matching
		{"version suffix v2", "TestFoo", "TestFoo_v2", true},
		{"version suffix V2", "TestFoo", "TestFoo_V2", true},

		// Normalized matching (underscore removal + lowercase)
		{"normalize underscore", "TestFoo_Bar", "TestFooBar", true},
		{"normalize case", "TestFOO", "Testfoo", true},

		// Similar names (reordering) - using Levenshtein
		{"similar names swap", "TestReadChapter", "TestChapterRead", true},
		{"similar names partial", "TestGetUser", "TestUserGet", true},

		// Completely different
		{"completely different", "TestFoo", "TestBar", false},
		{"very different", "TestAuthentication", "TestPayment", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyMatchTestName(tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("fuzzyMatchTestName(%q, %q) = %v, want %v", tt.expected, tt.actual, got, tt.want)
			}
		})
	}
}

func TestNormalizeTestName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "TestFoo", "foo"},
		{"with underscore", "TestFoo_Bar", "foobar"},
		{"with subtest", "TestFoo/SubTest", "foo"},
		{"with version suffix", "TestFoo_v2", "foo"},
		{"with V2 suffix", "TestFoo_V2", "foo"},
		{"complex", "TestFoo_Bar_v2/SubTest", "foobar"},
		{"uppercase", "TestFOO", "foo"},
		{"mixed case", "TestFooBar", "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTestName(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeTestName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLevenshteinSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		minSim   float64
		maxSim   float64
	}{
		{"identical", "foo", "foo", 1.0, 1.0},
		{"empty strings", "", "", 1.0, 1.0},
		{"one empty", "foo", "", 0.0, 0.0},
		{"one char diff", "foo", "foa", 0.6, 0.7},
		{"reordered words", "readchapter", "chapterread", 0.2, 0.4}, // Levenshtein gives low similarity for reordering
		{"completely different", "abc", "xyz", 0.0, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := levenshteinSimilarity(tt.a, tt.b)
			if got < tt.minSim || got > tt.maxSim {
				t.Errorf("levenshteinSimilarity(%q, %q) = %v, want between %v and %v", tt.a, tt.b, got, tt.minSim, tt.maxSim)
			}
		})
	}
}

func TestTokensMatch(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"same tokens reordered", "readchapter", "chapterread", true},
		{"same tokens", "getuser", "userget", true},
		{"single common token", "readdata", "writedata", true}, // both contain "data"
		{"no common tokens", "foo", "bar", false},
		{"one common token", "createuser", "deleteuser", true}, // both contain "user"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokensMatch(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("tokensMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestExtractTokens(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantMinTokens  int
		wantContains   []string
	}{
		{"read chapter", "readchapter", 2, []string{"read", "chapter"}},
		{"get user", "getuser", 2, []string{"get", "user"}},
		{"create item", "createitem", 2, []string{"create", "item"}},
		{"unknown word", "foobar", 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTokens(tt.input)
			if len(got) < tt.wantMinTokens {
				t.Errorf("extractTokens(%q) returned %d tokens, want at least %d", tt.input, len(got), tt.wantMinTokens)
			}
			for _, want := range tt.wantContains {
				found := false
				for _, token := range got {
					if token == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extractTokens(%q) = %v, want to contain %q", tt.input, got, want)
				}
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"identical", "foo", "foo", 0},
		{"empty strings", "", "", 0},
		{"one empty", "foo", "", 3},
		{"other empty", "", "bar", 3},
		{"one char diff", "foo", "foa", 1},
		{"insert", "foo", "fooo", 1},
		{"delete", "fooo", "foo", 1},
		{"replace", "foo", "bar", 3},
		{"kitten to sitting", "kitten", "sitting", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestFindSimilarTests(t *testing.T) {
	passedTests := map[string]bool{
		"TestReadChapter":  true,
		"TestWriteChapter": true,
		"TestDeleteUser":   true,
		"TestGetUser":      true,
		"TestCreateUser":   true,
	}

	tests := []struct {
		name           string
		expected       string
		threshold      float64
		wantMinMatches int
	}{
		// Note: findSimilarTests uses Levenshtein similarity only, not token matching
		// Word reordering results in low Levenshtein similarity
		{"find similar chapter", "TestChapterRead", 0.2, 1},  // Low threshold needed for reordering
		{"find similar with suffix", "TestGetUserById", 0.5, 1}, // Should find TestGetUser with higher similarity
		{"no similar", "TestPayment", 0.7, 0},                // Should find nothing
		{"very low threshold", "TestSomething", 0.1, 1},      // Very low threshold should find something
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findSimilarTests(tt.expected, passedTests, tt.threshold)
			if len(got) < tt.wantMinMatches {
				t.Errorf("findSimilarTests(%q, ..., %v) returned %d matches, want at least %d", tt.expected, tt.threshold, len(got), tt.wantMinMatches)
			}
		})
	}
}

// TestFuzzyMatchTestNameIntegration tests the fuzzy matching in a realistic scenario
func TestFuzzyMatchTestNameIntegration(t *testing.T) {
	// Simulate a scenario where exact match fails but fuzzy should work
	passedTests := map[string]bool{
		"TestReadChapter":          true,
		"TestReadChapter/SubTest1": true,
		"TestReadChapter/SubTest2": true,
		"TestWriteChapter_v2":      true,
	}

	expectedTests := []string{
		"TestReadChapter",    // exact match
		"TestChapterRead",    // reordering - should fuzzy match to TestReadChapter
		"TestWriteChapter",   // version suffix - should fuzzy match to TestWriteChapter_v2
	}

	for _, expected := range expectedTests {
		found := false

		// Try exact match first
		if passedTests[expected] {
			found = true
		}

		// Try fuzzy match
		if !found {
			for passedName := range passedTests {
				if fuzzyMatchTestName(expected, passedName) {
					found = true
					t.Logf("Fuzzy matched: %s -> %s", expected, passedName)
					break
				}
			}
		}

		if !found {
			t.Errorf("Expected test %q not found via exact or fuzzy match", expected)
		}
	}
}

// TestMin verifies the min helper function
func TestMin(t *testing.T) {
	tests := []struct {
		a, b, c  int
		expected int
	}{
		{1, 2, 3, 1},
		{3, 2, 1, 1},
		{2, 1, 3, 1},
		{1, 1, 1, 1},
		{0, 5, 10, 0},
		{-1, 0, 1, -1},
	}

	for _, tt := range tests {
		got := min(tt.a, tt.b, tt.c)
		if got != tt.expected {
			t.Errorf("min(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, got, tt.expected)
		}
	}
}

// Suppress unused import warning
var _ = math.Abs

// Tests for ParseAcceptanceCriteria
func TestParseAcceptanceCriteria(t *testing.T) {
	tests := []struct {
		name   string
		ticket string
		want   []string
	}{
		{
			name: "standard format with checkbox",
			ticket: `## Acceptance Criteria
- [ ] Feature A works
- [ ] Feature B works
- [x] Feature C completed`,
			want: []string{"Feature A works", "Feature B works", "Feature C completed"},
		},
		{
			name: "asterisk list",
			ticket: `## Acceptance Criteria
* [ ] First criterion
* [x] Second criterion`,
			want: []string{"First criterion", "Second criterion"},
		},
		{
			name:   "no criteria",
			ticket: "Just some text without criteria",
			want:   nil,
		},
		{
			name:   "empty string",
			ticket: "",
			want:   nil,
		},
		{
			name: "mixed content",
			ticket: `# Title
Some description
## Acceptance Criteria
- [ ] Must work correctly
- Not a criterion
- [x] Another criterion

## Other section
- [ ] Not in criteria section but still matches`,
			want: []string{"Must work correctly", "Another criterion", "Not in criteria section but still matches"},
		},
		{
			name: "whitespace handling",
			ticket: `- [ ] Extra space after
- [ ]   Trailing space trimmed`,
			want: []string{"Extra space after", "Trailing space trimmed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAcceptanceCriteria(tt.ticket)
			if len(got) != len(tt.want) {
				t.Errorf("ParseAcceptanceCriteria() returned %d criteria, want %d", len(got), len(tt.want))
				t.Errorf("got: %v", got)
				return
			}
			for i, c := range got {
				if c != tt.want[i] {
					t.Errorf("criteria[%d] = %q, want %q", i, c, tt.want[i])
				}
			}
		})
	}
}

// Tests for ValidateCompleteness
func TestValidateCompleteness(t *testing.T) {
	tests := []struct {
		name          string
		criteria      []string
		verifications []CriteriaVerification
		wantErr       bool
		errCode       int
	}{
		{
			name:     "all criteria verified",
			criteria: []string{"Feature A", "Feature B"},
			verifications: []CriteriaVerification{
				{Criteria: "Feature A", Implementation: "Implemented using X pattern", TestName: "TestFeatureA", Assertion: "assert.Equal"},
				{Criteria: "Feature B", Implementation: "Implemented using Y pattern", TestName: "TestFeatureB", Assertion: "assert.True"},
			},
			wantErr: false,
		},
		{
			name:     "missing verification",
			criteria: []string{"Feature A", "Feature B"},
			verifications: []CriteriaVerification{
				{Criteria: "Feature A", Implementation: "Implemented using X", TestName: "TestFeatureA", Assertion: "assert.Equal"},
			},
			wantErr: true,
			errCode: 1,
		},
		{
			name:     "missing implementation",
			criteria: []string{"Feature A"},
			verifications: []CriteriaVerification{
				{Criteria: "Feature A", Implementation: "", TestName: "TestFeatureA", Assertion: "assert.Equal"},
			},
			wantErr: true,
			errCode: 1,
		},
		{
			name:     "implementation too short",
			criteria: []string{"Feature A"},
			verifications: []CriteriaVerification{
				{Criteria: "Feature A", Implementation: "Short", TestName: "TestFeatureA", Assertion: "assert.Equal"},
			},
			wantErr: true,
			errCode: 1,
		},
		{
			name:     "missing test name (ok for meta-criteria)",
			criteria: []string{"Feature A"},
			verifications: []CriteriaVerification{
				{Criteria: "Feature A", Implementation: "Implemented using X pattern", TestName: "", Assertion: "assert.Equal"},
			},
			wantErr: false,
		},
		{
			name:     "missing assertion (ok for meta-criteria)",
			criteria: []string{"Feature A"},
			verifications: []CriteriaVerification{
				{Criteria: "Feature A", Implementation: "Implemented using X pattern", TestName: "TestFeatureA", Assertion: ""},
			},
			wantErr: false,
		},
		{
			name:          "empty criteria",
			criteria:      []string{},
			verifications: []CriteriaVerification{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompleteness(tt.criteria, tt.verifications)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if err.Code != tt.errCode {
					t.Errorf("error code = %d, want %d", err.Code, tt.errCode)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Tests for extractSection
func TestExtractSection(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		sectionName string
		wantEmpty   bool
		wantContain string
	}{
		{
			name: "extract h3 section",
			body: `### Introduction
Intro content

### Test Review
Test review content here

### Conclusion
End`,
			sectionName: "Test Review",
			wantEmpty:   false,
			wantContain: "Test review content",
		},
		{
			name: "extract h2 section",
			body: `## Overview
Overview content

## Implementation Review
Implementation details

## Summary`,
			sectionName: "Implementation Review",
			wantEmpty:   false,
			wantContain: "Implementation details",
		},
		{
			name: "section not found",
			body: `### Section A
Content A

### Section B
Content B`,
			sectionName: "Section C",
			wantEmpty:   true,
		},
		{
			name:        "empty body",
			body:        "",
			sectionName: "Test",
			wantEmpty:   true,
		},
		{
			name: "section at end of document",
			body: `### First
Content

### Target Section
Target content here`,
			sectionName: "Target Section",
			wantEmpty:   false,
			wantContain: "Target content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSection(tt.body, tt.sectionName)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("extractSection() = %q, want empty", got)
				}
			} else {
				if got == "" {
					t.Error("extractSection() returned empty, want non-empty")
				} else if tt.wantContain != "" && !strings.Contains(got, tt.wantContain) {
					t.Errorf("extractSection() = %q, want to contain %q", got, tt.wantContain)
				}
			}
		})
	}
}

// Tests for fuzzyMatch (criteria matching)
func TestFuzzyMatchCriteria(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"exact match", "Feature works", "Feature works", true},
		{"case insensitive", "Feature Works", "feature works", true},
		{"close contains match", "add authentication check", "add authentication", true},          // 18/24 = 75%
		{"too short contains", "Feature A works correctly", "Feature A", false},                   // 9/24 = 37% → rejected by ratio
		{"too short reverse", "Feature", "Feature works correctly", false},                        // 7/22 = 31% → rejected
		{"whitespace normalized", "Feature  works", "Feature works", true},
		{"completely different", "Feature A", "Something else", false},
		{"empty strings", "", "", true},
		{"one empty", "Feature", "", false},                                                       // empty vs non-empty → rejected
		{"similar criteria prevent false positive", "add logging", "add logging to all modules", false}, // 11/26 = 42% → rejected
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyMatch(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// Tests for normalizeForMatching
func TestNormalizeForMatching(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple string", "hello", "hello"},
		{"with backticks", "`code`", "code"},
		{"with tabs", "hello\tworld", "hello world"},
		{"with multiple spaces", "hello   world", "hello world"},
		{"with carriage return", "hello\r\nworld", "hello world"},
		{"leading/trailing space", "  hello  ", "hello"},
		{"complex", "  `code`\twith\r\n  spaces  ", "code with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeForMatching(tt.input)
			if got != tt.want {
				t.Errorf("normalizeForMatching(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Tests for parseImplementationReview
func TestParseImplementationReview(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantKey   string
		wantValue string
	}{
		{
			name: "standard format",
			body: `### Implementation Review

#### 1. Feature A works
**實作邏輯**: Using the new algorithm to process data

#### 2. Feature B works
**實作邏輯**: Implemented with caching layer`,
			wantCount: 2,
			wantKey:   "Feature A works",
			wantValue: "Using the new algorithm to process data",
		},
		{
			name: "english format",
			body: `### Implementation Review

#### 1. Feature A
**Implementation**: The feature is implemented using X pattern`,
			wantCount: 1,
			wantKey:   "Feature A",
			wantValue: "The feature is implemented using X pattern",
		},
		{
			name:      "no implementation section",
			body:      `### Other Section\nSome content`,
			wantCount: 0,
		},
		{
			name:      "empty body",
			body:      "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseImplementationReview(tt.body)
			if len(got) != tt.wantCount {
				t.Errorf("parseImplementationReview() returned %d items, want %d", len(got), tt.wantCount)
			}
			if tt.wantKey != "" {
				if val, ok := got[tt.wantKey]; !ok {
					t.Errorf("missing key %q", tt.wantKey)
				} else if tt.wantValue != "" && val != tt.wantValue {
					t.Errorf("value for %q = %q, want %q", tt.wantKey, val, tt.wantValue)
				}
			}
		})
	}
}

// Tests for ParseCriteriaVerifications
func TestParseCriteriaVerifications(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantErr   bool
	}{
		{
			name: "complete review body",
			body: `### Implementation Review

#### 1. Feature A works
**實作邏輯**: Using the new algorithm to process data efficiently

### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature A works | TestFeatureA | assert.Equal(t, expected, actual) |
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "only test review",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature A | TestA | assert.True |
| Feature B | TestB | assert.False |
`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty body",
			body:      "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "invalid test name",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature A | invalid test name | assert.True |
`,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCriteriaVerifications(tt.body, "go")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(got) != tt.wantCount {
					t.Errorf("ParseCriteriaVerifications() returned %d items, want %d", len(got), tt.wantCount)
				}
			}
		})
	}
}

// Tests for findTestFiles
func TestFindTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	files := map[string]string{
		"main.go":                  "package main",
		"main_test.go":             "package main",
		"sub/helper.go":            "package sub",
		"sub/helper_test.go":       "package sub",
		"js/app.js":                "console.log('app')",
		"js/app.test.js":           "test('app')",
		"ts/util.ts":               "export {}",
		"ts/util.spec.ts":          "describe('util')",
		"vendor/dep/dep_test.go":   "package dep",
		"node_modules/pkg/test.js": "test",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	testFiles, err := findTestFiles(tmpDir)
	if err != nil {
		t.Fatalf("findTestFiles() error: %v", err)
	}

	// Should find: main_test.go, sub/helper_test.go, js/app.test.js, ts/util.spec.ts
	// Should NOT find: vendor/*, node_modules/*
	expectedCount := 4
	if len(testFiles) != expectedCount {
		t.Errorf("findTestFiles() found %d files, want %d", len(testFiles), expectedCount)
		for _, f := range testFiles {
			t.Logf("  found: %s", f)
		}
	}

	// Verify vendor and node_modules are excluded
	for _, f := range testFiles {
		if strings.Contains(f, "vendor") {
			t.Errorf("vendor file should be excluded: %s", f)
		}
		if strings.Contains(f, "node_modules") {
			t.Errorf("node_modules file should be excluded: %s", f)
		}
	}
}

// Tests for VerifyAssertions
func TestVerifyAssertions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file with assertions
	testContent := `package main

import "testing"

func TestFeatureA(t *testing.T) {
	assert.Equal(t, expected, actual)
	require.NoError(t, err)
}

func TestFeatureB(t *testing.T) {
	assert.True(t, condition)
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main_test.go"), []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tests := []struct {
		name          string
		verifications []CriteriaVerification
		wantErr       bool
	}{
		{
			name: "assertions found",
			verifications: []CriteriaVerification{
				{TestName: "TestFeatureA", Assertion: "assert.Equal"},
				{TestName: "TestFeatureB", Assertion: "assert.True"},
			},
			wantErr: false,
		},
		{
			name: "assertion not found",
			verifications: []CriteriaVerification{
				{TestName: "TestFeatureA", Assertion: "assert.NotExists"},
			},
			wantErr: true,
		},
		{
			name: "empty assertion skipped",
			verifications: []CriteriaVerification{
				{TestName: "TestFeatureA", Assertion: ""},
			},
			wantErr: false,
		},
		{
			name:          "no verifications",
			verifications: []CriteriaVerification{},
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyAssertions(tmpDir, tt.verifications)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Tests for CriteriaVerification struct
func TestCriteriaVerificationStruct(t *testing.T) {
	cv := CriteriaVerification{
		Criteria:       "Feature A works",
		Implementation: "Implemented using X",
		TestName:       "TestFeatureA",
		Assertion:      "assert.Equal",
	}

	if cv.Criteria != "Feature A works" {
		t.Errorf("Criteria = %q", cv.Criteria)
	}
	if cv.Implementation != "Implemented using X" {
		t.Errorf("Implementation = %q", cv.Implementation)
	}
	if cv.TestName != "TestFeatureA" {
		t.Errorf("TestName = %q", cv.TestName)
	}
	if cv.Assertion != "assert.Equal" {
		t.Errorf("Assertion = %q", cv.Assertion)
	}
}

// Tests for VerifyOptions struct
func TestVerifyOptionsDefaults(t *testing.T) {
	opts := VerifyOptions{
		Ticket:       "ticket content",
		ReviewBody:   "review body",
		WorktreePath: "/path/to/worktree",
		TestCommand:  "go test ./...",
	}

	if opts.TestTimeout != 0 {
		t.Errorf("TestTimeout should be 0 initially, got %v", opts.TestTimeout)
	}
}

// Tests for extractImplementationLogic
func TestExtractImplementationLogic(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "chinese format",
			content: "**實作邏輯**: This is the implementation logic\n\n**Something else**",
			want:    "This is the implementation logic",
		},
		{
			name:    "english format",
			content: "**Implementation**: The feature is implemented using pattern X\n\n",
			want:    "The feature is implemented using pattern X",
		},
		{
			name:    "simple chinese format",
			content: "實作邏輯: Simple implementation\n\n",
			want:    "Simple implementation",
		},
		{
			name:    "no implementation",
			content: "Some other content without implementation section",
			want:    "",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "with colon variants",
			content: "**實作邏輯**：使用中文冒號\n\n",
			want:    "使用中文冒號",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractImplementationLogic(tt.content)
			if got != tt.want {
				t.Errorf("extractImplementationLogic() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Tests for VerifyTestEvidence validation paths
func TestVerifyTestEvidence_Validation(t *testing.T) {
	tests := []struct {
		name    string
		opts    VerifyOptions
		wantErr bool
		errCode int
		errMsg  string
	}{
		{
			name: "empty ticket",
			opts: VerifyOptions{
				Ticket:       "",
				ReviewBody:   "some review",
				WorktreePath: "/tmp/test",
				TestCommand:  "go test ./...",
			},
			wantErr: true,
			errCode: 1,
			errMsg:  "no acceptance criteria",
		},
		{
			name: "ticket without criteria",
			opts: VerifyOptions{
				Ticket:       "Just some text without checkboxes",
				ReviewBody:   "some review",
				WorktreePath: "/tmp/test",
				TestCommand:  "go test ./...",
			},
			wantErr: true,
			errCode: 1,
			errMsg:  "no acceptance criteria",
		},
		{
			name: "empty review body",
			opts: VerifyOptions{
				Ticket:       "- [ ] Feature A",
				ReviewBody:   "",
				WorktreePath: "/tmp/test",
				TestCommand:  "go test ./...",
			},
			wantErr: true,
			errCode: 1,
			errMsg:  "no criteria verifications",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyTestEvidence(nil, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else {
					if err.Code != tt.errCode {
						t.Errorf("error code = %d, want %d", err.Code, tt.errCode)
					}
					if tt.errMsg != "" && !strings.Contains(err.Message, tt.errMsg) {
						t.Errorf("error message = %q, want to contain %q", err.Message, tt.errMsg)
					}
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Tests for testMapping struct
func TestTestMappingStruct(t *testing.T) {
	tm := testMapping{
		Criteria:  "Feature A",
		TestName:  "TestFeatureA",
		Assertion: "assert.Equal",
	}

	if tm.Criteria != "Feature A" {
		t.Errorf("Criteria = %q", tm.Criteria)
	}
	if tm.TestName != "TestFeatureA" {
		t.Errorf("TestName = %q", tm.TestName)
	}
	if tm.Assertion != "assert.Equal" {
		t.Errorf("Assertion = %q", tm.Assertion)
	}
}

// Tests for parseTestReviewTable edge cases
func TestParseTestReviewTable_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "no test review section",
			body:      "### Other Section\nSome content",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "empty table",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "table with empty criteria",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
|  | TestA | assert.True |
`,
			wantCount: 0, // Empty criteria should be skipped
			wantErr:   false,
		},
		{
			name: "multiple valid rows",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature A | TestA | assert.True |
| Feature B | TestB | assert.False |
| Feature C | TestC | require.NoError |
`,
			wantCount: 3,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTestReviewTable(tt.body, "go")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(got) != tt.wantCount {
					t.Errorf("parseTestReviewTable() returned %d mappings, want %d", len(got), tt.wantCount)
				}
			}
		})
	}
}

// Test for findTestFiles with non-existent directory
func TestFindTestFiles_NonExistent(t *testing.T) {
	testFiles, err := findTestFiles("/non/existent/path")
	if err != nil {
		// Error is acceptable for non-existent path
		return
	}
	if len(testFiles) != 0 {
		t.Errorf("findTestFiles() should return 0 files for non-existent path, got %d", len(testFiles))
	}
}

// Test for VerifyAssertions with non-existent worktree
func TestVerifyAssertions_NonExistentWorktree(t *testing.T) {
	verifications := []CriteriaVerification{
		{TestName: "TestFeature", Assertion: "assert.True"},
	}

	err := VerifyAssertions("/non/existent/path", verifications)
	// Should not error - just won't find any assertions
	if err != nil && err.Code != 3 {
		t.Errorf("unexpected error code: %d", err.Code)
	}
}

// Test for ParseTestResults with Node/Jest format
func TestParseTestResults_NodeFormat(t *testing.T) {
	// The Node/Jest parser includes the timing in the test name
	// Test that the format is correctly parsed (spaces become underscores)
	tests := []struct {
		name           string
		output         string
		expectedPassed []string
		expectedFailed []string
	}{
		{
			name: "jest pass format",
			output: `  ✓ should handle empty input (5ms)
  ✓ should process valid data (10ms)`,
			// The parser converts "should handle empty input (5ms)" to "should_handle_empty_input_(5ms)"
			expectedPassed: []string{"should_handle_empty_input_(5ms)", "should_process_valid_data_(10ms)"},
			expectedFailed: []string{},
		},
		{
			name:           "jest fail format",
			output:         `  ✕ should fail gracefully (15ms)`,
			expectedFailed: []string{"should_fail_gracefully_(15ms)"},
			expectedPassed: []string{},
		},
		{
			name: "mixed go and node",
			output: `--- PASS: TestGoFunction (0.00s)
  ✓ node test passes`,
			expectedPassed: []string{"TestGoFunction", "node_test_passes"},
			expectedFailed: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed, failed := ParseTestResults(tt.output)

			for _, name := range tt.expectedPassed {
				if !passed[name] {
					t.Errorf("expected %q to be in passed tests, got %v", name, passed)
				}
			}

			for _, name := range tt.expectedFailed {
				if !failed[name] {
					t.Errorf("expected %q to be in failed tests, got %v", name, failed)
				}
			}
		})
	}
}

// Test for ParseTestResults with Vitest format
func TestParseTestResults_VitestFormat(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		expectedPassed []string
		expectedFailed []string
	}{
		{
			name: "vitest verbose pass format",
			output: ` ✓ __tests__/math.test.ts > addition > 2 + 2 should equal 4 1ms
 ✓ __tests__/math.test.ts > addition > 3 + 3 should equal 6 2ms
 ✓ __tests__/string.test.ts > concat > joins strings 1ms`,
			expectedPassed: []string{
				"2_+_2_should_equal_4",
				"3_+_3_should_equal_6",
				"joins_strings",
				"addition/2_+_2_should_equal_4",
				"addition/3_+_3_should_equal_6",
				"concat/joins_strings",
			},
			expectedFailed: []string{},
		},
		{
			name: "vitest verbose fail format",
			output: ` ✕ __tests__/math.test.ts > subtraction > 5 - 3 should equal 2 5ms`,
			expectedPassed: []string{},
			expectedFailed: []string{
				"5_-_3_should_equal_2",
				"subtraction/5_-_3_should_equal_2",
			},
		},
		{
			name: "vitest default pass format",
			output: ` ✓ test/example.test.ts (5 tests) 306ms
 ✓ src/utils.test.js (3 tests | 1 skipped) 150ms`,
			expectedPassed: []string{
				"test/example.test.ts",
				"src/utils.test.js",
			},
			expectedFailed: []string{},
		},
		{
			name: "vitest default fail format",
			output: ` ✕ test/broken.test.ts (2 failed) 100ms`,
			expectedPassed: []string{},
			expectedFailed: []string{
				"test/broken.test.ts",
			},
		},
		{
			name: "vitest mixed verbose and summary",
			output: ` ✓ __tests__/api.test.ts > GET /users > returns user list 10ms
 ✓ __tests__/api.test.ts > POST /users > creates new user 15ms

 Test Files  1 passed (1)
      Tests  2 passed (2)
   Duration  1.26s`,
			expectedPassed: []string{
				"returns_user_list",
				"creates_new_user",
				"GET_/users/returns_user_list",
				"POST_/users/creates_new_user",
			},
			expectedFailed: []string{},
		},
		{
			name: "vitest with tsx files",
			output: ` ✓ components/Button.test.tsx > Button > renders correctly 5ms
 ✓ components/Input.test.jsx > Input > handles change 3ms`,
			expectedPassed: []string{
				"renders_correctly",
				"handles_change",
				"Button/renders_correctly",
				"Input/handles_change",
			},
			expectedFailed: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed, failed := ParseTestResults(tt.output)

			for _, name := range tt.expectedPassed {
				if !passed[name] {
					t.Errorf("expected %q to be in passed tests, got passed=%v", name, passed)
				}
			}

			for _, name := range tt.expectedFailed {
				if !failed[name] {
					t.Errorf("expected %q to be in failed tests, got failed=%v", name, failed)
				}
			}
		})
	}
}

// Test for EvidenceError
func TestEvidenceError_Fields(t *testing.T) {
	err := &EvidenceError{
		Code:    2,
		Message: "test execution failed",
		Details: []string{"detail 1", "detail 2"},
	}

	if err.Code != 2 {
		t.Errorf("Code = %d, want 2", err.Code)
	}
	if err.Message != "test execution failed" {
		t.Errorf("Message = %q", err.Message)
	}
	if len(err.Details) != 2 {
		t.Errorf("Details length = %d, want 2", len(err.Details))
	}
	if err.Error() != "test execution failed" {
		t.Errorf("Error() = %q", err.Error())
	}
}

// Test for fuzzyMatch with Chinese stop words
func TestFuzzyMatchWithStopWords(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"with 的", "功能的實作", "功能實作", true},
		{"with 後", "完成後檢查", "完成檢查", true},
		{"with 要", "需要驗證", "需驗證", true},
		{"with 會", "系統會回應", "系統回應", true},
		{"with 應該", "應該成功", "成功", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyMatch(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// Tests for language-aware test name validators
func TestGoTestValidator(t *testing.T) {
	validator := &GoTestValidator{}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid simple", "TestFoo", true},
		{"valid with underscore", "TestFoo_Bar", true},
		{"valid with subtest", "TestFoo/SubTest", true},
		{"valid complex", "TestFoo_Bar123", true},
		{"invalid no Test prefix", "FooTest", false},
		{"invalid lowercase", "testFoo", false},
		{"invalid spaces", "Test Foo", false},
		{"invalid empty", "", false},
		{"invalid special chars", "Test@Foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.IsValid(tt.input)
			if got != tt.expected {
				t.Errorf("GoTestValidator.IsValid(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}

	// Test format hint
	hint := validator.FormatHint()
	if hint == "" {
		t.Error("GoTestValidator.FormatHint() should not be empty")
	}
}

func TestNodeTestValidator(t *testing.T) {
	validator := &NodeTestValidator{}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid natural language", "renders component correctly", true},
		{"valid with spaces", "should handle empty input", true},
		{"valid single word", "works", true},
		{"valid camelCase", "rendersLobbyShell", true},
		{"invalid empty", "", false},
		{"invalid N/A", "N/A", false},
		{"invalid All test", "All test functions", false},
		{"invalid test in file", "test in file.go", false},
		{"invalid TODO", "TODO: fix this", false},
		{"invalid SKIP", "SKIP: not implemented", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.IsValid(tt.input)
			if got != tt.expected {
				t.Errorf("NodeTestValidator.IsValid(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}

	// Test format hint
	hint := validator.FormatHint()
	if hint == "" {
		t.Error("NodeTestValidator.FormatHint() should not be empty")
	}
}

func TestPythonTestValidator(t *testing.T) {
	validator := &PythonTestValidator{}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid test_function", "test_foo", true},
		{"valid test_with_underscores", "test_foo_bar", true},
		{"valid TestClass", "TestFoo", true},
		{"valid TestClass::method", "TestFoo::test_bar", true},
		{"valid path::test", "tests/test_foo.py::test_bar", true},
		{"invalid empty", "", false},
		{"invalid camelCase", "testFoo", false},
		{"invalid no test prefix", "foo_test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.IsValid(tt.input)
			if got != tt.expected {
				t.Errorf("PythonTestValidator.IsValid(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}

	// Test format hint
	hint := validator.FormatHint()
	if hint == "" {
		t.Error("PythonTestValidator.FormatHint() should not be empty")
	}
}

func TestPermissiveValidator(t *testing.T) {
	validator := &PermissiveValidator{}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid any string", "anything goes here", true},
		{"valid numbers", "test123", true},
		{"valid unicode", "測試案例", true},
		{"valid special chars", "test@foo#bar", true},
		{"invalid empty", "", false},
		{"invalid N/A", "N/A", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.IsValid(tt.input)
			if got != tt.expected {
				t.Errorf("PermissiveValidator.IsValid(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}

	// Test format hint
	hint := validator.FormatHint()
	if hint == "" {
		t.Error("PermissiveValidator.FormatHint() should not be empty")
	}
}

func TestGetTestNameValidator(t *testing.T) {
	tests := []struct {
		language string
	}{
		{"go"},
		{"golang"},
		{"node"},
		{"nodejs"},
		{"typescript"},
		{"javascript"},
		{"ts"},
		{"js"},
		{"python"},
		{"py"},
		{"unknown"},
		{"rust"},
		{""},
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			got := GetTestNameValidator(tt.language)
			// Just check it returns a non-nil validator
			if got == nil {
				t.Errorf("GetTestNameValidator(%q) returned nil", tt.language)
			}
			// Verify it has FormatHint
			if got.FormatHint() == "" {
				t.Errorf("GetTestNameValidator(%q).FormatHint() should not be empty", tt.language)
			}
		})
	}
}

func TestIsValidTestNameForLanguage(t *testing.T) {
	tests := []struct {
		name     string
		testName string
		language string
		expected bool
	}{
		// Go tests
		{"go valid", "TestFoo", "go", true},
		{"go invalid", "test foo", "go", false},
		// Node tests
		{"node valid", "renders component correctly", "node", true},
		{"node invalid N/A", "N/A", "node", false},
		// Python tests
		{"python valid", "test_foo", "python", true},
		{"python invalid", "foo", "python", false},
		// Unknown language uses permissive
		{"unknown valid", "anything", "unknown", true},
		{"unknown invalid empty", "", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidTestNameForLanguage(tt.testName, tt.language)
			if got != tt.expected {
				t.Errorf("isValidTestNameForLanguage(%q, %q) = %v, want %v", tt.testName, tt.language, got, tt.expected)
			}
		})
	}
}

// TestVerifyTestEvidence_VitestFileLevelSkipsAssertions verifies that when
// test output is Vitest file-level only (no individual test names), both
// per-test matching AND assertion verification are skipped.
func TestVerifyTestEvidence_VitestFileLevelSkipsAssertions(t *testing.T) {
	// Create a temp worktree with a test file that does NOT contain
	// the assertion text from the review body
	tmpDir := t.TempDir()
	testContent := `import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'

describe('GameCanvas', () => {
  it('renders canvas element', () => {
    const { container } = render(<GameCanvas />)
    expect(container.querySelector('canvas')).toBeTruthy()
  })
})
`
	if err := os.WriteFile(filepath.Join(tmpDir, "GameCanvas.test.ts"), []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create a mock test command that produces Vitest file-level output and exits 0
	scriptPath := filepath.Join(tmpDir, "run-test.sh")
	scriptContent := `#!/bin/sh
echo ' ✓ GameCanvas.test.ts (1 test) 50ms'
echo ''
echo ' Test Files  1 passed (1)'
echo '      Tests  1 passed (1)'
echo '   Duration  0.50s'
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	opts := VerifyOptions{
		Ticket: `## Acceptance Criteria
- [ ] Canvas renders game state`,
		ReviewBody: `### Implementation Review

#### 1. Canvas renders game state
**Implementation**: Renders canvas using React component with game snapshot data

### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Canvas renders game state | TestRenderDrawsSnakeSegments | expect(canvas).toMatchSnapshot() |
`,
		WorktreePath: tmpDir,
		TestCommand:  "sh " + scriptPath,
		Language:     "node",
	}

	// This should NOT return an error because:
	// 1. Test command exits 0
	// 2. No individual test names → file-level fallback
	// 3. Both per-test matching AND assertion check are skipped
	err := VerifyTestEvidence(context.Background(), opts)
	if err != nil {
		t.Errorf("VerifyTestEvidence should pass in file-level mode, got error: code=%d msg=%s details=%v",
			err.Code, err.Message, err.Details)
	}
}

// TestParseTestResults_VitestDefaultBaseName verifies that Vitest default mode
// produces file-level entries and baseName entries (not individual test names).
func TestParseTestResults_VitestDefaultBaseName(t *testing.T) {
	output := ` ✓ src/components/GameCanvas.test.ts (4 tests) 306ms
 ✓ src/components/Lobby.test.ts (2 tests) 150ms

 Test Files  2 passed (2)
      Tests  6 passed (6)
   Duration  1.26s`

	passed, failed := ParseTestResults(output)

	// Should have file-level entries
	if !passed["src/components/GameCanvas.test.ts"] {
		t.Error("expected file-level entry for GameCanvas.test.ts")
	}
	if !passed["src/components/Lobby.test.ts"] {
		t.Error("expected file-level entry for Lobby.test.ts")
	}

	// Should NOT have individual test names like "TestRenderDrawsSnake..."
	if passed["TestRenderDrawsSnakeSegmentsAtExpectedGridPositions"] {
		t.Error("should not have Go-style individual test name in Vitest default output")
	}

	if len(failed) != 0 {
		t.Errorf("expected no failed tests, got %d", len(failed))
	}
}

func TestHasIndividualTestNames(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "Go test output",
			output: `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
PASS
ok  	example.com/pkg	0.001s`,
			want: true,
		},
		{
			name: "Vitest verbose output",
			output: ` ✓ __tests__/math.test.ts > addition > 2 + 2 should equal 4 1ms
 ✓ __tests__/math.test.ts > addition > 3 + 3 should equal 6 2ms`,
			want: true,
		},
		{
			name: "Vitest default (file-level only)",
			output: ` ✓ src/components/GameCanvas.test.ts (4 tests) 306ms
 ✓ src/components/Lobby.test.ts (2 tests) 150ms

 Test Files  2 passed (2)
      Tests  6 passed (6)
   Duration  1.26s`,
			want: false,
		},
		{
			name: "Jest individual test output",
			output: `  ✓ should handle empty input (5ms)
  ✓ should process valid data (10ms)`,
			want: true,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name: "Vitest summary only (no checkmarks)",
			output: ` Test Files  1 passed (1)
      Tests  4 passed (4)
   Duration  0.89s`,
			want: false,
		},
		{
			name: "Go test failure",
			output: `=== RUN   TestFoo
--- FAIL: TestFoo (0.01s)
FAIL`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasIndividualTestNames(tt.output)
			if got != tt.want {
				t.Errorf("hasIndividualTestNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTestReviewTable_MultiLanguage(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		language  string
		wantCount int
		wantErr   bool
	}{
		{
			name: "Go format valid",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature works | TestFeature | assert.True |
`,
			language:  "go",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "Go format invalid - has spaces",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature works | renders lobby shell | assert.True |
`,
			language:  "go",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "Node format valid - natural language",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature works | renders lobby shell with controls | expect(screen).toBeDefined |
`,
			language:  "node",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "Node format invalid - N/A",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature works | N/A | none |
`,
			language:  "node",
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "Python format valid",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature works | test_feature | assert result == expected |
`,
			language:  "python",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "Unknown language - permissive",
			body: `### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| Feature works | any_test_name_123 | assert something |
`,
			language:  "rust",
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTestReviewTable(tt.body, tt.language)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if len(got) != tt.wantCount {
					t.Errorf("parseTestReviewTable() returned %d mappings, want %d", len(got), tt.wantCount)
				}
			}
		})
	}
}

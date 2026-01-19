package reviewer

import (
	"math"
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

	mappings, err := parseTestReviewTable(reviewBody)
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

	mappings, err := parseTestReviewTable(reviewBody)
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
			_, err := parseTestReviewTable(tt.reviewBody)
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

package reviewer

import (
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

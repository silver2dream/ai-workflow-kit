package reviewer

import (
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

	mappings := parseTestReviewTable(reviewBody)

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

	mappings := parseTestReviewTable(reviewBody)

	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}

	if mappings[0].TestName != "TestMovement" {
		t.Errorf("expected TestName 'TestMovement', got %q", mappings[0].TestName)
	}
}

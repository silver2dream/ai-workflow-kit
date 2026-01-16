package reviewer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CriteriaVerification represents a single criteria verification entry
type CriteriaVerification struct {
	Criteria       string // The acceptance criteria text
	Implementation string // Description of how it's implemented
	TestName       string // Name of the test function
	Assertion      string // Key assertion from test code
}

// EvidenceError represents an evidence verification error
type EvidenceError struct {
	Code    int    // 0=OK, 1=missing, 2=test failed, 3=assertion not found
	Message string
	Details []string
}

func (e *EvidenceError) Error() string {
	return e.Message
}

// VerifyOptions contains options for verification
type VerifyOptions struct {
	Ticket       string        // Issue body with acceptance criteria
	ReviewBody   string        // Reviewer's review body
	WorktreePath string        // Path to worktree
	TestCommand  string        // Command to run tests
	TestTimeout  time.Duration // Timeout for test execution
}

// VerifyTestEvidence performs the complete verification
func VerifyTestEvidence(ctx context.Context, opts VerifyOptions) *EvidenceError {
	if opts.TestTimeout == 0 {
		opts.TestTimeout = 5 * time.Minute
	}

	// 1. Parse acceptance criteria from ticket
	criteria := ParseAcceptanceCriteria(opts.Ticket)
	if len(criteria) == 0 {
		return &EvidenceError{
			Code:    1,
			Message: "no acceptance criteria found in ticket",
		}
	}

	fmt.Printf("[VERIFY] Found %d acceptance criteria in ticket\n", len(criteria))

	// 2. Parse criteria verifications from review body
	verifications, err := ParseCriteriaVerifications(opts.ReviewBody)
	if err != nil {
		return &EvidenceError{
			Code:    1,
			Message: fmt.Sprintf("failed to parse review body: %v", err),
		}
	}

	if len(verifications) == 0 {
		return &EvidenceError{
			Code:    1,
			Message: "no criteria verifications found in review body",
		}
	}

	fmt.Printf("[VERIFY] Found %d verifications in review body\n", len(verifications))

	// 3. Validate completeness - each criteria has verification
	if err := ValidateCompleteness(criteria, verifications); err != nil {
		return err
	}

	fmt.Printf("[VERIFY] ✓ All criteria have complete verifications\n")

	// 4. Execute tests in worktree
	fmt.Printf("[VERIFY] Executing tests: %s\n", opts.TestCommand)
	testOutput, testErr := ExecuteTests(ctx, opts.WorktreePath, opts.TestCommand, opts.TestTimeout)

	// 5. Parse test results
	passedTests, failedTests := ParseTestResults(testOutput)
	fmt.Printf("[VERIFY] Tests: %d passed, %d failed\n", len(passedTests), len(failedTests))

	// Log parsed test names for debugging
	if len(passedTests) > 0 {
		var passedNames []string
		for name := range passedTests {
			passedNames = append(passedNames, name)
		}
		fmt.Printf("[VERIFY] Passed tests: %v\n", passedNames)
	}

	if testErr != nil && len(passedTests) == 0 {
		// Include first 500 chars of test output for debugging
		truncatedOutput := testOutput
		if len(truncatedOutput) > 500 {
			truncatedOutput = truncatedOutput[:500] + "... (truncated)"
		}
		return &EvidenceError{
			Code:    2,
			Message: fmt.Sprintf("test execution failed: %v", testErr),
			Details: []string{truncatedOutput},
		}
	}

	// 6. Verify each mapped test passed
	var missingTests []string
	var expectedTests []string
	for _, v := range verifications {
		expectedTests = append(expectedTests, v.TestName)
		if !passedTests[v.TestName] {
			if failedTests[v.TestName] {
				missingTests = append(missingTests, fmt.Sprintf("%s (FAILED)", v.TestName))
			} else {
				// Provide more specific diagnostic for why test was not found
				if strings.Contains(v.TestName, " ") {
					missingTests = append(missingTests, fmt.Sprintf("%s (invalid format: contains spaces, must be function name like TestXxx)", v.TestName))
				} else if !strings.HasPrefix(v.TestName, "Test") {
					missingTests = append(missingTests, fmt.Sprintf("%s (invalid format: must start with 'Test')", v.TestName))
				} else {
					missingTests = append(missingTests, fmt.Sprintf("%s (not found in test output)", v.TestName))
				}
			}
		}
	}

	if len(missingTests) > 0 {
		// Add diagnostic info about what was expected vs found
		details := missingTests
		details = append(details, fmt.Sprintf("Expected tests: %v", expectedTests))
		details = append(details, fmt.Sprintf("Found %d passed tests in output", len(passedTests)))
		return &EvidenceError{
			Code:    2,
			Message: "some tests did not pass",
			Details: details,
		}
	}

	fmt.Printf("[VERIFY] ✓ All mapped tests passed\n")

	// 7. Verify assertions exist in test files
	if err := VerifyAssertions(opts.WorktreePath, verifications); err != nil {
		return err
	}

	fmt.Printf("[VERIFY] ✓ All assertions verified in test files\n")

	return nil
}

// ParseAcceptanceCriteria extracts acceptance criteria from ticket
func ParseAcceptanceCriteria(ticket string) []string {
	// Match lines like "- [ ] criteria" or "- [x] criteria"
	re := regexp.MustCompile(`(?m)^[-*]\s*\[[x ]\]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(ticket, -1)

	var criteria []string
	for _, m := range matches {
		c := strings.TrimSpace(m[1])
		if c != "" {
			criteria = append(criteria, c)
		}
	}
	return criteria
}

// ParseCriteriaVerifications parses the review body to extract verifications
func ParseCriteriaVerifications(reviewBody string) ([]CriteriaVerification, error) {
	var verifications []CriteriaVerification

	// Parse Implementation Review section
	implementations := parseImplementationReview(reviewBody)

	// Parse Test Review table
	testMappings, err := parseTestReviewTable(reviewBody)
	if err != nil {
		return nil, err
	}

	// Merge implementations and test mappings
	for criteria, impl := range implementations {
		v := CriteriaVerification{
			Criteria:       criteria,
			Implementation: impl,
		}

		// Find matching test mapping
		for _, tm := range testMappings {
			if fuzzyMatch(criteria, tm.Criteria) {
				v.TestName = tm.TestName
				v.Assertion = tm.Assertion
				break
			}
		}

		verifications = append(verifications, v)
	}

	// Add any test mappings not matched to implementations
	for _, tm := range testMappings {
		found := false
		for _, v := range verifications {
			if fuzzyMatch(tm.Criteria, v.Criteria) {
				found = true
				break
			}
		}
		if !found {
			verifications = append(verifications, CriteriaVerification{
				Criteria:  tm.Criteria,
				TestName:  tm.TestName,
				Assertion: tm.Assertion,
			})
		}
	}

	return verifications, nil
}

type testMapping struct {
	Criteria  string
	TestName  string
	Assertion string
}

// isValidTestName checks if a test name is a valid Go test function name
func isValidTestName(name string) bool {
	// Go test function names: start with Test, contain only alphanumeric and underscore
	// Optionally support subtest format TestName/SubTest
	matched, _ := regexp.MatchString(`^Test[A-Za-z0-9_]*(/[A-Za-z0-9_]+)?$`, name)
	return matched
}

func parseImplementationReview(body string) map[string]string {
	implementations := make(map[string]string)

	// Find Implementation Review section
	implSection := extractSection(body, "Implementation Review")
	if implSection == "" {
		return implementations
	}

	// Parse each criteria block: "#### N. Criteria"
	re := regexp.MustCompile(`(?m)^#{3,4}\s*\d+\.\s*(.+)$`)
	matches := re.FindAllStringSubmatchIndex(implSection, -1)

	for i, match := range matches {
		criteriaStart := match[2]
		criteriaEnd := match[3]
		criteria := strings.TrimSpace(implSection[criteriaStart:criteriaEnd])

		// Find content until next header or end
		contentStart := match[1]
		var contentEnd int
		if i+1 < len(matches) {
			contentEnd = matches[i+1][0]
		} else {
			contentEnd = len(implSection)
		}

		content := implSection[contentStart:contentEnd]

		// Extract implementation logic
		impl := extractImplementationLogic(content)
		if impl != "" {
			implementations[criteria] = impl
		}
	}

	return implementations
}

func extractImplementationLogic(content string) string {
	// Look for "**實作邏輯**:" or "**Implementation**:" pattern
	patterns := []string{
		`(?s)\*\*實作邏輯\*\*[：:]\s*(.+?)(?:\n\n|\n\*\*|$)`,
		`(?s)\*\*Implementation\*\*[：:]\s*(.+?)(?:\n\n|\n\*\*|$)`,
		`(?s)實作邏輯[：:]\s*(.+?)(?:\n\n|\n\*\*|$)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if m := re.FindStringSubmatch(content); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}

	return ""
}

func parseTestReviewTable(body string) ([]testMapping, error) {
	var mappings []testMapping

	// Find Test Review section
	testSection := extractSection(body, "Test Review")
	if testSection == "" {
		return mappings, nil
	}

	// Parse markdown table rows
	// | Criteria | Test | Key Assertion |
	lines := strings.Split(testSection, "\n")
	inTable := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if inTable {
				break // End of table
			}
			continue
		}

		// Skip header separator
		if strings.Contains(line, "---") {
			inTable = true
			continue
		}

		// Parse table row
		cells := strings.Split(line, "|")
		if len(cells) >= 4 { // | cell1 | cell2 | cell3 |
			criteria := strings.TrimSpace(cells[1])
			testName := strings.Trim(strings.TrimSpace(cells[2]), "`") // Strip backticks from test name
			assertion := strings.TrimSpace(cells[3])

			// Skip header row
			if strings.EqualFold(criteria, "Criteria") {
				continue
			}

			if criteria != "" && testName != "" {
				// Validate test name format
				if !isValidTestName(testName) {
					return nil, fmt.Errorf("invalid test name in review table: %q (must be Go test function like TestXxx)", testName)
				}

				mappings = append(mappings, testMapping{
					Criteria:  criteria,
					TestName:  testName,
					Assertion: assertion,
				})
			}
		}
	}

	return mappings, nil
}

func extractSection(body, sectionName string) string {
	// Find section by header
	// Use \n###\s or \n### followed by space to avoid matching #### (4 hashes)
	patterns := []string{
		fmt.Sprintf(`(?s)###\s*%s\s*\n(.+?)(?:\n###\s|\n---|\z)`, regexp.QuoteMeta(sectionName)),
		fmt.Sprintf(`(?s)##\s*%s\s*\n(.+?)(?:\n##\s|\n---|\z)`, regexp.QuoteMeta(sectionName)),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if m := re.FindStringSubmatch(body); len(m) > 1 {
			return m[1]
		}
	}

	return ""
}

// ValidateCompleteness ensures each criteria has complete verification
func ValidateCompleteness(criteria []string, verifications []CriteriaVerification) *EvidenceError {
	var missing []string

	for _, c := range criteria {
		found := false
		for _, v := range verifications {
			if fuzzyMatch(c, v.Criteria) {
				// Check completeness
				var issues []string
				if v.Implementation == "" {
					issues = append(issues, "missing implementation description")
				} else if len(v.Implementation) < 20 {
					issues = append(issues, "implementation description too short (min 20 chars)")
				}
				if v.TestName == "" {
					issues = append(issues, "missing test name")
				}
				if v.Assertion == "" {
					issues = append(issues, "missing assertion")
				}

				if len(issues) > 0 {
					missing = append(missing, fmt.Sprintf("%s: %s", c, strings.Join(issues, ", ")))
				}

				found = true
				break
			}
		}
		if !found {
			missing = append(missing, fmt.Sprintf("%s: no verification found", c))
		}
	}

	if len(missing) > 0 {
		return &EvidenceError{
			Code:    1,
			Message: "incomplete criteria verification",
			Details: missing,
		}
	}

	return nil
}

// ExecuteTests runs the test command in the worktree
func ExecuteTests(ctx context.Context, worktreePath, testCommand string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", testCommand)
	cmd.Dir = worktreePath

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// ParseTestResults extracts passed and failed tests from output
func ParseTestResults(output string) (passed map[string]bool, failed map[string]bool) {
	passed = make(map[string]bool)
	failed = make(map[string]bool)

	// Go test format: "--- PASS: TestName" or "--- PASS: TestName/SubTest"
	// Use [\w/]+ to match subtests with slashes
	goPassRe := regexp.MustCompile(`--- PASS: ([\w/]+)`)
	goFailRe := regexp.MustCompile(`--- FAIL: ([\w/]+)`)

	for _, m := range goPassRe.FindAllStringSubmatch(output, -1) {
		testName := m[1]
		passed[testName] = true
		// Also mark parent test as passed if this is a subtest
		if idx := strings.Index(testName, "/"); idx != -1 {
			passed[testName[:idx]] = true
		}
	}
	for _, m := range goFailRe.FindAllStringSubmatch(output, -1) {
		testName := m[1]
		failed[testName] = true
		// Also mark parent test as failed if this is a subtest
		if idx := strings.Index(testName, "/"); idx != -1 {
			failed[testName[:idx]] = true
		}
	}

	// Node/Jest format: "✓ test name" or "✕ test name"
	nodePassRe := regexp.MustCompile(`✓\s+(.+)`)
	nodeFailRe := regexp.MustCompile(`✕\s+(.+)`)

	for _, m := range nodePassRe.FindAllStringSubmatch(output, -1) {
		// Convert to function-like name
		name := strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "_")
		passed[name] = true
	}
	for _, m := range nodeFailRe.FindAllStringSubmatch(output, -1) {
		name := strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "_")
		failed[name] = true
	}

	return passed, failed
}

// VerifyAssertions checks that assertions exist in test files
func VerifyAssertions(worktreePath string, verifications []CriteriaVerification) *EvidenceError {
	var missing []string

	// Find all test files
	testFiles, err := findTestFiles(worktreePath)
	if err != nil {
		return &EvidenceError{
			Code:    3,
			Message: fmt.Sprintf("failed to find test files: %v", err),
		}
	}

	// Read all test file contents
	var allTestContent strings.Builder
	for _, tf := range testFiles {
		content, err := os.ReadFile(tf)
		if err != nil {
			continue
		}
		allTestContent.WriteString(string(content))
		allTestContent.WriteString("\n")
	}
	testContent := allTestContent.String()

	// Check each assertion
	for _, v := range verifications {
		if v.Assertion == "" {
			continue
		}

		// Normalize and check
		normalizedAssertion := normalizeForMatching(v.Assertion)
		normalizedContent := normalizeForMatching(testContent)

		if !strings.Contains(normalizedContent, normalizedAssertion) {
			missing = append(missing, fmt.Sprintf("%s: assertion not found", v.TestName))
		}
	}

	if len(missing) > 0 {
		return &EvidenceError{
			Code:    3,
			Message: "assertions not found in test files",
			Details: missing,
		}
	}

	return nil
}

func findTestFiles(root string) ([]string, error) {
	var testFiles []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip common non-test directories
			name := info.Name()
			if name == "vendor" || name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Match test files
		name := info.Name()
		if strings.HasSuffix(name, "_test.go") ||
			strings.HasSuffix(name, ".test.js") ||
			strings.HasSuffix(name, ".test.ts") ||
			strings.HasSuffix(name, ".spec.js") ||
			strings.HasSuffix(name, ".spec.ts") {
			testFiles = append(testFiles, path)
		}

		return nil
	})

	return testFiles, err
}

// normalizeForMatching prepares a string for fuzzy matching
func normalizeForMatching(s string) string {
	s = strings.TrimSpace(s)
	// Remove backticks
	s = strings.ReplaceAll(s, "`", "")
	// Normalize whitespace
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", "")
	// Collapse multiple spaces
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	return s
}

// fuzzyMatch checks if two criteria strings match loosely
func fuzzyMatch(a, b string) bool {
	a = normalizeForMatching(strings.ToLower(a))
	b = normalizeForMatching(strings.ToLower(b))

	// Exact match
	if a == b {
		return true
	}

	// Contains match
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}

	// Remove common words and compare
	stopWords := []string{"的", "後", "要", "會", "應該", "可以", "需要"}
	for _, w := range stopWords {
		a = strings.ReplaceAll(a, w, "")
		b = strings.ReplaceAll(b, w, "")
	}

	return strings.Contains(a, b) || strings.Contains(b, a)
}

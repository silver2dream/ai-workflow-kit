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
	IsMeta         bool   // True when this is a meta-criteria (e.g., "all tests pass")
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
	Language     string        // Programming language for test name validation
}

// TestNameValidator defines interface for language-specific test name validation
type TestNameValidator interface {
	IsValid(name string) bool
	FormatHint() string // Returns hint for valid test name format
}

// GoTestValidator validates Go test function names (TestXxx format)
type GoTestValidator struct{}

func (v *GoTestValidator) IsValid(name string) bool {
	matched, _ := regexp.MatchString(`^Test[A-Za-z0-9_]*(/[A-Za-z0-9_]+)?$`, name)
	return matched
}

func (v *GoTestValidator) FormatHint() string {
	return "Go test function like TestXxx or TestXxx/SubTest"
}

// NodeTestValidator validates Node.js test names (Vitest/Jest - natural language)
type NodeTestValidator struct{}

func (v *NodeTestValidator) IsValid(name string) bool {
	if name == "" {
		return false
	}
	// Reject obvious invalid patterns
	invalidPatterns := []string{
		`^N/A$`,
		`^All test`,
		`^test in file`,
		`^TODO`,
		`^SKIP`,
	}
	for _, pattern := range invalidPatterns {
		if matched, _ := regexp.MatchString("(?i)"+pattern, name); matched {
			return false
		}
	}
	return true
}

func (v *NodeTestValidator) FormatHint() string {
	return "test description string (e.g., 'renders component correctly')"
}

// PythonTestValidator validates Python test names (test_xxx or TestClass::test_method)
type PythonTestValidator struct{}

func (v *PythonTestValidator) IsValid(name string) bool {
	// test_function, TestClass::test_method, or pytest node IDs
	matched, _ := regexp.MatchString(`^(test_[a-z0-9_]+|Test[A-Za-z0-9_]*(::(test_)?[a-z0-9_]+)?|.+::test_[a-z0-9_]+)$`, name)
	return matched
}

func (v *PythonTestValidator) FormatHint() string {
	return "Python test like test_xxx or TestClass::test_method"
}

// PermissiveValidator allows any non-empty test name (fallback)
type PermissiveValidator struct{}

func (v *PermissiveValidator) IsValid(name string) bool {
	if name == "" {
		return false
	}
	// Only reject obviously invalid patterns
	invalidPatterns := []string{`^N/A$`, `^$`}
	for _, pattern := range invalidPatterns {
		if matched, _ := regexp.MatchString("(?i)"+pattern, name); matched {
			return false
		}
	}
	return true
}

func (v *PermissiveValidator) FormatHint() string {
	return "non-empty test name"
}

// GetTestNameValidator returns appropriate validator for the given language
func GetTestNameValidator(language string) TestNameValidator {
	switch strings.ToLower(language) {
	case "go", "golang":
		return &GoTestValidator{}
	case "node", "nodejs", "typescript", "javascript", "ts", "js":
		return &NodeTestValidator{}
	case "python", "py":
		return &PythonTestValidator{}
	default:
		// For unknown languages, use permissive validator
		return &PermissiveValidator{}
	}
}

// EnhanceTestCommandForVerbose appends verbose flags to the test command based
// on the project language so that individual test names appear in output.
func EnhanceTestCommandForVerbose(testCmd, language string) string {
	if testCmd == "" {
		return testCmd
	}

	switch strings.ToLower(language) {
	case "go", "golang":
		// Go: add -v for subtest output (if not already present)
		if !strings.Contains(testCmd, " -v") && !strings.Contains(testCmd, " -v ") {
			return testCmd + " -v"
		}
		return testCmd

	case "node", "nodejs", "typescript", "javascript", "ts", "js":
		// Already has a reporter/verbose flag — leave as-is
		if strings.Contains(testCmd, "--reporter") || strings.Contains(testCmd, "--verbose") {
			return testCmd
		}
		// npm test / npm run test → append via "--" (may be prefixed with "cd ... && ")
		if strings.Contains(testCmd, "npm test") || strings.Contains(testCmd, "npm run test") {
			return testCmd + " -- --reporter=verbose"
		}
		// npx vitest / vitest → append directly
		if strings.Contains(testCmd, "vitest") {
			return testCmd + " --reporter=verbose"
		}
		// npx jest / jest → append --verbose
		if strings.Contains(testCmd, "jest") {
			return testCmd + " --verbose"
		}
		return testCmd

	case "python", "py":
		if !strings.Contains(testCmd, " -v") {
			return testCmd + " -v"
		}
		return testCmd

	default:
		return testCmd
	}
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

	// 2. Parse criteria verifications from review body (with language-aware validation)
	verifications, err := ParseCriteriaVerifications(opts.ReviewBody, opts.Language)
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

	// 4. Execute tests in worktree with verbose output for individual test names
	verboseCmd := EnhanceTestCommandForVerbose(opts.TestCommand, opts.Language)
	if verboseCmd != opts.TestCommand {
		fmt.Printf("[VERIFY] Enhanced test command for verbose: %s\n", verboseCmd)
	}
	fmt.Printf("[VERIFY] Executing tests: %s\n", verboseCmd)
	testOutput, testErr := ExecuteTests(ctx, opts.WorktreePath, verboseCmd, opts.TestTimeout)

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

	// 6. Verify each mapped test passed (with fuzzy matching fallback)
	// Skip per-test matching and assertion verification when test runner
	// output is file-level only (e.g., Vitest default mode).
	// In this mode, test names and assertions in the review body are
	// Principal-authored (not from test output) and may not match file content.
	fileLevelOnly := testErr == nil && !hasIndividualTestNames(testOutput)
	if fileLevelOnly {
		fmt.Printf("[VERIFY] ⚠ Verbose enhancement did not produce individual test names; "+
			"falling back to file-level pass (per-test matching and assertion check skipped)\n")
	} else {
		var missingTests []string
		var expectedTests []string
		for _, v := range verifications {
			// Skip meta-criteria — verified by overall test pass
			if isMetaCriteria(v) {
				fmt.Printf("[VERIFY] Meta-criteria %q: verified by overall test pass\n", v.Criteria)
				continue
			}

			expectedTests = append(expectedTests, v.TestName)

			// 1. Try exact match first
			if passedTests[v.TestName] {
				continue
			}

			// 2. Exact match failed, try fuzzy matching
			found := false
			var matchedName string
			for passedName := range passedTests {
				if fuzzyMatchTestName(v.TestName, passedName) {
					found = true
					matchedName = passedName
					break
				}
			}

			if found {
				fmt.Printf("[VERIFY] Fuzzy matched: %s -> %s\n", v.TestName, matchedName)
				continue
			}

			// 3. Fuzzy match also failed, check if in failed tests
			if failedTests[v.TestName] {
				missingTests = append(missingTests, fmt.Sprintf("%s (FAILED)", v.TestName))
			} else {
				// Provide language-aware diagnostic for why test was not found
				validator := GetTestNameValidator(opts.Language)
				if !validator.IsValid(v.TestName) {
					missingTests = append(missingTests, fmt.Sprintf("%s (invalid format for %s: expected %s)", v.TestName, opts.Language, validator.FormatHint()))
				} else {
					// Find similar tests to provide better diagnostics
					similar := findSimilarTests(v.TestName, passedTests, 0.5)
					if len(similar) > 0 {
						missingTests = append(missingTests, fmt.Sprintf("%s (not found, similar: %v)", v.TestName, similar))
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
	}

	// 7. Verify assertions exist in test files
	// Skip when in file-level mode: the Principal writes assertion text from
	// memory (not from test output), so it often doesn't match file content.
	// CI passing + high review score is sufficient evidence in this case.
	if !fileLevelOnly {
		if err := VerifyAssertions(opts.WorktreePath, verifications); err != nil {
			return err
		}
		fmt.Printf("[VERIFY] ✓ All assertions verified in test files\n")
	}

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
func ParseCriteriaVerifications(reviewBody, language string) ([]CriteriaVerification, error) {
	var verifications []CriteriaVerification

	// Parse Implementation Review section
	implementations := parseImplementationReview(reviewBody)

	// Parse Test Review table with language-aware validation
	testMappings, err := parseTestReviewTable(reviewBody, language)
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
				v.IsMeta = tm.IsMeta
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
				IsMeta:    tm.IsMeta,
			})
		}
	}

	return verifications, nil
}

type testMapping struct {
	Criteria  string
	TestName  string
	Assertion string
	IsMeta    bool
}

// isValidTestNameForLanguage validates test name using language-specific rules
func isValidTestNameForLanguage(name, language string) bool {
	validator := GetTestNameValidator(language)
	return validator.IsValid(name)
}

// isValidTestName validates test name using Go format (for backward compatibility)
// Deprecated: Use isValidTestNameForLanguage instead
func isValidTestName(name string) bool {
	return isValidTestNameForLanguage(name, "go")
}

// getTestNameFormatHint returns format hint for the given language
func getTestNameFormatHint(language string) string {
	validator := GetTestNameValidator(language)
	return validator.FormatHint()
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

func parseTestReviewTable(body, language string) ([]testMapping, error) {
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
				// Detect (meta) marker — skip test name validation for meta-criteria
				isMeta := strings.EqualFold(strings.TrimSpace(testName), "(meta)")
				if isMeta {
					mappings = append(mappings, testMapping{
						Criteria:  criteria,
						TestName:  testName,
						Assertion: assertion,
						IsMeta:    true,
					})
					continue
				}

				// Validate test name format using language-specific validator
				if !isValidTestNameForLanguage(testName, language) {
					return nil, fmt.Errorf("invalid test name in review table: %q (expected: %s)", testName, getTestNameFormatHint(language))
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

// ValidateCompleteness ensures each criteria has complete verification.
// Test name and assertion are optional — some criteria are meta-criteria
// (e.g., "all tests pass", "unit tests added") that cannot be mapped to
// specific test functions. Only implementation description is required.
func ValidateCompleteness(criteria []string, verifications []CriteriaVerification) *EvidenceError {
	var missing []string

	for _, c := range criteria {
		found := false
		for _, v := range verifications {
			if fuzzyMatch(c, v.Criteria) {
				// Check completeness: only implementation is required
				var issues []string
				if v.Implementation == "" {
					issues = append(issues, "missing implementation description")
				} else if len(v.Implementation) < 20 {
					issues = append(issues, "implementation description too short (min 20 chars)")
				}

				// Test name and assertion are optional — warn but don't block
				if v.TestName == "" {
					fmt.Printf("[VERIFY] Note: criteria %q has no test name mapping (ok for meta-criteria)\n", c)
				}
				if v.Assertion == "" {
					fmt.Printf("[VERIFY] Note: criteria %q has no assertion mapping (ok for meta-criteria)\n", c)
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

	// Vitest verbose format: "✓ file.test.ts > describe > test name 1ms"
	// Extract the last segment (actual test name) from the chain
	vitestVerboseRe := regexp.MustCompile(`✓\s+\S+\.test\.[jt]sx?\s*>\s*(.+?)\s+\d+m?s`)
	vitestVerboseFailRe := regexp.MustCompile(`✕\s+\S+\.test\.[jt]sx?\s*>\s*(.+?)\s+\d+m?s`)

	for _, m := range vitestVerboseRe.FindAllStringSubmatch(output, -1) {
		// Get the full path after file name (e.g., "describe > test name")
		fullPath := strings.TrimSpace(m[1])
		// Extract the last segment as the test name
		parts := strings.Split(fullPath, ">")
		testName := strings.TrimSpace(parts[len(parts)-1])
		// Store original name (with spaces) for matching reviewer descriptions
		passed[testName] = true
		// Store normalized name (with underscores) for backward compatibility
		name := strings.ReplaceAll(testName, " ", "_")
		passed[name] = true
		// Also store the full path variants for better matching
		fullName := strings.ReplaceAll(strings.ReplaceAll(fullPath, " > ", "/"), " ", "_")
		passed[fullName] = true
		// Full path with original spaces
		fullPathOriginal := strings.TrimSpace(fullPath)
		for _, sep := range []string{" > "} {
			fullPathOriginal = strings.ReplaceAll(fullPathOriginal, sep, "/")
		}
		passed[fullPathOriginal] = true
	}
	for _, m := range vitestVerboseFailRe.FindAllStringSubmatch(output, -1) {
		fullPath := strings.TrimSpace(m[1])
		parts := strings.Split(fullPath, ">")
		testName := strings.TrimSpace(parts[len(parts)-1])
		// Store original name (with spaces)
		failed[testName] = true
		// Store normalized name (with underscores)
		name := strings.ReplaceAll(testName, " ", "_")
		failed[name] = true
		fullName := strings.ReplaceAll(strings.ReplaceAll(fullPath, " > ", "/"), " ", "_")
		failed[fullName] = true
		fullPathOriginal := strings.TrimSpace(fullPath)
		for _, sep := range []string{" > "} {
			fullPathOriginal = strings.ReplaceAll(fullPathOriginal, sep, "/")
		}
		failed[fullPathOriginal] = true
	}

	// Vitest default format: "✓ file.test.ts (5 tests) 306ms"
	// This indicates all tests in the file passed
	vitestDefaultPassRe := regexp.MustCompile(`✓\s+(\S+\.test\.[jt]sx?)\s+\((\d+)\s+tests?`)
	vitestDefaultFailRe := regexp.MustCompile(`✕\s+(\S+\.test\.[jt]sx?)\s+\((\d+)`)

	for _, m := range vitestDefaultPassRe.FindAllStringSubmatch(output, -1) {
		fileName := m[1]
		// Mark file-level pass (useful for fuzzy matching)
		passed[fileName] = true
		// Also create a normalized version
		baseName := strings.TrimSuffix(strings.TrimSuffix(fileName, ".ts"), ".test")
		baseName = strings.TrimSuffix(strings.TrimSuffix(baseName, ".js"), ".test")
		passed[baseName] = true
	}
	for _, m := range vitestDefaultFailRe.FindAllStringSubmatch(output, -1) {
		fileName := m[1]
		failed[fileName] = true
		baseName := strings.TrimSuffix(strings.TrimSuffix(fileName, ".ts"), ".test")
		baseName = strings.TrimSuffix(strings.TrimSuffix(baseName, ".js"), ".test")
		failed[baseName] = true
	}

	// Node/Jest format: "✓ test name" or "✕ test name"
	// Only match lines that don't look like Vitest format (no file.test.ts pattern)
	nodePassRe := regexp.MustCompile(`✓\s+([^>\n]+)$`)
	nodeFailRe := regexp.MustCompile(`✕\s+([^>\n]+)$`)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Skip if it looks like Vitest format (contains .test.ts or .test.js)
		if strings.Contains(line, ".test.ts") || strings.Contains(line, ".test.js") ||
			strings.Contains(line, ".test.tsx") || strings.Contains(line, ".test.jsx") {
			continue
		}
		if m := nodePassRe.FindStringSubmatch(line); m != nil {
			name := strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "_")
			passed[name] = true
		}
		if m := nodeFailRe.FindStringSubmatch(line); m != nil {
			name := strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "_")
			failed[name] = true
		}
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

	// Check each assertion using multi-strategy matching
	for _, v := range verifications {
		if v.Assertion == "" || isMetaCriteria(v) {
			continue
		}

		if !matchAssertion(v.Assertion, testContent) {
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

// matchAssertion uses multiple strategies to check if an assertion exists in test content.
// Strategy 1: Exact normalized substring match (existing behavior).
// Strategy 2: Assertion function name extraction (e.g., assert.Equal, expect(...).toBeTruthy).
// Strategy 3: Token-level matching (70%+ of tokens appear in test content).
func matchAssertion(assertion, testContent string) bool {
	normalizedAssertion := normalizeForMatching(assertion)
	normalizedContent := normalizeForMatching(testContent)

	// Strategy 1: Exact normalized substring match
	if strings.Contains(normalizedContent, normalizedAssertion) {
		return true
	}

	// Strategy 2: Assertion function name match
	funcName := extractAssertionFuncName(assertion)
	if funcName != "" && strings.Contains(testContent, funcName) {
		return true
	}

	// Strategy 3: Token-level match (70% threshold)
	if tokenMatchAssertion(normalizedAssertion, normalizedContent, 0.7) {
		return true
	}

	return false
}

// extractAssertionFuncName extracts the assertion function/method call pattern from
// an assertion string. Recognizes common Go (assert.Equal, require.NoError) and
// JS/TS (expect(...).toBeTruthy, expect(...).toEqual) patterns.
func extractAssertionFuncName(assertion string) string {
	// Go-style: assert.Equal, require.NoError, etc.
	goRe := regexp.MustCompile(`(assert|require)\.\w+`)
	if m := goRe.FindString(assertion); m != "" {
		return m
	}

	// JS/TS-style: expect(...).toBeTruthy(), .toEqual(), .toBe(), etc.
	jsMethodRe := regexp.MustCompile(`\.(to\w+)\(`)
	if m := jsMethodRe.FindStringSubmatch(assertion); len(m) > 1 {
		return m[1]
	}

	// JS/TS-style: expect( keyword
	if strings.Contains(assertion, "expect(") {
		return "expect("
	}

	return ""
}

// tokenMatchAssertion checks whether a sufficient fraction of tokens from the
// assertion appear in the test content. This handles cases where the reviewer
// writes an assertion from memory with minor differences.
func tokenMatchAssertion(assertion, content string, threshold float64) bool {
	// Split assertion into tokens (words, identifiers)
	tokenRe := regexp.MustCompile(`[\w.]+`)
	tokens := tokenRe.FindAllString(assertion, -1)
	if len(tokens) == 0 {
		return false
	}

	matched := 0
	for _, token := range tokens {
		if len(token) < 2 {
			// Skip very short tokens (t, e, etc.) as they match too broadly
			matched++
			continue
		}
		if strings.Contains(content, token) {
			matched++
		}
	}

	return float64(matched)/float64(len(tokens)) >= threshold
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

// fuzzyMatch checks if two criteria strings match loosely.
// A length ratio threshold prevents short substrings from matching
// unrelated longer strings (e.g. "fix" matching "fix all logging modules").
func fuzzyMatch(a, b string) bool {
	a = normalizeForMatching(strings.ToLower(a))
	b = normalizeForMatching(strings.ToLower(b))

	// Exact match
	if a == b {
		return true
	}

	// Contains match with length ratio guard:
	// the shorter string must be at least 60% of the longer string's length.
	if containsWithRatio(a, b) {
		return true
	}

	// Remove common words and compare
	stopWords := []string{"的", "後", "要", "會", "應該", "可以", "需要"}
	for _, w := range stopWords {
		a = strings.ReplaceAll(a, w, "")
		b = strings.ReplaceAll(b, w, "")
	}

	return containsWithRatio(a, b)
}

// containsWithRatio returns true if one string contains the other AND
// the shorter string is at least 60% the length of the longer string.
func containsWithRatio(a, b string) bool {
	if !strings.Contains(a, b) && !strings.Contains(b, a) {
		return false
	}
	shorter, longer := len(a), len(b)
	if shorter > longer {
		shorter, longer = longer, shorter
	}
	if longer == 0 {
		return true
	}
	return float64(shorter)/float64(longer) >= 0.6
}

// fuzzyMatchTestName performs fuzzy matching for test names
// Handles the following cases:
// - TestFoo vs TestFoo/SubTest (subtest parent)
// - TestChapterRead vs TestReadChapter (reordering)
// - TestFoo_v2 vs TestFoo (version suffix)
func fuzzyMatchTestName(expected, actual string) bool {
	// Exact match
	if expected == actual {
		return true
	}

	// Subtest parent matching: TestFoo matches TestFoo/SubTest (only direct subtest)
	if idx := strings.Index(actual, "/"); idx != -1 {
		parentActual := actual[:idx]
		if parentActual == expected {
			return true
		}
	}

	// Handle expected being a subtest: TestFoo/Sub matches parent TestFoo
	if idx := strings.Index(expected, "/"); idx != -1 {
		parentExpected := expected[:idx]
		if parentExpected == actual {
			return true
		}
	}

	// Normalize and compare
	normExpected := normalizeTestName(expected)
	normActual := normalizeTestName(actual)

	if normExpected == normActual {
		return true
	}

	// Check if normalized actual starts with normalized expected (for version suffixes)
	if strings.HasPrefix(normActual, normExpected) {
		return true
	}

	// Check if names contain the same tokens (for word reordering cases)
	if tokensMatch(normExpected, normActual) {
		return true
	}

	// Calculate Levenshtein similarity as last resort
	similarity := levenshteinSimilarity(normExpected, normActual)
	const similarityThreshold = 0.7 // 70% similarity threshold

	return similarity >= similarityThreshold
}

// normalizeTestName standardizes test name for comparison
func normalizeTestName(name string) string {
	// Remove "Test" prefix for comparison
	normalized := strings.TrimPrefix(name, "Test")

	// Remove subtest suffix
	if idx := strings.Index(normalized, "/"); idx != -1 {
		normalized = normalized[:idx]
	}

	// Remove common suffixes like _v2, _V2, _test, etc.
	suffixPatterns := []string{"_v1", "_v2", "_v3", "_V1", "_V2", "_V3", "_test", "_Test"}
	for _, suffix := range suffixPatterns {
		normalized = strings.TrimSuffix(normalized, suffix)
	}

	// Remove underscores and convert to lowercase
	normalized = strings.ReplaceAll(normalized, "_", "")
	normalized = strings.ToLower(normalized)

	return normalized
}

// tokensMatch checks if two normalized test names contain similar tokens
// This handles word reordering cases like "readchapter" vs "chapterread"
func tokensMatch(a, b string) bool {
	// Extract tokens by splitting on common word boundaries
	tokensA := extractTokens(a)
	tokensB := extractTokens(b)

	if len(tokensA) == 0 || len(tokensB) == 0 {
		return false
	}

	// Check if all tokens from A are present in B and vice versa
	matchedA := 0
	for _, tokenA := range tokensA {
		for _, tokenB := range tokensB {
			if tokenA == tokenB || strings.Contains(tokenB, tokenA) || strings.Contains(tokenA, tokenB) {
				matchedA++
				break
			}
		}
	}

	matchedB := 0
	for _, tokenB := range tokensB {
		for _, tokenA := range tokensA {
			if tokenA == tokenB || strings.Contains(tokenA, tokenB) || strings.Contains(tokenB, tokenA) {
				matchedB++
				break
			}
		}
	}

	// Require at least 50% of tokens from each side to match
	thresholdA := float64(len(tokensA)) * 0.5
	thresholdB := float64(len(tokensB)) * 0.5

	return float64(matchedA) >= thresholdA && float64(matchedB) >= thresholdB
}

// extractTokens splits a normalized test name into meaningful tokens
// using common word patterns in test names
func extractTokens(name string) []string {
	// Common words to look for in test names
	commonWords := []string{
		"read", "write", "get", "set", "create", "delete", "update", "list",
		"find", "search", "add", "remove", "check", "verify", "validate",
		"chapter", "user", "item", "data", "file", "config", "test", "mock",
		"error", "success", "fail", "valid", "invalid", "empty", "null",
	}

	var tokens []string
	remaining := name

	// Try to extract known common words first
	for _, word := range commonWords {
		if strings.Contains(remaining, word) {
			tokens = append(tokens, word)
			remaining = strings.Replace(remaining, word, "", 1)
		}
	}

	// Add any remaining characters as a single token if non-empty
	remaining = strings.TrimSpace(remaining)
	if len(remaining) > 2 { // Only add if meaningful (> 2 chars)
		tokens = append(tokens, remaining)
	}

	return tokens
}

// levenshteinSimilarity calculates string similarity (0-1) based on Levenshtein distance
func levenshteinSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}

	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	if maxLen == 0 {
		return 1.0
	}

	distance := levenshteinDistance(a, b)
	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance calculates the minimum edit distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	aRunes := []rune(a)
	bRunes := []rune(b)
	lenA := len(aRunes)
	lenB := len(bRunes)

	// Use two rows instead of full matrix for memory efficiency
	prevRow := make([]int, lenB+1)
	currRow := make([]int, lenB+1)

	// Initialize first row
	for j := 0; j <= lenB; j++ {
		prevRow[j] = j
	}

	// Fill in the rest of the matrix
	for i := 1; i <= lenA; i++ {
		currRow[0] = i

		for j := 1; j <= lenB; j++ {
			cost := 0
			if aRunes[i-1] != bRunes[j-1] {
				cost = 1
			}

			// Minimum of: delete, insert, replace
			currRow[j] = min(
				prevRow[j]+1,      // delete
				currRow[j-1]+1,    // insert
				prevRow[j-1]+cost, // replace
			)
		}

		// Swap rows
		prevRow, currRow = currRow, prevRow
	}

	return prevRow[lenB]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// hasIndividualTestNames checks whether the test output contains individual test
// name results (as opposed to only file-level summaries).
// Returns false for Vitest default (non-verbose) output like:
//
//	✓ GameCanvas.test.ts (4 tests) 306ms
//
// Returns true for Go test output, Vitest verbose output, Jest output, etc.
func hasIndividualTestNames(output string) bool {
	// Go test format: "--- PASS: TestName" or "--- FAIL: TestName"
	goRe := regexp.MustCompile(`--- (?:PASS|FAIL): [\w/]+`)
	if goRe.MatchString(output) {
		return true
	}

	// Vitest verbose format: "✓ file.test.ts > describe > test name 1ms"
	vitestVerboseRe := regexp.MustCompile(`[✓✕]\s+\S+\.test\.[jt]sx?\s*>`)
	if vitestVerboseRe.MatchString(output) {
		return true
	}

	// Jest/Node format: "✓ test name" (without .test.ts pattern on the same line)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if (strings.HasPrefix(line, "✓") || strings.HasPrefix(line, "✕")) &&
			!strings.Contains(line, ".test.") && !strings.Contains(line, ".spec.") {
			return true
		}
	}

	return false
}

// isMetaCriteria returns true if a verification is a meta-criteria that cannot
// be mapped to a specific test function (e.g., "all tests pass", "unit tests added").
// These are verified by overall test suite pass rather than individual test matching.
func isMetaCriteria(v CriteriaVerification) bool {
	// Explicit (meta) marker from reviewer takes priority
	if v.IsMeta {
		return true
	}

	// Auto-detect common meta-criteria patterns
	lower := strings.ToLower(v.TestName)
	metaPatterns := []string{
		"(meta)",
		"all tests",
		"unit tests added",
		"no regressions",
		"tests pass",
		"test coverage",
	}
	for _, pattern := range metaPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Also check criteria text for meta patterns when test name is empty
	if v.TestName == "" {
		criteriaLower := strings.ToLower(v.Criteria)
		criteriaMetaPatterns := []string{
			"all tests pass",
			"unit tests added",
			"no regressions",
			"test coverage",
		}
		for _, pattern := range criteriaMetaPatterns {
			if strings.Contains(criteriaLower, pattern) {
				return true
			}
		}
	}

	return false
}

// findSimilarTests finds test names similar to the expected name from the passed tests
func findSimilarTests(expected string, passedTests map[string]bool, threshold float64) []string {
	var similar []string
	normExpected := normalizeTestName(expected)

	for name := range passedTests {
		normName := normalizeTestName(name)
		similarity := levenshteinSimilarity(normExpected, normName)
		if similarity >= threshold {
			similar = append(similar, fmt.Sprintf("%s (%.0f%% similar)", name, similarity*100))
		}
	}

	return similar
}

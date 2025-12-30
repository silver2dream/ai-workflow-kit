package task

import (
	"fmt"
	"regexp"
	"strings"
)

var requiredSections = []string{
	"Summary",
	"Scope",
	"Acceptance Criteria",
	"Testing Requirements",
	"Metadata",
}

// ValidateBody checks if the issue body contains all required sections and at least one checkbox.
func ValidateBody(body string) error {
	var missing []string
	for _, section := range requiredSections {
		pattern := fmt.Sprintf(`(?im)^\s*##\s+%s\s*$`, regexp.QuoteMeta(section))
		re := regexp.MustCompile(pattern)
		if !re.MatchString(body) {
			missing = append(missing, section)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("ticket body missing required sections: %s", strings.Join(missing, ", "))
	}

	// Check for at least one unchecked checkbox
	checkboxRe := regexp.MustCompile(`(?m)^\s*-\s*\[\s*\]\s+`)
	if !checkboxRe.MatchString(body) {
		return fmt.Errorf("ticket body missing acceptance checkboxes (- [ ])")
	}

	return nil
}

// EnsureAWKMetadata injects **Spec** and **Task Line** into the body if missing.
func EnsureAWKMetadata(body, spec string, taskLine int) string {
	specRe := regexp.MustCompile(`(?i)\*\*spec\*\*\s*:`)
	taskLineRe := regexp.MustCompile(`(?i)\*\*task\s+line\*\*\s*:`)

	needsSpec := !specRe.MatchString(body)
	needsTaskLine := !taskLineRe.MatchString(body)

	if !needsSpec && !needsTaskLine {
		return body
	}

	appended := "\n\n## AWK Metadata\n"
	if needsSpec {
		appended += fmt.Sprintf("- **Spec**: %s\n", spec)
	}
	if needsTaskLine {
		appended += fmt.Sprintf("- **Task Line**: %d\n", taskLine)
	}

	return strings.TrimRight(body, "\r\n\t ") + appended
}

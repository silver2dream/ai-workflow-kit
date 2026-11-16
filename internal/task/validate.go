package task

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/ticket"
)

// The sections the Worker and the review read.
var requiredSections = []string{
	"Summary",
	"Scope",
	"Acceptance Criteria",
	"Testing Requirements",
}

// ValidateBody checks that a ticket body can be worked on: it has the sections
// the Worker and the review read, at least one open acceptance checkbox, and
// an awk-ticket block the workflow can use.
func ValidateBody(body string) error {
	_, err := validatedBlock(body)
	return err
}

// validatedBlock validates a ticket body and returns its awk-ticket block.
func validatedBlock(body string) (ticket.Meta, error) {
	var missing []string
	for _, section := range requiredSections {
		pattern := fmt.Sprintf(`(?im)^\s*##\s+%s\s*$`, regexp.QuoteMeta(section))
		re := regexp.MustCompile(pattern)
		if !re.MatchString(body) {
			missing = append(missing, section)
		}
	}

	if len(missing) > 0 {
		return ticket.Meta{}, fmt.Errorf("ticket body missing required sections: %s", strings.Join(missing, ", "))
	}

	// Check for at least one unchecked checkbox
	checkboxRe := regexp.MustCompile(`(?m)^\s*-\s*\[\s*\]\s+`)
	if !checkboxRe.MatchString(body) {
		return ticket.Meta{}, fmt.Errorf("ticket body missing acceptance checkboxes (- [ ])")
	}

	meta, found, err := ticket.Parse(body)
	if err != nil {
		return ticket.Meta{}, fmt.Errorf("ticket body: %w", err)
	}
	if !found {
		return ticket.Meta{}, fmt.Errorf("ticket body has no `%s` block: it is where the workflow reads which repo the ticket is for and which tickets it waits for", ticket.Info)
	}
	return meta, nil
}

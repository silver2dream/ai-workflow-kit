package kickoff

import (
	"regexp"
	"strconv"
)

// OutputParser parses Principal stdout to detect STEP-3 and STEP-4 events
type OutputParser struct {
	onIssueStart func(issueID int) // Called when STEP-3 + issue #N detected
	onIssueEnd   func()            // Called when STEP-4 detected
}

// Pattern: [PRINCIPAL] HH:MM:SS | STEP-3 | ... issue #N ...
var step3Pattern = regexp.MustCompile(`\[PRINCIPAL\].*\|\s*STEP-3\s*\|.*issue #(\d+)`)
var step4Pattern = regexp.MustCompile(`\[PRINCIPAL\].*\|\s*STEP-4\s*\|`)

// NewOutputParser creates a new OutputParser with the given callbacks
func NewOutputParser(onStart func(int), onEnd func()) *OutputParser {
	return &OutputParser{
		onIssueStart: onStart,
		onIssueEnd:   onEnd,
	}
}

// Parse processes a line of output and triggers callbacks as needed
func (p *OutputParser) Parse(line string) {
	// Check for STEP-3 with issue number
	if matches := step3Pattern.FindStringSubmatch(line); len(matches) > 1 {
		if issueID, err := strconv.Atoi(matches[1]); err == nil {
			if p.onIssueStart != nil {
				p.onIssueStart(issueID)
			}
		}
	}

	// Check for STEP-4
	if step4Pattern.MatchString(line) {
		if p.onIssueEnd != nil {
			p.onIssueEnd()
		}
	}
}

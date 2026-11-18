package main

import (
	"fmt"

	"github.com/silver2dream/ai-workflow-kit/internal/doctor"
)

// doctorTotals counts what a list of checks found.
func doctorTotals(results []doctor.CheckResult) (errors, warnings int) {
	for _, r := range results {
		switch r.Status {
		case "error":
			errors++
		case "warning":
			warnings++
		}
	}
	return errors, warnings
}

// doctorVerdict is the last line of the doctor: what a person does next.
func doctorVerdict(errors, warnings int) string {
	switch {
	case errors > 0 && warnings > 0:
		return fmt.Sprintf("%d problem%s to fix before a run, and %d warning%s.", errors, plural(errors), warnings, plural(warnings))
	case errors > 0:
		return fmt.Sprintf("%d problem%s to fix before a run.", errors, plural(errors))
	case warnings > 0:
		return fmt.Sprintf("Nothing stands in the way of a run. %d warning%s.", warnings, plural(warnings))
	default:
		return "Nothing stands in the way of a run."
	}
}

package reviewer

import (
	"fmt"
	"regexp"
	"strings"
)

// Evidence represents a single EVIDENCE: line from the review
type Evidence struct {
	File   string // optional: file path
	Needle string // required: substring to find
	Raw    string // original line
}

// EvidenceError represents an evidence verification error
type EvidenceError struct {
	Code    int    // 0=OK, 1=missing/not found, 2=no evidence, 3=diff error
	Message string
	Missing []string
}

func (e *EvidenceError) Error() string {
	return e.Message
}

// ParseEvidence extracts EVIDENCE: lines from review body
func ParseEvidence(reviewBody string) []Evidence {
	var results []Evidence

	for _, line := range strings.Split(reviewBody, "\n") {
		stripped := strings.TrimSpace(line)
		if !strings.HasPrefix(stripped, "EVIDENCE:") {
			continue
		}

		rest := strings.TrimSpace(strings.TrimPrefix(stripped, "EVIDENCE:"))
		if rest == "" {
			continue
		}

		var file, needle string
		if idx := strings.Index(rest, "|"); idx >= 0 {
			file = strings.TrimSpace(rest[:idx])
			needle = strings.TrimSpace(rest[idx+1:])
		} else {
			needle = rest
		}

		// Strip common wrappers
		needle = cleanNeedle(needle)
		if needle == "" {
			continue
		}

		results = append(results, Evidence{
			File:   file,
			Needle: needle,
			Raw:    stripped,
		})
	}

	return results
}

// cleanNeedle removes quotes and backticks from needle
func cleanNeedle(needle string) string {
	needle = strings.TrimSpace(needle)

	// Remove surrounding quotes
	if len(needle) >= 2 {
		if (needle[0] == '"' && needle[len(needle)-1] == '"') ||
			(needle[0] == '\'' && needle[len(needle)-1] == '\'') ||
			(needle[0] == '`' && needle[len(needle)-1] == '`') {
			needle = needle[1 : len(needle)-1]
		}
	}

	return strings.TrimSpace(needle)
}

// SplitDiffByFile splits a unified diff into sections by file
func SplitDiffByFile(diff string) map[string]string {
	sections := make(map[string][]string)
	var currentFile string

	diffHeader := regexp.MustCompile(`^diff --git a/(.+?) b/(.+?)$`)

	for _, line := range strings.Split(diff, "\n") {
		if m := diffHeader.FindStringSubmatch(line); m != nil {
			currentFile = m[2]
			sections[currentFile] = append(sections[currentFile], line)
			continue
		}
		if currentFile != "" {
			sections[currentFile] = append(sections[currentFile], line)
		}
	}

	result := make(map[string]string)
	for k, v := range sections {
		result[k] = strings.Join(v, "\n")
	}
	return result
}

// VerifyEvidence checks that all evidence lines are present in the diff
func VerifyEvidence(diff string, evidence []Evidence, minRequired int) *EvidenceError {
	if minRequired < 1 {
		minRequired = 1
	}

	if strings.TrimSpace(diff) == "" {
		return &EvidenceError{
			Code:    3,
			Message: "diff is empty",
		}
	}

	if len(evidence) == 0 {
		return &EvidenceError{
			Code:    2,
			Message: "no EVIDENCE lines found in review body",
		}
	}

	if len(evidence) < minRequired {
		return &EvidenceError{
			Code:    1,
			Message: fmt.Sprintf("insufficient EVIDENCE lines: %d < %d", len(evidence), minRequired),
		}
	}

	byFile := SplitDiffByFile(diff)
	var missing []string

	for _, item := range evidence {
		haystack := diff
		if item.File != "" {
			if content, ok := byFile[item.File]; ok {
				haystack = content
			} else {
				missing = append(missing, fmt.Sprintf("%s (file not in diff)", item.Raw))
				continue
			}
		}

		if !strings.Contains(haystack, item.Needle) {
			missing = append(missing, fmt.Sprintf("%s (needle not found)", item.Raw))
		}
	}

	if len(missing) > 0 {
		return &EvidenceError{
			Code:    1,
			Message: "evidence verification failed",
			Missing: missing,
		}
	}

	return nil
}

// GetMinEvidence returns the minimum required evidence count from env
func GetMinEvidence() int {
	return 3 // Default value, can be overridden via AWK_REVIEW_EVIDENCE_MIN
}

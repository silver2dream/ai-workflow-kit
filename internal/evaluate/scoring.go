package evaluate

// Gate weights for score calculation.
// Higher weights indicate more critical gates.
const (
	// Offline gate weights
	weightO0  = 10.0 // Git ignore - critical for not polluting repo
	weightO1  = 5.0  // Scan repo
	weightO3  = 5.0  // Audit project
	weightO5  = 15.0 // Config validation - critical for operation
	weightO7  = 5.0  // Version sync
	weightO8  = 10.0 // File encoding - affects cross-platform
	weightO10 = 10.0 // Test suite

	// Online gate weights
	weightN1 = 20.0 // Kickoff dry-run - critical for operation
	weightN2 = 10.0 // Rollback output
	weightN3 = 10.0 // Stats JSON
)

// Maximum possible score
const maxScore = weightO0 + weightO1 + weightO3 + weightO5 + weightO7 + weightO8 + weightO10 +
	weightN1 + weightN2 + weightN3

// CalculateScoreCap calculates the score cap based on gate results.
// Returns a value between 0.0 and 100.0.
func CalculateScoreCap(offline OfflineGateResults, online OnlineGateResults) float64 {
	var score float64

	// Offline gates
	score += gateScore(offline.O0, weightO0)
	score += gateScore(offline.O1, weightO1)
	score += gateScore(offline.O3, weightO3)
	score += gateScore(offline.O5, weightO5)
	score += gateScore(offline.O7, weightO7)
	score += gateScore(offline.O8, weightO8)
	score += gateScore(offline.O10, weightO10)

	// Online gates
	score += gateScore(online.N1, weightN1)
	score += gateScore(online.N2, weightN2)
	score += gateScore(online.N3, weightN3)

	// Convert to percentage
	return (score / maxScore) * 100.0
}

// gateScore returns the score for a single gate based on its result.
// PASS = full weight, SKIP = half weight, FAIL = 0
func gateScore(result GateResult, weight float64) float64 {
	switch result.Status {
	case "PASS":
		return weight
	case "SKIP":
		return weight * 0.5 // Skipped gates count as partial
	case "FAIL":
		return 0
	default:
		return 0
	}
}

// ScoreToGrade converts a numeric score (0-100) to a letter grade.
func ScoreToGrade(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

// GradeDescription returns a human-readable description of a grade.
func GradeDescription(grade string) string {
	descriptions := map[string]string{
		"A": "Excellent - ready for production",
		"B": "Good - minor improvements recommended",
		"C": "Acceptable - some issues to address",
		"D": "Below standard - significant issues",
		"F": "Failing - critical issues must be fixed",
	}
	if desc, ok := descriptions[grade]; ok {
		return desc
	}
	return "Unknown grade"
}

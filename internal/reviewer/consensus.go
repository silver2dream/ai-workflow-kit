package reviewer

import "math"

const (
	// DefaultPrimaryWeight is the default weight for the primary reviewer score.
	DefaultPrimaryWeight = 0.7

	// DefaultSecondaryTotalWeight is the total weight split among all secondary reviewers.
	DefaultSecondaryTotalWeight = 0.3

	// ErrorFindingsCap is the maximum consensus score when any secondary reviewer finds error-severity issues.
	ErrorFindingsCap = 6
)

// CalculateConsensusScore computes a weighted consensus score from primary and secondary reviews.
//
// Default weights: primary gets 0.7, secondary reviewers split the remaining 0.3 equally.
// If custom weights are provided (len(weights) == 1 + len(secondaryScores)), they are used instead.
// If any secondary reviewer's findings contain severity=error issues, the final score is capped at 6.
//
// The hasErrors parameter indicates whether any secondary reviewer found error-severity findings.
func CalculateConsensusScore(primaryScore int, secondaryScores []int, weights []float64, hasErrors bool) int {
	if len(secondaryScores) == 0 {
		return primaryScore
	}

	// Determine weights
	effectiveWeights := computeWeights(len(secondaryScores), weights)

	// Calculate weighted sum
	sum := float64(primaryScore) * effectiveWeights[0]
	for i, score := range secondaryScores {
		sum += float64(score) * effectiveWeights[i+1]
	}

	// Round to nearest integer
	result := int(math.Round(sum))

	// Clamp to [1, 10]
	if result < 1 {
		result = 1
	}
	if result > 10 {
		result = 10
	}

	// Cap at ErrorFindingsCap if any secondary found error-severity issues
	if hasErrors && result > ErrorFindingsCap {
		result = ErrorFindingsCap
	}

	return result
}

// computeWeights returns the effective weight slice.
// If custom weights are valid (correct length, sums to ~1.0), use them.
// Otherwise, use default weights: primary 0.7, secondaries split 0.3.
func computeWeights(numSecondary int, custom []float64) []float64 {
	totalSlots := 1 + numSecondary

	// Check if custom weights are valid
	if len(custom) == totalSlots {
		sum := 0.0
		for _, w := range custom {
			sum += w
		}
		// Accept if weights sum to approximately 1.0 (within 0.01 tolerance)
		if math.Abs(sum-1.0) < 0.01 {
			return custom
		}
	}

	// Default weights
	weights := make([]float64, totalSlots)
	weights[0] = DefaultPrimaryWeight
	secondaryWeight := DefaultSecondaryTotalWeight / float64(numSecondary)
	for i := 1; i < totalSlots; i++ {
		weights[i] = secondaryWeight
	}

	return weights
}

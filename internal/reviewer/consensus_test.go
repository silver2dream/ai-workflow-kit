package reviewer

import "testing"

func TestCalculateConsensusScore_NoSecondary(t *testing.T) {
	result := CalculateConsensusScore(8, nil, nil, false)
	if result != 8 {
		t.Errorf("expected 8, got %d", result)
	}
}

func TestCalculateConsensusScore_DefaultWeights(t *testing.T) {
	// primary=8 (weight 0.7) = 5.6
	// secondary=[6] (weight 0.3) = 1.8
	// total = 7.4, rounded = 7
	result := CalculateConsensusScore(8, []int{6}, nil, false)
	if result != 7 {
		t.Errorf("expected 7, got %d", result)
	}
}

func TestCalculateConsensusScore_MultipleSecondary(t *testing.T) {
	// primary=8 (weight 0.7) = 5.6
	// secondary=[6, 4] (weight 0.15 each) = 0.9 + 0.6 = 1.5
	// total = 7.1, rounded = 7
	result := CalculateConsensusScore(8, []int{6, 4}, nil, false)
	if result != 7 {
		t.Errorf("expected 7, got %d", result)
	}
}

func TestCalculateConsensusScore_ErrorFindingsCapped(t *testing.T) {
	// primary=9 (weight 0.7) = 6.3
	// secondary=[8] (weight 0.3) = 2.4
	// total = 8.7, rounded = 9
	// but hasErrors=true caps at 6
	result := CalculateConsensusScore(9, []int{8}, nil, true)
	if result != 6 {
		t.Errorf("expected 6 (capped due to error findings), got %d", result)
	}
}

func TestCalculateConsensusScore_ErrorFindingsNoCap(t *testing.T) {
	// primary=4 (weight 0.7) = 2.8
	// secondary=[3] (weight 0.3) = 0.9
	// total = 3.7, rounded = 4
	// hasErrors=true but 4 <= 6, no cap effect
	result := CalculateConsensusScore(4, []int{3}, nil, true)
	if result != 4 {
		t.Errorf("expected 4 (already below cap), got %d", result)
	}
}

func TestCalculateConsensusScore_CustomWeights(t *testing.T) {
	// primary=10 (weight 0.5) = 5.0
	// secondary=[10] (weight 0.5) = 5.0
	// total = 10.0
	result := CalculateConsensusScore(10, []int{10}, []float64{0.5, 0.5}, false)
	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}
}

func TestCalculateConsensusScore_InvalidCustomWeightsFallback(t *testing.T) {
	// Custom weights wrong length -> falls back to default
	// primary=8 (weight 0.7) = 5.6
	// secondary=[6] (weight 0.3) = 1.8
	// total = 7.4, rounded = 7
	result := CalculateConsensusScore(8, []int{6}, []float64{0.5}, false)
	if result != 7 {
		t.Errorf("expected 7 (fallback to default weights), got %d", result)
	}
}

func TestCalculateConsensusScore_ClampLow(t *testing.T) {
	result := CalculateConsensusScore(1, []int{1}, nil, false)
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestCalculateConsensusScore_ClampHigh(t *testing.T) {
	result := CalculateConsensusScore(10, []int{10}, nil, false)
	if result != 10 {
		t.Errorf("expected 10, got %d", result)
	}
}

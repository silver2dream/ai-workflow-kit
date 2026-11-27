package worker

import (
	"sync"
	"testing"
)

// The engine's lanes run side by side: each failure counts, however many
// come at once.
func TestAddConsecutiveFailure_SideBySide(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 5 {
				if _, err := AddConsecutiveFailure(root); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if got := ReadConsecutiveFailures(root); got != 40 {
		t.Errorf("%d failures counted, want 40", got)
	}
	if err := ResetConsecutiveFailures(root); err != nil || ReadConsecutiveFailures(root) != 0 {
		t.Errorf("reset: %v, %d", err, ReadConsecutiveFailures(root))
	}
	if n, err := AddConsecutiveFailure(root); n != 1 || err != nil {
		t.Errorf("after a reset: %d, %v", n, err)
	}
}

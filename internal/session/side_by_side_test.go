package session

import (
	"fmt"
	"sync"
	"testing"
)

// The engine's lanes run side by side, and each records what its Worker did
// through a manager of its own: none of what they append is lost.
func TestAppendAction_ManagersSideBySide(t *testing.T) {
	dir := t.TempDir()
	sessionID, err := NewManager(dir).InitPrincipal()
	if err != nil {
		t.Fatalf("InitPrincipal: %v", err)
	}
	var wg sync.WaitGroup
	for lane := range 4 {
		wg.Go(func() {
			for i := range 10 {
				if err := NewManager(dir).AppendAction(sessionID, "worker_dispatched", fmt.Sprintf(`{"lane": %d, "n": %d}`, lane, i)); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	session, err := NewManager(dir).loadSessionLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Actions) != 40 {
		t.Errorf("%d actions, want 40", len(session.Actions))
	}
}

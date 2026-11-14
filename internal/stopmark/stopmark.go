// Package stopmark is the STOP marker of a project: the file that ends a run.
//
// A run looks for it between the steps of its loop. `awkit stop` writes it to
// end the run a person started, and `awkit stop-workflow` writes it when the
// Principal ends the run itself, with the reason. It ends the run that is
// going; it does not stand in the way of the next one, which clears it when
// it starts.
package stopmark

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// File is where the marker is, from the project's root.
const File = layout.StateDir + "/STOP"

// ByUser is the reason of a marker a person asked for: with `awkit stop`, or
// by creating the file by hand, which leaves it empty.
const ByUser = "user_stopped"

// Path returns the marker's path in the project at root.
func Path(root string) string { return layout.In(root, File) }

// Write puts the marker there, saying who asked for the stop and why.
func Write(root, by, reason string) error {
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("STOP requested by %s\nReason: %s\n", by, reason)), 0o644)
}

// Read says whether the marker is there, and the reason it carries. A marker
// made by hand carries none.
func Read(root string) (reason string, present bool) {
	data, err := os.ReadFile(Path(root))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "Reason:"); ok {
			return strings.TrimSpace(value), true
		}
	}
	return "", true
}

// Clear removes the marker. A marker that is not there is no error.
func Clear(root string) error {
	if err := os.Remove(Path(root)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

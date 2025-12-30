package buildinfo

import "testing"

func TestVersionDefault(t *testing.T) {
	// Version should have a default value
	if Version == "" {
		t.Error("Version should not be empty")
	}
}

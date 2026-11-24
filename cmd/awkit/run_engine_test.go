package main

import "testing"

// AWKIT_ENGINE says how `awkit run` runs the workflow: the engine, the
// engine with its shadow, or, by default and for any other word, the
// Principal.
func TestEngineMode(t *testing.T) {
	for value, want := range map[string]string{
		"":          "",
		"engine":    modeEngine,
		" Engine ":  modeEngine,
		"shadow":    modeShadow,
		"SHADOW":    modeShadow,
		"principal": "",
		"engines":   "",
	} {
		t.Setenv("AWKIT_ENGINE", value)
		if got := engineMode(); got != want {
			t.Errorf("AWKIT_ENGINE=%q: %q, want %q", value, got, want)
		}
	}
}

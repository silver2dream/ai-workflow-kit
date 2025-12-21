package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestAvailablePresetNames(t *testing.T) {
	names := availablePresetNames()
	if !strings.Contains(names, "generic") {
		t.Error("expected 'generic' in preset names")
	}
	if !strings.Contains(names, "react-go") {
		t.Error("expected 'react-go' in preset names")
	}
}

func TestColorOutput(t *testing.T) {
	// Test that color functions don't panic
	_ = bold("test")
	_ = cyan("test")

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	info("test message\n")

	w.Close()
	os.Stdout = oldStdout
	buf.ReadFrom(r)

	if !strings.Contains(buf.String(), "test message") {
		t.Error("info() should output message")
	}
}

func TestBashCompletion(t *testing.T) {
	if !strings.Contains(bashCompletion, "_awkit") {
		t.Error("bash completion should contain _awkit function")
	}
	if !strings.Contains(bashCompletion, "init") {
		t.Error("bash completion should contain init command")
	}
}

func TestZshCompletion(t *testing.T) {
	if !strings.Contains(zshCompletion, "#compdef awkit") {
		t.Error("zsh completion should start with #compdef")
	}
	if !strings.Contains(zshCompletion, "init") {
		t.Error("zsh completion should contain init command")
	}
}

func TestFishCompletion(t *testing.T) {
	if !strings.Contains(fishCompletion, "complete -c awkit") {
		t.Error("fish completion should contain complete -c awkit")
	}
	if !strings.Contains(fishCompletion, "init") {
		t.Error("fish completion should contain init command")
	}
}

func TestPresets(t *testing.T) {
	if len(presets) < 2 {
		t.Error("expected at least 2 presets")
	}

	foundGeneric := false
	foundReactGo := false
	for _, p := range presets {
		if p.Name == "generic" {
			foundGeneric = true
		}
		if p.Name == "react-go" {
			foundReactGo = true
		}
	}

	if !foundGeneric {
		t.Error("missing 'generic' preset")
	}
	if !foundReactGo {
		t.Error("missing 'react-go' preset")
	}
}

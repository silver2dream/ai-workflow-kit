package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// formatDuration (kickoff.go)
// ---------------------------------------------------------------------------

func TestFormatDuration_UnderMinute(t *testing.T) {
	got := formatDuration(45 * time.Second)
	if got != "45s" {
		t.Errorf("formatDuration(45s) = %q, want 45s", got)
	}
}

func TestFormatDuration_ExactMinute(t *testing.T) {
	got := formatDuration(60 * time.Second)
	if got != "1:00" {
		t.Errorf("formatDuration(60s) = %q, want 1:00", got)
	}
}

func TestFormatDuration_MinutesAndSeconds(t *testing.T) {
	got := formatDuration(2*time.Minute + 30*time.Second)
	if got != "2:30" {
		t.Errorf("formatDuration(2m30s) = %q, want 2:30", got)
	}
}

func TestFormatDuration_SingleSecond(t *testing.T) {
	got := formatDuration(1 * time.Second)
	if got != "1s" {
		t.Errorf("formatDuration(1s) = %q, want 1s", got)
	}
}

func TestFormatDuration_Zero(t *testing.T) {
	got := formatDuration(0)
	if got != "0s" {
		t.Errorf("formatDuration(0) = %q, want 0s", got)
	}
}

func TestFormatDuration_PadSeconds(t *testing.T) {
	got := formatDuration(1*time.Minute + 5*time.Second)
	if got != "1:05" {
		t.Errorf("formatDuration(1m5s) = %q, want 1:05", got)
	}
}

// ---------------------------------------------------------------------------
// fileExists (kickoff.go)
// ---------------------------------------------------------------------------

func TestFileExists_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "test")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()

	if !fileExists(f.Name()) {
		t.Errorf("fileExists(%q) = false, want true", f.Name())
	}
}

func TestFileExists_NonExistentFile(t *testing.T) {
	if fileExists("/nonexistent/path/file.txt") {
		t.Error("fileExists for non-existent file should return false")
	}
}

func TestFileExists_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	if !fileExists(dir) {
		t.Errorf("fileExists(dir) = false, want true")
	}
}

// ---------------------------------------------------------------------------
// getEnvInt (kickoff.go)
// ---------------------------------------------------------------------------

func TestGetEnvInt_Default_EnvNotSet(t *testing.T) {
	t.Setenv("TEST_ENV_INT_UNSET", "")
	got := getEnvInt("TEST_ENV_INT_UNSET", 42)
	if got != 42 {
		t.Errorf("getEnvInt(unset) = %d, want 42 (default)", got)
	}
}

func TestGetEnvInt_ValidValue(t *testing.T) {
	t.Setenv("TEST_ENV_INT_VALID", "10")
	got := getEnvInt("TEST_ENV_INT_VALID", 5)
	if got != 10 {
		t.Errorf("getEnvInt(10) = %d, want 10", got)
	}
}

func TestGetEnvInt_InvalidValue_FallsBack(t *testing.T) {
	t.Setenv("TEST_ENV_INT_INVALID", "notanumber")
	got := getEnvInt("TEST_ENV_INT_INVALID", 7)
	if got != 7 {
		t.Errorf("getEnvInt(invalid) = %d, want 7 (default)", got)
	}
}

func TestGetEnvInt_ZeroValue_FallsBack(t *testing.T) {
	t.Setenv("TEST_ENV_INT_ZERO", "0")
	got := getEnvInt("TEST_ENV_INT_ZERO", 99)
	// 0 is <= 0 so should fall back
	if got != 99 {
		t.Errorf("getEnvInt(0) = %d, want 99 (default, 0 <= 0)", got)
	}
}

func TestGetEnvInt_NegativeValue_FallsBack(t *testing.T) {
	t.Setenv("TEST_ENV_INT_NEG", "-5")
	got := getEnvInt("TEST_ENV_INT_NEG", 3)
	// Negative is <= 0 so should fall back
	if got != 3 {
		t.Errorf("getEnvInt(-5) = %d, want 3 (default)", got)
	}
}

// ---------------------------------------------------------------------------
// plural (hooks.go)
// ---------------------------------------------------------------------------

func TestPlural_One(t *testing.T) {
	if plural(1) != "" {
		t.Errorf("plural(1) = %q, want empty", plural(1))
	}
}

func TestPlural_Zero(t *testing.T) {
	if plural(0) != "s" {
		t.Errorf("plural(0) = %q, want 's'", plural(0))
	}
}

func TestPlural_Many(t *testing.T) {
	if plural(5) != "s" {
		t.Errorf("plural(5) = %q, want 's'", plural(5))
	}
}

// ---------------------------------------------------------------------------
// resolveRepo (main.go)
// ---------------------------------------------------------------------------

func TestResolveRepo_FlagOverridesEnv(t *testing.T) {
	t.Setenv("AWKIT_REPO", "env-owner/env-repo")
	got := resolveRepo("flag-owner/flag-repo")
	if got != "flag-owner/flag-repo" {
		t.Errorf("resolveRepo(flag) = %q, want flag-owner/flag-repo", got)
	}
}

func TestResolveRepo_EnvUsedWhenNoFlag(t *testing.T) {
	t.Setenv("AWKIT_REPO", "env-owner/env-repo")
	got := resolveRepo("")
	if got != "env-owner/env-repo" {
		t.Errorf("resolveRepo('') with env = %q, want env-owner/env-repo", got)
	}
}

func TestResolveRepo_DefaultWhenNoneSet(t *testing.T) {
	t.Setenv("AWKIT_REPO", "")
	got := resolveRepo("")
	if got == "" {
		t.Error("resolveRepo should return default repo when nothing set")
	}
}

// ---------------------------------------------------------------------------
// updateCommand (main.go)
// ---------------------------------------------------------------------------

func TestUpdateCommand_ContainsRepo(t *testing.T) {
	cmd := updateCommand("myowner/myrepo")
	if !strings.Contains(cmd, "myowner/myrepo") {
		t.Errorf("updateCommand() should contain repo, got: %q", cmd)
	}
}

func TestUpdateCommand_EmptyRepo_UsesDefault(t *testing.T) {
	cmd := updateCommand("")
	if cmd == "" {
		t.Error("updateCommand('') should return non-empty command")
	}
}

func TestUpdateCommand_ContainsInstallScript(t *testing.T) {
	cmd := updateCommand("owner/repo")
	if !strings.Contains(cmd, "install") {
		t.Errorf("updateCommand should reference install script, got: %q", cmd)
	}
}

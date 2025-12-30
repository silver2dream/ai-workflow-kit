// Package session manages Principal and Worker session lifecycle
package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Manager handles session lifecycle operations
type Manager struct {
	StateRoot string
}

// NewManager creates a new session manager
func NewManager(stateRoot string) *Manager {
	if stateRoot == "" {
		stateRoot = resolveStateRoot()
	}
	return &Manager{StateRoot: stateRoot}
}

// SessionStateDir returns the path to the session state directory
func (m *Manager) SessionStateDir() string {
	return filepath.Join(m.StateRoot, ".ai", "state", "principal")
}

// SessionFile returns the path to the current session file
func (m *Manager) SessionFile() string {
	return filepath.Join(m.SessionStateDir(), "session.json")
}

// SessionsDir returns the path to the sessions directory
func (m *Manager) SessionsDir() string {
	return filepath.Join(m.SessionStateDir(), "sessions")
}

// ResultsDir returns the path to the results directory
func (m *Manager) ResultsDir() string {
	return filepath.Join(m.StateRoot, ".ai", "results")
}

// GenerateSessionID generates a unique session ID
// Format: <role>-<YYYYMMDD>-<HHMMSS>-<random_hex_4>
func GenerateSessionID(role string) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	randomBytes := make([]byte, 2)
	_, _ = rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s-%s-%s", role, timestamp, randomHex)
}

// resolveStateRoot returns the main worktree root
func resolveStateRoot() string {
	// Honor AI_STATE_ROOT if set
	if root := os.Getenv("AI_STATE_ROOT"); root != "" {
		return root
	}

	// Use git common dir to find main worktree root
	cmd := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	output, err := cmd.Output()
	if err == nil {
		commonDir := strings.TrimSpace(string(output))
		if commonDir != "" {
			return filepath.Dir(commonDir)
		}
	}

	// Fallback to git toplevel
	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	output, err = cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// Final fallback to current directory
	wd, _ := os.Getwd()
	return wd
}

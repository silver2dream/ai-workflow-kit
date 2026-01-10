package kickoff

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// StateStaleThreshold is the time after which saved state is considered stale
	StateStaleThreshold = 24 * time.Hour
)

// RunState represents the saved state of a kickoff run
type RunState struct {
	Phase            string    `json:"phase"`
	CompletedTasks   []string  `json:"completed_tasks"`
	PendingTasks     []string  `json:"pending_tasks"`
	IssuesInProgress []int     `json:"issues_in_progress"`
	SavedAt          time.Time `json:"saved_at"`
}

// StateManager handles saving and loading run state
type StateManager struct {
	stateFile string
}

// NewStateManager creates a new StateManager for the given state file
func NewStateManager(stateFile string) *StateManager {
	return &StateManager{
		stateFile: stateFile,
	}
}

// SaveState saves the current run state to disk atomically (G5 fix)
// Note: On Windows, os.Rename cannot overwrite existing files, so we remove first
func (s *StateManager) SaveState(state *RunState) error {
	state.SavedAt = time.Now()

	// Ensure directory exists
	dir := filepath.Dir(s.stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file first for atomic update (G5 fix)
	tmpFile := s.stateFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Remove target file first for Windows compatibility
	// On Windows, os.Rename fails if destination exists
	_ = os.Remove(s.stateFile)

	// Atomically rename temp file to target
	if err := os.Rename(tmpFile, s.stateFile); err != nil {
		os.Remove(tmpFile) // Cleanup on failure
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// LoadState loads the saved run state from disk
func (s *StateManager) LoadState() (*RunState, error) {
	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		return nil, err
	}

	var state RunState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// HasState returns true if a valid saved state exists (G13 fix: validates content)
func (s *StateManager) HasState() bool {
	// Check file exists
	if _, err := os.Stat(s.stateFile); err != nil {
		return false
	}
	// Validate content is parseable
	state, err := s.LoadState()
	if err != nil {
		return false
	}
	// Validate required fields
	return !state.SavedAt.IsZero()
}

// IsStale returns true if the saved state is older than 24 hours
func (s *StateManager) IsStale() bool {
	state, err := s.LoadState()
	if err != nil {
		return false
	}

	return time.Since(state.SavedAt) > StateStaleThreshold
}

// ClearState removes the saved state file
func (s *StateManager) ClearState() error {
	if err := os.Remove(s.stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear state: %w", err)
	}
	return nil
}

// PromptResumeOrFresh asks the user whether to resume or start fresh
func PromptResumeOrFresh(reader *bufio.Reader) (resume bool, err error) {
	fmt.Print("Found saved state. Resume previous run? [Y/n]: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	input = strings.TrimSpace(strings.ToLower(input))

	// Default to yes if empty
	if input == "" || input == "y" || input == "yes" {
		return true, nil
	}

	return false, nil
}

package workbench

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"
)

// SettingsEnv names a settings file to use instead of the one in the user's
// settings folder: for tests, and for a person who keeps it elsewhere.
const SettingsEnv = "AWKIT_WORKBENCH_SETTINGS"

// Settings is what the workbench keeps for the person on this computer: the
// projects they open, and how far they are in learning the workbench. It is
// theirs, not a project's: it is not committed, and someone else working on
// the same project has their own. It is not kept by the browser either: the
// workbench may listen on another port each time it starts, and what a
// browser keeps belongs to one address.
type Settings struct {
	Projects   []Project  `json:"projects"`
	Onboarding Onboarding `json:"onboarding"`
}

// Project is a folder the person opened in the workbench.
type Project struct {
	Path    string    `json:"path"`
	AddedAt time.Time `json:"added_at"`
}

// Onboarding is how far the person is in learning the workbench.
type Onboarding struct {
	// Status is new (never started), touring (in the guided tour), paused
	// (left the tour to come back to it), ended (left the tour, kept the
	// checklist), done (finished the tour) or skipped (wants no teaching).
	Status string `json:"status"`
	// Step is the step of the tour they are on, or stopped at.
	Step int `json:"step"`
	// Done lists the things of the checklist they have done.
	Done []string `json:"done"`
	// ListHidden says they put the checklist away.
	ListHidden bool `json:"list_hidden"`
}

// OnboardingStatuses are the values Onboarding.Status takes.
var OnboardingStatuses = []string{"new", "touring", "paused", "ended", "done", "skipped"}

const maxTourStep = 50

var doneKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// Validate checks onboarding the browser sends, before it is kept.
func (o Onboarding) Validate() error {
	if !slices.Contains(OnboardingStatuses, o.Status) {
		return fmt.Errorf("status %q is none of %v", o.Status, OnboardingStatuses)
	}
	if o.Step < 0 || o.Step > maxTourStep {
		return fmt.Errorf("step %d is outside 0 to %d", o.Step, maxTourStep)
	}
	if len(o.Done) > 32 {
		return fmt.Errorf("%d things done; at most 32", len(o.Done))
	}
	for _, key := range o.Done {
		if !doneKey.MatchString(key) {
			return fmt.Errorf("%q is not the name of a thing to do", key)
		}
	}
	return nil
}

// Store keeps the settings in one file, written whole each time so that a
// crash never leaves half of it.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore keeps the settings in a file.
func NewStore(path string) *Store { return &Store{path: path} }

// DefaultSettingsPath is the settings file of the person running awkit: in
// their settings folder (%AppData% on Windows, ~/.config elsewhere), unless
// SettingsEnv names another.
func DefaultSettingsPath() (string, error) {
	if path := os.Getenv(SettingsEnv); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("your settings folder cannot be found: %w", err)
	}
	return filepath.Join(dir, "awkit", "workbench.json"), nil
}

// Path is the file the settings are kept in.
func (s *Store) Path() string { return s.path }

// Load reads the settings. A person who never opened the workbench has none:
// they get a new onboarding and no projects.
func (s *Store) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) load() (Settings, error) {
	settings := Settings{Onboarding: Onboarding{Status: "new"}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{Onboarding: Onboarding{Status: "new"}}, fmt.Errorf("%s cannot be read: %w", s.path, err)
	}
	if settings.Onboarding.Validate() != nil {
		// Written by a later version, or by hand: start the teaching over
		// rather than refuse to open.
		settings.Onboarding = Onboarding{Status: "new"}
	}
	return settings, nil
}

// Update changes the settings and keeps them.
func (s *Store) Update(change func(*Settings) error) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.load()
	if err != nil {
		return settings, err
	}
	if err := change(&settings); err != nil {
		return settings, err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return settings, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return settings, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".workbench-*.json")
	if err != nil {
		return settings, err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return settings, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return settings, err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		os.Remove(tmp.Name())
		return settings, err
	}
	return settings, nil
}

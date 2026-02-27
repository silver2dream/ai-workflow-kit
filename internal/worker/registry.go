package worker

import (
	"fmt"
	"sort"
	"sync"
)

// BackendRegistry manages available worker backends.
type BackendRegistry struct {
	backends map[string]WorkerBackend
	mu       sync.RWMutex
}

// NewBackendRegistry creates a new empty BackendRegistry.
func NewBackendRegistry() *BackendRegistry {
	return &BackendRegistry{
		backends: make(map[string]WorkerBackend),
	}
}

// Register adds a backend to the registry.
func (r *BackendRegistry) Register(b WorkerBackend) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[b.Name()] = b
}

// Get retrieves a backend by name.
func (r *BackendRegistry) Get(name string) (WorkerBackend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	if !ok {
		return nil, fmt.Errorf("unknown worker backend %q (available: %v)", name, r.Names())
	}
	return b, nil
}

// Names returns sorted names of all registered backends.
func (r *BackendRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultRegistry creates a registry with codex + claude-code pre-registered.
func DefaultRegistry(claudeModel string, claudeMaxTurns int, claudeSkipPerms bool) *BackendRegistry {
	reg := NewBackendRegistry()
	reg.Register(NewCodexBackend())
	reg.Register(NewClaudeCodeBackend(claudeModel, claudeMaxTurns, claudeSkipPerms))
	return reg
}

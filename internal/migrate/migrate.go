package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LatestVersion is the most recent config version.
const LatestVersion = "1.1"

// Migration represents a single config migration step.
type Migration struct {
	FromVersion string
	ToVersion   string
	Description string
	Apply       func(doc *yaml.Node) error
}

// AppliedMigration records a migration that was applied (or would be applied in dry-run).
type AppliedMigration struct {
	FromVersion string
	ToVersion   string
	Description string
}

// registry holds all known migrations in order.
var registry = []Migration{
	migrationV1_0ToV1_1,
}

// NeedsMigration checks if a config file needs migration and returns
// the current version found.
func NeedsMigration(configPath string) (currentVersion string, needed bool, err error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false, fmt.Errorf("reading config: %w", err)
	}

	ver := detectVersion(data)
	return ver, ver != LatestVersion, nil
}

// Run detects the current config version and applies all needed migrations.
// If dryRun is true, it reports what would change without modifying the file.
func Run(configPath string, dryRun bool) ([]AppliedMigration, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	currentVersion := detectVersion(data)
	if currentVersion == LatestVersion {
		return nil, nil // already up to date
	}

	// Parse as yaml.Node to preserve comments and formatting
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	var applied []AppliedMigration
	ver := currentVersion

	for _, m := range registry {
		if m.FromVersion != ver {
			continue
		}

		if dryRun {
			applied = append(applied, AppliedMigration{
				FromVersion: m.FromVersion,
				ToVersion:   m.ToVersion,
				Description: m.Description,
			})
			ver = m.ToVersion
			continue
		}

		if err := m.Apply(&doc); err != nil {
			return applied, fmt.Errorf("migration %s→%s failed: %w", m.FromVersion, m.ToVersion, err)
		}

		applied = append(applied, AppliedMigration{
			FromVersion: m.FromVersion,
			ToVersion:   m.ToVersion,
			Description: m.Description,
		})
		ver = m.ToVersion
	}

	if dryRun || len(applied) == 0 {
		return applied, nil
	}

	// Marshal updated doc
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return applied, fmt.Errorf("marshaling migrated config: %w", err)
	}

	// Create backup
	bakPath := configPath + ".bak"
	if err := os.WriteFile(bakPath, data, 0644); err != nil {
		return applied, fmt.Errorf("creating backup: %w", err)
	}

	// Atomic write
	if err := writeFileAtomic(configPath, out, 0644); err != nil {
		return applied, fmt.Errorf("writing migrated config: %w", err)
	}

	return applied, nil
}

// detectVersion extracts the version field from raw YAML bytes.
// Returns "1.0" if no version is found (assume oldest).
func detectVersion(data []byte) string {
	var doc struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil || doc.Version == "" {
		return "1.0"
	}
	return doc.Version
}

// writeFileAtomic writes data to a file atomically via tmp+rename.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}

	// Sync to ensure data is flushed
	if f, err := os.OpenFile(tmpPath, os.O_RDWR, 0); err == nil {
		syncErr := f.Sync()
		closeErr := f.Close()
		if syncErr != nil {
			os.Remove(tmpPath)
			return syncErr
		}
		if closeErr != nil {
			os.Remove(tmpPath)
			return closeErr
		}
	}

	// On Windows, remove target first
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}

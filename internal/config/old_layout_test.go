package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// A project whose configuration is still where an earlier awkit kept it is
// not a project without one. Nothing is read from the earlier place: the
// error says what to do, and it is not the error of a file that is not there,
// so that a command that goes on without a configuration does not go on here.
func TestLoad_AProjectThatHasNotBeenUpgraded(t *testing.T) {
	root := t.TempDir()
	earlier := filepath.Join(root, ".ai", "config", "workflow.yaml")
	if err := os.MkdirAll(filepath.Dir(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(earlier, []byte("project: {name: at-the-earlier-place}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, load := range map[string]func() (*Config, error){
		"Load":     func() (*Config, error) { return Load(root) },
		"LoadFile": func() (*Config, error) { return LoadFile(Path(root)) },
	} {
		cfg, err := load()
		var old *OldLayoutError
		if cfg != nil || !errors.As(err, &old) {
			t.Fatalf("%s: read as %+v, err %v (%T); want an *OldLayoutError and no configuration", name, cfg, err, err)
		}
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: the project is taken for one without a configuration: %v", name, err)
		}
		if old.Root != root || old.Found != earlier {
			t.Errorf("%s: the error names %q in %q, want %q in %q", name, old.Found, old.Root, earlier, root)
		}
		for _, want := range []string{"awkit upgrade", layout.OldConfigFile, layout.ConfigFile} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: the error does not mention %q: %v", name, want, err)
			}
		}
	}

	// A file that was named by itself has no earlier place: it is not there,
	// and that is all.
	_, err := LoadFile(filepath.Join(root, ".ai", "another.yaml"))
	var old *OldLayoutError
	if !os.IsNotExist(err) || errors.As(err, &old) {
		t.Errorf("a file named by itself: err = %v, want one that says it is not there", err)
	}

	// With the configuration at its place, the earlier file is not looked at.
	if err := os.WriteFile(Path(root), []byte("project: {name: at-its-place}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil || cfg.Project.Name != "at-its-place" {
		t.Errorf("with both files: %+v, err %v; want the one at its place", cfg, err)
	}
}

// A directory with no configuration at either place is a project without
// one, as before.
func TestLoad_NoConfigurationAtEitherPlace(t *testing.T) {
	_, err := Load(t.TempDir())
	var old *OldLayoutError
	if !os.IsNotExist(err) || errors.As(err, &old) {
		t.Errorf("err = %v, want one that says the file is not there", err)
	}
}

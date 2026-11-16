package kickoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
)

// The two ways a configuration can fail are told apart, because what a
// person does about them differs: a file that is not there is created with
// `awkit init`, a file that cannot be used is corrected.
func TestLoadConfig_SaysWhyTheFileCannotBeUsed(t *testing.T) {
	var parse *config.ParseError

	_, err := LoadConfig(filepath.Join(t.TempDir(), "workflow.yaml"))
	if err == nil || !strings.HasPrefix(err.Error(), "failed to read config file: ") || !errors.Is(err, os.ErrNotExist) || errors.As(err, &parse) {
		t.Errorf("a file that is not there: err = %v, want one that says it could not be read", err)
	}

	for name, content := range map[string]string{
		"not YAML":                  "project: [unclosed\n",
		"a value of the wrong kind": "timeouts: {gh_seconds: soon}\n",
	} {
		path := filepath.Join(t.TempDir(), "workflow.yaml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(path)
		if err == nil || cfg != nil {
			t.Errorf("%s: read as %+v, err %v; want an error and no configuration", name, cfg, err)
			continue
		}
		if !strings.HasPrefix(err.Error(), "failed to parse config file: ") || !strings.Contains(err.Error(), "workflow.yaml is not valid: ") {
			t.Errorf("%s: err = %v, want one that says the file is there and not valid", name, err)
		}
		if !errors.As(err, &parse) || errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: err = %v (%T), want one that carries a *config.ParseError", name, err, err)
		}
	}
}

package install

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"gopkg.in/yaml.v3"
)

// ruleFile reads a rule file of a project, or returns "" when there is none.
func ruleFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(layout.Rules(dir), name+".md"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The godot-go preset knows its stack, so its projects start with rules for
// it. The generic preset assumes no stack and has none to give.
func TestStarterRules_PerPreset(t *testing.T) {
	if got, want := StarterRules(PresetGodotGo), []string{"backend", "frontend", "testing"}; !reflect.DeepEqual(got, want) {
		t.Errorf("StarterRules(godot-go) = %v, want %v", got, want)
	}
	for _, preset := range []string{PresetGeneric, "", "no-such-preset"} {
		if got := StarterRules(preset); len(got) != 0 {
			t.Errorf("StarterRules(%q) = %v, want none", preset, got)
		}
	}
}

// A new project gets the rule files, and its configuration lists exactly
// those: a listed rule that is not there stops the Worker, and a rule that is
// there and not listed is never shown to it.
func TestInstall_ANewProjectStartsWithTheRulesOfItsPreset(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(createMinimalMockFS(), dir, Options{Preset: PresetGodotGo, ProjectName: "demo", NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	for _, name := range StarterRules(PresetGodotGo) {
		want, err := starterRule(PresetGodotGo, name)
		if err != nil {
			t.Fatal(err)
		}
		if got := ruleFile(t, dir, name); got != string(want) {
			t.Errorf(".claude/rules/%s.md is not the preset's starter rule (%d bytes, want %d)", name, len(got), len(want))
		}
	}
	if got, want := installedConfig(t, PresetGodotGo).Rules.Custom, StarterRules(PresetGodotGo); !reflect.DeepEqual(got, want) {
		t.Errorf("rules.custom = %v, want the starter rules %v", got, want)
	}

	generic := t.TempDir()
	if _, err := Install(createMinimalMockFS(), generic, Options{Preset: PresetGeneric, NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	entries, _ := os.ReadDir(layout.Rules(generic))
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("the generic preset wrote the rule file %s", entry.Name())
		}
	}
}

// A rule file is the project's own from the moment it is written. Another
// init keeps it, forced or not; so does an upgrade, which writes none.
func TestInstall_AStarterRuleIsNeverReplaced(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(layout.Rules(dir), "backend.md")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("the project's rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, opts := range []Options{
		{Preset: PresetGodotGo, NoGenerate: true},
		{Preset: PresetGodotGo, NoGenerate: true, Force: true},
		{Preset: PresetGodotGo, NoGenerate: true, ForceConfig: true},
	} {
		if _, err := Install(createMinimalMockFS(), dir, opts); err != nil {
			t.Fatalf("Install(%+v): %v", opts, err)
		}
		if got := ruleFile(t, dir, "backend"); got != "the project's rule" {
			t.Fatalf("Install(%+v) replaced the project's rule: %q", opts, got)
		}
	}
	// The ones the project did not have were written beside it.
	if ruleFile(t, dir, "frontend") == "" || ruleFile(t, dir, "testing") == "" {
		t.Error("the rules the project did not have were not written")
	}
}

// The rules go with the configuration that lists them. A project that keeps
// the configuration it has (an upgrade, or an init over an existing project)
// gets no rule file: its configuration would not name it.
func TestInstall_NoStarterRulesWithoutThePresetsConfiguration(t *testing.T) {
	upgraded := t.TempDir()
	if _, err := Install(createMinimalMockFS(), upgraded, Options{Preset: PresetGodotGo, NoGenerate: true, SkipConfig: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := ruleFile(t, upgraded, "backend"); got != "" {
		t.Errorf("an upgrade wrote a starter rule: %q", got)
	}

	existing := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(layout.Config(existing)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.Config(existing), []byte("project: {name: demo}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(createMinimalMockFS(), existing, Options{Preset: PresetGodotGo, NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := ruleFile(t, existing, "backend"); got != "" {
		t.Errorf("an init that kept the project's configuration wrote a starter rule: %q", got)
	}
}

// A starter rule says which files it is about (`paths:`), so that it is read
// when those files are worked on and not otherwise. Every path it names is
// inside a repo of the preset's configuration.
func TestStarterRules_AreAboutTheReposOfTheirPreset(t *testing.T) {
	for _, preset := range Presets {
		var cfg presetConfig
		if err := yaml.Unmarshal(presetConfigs[preset]("demo"), &cfg); err != nil {
			t.Fatalf("preset %q: %v", preset, err)
		}
		for _, name := range StarterRules(preset) {
			data, err := starterRule(preset, name)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(data), "\r\n", "\n")
			rest, found := strings.CutPrefix(text, "---\n")
			head, body, closed := strings.Cut(rest, "\n---\n")
			if !found || !closed {
				t.Errorf("%s/%s: no frontmatter block", preset, name)
				continue
			}
			var meta struct {
				Paths []string `yaml:"paths"`
			}
			if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
				t.Errorf("%s/%s: the frontmatter is not valid YAML: %v", preset, name, err)
				continue
			}
			if len(meta.Paths) == 0 {
				t.Errorf("%s/%s: no paths: the rule would be loaded into every session, the Principal's included", preset, name)
			}
			for _, pattern := range meta.Paths {
				inRepo := false
				for _, repo := range cfg.Repos {
					if strings.HasPrefix(pattern, repo.Path) {
						inRepo = true
					}
				}
				if !inRepo {
					t.Errorf("%s/%s: the path %q is in no repo of the preset", preset, name, pattern)
				}
			}
			if !strings.HasPrefix(strings.TrimLeft(body, "\n"), "# ") {
				t.Errorf("%s/%s: the rule does not start with a title", preset, name)
			}
		}
	}
}

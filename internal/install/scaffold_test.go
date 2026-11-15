package install

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// scaffolded writes the godot-go skeleton into a fresh directory.
func scaffolded(t *testing.T, projectName string) (dir string, result *ScaffoldResult) {
	t.Helper()
	dir = t.TempDir()
	result, err := Scaffold(dir, ScaffoldOptions{Preset: PresetGodotGo, ProjectName: projectName})
	if err != nil {
		t.Fatalf("Scaffold: %v (failed: %v, errors: %v)", err, result.Failed, result.Errors)
	}
	return dir, result
}

// filesUnder lists the files below dir, slash-separated and sorted.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestScaffold_GodotGoWritesTheSkeleton(t *testing.T) {
	dir, result := scaffolded(t, "My Game")

	want := []string{
		"backend/README.md",
		"backend/go.mod",
		"backend/health/health.go",
		"backend/health/health_test.go",
		"backend/main.go",
		"frontend/.gitignore",
		"frontend/Directory.Build.props",
		"frontend/Frontend.sln",
		"frontend/README.md",
		"frontend/game/Game.csproj",
		"frontend/game/project.godot",
		"frontend/src/Game.Core/Counter.cs",
		"frontend/src/Game.Core/Game.Core.csproj",
		"frontend/tests/Game.Core.Tests/CounterTests.cs",
		"frontend/tests/Game.Core.Tests/Game.Core.Tests.csproj",
	}
	got := filesUnder(t, dir)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("skeleton files:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(result.Created) != len(want) || len(result.Skipped) != 0 {
		t.Errorf("created %d, skipped %d; want %d created", len(result.Created), len(result.Skipped), len(want))
	}

	for _, file := range got {
		content := readFile(t, dir, filepath.FromSlash(file))
		if strings.Contains(content, "{{") {
			t.Errorf("%s still has a placeholder", file)
		}
	}
	if mod := readFile(t, dir, "backend", "go.mod"); !strings.HasPrefix(mod, "module my-game/backend\n") {
		t.Errorf("go.mod = %q, want the module named after the project", mod)
	}
	if main := readFile(t, dir, "backend", "main.go"); !strings.Contains(main, `"my-game/backend/health"`) {
		t.Errorf("main.go must import the health package of the same module:\n%s", main)
	}
	if godot := readFile(t, dir, "frontend", "game", "project.godot"); !strings.Contains(godot, `config/name="My Game"`) ||
		!strings.Contains(godot, `"C#"`) || !strings.Contains(godot, `project/assembly_name="Game"`) {
		t.Errorf("project.godot should name the project and be a C# project:\n%s", godot)
	}
	if csproj := readFile(t, dir, "frontend", "game", "Game.csproj"); !strings.Contains(csproj, `Sdk="Godot.NET.Sdk/`) ||
		!strings.Contains(csproj, "../src/Game.Core/Game.Core.csproj") {
		t.Errorf("the Godot project should use the Godot SDK and reference the core library:\n%s", csproj)
	}
	if core := readFile(t, dir, "frontend", "src", "Game.Core", "Game.Core.csproj"); strings.Contains(core, "Godot") {
		t.Errorf("Game.Core must not depend on the engine, so it can be tested without it:\n%s", core)
	}
}

// The Go half of the skeleton has no dependencies, so it can be built and
// tested right here. The C# half needs NuGet and is exercised in CI.
func TestScaffold_BackendBuildsAndPassesItsTests(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on the PATH")
	}
	dir, _ := scaffolded(t, "demo")
	backend := filepath.Join(dir, "backend")

	for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}, {"test", "./..."}} {
		cmd := exec.Command(goBin, args...)
		cmd.Dir = backend
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s in the scaffolded backend: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func TestScaffold_PresetsWithoutASkeleton(t *testing.T) {
	for _, preset := range []string{PresetGeneric, ""} {
		dir := t.TempDir()
		result, err := Scaffold(dir, ScaffoldOptions{Preset: preset, ProjectName: "demo"})
		if !errors.Is(err, ErrNoScaffold) {
			t.Errorf("Scaffold(%q) error = %v, want ErrNoScaffold", preset, err)
		}
		if result == nil {
			t.Fatalf("Scaffold(%q) returned no result", preset)
		}
		if files := filesUnder(t, dir); len(files) != 0 {
			t.Errorf("Scaffold(%q) wrote %v", preset, files)
		}
	}

	_, err := Scaffold(t.TempDir(), ScaffoldOptions{Preset: "react-go", ProjectName: "demo"})
	if !errors.Is(err, ErrUnknownPreset) || !strings.Contains(err.Error(), PresetGodotGo) {
		t.Errorf("a removed preset should be reported as unknown, with the available ones named; got %v", err)
	}
}

func TestScaffold_KeepsExistingFilesUnlessForced(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "backend", "go.mod")
	if err := os.MkdirAll(filepath.Dir(goMod), 0o755); err != nil {
		t.Fatal(err)
	}
	const own = "module example.com/mine\n\ngo 1.25\n"
	if err := os.WriteFile(goMod, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Scaffold(dir, ScaffoldOptions{Preset: PresetGodotGo, ProjectName: "demo"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if got := readFile(t, goMod); got != own {
		t.Errorf("an existing file was overwritten without force:\n%s", got)
	}
	if len(result.Skipped) != 1 || filepath.Clean(result.Skipped[0]) != filepath.Clean(goMod) {
		t.Errorf("Skipped = %v, want only the existing go.mod", result.Skipped)
	}
	if _, err := os.Stat(filepath.Join(dir, "frontend", "Frontend.sln")); err != nil {
		t.Errorf("the missing files should still be written: %v", err)
	}

	result, err = Scaffold(dir, ScaffoldOptions{Preset: PresetGodotGo, ProjectName: "demo", Force: true})
	if err != nil {
		t.Fatalf("Scaffold with force: %v", err)
	}
	if got := readFile(t, goMod); !strings.HasPrefix(got, "module demo/backend\n") {
		t.Errorf("force should replace the file, got:\n%s", got)
	}
	if len(result.Skipped) != 0 {
		t.Errorf("force skips nothing, got %v", result.Skipped)
	}
}

func TestScaffold_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "backend", "main.go")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Scaffold(dir, ScaffoldOptions{Preset: PresetGodotGo, ProjectName: "demo", DryRun: true})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if files := filesUnder(t, dir); len(files) != 1 {
		t.Errorf("a dry run must not write anything, found %v", files)
	}
	if len(result.Created) != 14 || len(result.Skipped) != 1 {
		t.Errorf("dry run reports %d to create and %d to skip, want 14 and 1", len(result.Created), len(result.Skipped))
	}
}

func TestScaffold_RejectsAnUnsafeProjectName(t *testing.T) {
	dir := t.TempDir()
	_, err := Scaffold(dir, ScaffoldOptions{Preset: PresetGodotGo, ProjectName: `demo"; rm -rf`})
	if !errors.Is(err, ErrInvalidProjectName) {
		t.Errorf("error = %v, want ErrInvalidProjectName", err)
	}
	if files := filesUnder(t, dir); len(files) != 0 {
		t.Errorf("nothing should be written for a rejected name, found %v", files)
	}
}

func TestModuleSlug(t *testing.T) {
	tests := map[string]string{
		"demo":             "demo",
		"My Game":          "my-game",
		"九霄 Game":          "game",
		"a__b.c":           "a__b.c",
		"  spaced  out  ":  "spaced-out",
		"UPPER/lower\\mix": "upper-lower-mix",
		"...":              "app",
		"":                 "app",
		"九霄":               "app",
	}
	for name, want := range tests {
		if got := moduleSlug(name); got != want {
			t.Errorf("moduleSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestScaffoldVars_UnsafeDirectoryNameIsNotWrittenVerbatim(t *testing.T) {
	// The name can come from a directory, which is not validated.
	vars := scaffoldVars(`we"ird`)
	if vars["{{PROJECT_NAME}}"] != "we-ird" {
		t.Errorf("display name = %q, want the slug: a quote would break project.godot", vars["{{PROJECT_NAME}}"])
	}
	if vars["{{BACKEND_MODULE}}"] != "we-ird/backend" {
		t.Errorf("module = %q", vars["{{BACKEND_MODULE}}"])
	}

	safe := scaffoldVars("My Game")
	if safe["{{PROJECT_NAME}}"] != "My Game" || safe["{{BACKEND_MODULE}}"] != "my-game/backend" {
		t.Errorf("vars = %v, want the name kept for display and slugged for the module", safe)
	}
}

func TestScaffoldFS_EveryFileIsATemplate(t *testing.T) {
	count := 0
	err := fs.WalkDir(scaffoldFS, "scaffold", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		count++
		if !strings.HasSuffix(name, scaffoldSuffix) {
			t.Errorf("%s lacks the %s suffix: Go would compile it or refuse to embed it", name, scaffoldSuffix)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no scaffold files are embedded")
	}
	for preset, root := range scaffoldRoots {
		if _, err := fs.Stat(scaffoldFS, root); err != nil {
			t.Errorf("preset %q points at %s, which is not embedded: %v", preset, root, err)
		}
	}
}

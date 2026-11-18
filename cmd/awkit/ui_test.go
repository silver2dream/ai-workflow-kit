package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/workbench"
)

// uiRun runs `awkit ui` with the serving played by a function: it records the
// address and whether a browser was opened, and does not wait.
func uiRun(t *testing.T, args ...string) (code int, url string, opened bool, settings workbench.Settings) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "workbench.json")
	t.Setenv(workbench.SettingsEnv, file)
	serve, browse := serveWorkbench, openBrowser
	t.Cleanup(func() { serveWorkbench, openBrowser = serve, browse })
	openBrowser = func(string) error { opened = true; return nil }
	serveWorkbench = func(ctx context.Context, server *workbench.Server, port int, ready func(string)) error {
		url = "http://127.0.0.1:4747/?token=" + server.Token()
		ready(url)
		return nil
	}
	code = cmdUI(args)
	settings, _ = workbench.NewStore(file).Load()
	return code, url, opened, settings
}

func TestUI(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".ai", "config.yaml"), []byte("project: {name: rally}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Started in a project: it is added, and the browser opens the address.
	t.Chdir(project)
	code, url, opened, settings := uiRun(t)
	if code != 0 || !strings.Contains(url, "?token=") || !opened {
		t.Errorf("code %d, url %q, opened %v", code, url, opened)
	}
	if len(settings.Projects) != 1 || settings.Projects[0].Path != project {
		t.Errorf("projects = %+v; want the one it started in", settings.Projects)
	}

	// Started elsewhere: nothing is added, and that is no error.
	t.Chdir(t.TempDir())
	if code, _, _, settings := uiRun(t, "--no-open"); code != 0 || len(settings.Projects) != 0 {
		t.Errorf("started outside a project: code %d, projects %+v", code, settings.Projects)
	}
	// --no-open opens nothing.
	if _, _, opened, _ := uiRun(t, "--no-open"); opened {
		t.Error("--no-open opened a browser")
	}
	// A folder that is named is added; one that is no project is refused.
	if code, _, _, settings := uiRun(t, "--no-open", project); code != 0 || len(settings.Projects) != 1 {
		t.Errorf("a named project: code %d, projects %+v", code, settings.Projects)
	}
	if code, _, _, _ := uiRun(t, "--no-open", t.TempDir()); code != 1 {
		t.Errorf("a named folder that is no project: code %d, want 1", code)
	}
	if code, _, _, _ := uiRun(t, "a", "b"); code != 2 {
		t.Errorf("two folders: code %d, want 2", code)
	}
	if code, _, _, _ := uiRun(t, "--help"); code != 0 {
		t.Errorf("--help: code %d", code)
	}
}

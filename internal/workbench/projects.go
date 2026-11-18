package workbench

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// ProjectView is a project as the workbench lists it.
type ProjectView struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
	// Problem says why the project cannot be opened now: its folder is gone,
	// or its configuration cannot be read. Empty when it can.
	Problem string `json:"problem,omitempty"`
}

// projectID names a project in the workbench's addresses: a short hash of its
// folder, so that the path itself is not in a URL. Windows does not tell the
// case of a path apart, so neither does the hash there.
func projectID(path string) string {
	key := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:6])
}

// ResolveProject turns a folder a person names into the root of an AWK
// project: the folder itself, made absolute, when it has the project's
// configuration.
func ResolveProject(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("name the folder of the project")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%s cannot be opened: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is a file, not a folder", abs)
	}
	if _, err := os.Stat(layout.Config(abs)); err != nil {
		return "", fmt.Errorf("%s is not an AWK project: it has no %s (run `awkit init` in it first)", abs, layout.ConfigFile)
	}
	return abs, nil
}

// view describes a project the person added.
func view(project Project) ProjectView {
	v := ProjectView{ID: projectID(project.Path), Path: project.Path, Name: filepath.Base(project.Path)}
	cfg, err := config.Load(project.Path)
	switch {
	case err != nil && errors.Is(err, os.ErrNotExist):
		v.Problem = fmt.Sprintf("the project is gone, or no longer has %s", layout.ConfigFile)
	case err != nil:
		v.Problem = fmt.Sprintf("its configuration cannot be read: %v", err)
	case cfg.Project.Name != "":
		v.Name = cfg.Project.Name
	}
	return v
}

// addProject keeps a project in the settings, once; a project already there
// keeps its place.
func addProject(settings *Settings, root string, now time.Time) {
	for _, p := range settings.Projects {
		if projectID(p.Path) == projectID(root) {
			return
		}
	}
	settings.Projects = append(settings.Projects, Project{Path: root, AddedAt: now})
}

// removeProject takes a project out of the settings. Its folder is not
// touched.
func removeProject(settings *Settings, id string) bool {
	for i, p := range settings.Projects {
		if projectID(p.Path) == id {
			settings.Projects = append(settings.Projects[:i], settings.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// findProject returns the root of a project the settings hold, by its id.
func findProject(settings Settings, id string) (string, bool) {
	for _, p := range settings.Projects {
		if projectID(p.Path) == id {
			return p.Path, true
		}
	}
	return "", false
}

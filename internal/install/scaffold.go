package install

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// scaffoldFS holds the project skeleton of each preset that has one, as real
// files under scaffold/<preset>/. Every file carries the .tmpl suffix: without
// it the Go sources among them would be compiled as packages of this module,
// and a go.mod cannot be embedded at all.
//
//go:embed all:scaffold
var scaffoldFS embed.FS

const scaffoldSuffix = ".tmpl"

// scaffoldRoots maps a preset to the directory of its skeleton in scaffoldFS.
var scaffoldRoots = map[string]string{
	PresetGodotGo: "scaffold/godot-go",
}

// HasScaffold reports whether a preset comes with a project skeleton.
func HasScaffold(preset string) bool {
	_, ok := scaffoldRoots[presetName(preset)]
	return ok
}

// ScaffoldPresets lists the presets that come with a project skeleton, in
// the order presets are shown.
func ScaffoldPresets() []string {
	var names []string
	for _, name := range Presets {
		if HasScaffold(name) {
			names = append(names, name)
		}
	}
	return names
}

// Scaffold writes the project skeleton of a preset into targetDir. Existing
// files are kept unless Force is set.
func Scaffold(targetDir string, opts ScaffoldOptions) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	if opts.ProjectName != "" {
		if err := validateProjectName(opts.ProjectName); err != nil {
			return result, err
		}
	}
	projectName := opts.ProjectName
	if projectName == "" {
		projectName = filepath.Base(targetDir)
	}

	preset := presetName(opts.Preset)
	if _, known := presetConfigs[preset]; !known {
		return result, unknownPreset(opts.Preset)
	}
	root, ok := scaffoldRoots[preset]
	if !ok {
		return result, fmt.Errorf("%w: %q", ErrNoScaffold, preset)
	}

	files, err := scaffoldFilesFor(root, targetDir, scaffoldVars(projectName))
	if err != nil {
		return result, err
	}
	return scaffoldFiles(files, opts.Force, opts.DryRun, result)
}

// scaffoldVars are the placeholders a skeleton file may use.
func scaffoldVars(projectName string) map[string]string {
	display := projectName
	if validateProjectName(display) != nil {
		// A name taken from a directory can hold characters that would break
		// the files it is written into.
		display = moduleSlug(projectName)
	}
	return map[string]string{
		"{{PROJECT_NAME}}":   display,
		"{{BACKEND_MODULE}}": moduleSlug(projectName) + "/backend",
	}
}

// moduleSlug turns a project name into something usable as an element of a Go
// module path.
func moduleSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-.")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		return "app"
	}
	return slug
}

// scaffoldFilesFor reads a skeleton from scaffoldFS and returns the files to
// write: placeholders replaced, the .tmpl suffix dropped, paths under
// targetDir.
func scaffoldFilesFor(root, targetDir string, vars map[string]string) ([]struct {
	path    string
	content []byte
}, error) {
	var pairs []string
	for placeholder, value := range vars {
		pairs = append(pairs, placeholder, value)
	}
	replacer := strings.NewReplacer(pairs...)

	var files []struct {
		path    string
		content []byte
	}
	err := fs.WalkDir(scaffoldFS, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(name, scaffoldSuffix) {
			return fmt.Errorf("scaffold file %s lacks the %s suffix", name, scaffoldSuffix)
		}
		data, err := scaffoldFS.ReadFile(name)
		if err != nil {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(name, root+"/"), scaffoldSuffix)
		if rel == "" || strings.HasPrefix(path.Clean(rel), "..") {
			return fmt.Errorf("scaffold file %s has no usable target path", name)
		}
		files = append(files, struct {
			path    string
			content []byte
		}{
			path:    filepath.Join(targetDir, filepath.FromSlash(rel)),
			content: []byte(replacer.Replace(string(data))),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

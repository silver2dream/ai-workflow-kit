package install

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/generate"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// Uninstall removes AWK from the project at targetDir and returns what it
// removed, relative to the project and with forward slashes. With dryRun it
// removes nothing and returns what it would remove.
//
// It removes what AWK put there, and nothing else: the kit's rules, skills
// and subagents, the subagents generated from the configuration, what the
// runs left behind, the Workers' worktrees, and the configuration and the
// roster unless keepConfig. What the project keeps under .claude itself
// stays; so do CLAUDE.md and AGENTS.md, which a project may have made its
// own, and the settings of this machine.
func Uninstall(kit fs.FS, targetDir string, keepConfig, dryRun bool) ([]string, error) {
	var removed, emptied []string
	remove := func(location string, all bool) error {
		path := layout.In(targetDir, location)
		if _, err := os.Lstat(path); err != nil {
			return nil
		}
		removed = append(removed, location)
		emptied = append(emptied, parents(location)...)
		if dryRun {
			return nil
		}
		if all {
			return os.RemoveAll(path)
		}
		return os.Remove(path)
	}

	// The kit's files, at the paths the kit has them.
	var kitFiles []string
	err := fs.WalkDir(kit, KitRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			kitFiles = append(kitFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, file := range kitFiles {
		if err := remove(file, false); err != nil {
			return removed, err
		}
	}

	// The subagents the generator wrote for the custom agents of the
	// configuration. A subagent without its mark is the project's own.
	agents, _ := os.ReadDir(layout.Agents(targetDir))
	for _, agent := range agents {
		if agent.IsDir() || !strings.HasSuffix(agent.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(layout.Agents(targetDir), agent.Name()))
		if err == nil && strings.Contains(string(content), generate.GeneratedAgentMark) {
			if err := remove(layout.AgentsDir+"/"+agent.Name(), false); err != nil {
				return removed, err
			}
		}
	}

	for _, dir := range []string{layout.StateDir, ".worktrees"} {
		if err := remove(dir, true); err != nil {
			return removed, err
		}
	}
	if !keepConfig {
		for _, file := range []string{layout.ConfigFile, layout.RosterFile} {
			if err := remove(file, false); err != nil {
				return removed, err
			}
		}
	}

	// A directory that held only what was removed goes too, the deepest
	// first. One that still holds something is the project's.
	if !dryRun {
		sort.Slice(emptied, func(i, j int) bool { return strings.Count(emptied[i], "/") > strings.Count(emptied[j], "/") })
		for _, dir := range emptied {
			_ = os.Remove(layout.In(targetDir, dir)) // fails, rightly, when it is not empty
		}
	}
	return removed, nil
}

// parents lists the directories above a location, up to the project itself.
func parents(location string) []string {
	var dirs []string
	for dir := path.Dir(location); dir != "." && dir != "/"; dir = path.Dir(dir) {
		dirs = append(dirs, dir)
	}
	return dirs
}

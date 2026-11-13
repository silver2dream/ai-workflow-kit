package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// LayoutStep is one thing MoveLayout did, or would do, to a project.
type LayoutStep struct {
	// From and To are relative to the project, with forward slashes. To is
	// empty when From is removed, and both are empty for a remark.
	From, To string
	// Note says why, where it is not plain from the paths.
	Note string
}

func (s LayoutStep) String() string {
	switch {
	case s.From == "":
		return s.Note
	case s.To == "":
		return fmt.Sprintf("remove %s (%s)", s.From, s.Note)
	case s.Note != "":
		return fmt.Sprintf("%s -> %s (%s)", s.From, s.To, s.Note)
	default:
		return fmt.Sprintf("%s -> %s", s.From, s.To)
	}
}

// oldKitSkills are the skills an earlier awkit installed under .ai/skills.
// They were the kit's, and an upgrade installs the kit anew at the new place,
// so they are removed, not moved. Any other skill there is the project's.
var oldKitSkills = []string{"principal-workflow", "post-mortem", "release-checklist"}

// oldRuntimeDirs are the directories a run wrote to, and where each one's
// content is kept now. All of it is under the state directory.
var oldRuntimeDirs = []struct{ from, to string }{
	{layout.OldResultsDir, layout.ResultsDir},
	{layout.OldRunsDir, layout.RunsDir},
	{layout.OldLogsDir, layout.LogsDir},
	{".ai/logs", layout.LogsDir},
	{layout.OldTempDir, layout.TempDir},
}

// UsesOldLayout reports whether the project at root still keeps files where
// an earlier awkit put them.
func UsesOldLayout(root string) bool {
	old := []string{layout.OldConfigFile, layout.OldRulesDir, layout.OldSkillsDir}
	for _, dir := range oldRuntimeDirs {
		old = append(old, dir.from)
	}
	for _, location := range old {
		if _, err := os.Lstat(layout.In(root, location)); err == nil {
			return true
		}
	}
	return false
}

// MoveLayout moves the files of the project at root from where an earlier
// awkit kept them to where they are kept now, and returns what it did. With
// dryRun it changes nothing and returns what it would do.
//
// What is the project's own is moved: its configuration, the rules and
// skills it added, what its runs left behind. What was the kit's is removed:
// the upgrade that calls this installs the kit at the new place.
//
// It can be run again after it failed half way: what has been moved is no
// longer there to move.
func MoveLayout(root string, dryRun bool) ([]LayoutStep, error) {
	m := &layoutMove{root: root, dryRun: dryRun}

	if err := m.config(); err != nil {
		return m.steps, err
	}
	// .claude/rules and .claude/skills were links to .ai/rules and .ai/skills,
	// or copies where a link could not be made. They become the real thing,
	// so a link goes first: moving into it would move the files onto
	// themselves.
	for _, dir := range []string{layout.RulesDir, layout.SkillsDir, ".claude/commands"} {
		if err := m.unlink(dir); err != nil {
			return m.steps, err
		}
	}
	if err := m.rules(); err != nil {
		return m.steps, err
	}
	if err := m.skills(); err != nil {
		return m.steps, err
	}
	for _, dir := range oldRuntimeDirs {
		if err := m.merge(dir.from, dir.to, "what runs left behind"); err != nil {
			return m.steps, err
		}
	}
	// Commands were replaced by skills long ago; installs removed this
	// directory every time, and it is not part of a project any more.
	if err := m.remove(".ai/commands", "replaced by skills"); err != nil {
		return m.steps, err
	}
	m.leftovers()
	return m.steps, nil
}

type layoutMove struct {
	root   string
	dryRun bool
	steps  []LayoutStep
}

func (m *layoutMove) path(location string) string { return layout.In(m.root, location) }

func (m *layoutMove) note(step LayoutStep) { m.steps = append(m.steps, step) }

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// isDir reports whether path is a directory of its own: not a link to one.
func isDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

// config moves the configuration, and removes the schema the kit kept next
// to it: nothing reads that file.
func (m *layoutMove) config() error {
	from, to := m.path(layout.OldConfigFile), m.path(layout.ConfigFile)
	if exists(from) {
		if exists(to) {
			return fmt.Errorf("both %s and %s are there: awkit cannot tell which one is the project's configuration. Remove the other one, then run `awkit upgrade` again",
				layout.ConfigFile, layout.OldConfigFile)
		}
		if err := m.rename(layout.OldConfigFile, layout.ConfigFile, ""); err != nil {
			return err
		}
	}
	if err := m.remove(".ai/config/workflow.schema.json", "the kit's; nothing reads it"); err != nil {
		return err
	}
	return m.removeIfEmpty(".ai/config")
}

// unlink removes a link at a place where a directory of the project's own is
// kept now. A directory that is there is left: it is a copy, and what moves
// in writes over what it holds.
func (m *layoutMove) unlink(location string) error {
	path := m.path(location)
	info, err := os.Lstat(path)
	if err != nil || isDir(path) {
		return nil
	}
	if info.Mode().IsRegular() {
		return fmt.Errorf("%s is a file; awkit keeps a directory there. Move the file away, then run `awkit upgrade` again", location)
	}
	m.note(LayoutStep{From: location, Note: "a link to its earlier place"})
	if m.dryRun {
		return nil
	}
	// os.Remove takes a link away without following it, whatever kind it is.
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("%s is neither a directory nor a link that can be removed: %w", location, err)
	}
	return nil
}

func (m *layoutMove) rules() error {
	for _, kit := range []string{"_kit", "_examples"} {
		if err := m.remove(layout.OldRulesDir+"/"+kit, "the kit's"); err != nil {
			return err
		}
	}
	// Where .claude/rules was a copy, it holds copies of the kit's files too.
	// The examples are no longer installed into a project at all: Claude Code
	// read them as rules of the project. The kit's own rules are installed
	// anew.
	if err := m.remove(layout.RulesDir+"/_examples", "a copy of the kit's examples; they are documents of the AWK repository now"); err != nil {
		return err
	}
	if err := m.remove(layout.KitRulesDir, "a copy of the kit's; installed anew"); err != nil {
		return err
	}
	return m.merge(layout.OldRulesDir, layout.RulesDir, "the project's rules")
}

func (m *layoutMove) skills() error {
	for _, kit := range oldKitSkills {
		if err := m.remove(layout.OldSkillsDir+"/"+kit, "the kit's"); err != nil {
			return err
		}
		if err := m.remove(layout.SkillsDir+"/"+kit, "a copy of the kit's; installed anew"); err != nil {
			return err
		}
	}
	return m.merge(layout.OldSkillsDir, layout.SkillsDir, "the project's skills")
}

// leftovers says what else is under .ai. AWK keeps three things there, and
// does not remove what it does not know.
func (m *layoutMove) leftovers() {
	if m.dryRun {
		return
	}
	entries, err := os.ReadDir(m.path(".ai"))
	if err != nil {
		return
	}
	known := map[string]bool{
		filepath.Base(layout.ConfigFile): true,
		filepath.Base(layout.RosterFile): true,
		filepath.Base(layout.StateDir):   true,
	}
	var unknown []string
	for _, entry := range entries {
		if !known[entry.Name()] {
			unknown = append(unknown, ".ai/"+entry.Name())
		}
	}
	if len(unknown) > 0 {
		m.note(LayoutStep{Note: fmt.Sprintf("left as they are, awkit does not use them: %s", strings.Join(unknown, ", "))})
	}
}

// rename moves one file or directory to a place where nothing is.
func (m *layoutMove) rename(from, to, note string) error {
	m.note(LayoutStep{From: from, To: to, Note: note})
	if m.dryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.path(to)), 0o755); err != nil {
		return err
	}
	if err := os.Rename(m.path(from), m.path(to)); err != nil {
		return fmt.Errorf("cannot move %s to %s: %w", from, to, err)
	}
	return nil
}

func (m *layoutMove) remove(location, note string) error {
	if !exists(m.path(location)) {
		return nil
	}
	m.note(LayoutStep{From: location, Note: note})
	if m.dryRun {
		return nil
	}
	if err := os.RemoveAll(m.path(location)); err != nil {
		return fmt.Errorf("cannot remove %s: %w", location, err)
	}
	return nil
}

func (m *layoutMove) removeIfEmpty(location string) error {
	entries, err := os.ReadDir(m.path(location))
	if err != nil {
		return nil // not there, or not a directory
	}
	if len(entries) > 0 {
		if !m.dryRun {
			m.note(LayoutStep{Note: fmt.Sprintf("left %s as it is: it holds files awkit does not know", location)})
		}
		return nil
	}
	if m.dryRun {
		return nil
	}
	return os.Remove(m.path(location))
}

// merge moves what the directory from holds into the directory to. A file
// that is already at the new place is written over: the earlier place was
// where the file was kept, and what is at the new one is a copy made from
// it. The placeholder files that kept empty directories in git are dropped.
func (m *layoutMove) merge(from, to, note string) error {
	if !isDir(m.path(from)) {
		return nil
	}
	entries, err := os.ReadDir(m.path(from))
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	moved := 0
	for _, entry := range entries {
		source, target := from+"/"+entry.Name(), to+"/"+entry.Name()
		if entry.Name() == ".gitkeep" {
			if !m.dryRun {
				if err := os.Remove(m.path(source)); err != nil {
					return err
				}
			}
			continue
		}
		moved++
		if entry.IsDir() && isDir(m.path(target)) {
			if err := m.merge(source, target, ""); err != nil {
				return err
			}
			continue
		}
		if m.dryRun {
			continue
		}
		if err := os.MkdirAll(m.path(to), 0o755); err != nil {
			return err
		}
		if exists(m.path(target)) {
			if err := os.RemoveAll(m.path(target)); err != nil {
				return fmt.Errorf("cannot replace %s: %w", target, err)
			}
		}
		if err := os.Rename(m.path(source), m.path(target)); err != nil {
			return fmt.Errorf("cannot move %s to %s: %w", source, target, err)
		}
	}
	if note != "" && moved > 0 {
		m.note(LayoutStep{From: from, To: to, Note: note})
	}
	if m.dryRun {
		return nil
	}
	// What is left is an empty directory, or one merge could not empty.
	if left, err := os.ReadDir(m.path(from)); err == nil && len(left) == 0 {
		return os.Remove(m.path(from))
	}
	return nil
}

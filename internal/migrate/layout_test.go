package migrate

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// tree writes files below root: a path with forward slashes, and its content.
func tree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// files reads every file below root: path with forward slashes to content.
func files(t *testing.T, root string) map[string]string {
	t.Helper()
	found := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		found[filepath.ToSlash(rel)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// dirs lists every directory below root, with forward slashes.
func dirs(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			rel, _ := filepath.Rel(root, path)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(found)
	return found
}

// A project as an earlier awkit left it on a machine where links could not be
// made: the kit and the project's own rules and skills under .ai, a copy of
// both under .claude, and a directory for everything a run wrote.
func earlierProject() map[string]string {
	return map[string]string{
		".ai/config/workflow.yaml":                         "project: {name: demo}\n",
		".ai/config/workflow.schema.json":                  "{}",
		".ai/roster.yaml":                                  "roles: []\n",
		".ai/rules/_kit/git-workflow.md":                   "the kit's rule",
		".ai/rules/_examples/testing-strategy.md":          "the kit's example",
		".ai/rules/backend.md":                             "the project's rule",
		".ai/rules/areas/payments.md":                      "the project's rule, in a directory",
		".ai/skills/principal-workflow/SKILL.md":           "the kit's skill",
		".ai/skills/principal-workflow/tasks/review-pr.md": "a file the kit no longer has",
		".ai/skills/post-mortem/SKILL.md":                  "the kit's skill",
		".ai/skills/release-checklist/SKILL.md":            "the kit's skill",
		".ai/skills/release-notes/SKILL.md":                "the project's skill",
		".ai/results/issue-3.json":                         "{}",
		".ai/results/.gitkeep":                             "\n",
		".ai/runs/issue-3/summary.txt":                     "done",
		".ai/runs/.gitkeep":                                "\n",
		".ai/exe-logs/principal.log":                       "the Principal's log",
		".ai/exe-logs/.gitkeep":                            "\n",
		".ai/logs/audit.log":                               "a log of an even earlier version",
		".ai/temp/ticket-3.md":                             "a ticket",
		".ai/state/loop_count":                             "4",
		".ai/commands/start-work.md":                       "a command; skills replaced them",
		".claude/agents/pr-reviewer.md":                    "the kit's subagent",
		// The copy. The project's rule was changed under .ai after it was made.
		".claude/rules/_kit/git-workflow.md":                   "the kit's rule",
		".claude/rules/_examples/testing-strategy.md":          "the kit's example",
		".claude/rules/backend.md":                             "the project's rule, as it was when the copy was made",
		".claude/skills/principal-workflow/SKILL.md":           "the kit's skill",
		".claude/skills/principal-workflow/tasks/review-pr.md": "a file the kit no longer has",
		".claude/skills/post-mortem/SKILL.md":                  "the kit's skill",
		".claude/skills/release-checklist/SKILL.md":            "the kit's skill",
		".claude/skills/release-notes/SKILL.md":                "the project's skill",
		// The project's own, under .claude from the start.
		".claude/settings.json":      "{}",
		".claude/commands/deploy.md": "the project's command",
		"README.md":                  "# demo\n",
	}
}

// What is the project's is moved, what was the kit's is removed, and nothing
// else is touched. The kit is not installed here: the upgrade that moves a
// project does that next.
func TestMoveLayout(t *testing.T) {
	root := t.TempDir()
	tree(t, root, earlierProject())
	if !UsesOldLayout(root) {
		t.Fatal("a project of an earlier awkit is not seen as one")
	}

	steps, err := MoveLayout(root, false)
	if err != nil {
		t.Fatalf("MoveLayout: %v", err)
	}

	want := map[string]string{
		".ai/config.yaml":                       "project: {name: demo}\n",
		".ai/roster.yaml":                       "roles: []\n",
		".ai/state/loop_count":                  "4",
		".ai/state/results/issue-3.json":        "{}",
		".ai/state/runs/issue-3/summary.txt":    "done",
		".ai/state/logs/principal.log":          "the Principal's log",
		".ai/state/logs/audit.log":              "a log of an even earlier version",
		".ai/state/tmp/ticket-3.md":             "a ticket",
		".claude/agents/pr-reviewer.md":         "the kit's subagent",
		".claude/rules/backend.md":              "the project's rule",
		".claude/rules/areas/payments.md":       "the project's rule, in a directory",
		".claude/skills/release-notes/SKILL.md": "the project's skill",
		".claude/settings.json":                 "{}",
		".claude/commands/deploy.md":            "the project's command",
		"README.md":                             "# demo\n",
	}
	if got := files(t, root); !reflect.DeepEqual(got, want) {
		for rel := range got {
			if _, ok := want[rel]; !ok {
				t.Errorf("still there, or new: %s", rel)
			}
		}
		for rel, content := range want {
			if got[rel] != content {
				t.Errorf("%s = %q, want %q", rel, got[rel], content)
			}
		}
	}
	// No empty directory is left where something was.
	wantDirs := []string{".ai", ".ai/state", ".ai/state/logs", ".ai/state/results", ".ai/state/runs", ".ai/state/runs/issue-3", ".ai/state/tmp",
		".claude", ".claude/agents", ".claude/commands", ".claude/rules", ".claude/rules/areas", ".claude/skills", ".claude/skills/release-notes"}
	if got := dirs(t, root); !reflect.DeepEqual(got, wantDirs) {
		t.Errorf("directories =\n%v\nwant\n%v", got, wantDirs)
	}

	said := make([]string, len(steps))
	for i, step := range steps {
		said[i] = step.String()
	}
	for _, line := range []string{
		".ai/config/workflow.yaml -> .ai/config.yaml",
		"remove .ai/config/workflow.schema.json (the kit's; nothing reads it)",
		"remove .ai/rules/_kit (the kit's)",
		"remove .ai/rules/_examples (the kit's)",
		".ai/rules -> .claude/rules (the project's rules)",
		"remove .ai/skills/principal-workflow (the kit's)",
		".ai/skills -> .claude/skills (the project's skills)",
		".ai/results -> .ai/state/results (what runs left behind)",
		".ai/runs -> .ai/state/runs (what runs left behind)",
		".ai/exe-logs -> .ai/state/logs (what runs left behind)",
		".ai/logs -> .ai/state/logs (what runs left behind)",
		".ai/temp -> .ai/state/tmp (what runs left behind)",
		"remove .ai/commands (replaced by skills)",
	} {
		if !contains(said, line) {
			t.Errorf("the steps do not say %q:\n%s", line, strings.Join(said, "\n"))
		}
	}

	// Moved once, there is nothing left to move.
	if UsesOldLayout(root) {
		t.Error("the project is still seen as one of an earlier awkit")
	}
	again, err := MoveLayout(root, false)
	if err != nil || len(again) != 0 {
		t.Errorf("a second move did something: %v (err %v)", again, err)
	}
}

func stepsOf(steps []LayoutStep) []string {
	said := make([]string, len(steps))
	for i, step := range steps {
		said[i] = step.String()
	}
	return said
}

func contains(list []string, item string) bool {
	for _, each := range list {
		if each == item {
			return true
		}
	}
	return false
}

// A dry run says what would be done and does none of it.
func TestMoveLayout_DryRun(t *testing.T) {
	root := t.TempDir()
	tree(t, root, earlierProject())
	before, beforeDirs := files(t, root), dirs(t, root)

	steps, err := MoveLayout(root, true)

	if err != nil || len(steps) == 0 {
		t.Fatalf("a dry run said nothing: %v (err %v)", steps, err)
	}
	if after := files(t, root); !reflect.DeepEqual(after, before) {
		t.Error("a dry run changed files")
	}
	if after := dirs(t, root); !reflect.DeepEqual(after, beforeDirs) {
		t.Error("a dry run changed directories")
	}
	done, err := MoveLayout(root, false)
	if err != nil {
		t.Fatal(err)
	}
	// It says what the move then does. The remark about what is left under
	// .ai can only be made once the move is done.
	var movesOf = func(steps []LayoutStep) []string {
		var said []string
		for _, step := range steps {
			if step.From != "" {
				said = append(said, step.String())
			}
		}
		return said
	}
	if !reflect.DeepEqual(movesOf(steps), movesOf(done)) {
		t.Errorf("the dry run said\n%v\nthe move did\n%v", movesOf(steps), movesOf(done))
	}
}

// Where .claude/rules and .claude/skills were links, the links go and the
// project's files take their place. A link is never followed: removing what
// it points to would remove the files that are about to be moved.
func TestMoveLayout_LinksBecomeDirectories(t *testing.T) {
	root := t.TempDir()
	tree(t, root, map[string]string{
		".ai/config/workflow.yaml":               "project: {name: demo}\n",
		".ai/rules/_kit/git-workflow.md":         "the kit's rule",
		".ai/rules/backend.md":                   "the project's rule",
		".ai/skills/principal-workflow/SKILL.md": "the kit's skill",
		".ai/skills/release-notes/SKILL.md":      "the project's skill",
		".ai/commands/start-work.md":             "a command",
		".claude/agents/pr-reviewer.md":          "the kit's subagent",
	})
	// Without the right to make symlinks, as on Windows, a junction is made:
	// it needs no right, and it is a link all the same.
	for _, name := range []string{"rules", "skills", "commands"} {
		link, target := filepath.Join(root, ".claude", name), filepath.Join(root, ".ai", name)
		if err := os.Symlink(filepath.Join("..", ".ai", name), link); err == nil {
			continue
		} else if runtime.GOOS != "windows" {
			t.Skipf("links cannot be made here: %v", err)
		}
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("neither a symlink nor a junction can be made here: %v: %s", err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "rules", "backend.md")); err != nil {
		t.Fatalf("the link does not lead to the rules: %v", err)
	}

	// A dry run names the links and leaves them.
	would, err := MoveLayout(root, true)
	if err != nil {
		t.Fatalf("MoveLayout, dry: %v", err)
	}
	for _, name := range []string{"rules", "skills", "commands"} {
		if _, err := os.Stat(filepath.Join(root, ".claude", name)); err != nil {
			t.Fatalf("a dry run removed the link .claude/%s: %v", name, err)
		}
		if !contains(stepsOf(would), "remove .claude/"+name+" (a link to its earlier place)") {
			t.Errorf("the dry run does not name the link .claude/%s:\n%s", name, strings.Join(stepsOf(would), "\n"))
		}
	}

	if _, err := MoveLayout(root, false); err != nil {
		t.Fatalf("MoveLayout: %v", err)
	}

	want := map[string]string{
		".ai/config.yaml":                       "project: {name: demo}\n",
		".claude/agents/pr-reviewer.md":         "the kit's subagent",
		".claude/rules/backend.md":              "the project's rule",
		".claude/skills/release-notes/SKILL.md": "the project's skill",
	}
	if got := files(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("after the move:\n%v\nwant\n%v", got, want)
	}
	for _, name := range []string{"rules", "skills"} {
		info, err := os.Lstat(filepath.Join(root, ".claude", name))
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Errorf(".claude/%s is not a directory of its own (err %v)", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "commands")); !os.IsNotExist(err) {
		t.Errorf("the link .claude/commands is still there (err %v)", err)
	}
}

// What cannot be decided is not decided: the move stops and says why, and
// what it did not get to is as it was.
func TestMoveLayout_StopsWhereItCannotTell(t *testing.T) {
	t.Run("a configuration at both places", func(t *testing.T) {
		root := t.TempDir()
		tree(t, root, map[string]string{
			".ai/config/workflow.yaml": "the earlier one\n",
			".ai/config.yaml":          "the one at the new place\n",
			".ai/rules/backend.md":     "the project's rule",
		})

		_, err := MoveLayout(root, false)

		if err == nil || !strings.Contains(err.Error(), "both .ai/config.yaml and .ai/config/workflow.yaml are there") {
			t.Fatalf("err = %v, want one that names both files", err)
		}
		got := files(t, root)
		if got[".ai/config/workflow.yaml"] != "the earlier one\n" || got[".ai/config.yaml"] != "the one at the new place\n" {
			t.Errorf("a configuration was touched: %v", got)
		}
		if got[".ai/rules/backend.md"] != "the project's rule" {
			t.Errorf("the move went on after it could not tell: %v", got)
		}
	})

	t.Run("a file where a directory is kept", func(t *testing.T) {
		root := t.TempDir()
		tree(t, root, map[string]string{
			".ai/rules/backend.md": "the project's rule",
			".claude/rules":        "a file of the project's, with this name",
		})

		_, err := MoveLayout(root, false)

		if err == nil || !strings.Contains(err.Error(), ".claude/rules is a file") {
			t.Fatalf("err = %v, want one that says the file is in the way", err)
		}
		got := files(t, root)
		if got[".claude/rules"] != "a file of the project's, with this name" || got[".ai/rules/backend.md"] != "the project's rule" {
			t.Errorf("the file was removed, or the rules moved: %v", got)
		}
	})
}

// A move that failed half way is finished by running it again: what is at
// the new place already stays, and what is still at the earlier one joins it.
func TestMoveLayout_FinishesWhatWasBegun(t *testing.T) {
	root := t.TempDir()
	tree(t, root, map[string]string{
		".ai/config.yaml":                    "project: {name: demo}\n",
		".ai/state/results/issue-3.json":     "moved the first time",
		".ai/results/issue-4.json":           "still at the earlier place",
		".ai/state/runs/issue-3/summary.txt": "moved the first time",
		".ai/runs/issue-3/prompt.txt":        "of the same run, still at the earlier place",
		".ai/runs/issue-4/summary.txt":       "still at the earlier place",
	})
	if !UsesOldLayout(root) {
		t.Fatal("a project with a run's directory at the earlier place is not seen as one to move")
	}

	if _, err := MoveLayout(root, false); err != nil {
		t.Fatalf("MoveLayout: %v", err)
	}

	want := map[string]string{
		".ai/config.yaml":                    "project: {name: demo}\n",
		".ai/state/results/issue-3.json":     "moved the first time",
		".ai/state/results/issue-4.json":     "still at the earlier place",
		".ai/state/runs/issue-3/summary.txt": "moved the first time",
		".ai/state/runs/issue-3/prompt.txt":  "of the same run, still at the earlier place",
		".ai/state/runs/issue-4/summary.txt": "still at the earlier place",
	}
	if got := files(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("after the second move:\n%v\nwant\n%v", got, want)
	}
}

// AWK keeps three things under .ai. What else is there it does not know, and
// does not remove: it says so.
func TestMoveLayout_SaysWhatItLeaves(t *testing.T) {
	root := t.TempDir()
	tree(t, root, map[string]string{
		".ai/config/workflow.yaml":  "project: {name: demo}\n",
		".ai/config/notes.md":       "kept next to the configuration by the project",
		".ai/analysis/2025-12.md":   "the project's own",
		".ai/specs/login/design.md": "the project's own",
		".ai/roster.yaml":           "roles: []\n",
		".ai/state/loop_count":      "1",
	})

	steps, err := MoveLayout(root, false)
	if err != nil {
		t.Fatalf("MoveLayout: %v", err)
	}

	got := files(t, root)
	for _, kept := range []string{".ai/config/notes.md", ".ai/analysis/2025-12.md", ".ai/specs/login/design.md"} {
		if _, ok := got[kept]; !ok {
			t.Errorf("%s was removed", kept)
		}
	}
	var remarks []string
	for _, step := range steps {
		if step.From == "" {
			remarks = append(remarks, step.Note)
		}
	}
	want := []string{
		"left .ai/config as it is: it holds files awkit does not know",
		"left as they are, awkit does not use them: .ai/analysis, .ai/config, .ai/specs",
	}
	if !reflect.DeepEqual(remarks, want) {
		t.Errorf("remarks = %q, want %q", remarks, want)
	}
}

// A project is one to move as long as anything of it is at an earlier place.
func TestUsesOldLayout(t *testing.T) {
	for _, location := range []string{
		".ai/config/workflow.yaml", ".ai/rules/backend.md", ".ai/skills/x/SKILL.md",
		".ai/results/issue-1.json", ".ai/runs/issue-1/summary.txt", ".ai/exe-logs/principal.log", ".ai/logs/a.log", ".ai/temp/ticket-1.md",
	} {
		root := t.TempDir()
		tree(t, root, map[string]string{".ai/config.yaml": "project: {name: demo}\n", location: "x"})
		if !UsesOldLayout(root) {
			t.Errorf("a project with %s is not seen as one to move", location)
		}
	}

	root := t.TempDir()
	tree(t, root, map[string]string{
		".ai/config.yaml":                    "project: {name: demo}\n",
		".ai/roster.yaml":                    "roles: []\n",
		".ai/state/results/issue-1.json":     "{}",
		".claude/rules/_kit/git-workflow.md": "the kit's rule",
		".claude/skills/x/SKILL.md":          "a skill",
	})
	if UsesOldLayout(root) {
		t.Error("a project that keeps its files where awkit keeps them now is seen as one to move")
	}
	if UsesOldLayout(t.TempDir()) {
		t.Error("a directory without a project is seen as one to move")
	}
}

// A directory that held nothing, or only the placeholder that kept it in git,
// is removed, and nothing is said of a move: nothing was moved. A dry run
// leaves it and its placeholder where they are.
func TestMoveLayout_ADirectoryThatHeldNothing(t *testing.T) {
	root := t.TempDir()
	tree(t, root, map[string]string{
		".ai/config.yaml":      "project: {name: demo}\n",
		".ai/temp/.gitkeep":    "\n",
		".ai/results/.gitkeep": "\n",
	})
	if err := os.MkdirAll(filepath.Join(root, ".ai", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, beforeDirs := files(t, root), dirs(t, root)

	would, err := MoveLayout(root, true)
	if err != nil || len(would) != 0 {
		t.Errorf("a dry run says %v (err %v), want nothing: there is nothing to move", stepsOf(would), err)
	}
	if !reflect.DeepEqual(files(t, root), before) || !reflect.DeepEqual(dirs(t, root), beforeDirs) {
		t.Errorf("a dry run changed the project: %v, %v", files(t, root), dirs(t, root))
	}

	done, err := MoveLayout(root, false)
	if err != nil || len(done) != 0 {
		t.Errorf("the move says %v (err %v), want nothing", stepsOf(done), err)
	}
	if got, want := files(t, root), map[string]string{".ai/config.yaml": "project: {name: demo}\n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after the move: %v, want %v", got, want)
	}
	if got, want := dirs(t, root), []string{".ai"}; !reflect.DeepEqual(got, want) {
		t.Errorf("directories after the move: %v, want %v", got, want)
	}
}

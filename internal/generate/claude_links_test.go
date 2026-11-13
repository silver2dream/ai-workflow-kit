package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// linkSource is a directory like .ai/skills: what .claude/skills must show.
func linkSource(t *testing.T) (root, source string) {
	t.Helper()
	root = t.TempDir()
	source = layout.Skills(root)
	if err := os.MkdirAll(filepath.Join(source, "principal-workflow"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "principal-workflow", "SKILL.md"), []byte("current skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, source
}

// assertShowsSource checks the two things that matter after a replace: the
// target shows the source's content, and the source itself is unharmed.
func assertShowsSource(t *testing.T, source, target string) {
	t.Helper()
	for _, dir := range []string{target, source} {
		data, err := os.ReadFile(filepath.Join(dir, "principal-workflow", "SKILL.md"))
		if err != nil {
			t.Fatalf("%s does not hold the skill: %v", dir, err)
		}
		if string(data) != "current skill\n" {
			t.Errorf("%s holds %q", dir, data)
		}
	}
}

func TestCreateSymlinkOrCopy_NothingThereYet(t *testing.T) {
	root, source := linkSource(t)
	target := filepath.Join(root, ".claude", "skills")

	if err := createSymlinkOrCopy(source, target, "skills"); err != nil {
		t.Fatalf("createSymlinkOrCopy: %v", err)
	}
	assertShowsSource(t, source, target)
}

// An earlier run without symlink rights left a copy. It is replaced, old files
// and all.
func TestCreateSymlinkOrCopy_ReplacesAnEarlierCopy(t *testing.T) {
	root, source := linkSource(t)
	target := filepath.Join(root, ".claude", "skills")
	stale := filepath.Join(target, "removed-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("a skill the kit no longer ships\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := createSymlinkOrCopy(source, target, "skills"); err != nil {
		t.Fatalf("createSymlinkOrCopy: %v", err)
	}
	assertShowsSource(t, source, target)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("a file of the earlier copy is still there")
	}
}

// Something that is neither a link nor a directory sits in the way. Go reports
// the link another tool made on Windows the same way: not a symlink, not a
// directory. It used to be left alone, and every later run then failed.
func TestCreateSymlinkOrCopy_ReplacesAFileInTheWay(t *testing.T) {
	root, source := linkSource(t)
	target := filepath.Join(root, ".claude", "skills")
	if err := os.WriteFile(target, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := createSymlinkOrCopy(source, target, "skills"); err != nil {
		t.Fatalf("createSymlinkOrCopy: %v", err)
	}
	assertShowsSource(t, source, target)
}

// A link to the source is removed as a link. Its target, the project's real
// skills, must still be there afterwards.
func TestCreateSymlinkOrCopy_AnExistingLinkIsNotFollowed(t *testing.T) {
	root, source := linkSource(t)
	target := filepath.Join(root, ".claude", "skills")

	if err := os.Symlink(source, target); err != nil {
		if runtime.GOOS != "windows" {
			t.Skipf("cannot create a symlink here: %v", err)
		}
		// No symlink rights: a junction needs none and is a link all the same.
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", target, source).CombinedOutput(); err != nil {
			t.Skipf("cannot create a junction either: %v\n%s", err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "principal-workflow", "SKILL.md")); err != nil {
		t.Fatalf("the link does not lead to the source: %v", err)
	}

	if err := createSymlinkOrCopy(source, target, "skills"); err != nil {
		t.Fatalf("createSymlinkOrCopy: %v", err)
	}
	assertShowsSource(t, source, target)
}

func TestRemoveGenerated(t *testing.T) {
	dir := t.TempDir()

	if err := removeGenerated(filepath.Join(dir, "absent")); err != nil {
		t.Errorf("nothing to remove should not be an error: %v", err)
	}

	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, "full")
	if err := os.MkdirAll(filepath.Join(full, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "sub", "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, full, empty} {
		if err := removeGenerated(path); err != nil {
			t.Errorf("removeGenerated(%s): %v", filepath.Base(path), err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("%s is still there", filepath.Base(path))
		}
	}
}

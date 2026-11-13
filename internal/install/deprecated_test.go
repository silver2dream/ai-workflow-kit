package install

import (
	"os"
	"path/filepath"
	"testing"
)

// The skills keep their reference texts in references/ now. A project that
// has the earlier files loses them on the next init or upgrade, and the
// directories they emptied; a file of the project's own that sits beside
// them stays, and so does its directory.
func TestInstall_RemovesTheFilesTheSkillsNoLongerHave(t *testing.T) {
	dir := t.TempDir()
	earlier := []string{
		".claude/skills/principal-workflow/phases/main-loop.md",
		".claude/skills/principal-workflow/phases/dispatch.md",
		".claude/skills/principal-workflow/phases/review.md",
		".claude/skills/principal-workflow/phases/conflict-resolution.md",
		".claude/skills/principal-workflow/tasks/create-task.md",
		".claude/skills/principal-workflow/references/compaction-strategy.md",
		".claude/skills/post-mortem/phases/analyze-failure.md",
		".claude/skills/release-checklist/phases/verify-release.md",
	}
	own := ".claude/skills/release-checklist/phases/our-checks.md"
	for _, file := range append(append([]string{}, earlier...), own) {
		path := filepath.Join(dir, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("as an earlier kit had it"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Install(createMinimalMockFS(), dir, Options{NoGenerate: true, SkipConfig: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	for _, file := range earlier {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file))); !os.IsNotExist(err) {
			t.Errorf("%s is still there (err %v)", file, err)
		}
	}
	for _, gone := range []string{
		".claude/skills/principal-workflow/phases",
		".claude/skills/principal-workflow/tasks",
		".claude/skills/post-mortem/phases",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(gone))); !os.IsNotExist(err) {
			t.Errorf("the emptied directory %s is still there (err %v)", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(own))); err != nil {
		t.Errorf("the project's own file beside them went too: %v", err)
	}
}

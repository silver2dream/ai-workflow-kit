package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/install"
)

// gameProject is a project with the game data a game the kit sets up starts
// with, and one table.
func gameProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := install.Scaffold(dir, install.ScaffoldOptions{Preset: install.PresetGodotGo, ProjectName: "Rally"}); err != nil {
		t.Fatal(err)
	}
	put := func(rel, content string) {
		file := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("gamedata/Defines/racket.xml", `<module name="">
	<bean name="Racket" comment="the rackets">
		<var name="id" type="string" group="c,s" comment="code"/>
		<var name="name" type="string" group="c,s" comment="文字鍵 of its name"/>
		<var name="power" type="int" group="c,s" comment="how hard it hits"/>
	</bean>
	<table name="TbRacket" value="Racket" input="racket.csv" mode="map" group="c,s"/>
</module>
`)
	put("gamedata/Datas/racket.csv", "##var,id,name,power\n##,code,name,power\n,wood,ui.title.name,10\n")
	return dir
}

func gamedataCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var code int
	var stdout string
	stderr := captureStderr(func() {
		stdout = captureStdout(t, func() { code = cmdGamedata(args) })
	})
	return code, stdout, stderr
}

func TestCmdGamedata_Check(t *testing.T) {
	dir := gameProject(t)
	code, out, errs := gamedataCmd(t, "check", "--state-root", dir)
	if code != 0 || !strings.Contains(out, "The game data (gamedata/) passes its check: 1 tables, 1 text keys") {
		t.Fatalf("check = %d\n%s%s", code, out, errs)
	}

	if err := os.WriteFile(filepath.Join(dir, "gamedata", "Datas", "racket.csv"), []byte("##var,id,name,power\n,wood,ui.title.name,\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs = gamedataCmd(t, "check", "--state-root", dir)
	if code != 1 || !strings.Contains(out, "gamedata/Datas/racket.csv:2: power is blank") || !strings.Contains(errs, "1 point(s) in the game data (gamedata/)") {
		t.Errorf("check of a blank number = %d\n%s%s", code, out, errs)
	}
}

func TestCmdGamedata_Review(t *testing.T) {
	dir := gameProject(t)
	code, out, errs := gamedataCmd(t, "review", "copy", "--state-root", dir)
	if code != 0 || !strings.HasPrefix(out, "# The copy\n\n## gamedata/Datas/text/ui.csv\n") ||
		!strings.Contains(out, "| `ui.title.name` | Rally |  | Rally |  | TbRacket.name (racket.csv:3) |  |") {
		t.Errorf("review copy = %d\n%s%s", code, out, errs)
	}
	page := filepath.Join(dir, "out", "tables.md")
	code, out, errs = gamedataCmd(t, "review", "--state-root", dir, "tables", "--out", page)
	if code != 0 || !strings.Contains(out, "Wrote "+page) {
		t.Fatalf("review tables --out = %d\n%s%s", code, out, errs)
	}
	written, err := os.ReadFile(page)
	if err != nil || !strings.Contains(string(written), "## TbRacket") || !strings.Contains(string(written), "`ui.title.name`<br>Rally") {
		t.Errorf("the tables page: %v\n%s", err, written)
	}
}

func TestCmdGamedata_Refuses(t *testing.T) {
	dir := gameProject(t)
	for name, args := range map[string][]string{
		"no command":          nil,
		"an unknown command":  {"lint"},
		"a page to check":     {"check", "copy", "--state-root", dir},
		"--out to check":      {"check", "--out", "x.md", "--state-root", dir},
		"no page to review":   {"review", "--state-root", dir},
		"an unknown page":     {"review", "numbers", "--state-root", dir},
		"two pages to review": {"review", "copy", "tables", "--state-root", dir},
		"an unknown flag":     {"check", "--fix", "--state-root", dir},
	} {
		if code, out, errs := gamedataCmd(t, args...); code != 2 {
			t.Errorf("%s: code %d\n%s%s", name, code, out, errs)
		}
	}
	if code, _, errs := gamedataCmd(t, "check", "--state-root", t.TempDir()); code != 1 || !strings.Contains(errs, "the project keeps no game data: no folder at its root holds a luban.conf") {
		t.Errorf("a project without game data: %d\n%s", code, errs)
	}
	for _, help := range [][]string{{"--help"}, {"help"}, {"check", "--help"}} {
		if code, _, errs := gamedataCmd(t, help...); code != 0 || !strings.Contains(errs, "awkit gamedata review copy|tables") {
			t.Errorf("%v: %d\n%s", help, code, errs)
		}
	}
}

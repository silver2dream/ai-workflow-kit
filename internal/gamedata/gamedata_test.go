package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

// The luban.conf a project starts with: the one of the project the kit's
// rules for game data come from.
const testConf = `{
	"groups":
	[
		{"names":["c"], "default":true},
		{"names":["s"], "default":true}
	],
	"schemaFiles":
	[
		{"fileName":"Defines", "type":""}
	],
	"dataDir": "Datas",
	"targets":
	[
		{"name":"server", "manager":"Tables", "groups":["s"], "topModule":"gamecfg"},
		{"name":"client", "manager":"Tables", "groups":["c"], "topModule":"cfg"},
		{"name":"all", "manager":"Tables", "groups":["c","s"], "topModule":"cfg"}
	]
}
`

const testSchema = `<!-- the grades, and the rules of a match -->
<module name="">
	<bean name="Grade" comment="the grades of a racket">
		<var name="id" type="string" group="c,s" comment="the grade's code"/>
		<var name="name" type="string" group="c,s" comment="文字鍵 of the grade's name"/>
		<var name="power" type="int#range=[0,100]" group="c,s" comment="how hard it hits"/>
		<var name="bonus" type="int?" group="c,s" comment="what it adds, when it adds anything"/>
		<var name="tags" type="(list#sep=|),string" group="c,s" comment="what it is known for"/>
		<var name="drop_weight" type="int" group="s" comment="how often it drops"/>
	</bean>
	<bean name="Rule" comment="the rules of a match">
		<var name="sets" type="int" group="s" comment="sets to win"/>
		<var name="tiebreak" type="bool" group="s" comment="whether a set ends in a tiebreak"/>
	</bean>
	<bean name="Shot" comment="a shot">
		<var name="id" type="string" group="c,s" comment="the shot's code"/>
		<bean name="Smash" comment="a shot from above">
			<var name="height" type="int" group="c,s" comment="how high it is hit from"/>
		</bean>
	</bean>
	<table name="TbGrade" value="Grade" input="grade.csv" mode="map" group="c,s"/>
	<table name="TbRule" value="Rule" input="rule.csv" mode="one" group="s"/>
	<table name="TbSmash" value="Smash" input="smash.csv" mode="map" group="c,s"/>
</module>
`

const testGrades = "##var,id,name,power,bonus,tags,drop_weight\n" +
	"##,code,name (text key),power,bonus,tags,drop weight\n" +
	",wood,grade.wood.name,10,null,,60\n" +
	",steel,grade.steel.name,40,5,fast|loud,30\n"

const testRule = "##var,sets,tiebreak\n##,sets to win,tiebreak\n,2,true\n"

const testSmash = "##var,id,height\n,high,3\n"

const testText = "key,zh_TW,zh_CN,en,_en_依據,_上限\n" +
	"grade.wood.name,木拍,,Wood,木拍,12\n" +
	"grade.steel.name,鋼拍,,Steel,鋼拍,12\n" +
	"ui.title.start,開始 {name},,Start {name},開始 {name},\n" +
	"ui.title.quit,[em]離開[/em],,[em]Quit[/em],[em]離開[/em],\n"

func testProject() fstest.MapFS {
	return fstest.MapFS{
		"gamedata/luban.conf":           {Data: []byte(testConf)},
		"gamedata/Defines/world.xml":    {Data: []byte(testSchema)},
		"gamedata/Datas/grade.csv":      {Data: []byte(testGrades)},
		"gamedata/Datas/rule.csv":       {Data: []byte(testRule)},
		"gamedata/Datas/smash.csv":      {Data: []byte(testSmash)},
		"gamedata/Datas/text/world.csv": {Data: []byte(testText)},
		"backend/go.mod":                {Data: []byte("module x\n")},
	}
}

func check(t *testing.T, project fstest.MapFS) (*Data, []Finding) {
	t.Helper()
	layout, err := Find(project)
	if err != nil || layout == nil {
		t.Fatalf("Find: %v, %v", layout, err)
	}
	d, findings, err := Check(project, layout)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return d, findings
}

func TestFind(t *testing.T) {
	layout, err := Find(testProject())
	if err != nil {
		t.Fatal(err)
	}
	if layout.Dir != "gamedata/" || layout.Data != "gamedata/Datas/" || layout.Text() != "gamedata/Datas/text/" ||
		len(layout.Schemas) != 1 || layout.Schemas[0] != "gamedata/Defines/" || strings.Join(layout.Groups, ",") != "c,s" {
		t.Errorf("layout: %+v", *layout)
	}
	if layout.Conf() != "gamedata/luban.conf" {
		t.Errorf("Conf: %s", layout.Conf())
	}
	for file, holds := range map[string]bool{
		"gamedata/Datas/grade.csv":      true,
		"gamedata/Datas/text/world.csv": true,
		"gamedata/Defines/world.xml":    true,
		"gamedata/README.md":            false,
		"gamedata/luban.conf":           false,
		"docs/balance/grades.csv":       false,
		"gamedata/Datas/card/a.yml":     false,
	} {
		if layout.Holds(file) != holds {
			t.Errorf("Holds(%s) = %v", file, !holds)
		}
	}
	if !layout.IsText("gamedata/Datas/text/world.csv") || layout.IsText("gamedata/Datas/grade.csv") {
		t.Error("IsText tells a text table from a table wrongly")
	}

	if layout, err := Find(fstest.MapFS{"backend/go.mod": {Data: []byte("module x\n")}}); layout != nil || err != nil {
		t.Errorf("a project without game data: %v, %v", layout, err)
	}
	two := testProject()
	two["data/luban.conf"] = &fstest.MapFile{Data: []byte(testConf)}
	if _, err := Find(two); err == nil || !strings.Contains(err.Error(), "one folder") {
		t.Errorf("two folders of game data: %v", err)
	}
	hidden := fstest.MapFS{".cache/luban.conf": {Data: []byte(testConf)}}
	if layout, err := Find(hidden); layout != nil || err != nil {
		t.Errorf("a luban.conf in a hidden folder is a tool's: %v, %v", layout, err)
	}
}

func TestReadLayout_Refuses(t *testing.T) {
	for name, conf := range map[string]string{
		"not json":            "{",
		"no dataDir":          `{"schemaFiles":[{"fileName":"Defines"}]}`,
		"no schema":           `{"dataDir":"Datas"}`,
		"data outside":        `{"dataDir":"../Datas","schemaFiles":[{"fileName":"Defines"}]}`,
		"schema outside":      `{"dataDir":"Datas","schemaFiles":[{"fileName":"../../Defines"}]}`,
		"data the folder":     `{"dataDir":".","schemaFiles":[{"fileName":"Defines"}]}`,
		"an absolute path":    `{"dataDir":"/Datas","schemaFiles":[{"fileName":"Defines"}]}`,
		"a path with a drive": `{"dataDir":"C:/Datas","schemaFiles":[{"fileName":"Defines"}]}`,
	} {
		project := fstest.MapFS{"gamedata/luban.conf": {Data: []byte(conf)}}
		if _, err := Find(project); err == nil {
			t.Errorf("%s: read as a layout", name)
		}
	}
	// A schema file, not a folder.
	project := fstest.MapFS{"gamedata/luban.conf": {Data: []byte(`{"dataDir":"Datas","schemaFiles":[{"fileName":"Defines/all.xml"}]}`)}}
	layout, err := Find(project)
	if err != nil || layout.Schemas[0] != "gamedata/Defines/all.xml" {
		t.Errorf("a schema file: %+v, %v", layout, err)
	}
}

func TestCheck_TheDataAsItHasToBe(t *testing.T) {
	d, findings := check(t, testProject())
	for _, f := range findings {
		t.Errorf("found: %s", f)
	}
	if len(d.Tables) != 3 || len(d.Texts) != 1 || len(d.Texts[0].Rows) != 4 {
		t.Fatalf("read %d tables, %d text tables", len(d.Tables), len(d.Texts))
	}
	smash := d.Tables[2]
	if smash.Bean == nil || len(smash.Bean.Vars) != 2 || smash.Bean.Vars[0].Name != "id" {
		t.Errorf("a child bean has its parent's fields first: %+v", smash.Bean)
	}
	text := d.Texts[0]
	if text.Source() != "zh_TW" || strings.Join(text.Languages, ",") != "zh_TW,zh_CN,en" || strings.Join(text.Translated, ",") != "en" {
		t.Errorf("the languages of a text table: %v, translated %v", text.Languages, text.Translated)
	}
	if row := text.Rows[0]; row.Limit != 12 || row.Basis["en"] != "木拍" || row.Line != 2 {
		t.Errorf("a text row: %+v", row)
	}
	if got, ok := d.SourceText("grade.steel.name"); !ok || got != "鋼拍" {
		t.Errorf("SourceText: %q, %v", got, ok)
	}
	grades := d.Tables[0].Sheets[0]
	if grades.Rows[1].Line != 4 || grades.Cell(grades.Rows[1], "power") != "40" || grades.Notes[1] != "name (text key)" {
		t.Errorf("a sheet: %+v", grades)
	}
}

// Each way the data can be wrong is found, for its own reason.
func TestCheck_WhatIsWrongIsFound(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		from  string
		to    string
		found string
	}{
		{"a blank number", "gamedata/Datas/grade.csv", ",wood,grade.wood.name,10,", ",wood,grade.wood.name,,", "power is blank"},
		{"a blank truth value", "gamedata/Datas/rule.csv", ",2,true", ",2,", "tiebreak is blank"},
		{"a blank number that may have none", "gamedata/Datas/grade.csv", "10,null,", "10,,", "bonus is blank"},
		{"a column the schema does not have", "gamedata/Datas/grade.csv", "##var,id,", "##var,colour,id,", "it has colour"},
		{"a field without a column", "gamedata/Datas/grade.csv", ",drop_weight\n", "\n", "it lacks drop_weight"},
		{"no ##var row", "gamedata/Datas/smash.csv", "##var,id,height\n", "", "has no ##var row"},
		{"a second ##var row", "gamedata/Datas/smash.csv", ",high,3\n", "##var,id,height\n,high,3\n", "a second ##var row"},
		{"more cells than fields", "gamedata/Datas/smash.csv", ",high,3\n", ",high,3,9\n", "has 3 cells"},
		{"a table without a group", "gamedata/Defines/world.xml", `mode="one" group="s"`, `mode="one"`, "TbRule marks no group"},
		{"a group luban.conf does not have", "gamedata/Defines/world.xml", `mode="one" group="s"`, `mode="one" group="server"`, `has the group "server"`},
		{"a field the client has, unmarked", "gamedata/Defines/world.xml", `type="int#range=[0,100]" group="c,s"`, `type="int#range=[0,100]"`, "Grade.power marks no group"},
		{"a weight the client has", "gamedata/Defines/world.xml", `type="int" group="s" comment="how often`, `type="int" group="c,s" comment="how often`, "Grade.drop_weight is an odds or a weight"},
		{"a field of the server's table marked c", "gamedata/Defines/world.xml", `type="bool" group="s"`, `type="bool" group="c,s"`, "Rule.tiebreak is marked c"},
		{"a field with an unknown group", "gamedata/Defines/world.xml", `type="int" group="s" comment="sets`, `type="int" group="x" comment="sets`, `Rule.sets has the group "x"`},
		{"a table of a bean the schema lacks", "gamedata/Defines/world.xml", `value="Rule"`, `value="Rules"`, "does not define as a bean"},
		{"a data file that is not there", "gamedata/Defines/world.xml", `input="rule.csv"`, `input="rules.csv"`, "reads gamedata/Datas/rules.csv, which is not there"},
		{"a table that reads nothing", "gamedata/Defines/world.xml", `input="rule.csv"`, `input=""`, "TbRule reads nothing"},
		{"a parent the schema lacks", "gamedata/Defines/world.xml", `<bean name="Rule" comment`, `<bean name="Rule" parent="Base" comment`, "the parent Base"},
		{"two tables of one name", "gamedata/Defines/world.xml", `<table name="TbRule"`, `<table name="TbGrade"`, "defines the table TbGrade"},
		{"a schema that is not XML", "gamedata/Defines/world.xml", "</module>", "", "is not a schema Luban can read"},
		{"a schema that is not a module", "gamedata/Defines/world.xml", "<module name=\"\">", "<modules>", "is not a schema"},
		{"a text key the text tables lack", "gamedata/Datas/grade.csv", "grade.steel.name", "grade.iron.name", MissingKey + "grade.iron.name"},
		{"a blank text key", "gamedata/Datas/grade.csv", "grade.steel.name", "", "holds no text key"},
		{"a key that is not one", "gamedata/Datas/text/world.csv", "ui.title.quit,", "UI.Quit,", `the key "UI.Quit" is not a key`},
		{"no source text", "gamedata/Datas/text/world.csv", "grade.wood.name,木拍,", "grade.wood.name,,", "has no zh_TW text"},
		{"no translation", "gamedata/Datas/text/world.csv", ",Wood,", ",,", "grade.wood.name has no en text"},
		{"a translation out of date", "gamedata/Datas/text/world.csv", "鋼拍,,Steel,鋼拍,", "鐵拍,,Steel,鋼拍,", "the en of grade.steel.name is out of date"},
		{"a translation over its limit", "gamedata/Datas/text/world.csv", ",Steel,鋼拍,12", ",Steel racket of old,鋼拍,12", "over its limit of 12"},
		{"a limit that is not a number", "gamedata/Datas/text/world.csv", ",Steel,鋼拍,12", ",Steel,鋼拍,twelve", `the limit "twelve"`},
		{"a placeholder the translation lacks", "gamedata/Datas/text/world.csv", ",Start {name},", ",Start {player},", "fill in different things"},
		{"markup the translation lacks", "gamedata/Datas/text/world.csv", ",[em]Quit[/em],", ",Quit,", "have different markup"},
		{"a color in the text", "gamedata/Datas/text/world.csv", "[em]離開[/em],,[em]Quit[/em],[em]離開[/em]", "#ff0000離開,,#ff0000Quit,#ff0000離開", `holds "#ff0000"`},
		{"markup left open", "gamedata/Datas/text/world.csv", "[em]離開[/em],,[em]Quit[/em],[em]離開[/em]", "[em][em]離開[/em],,[em][em]Quit[/em],[em][em]離開[/em]", "opens [em] and does not close it"},
		{"markup closed before it opens", "gamedata/Datas/text/world.csv", "[em]離開[/em],,[em]Quit[/em],[em]離開[/em]", "[/em]離開[em],,[/em]Quit[em],[/em]離開[em]", "closes [em] where it is not open"},
		{"a text table without a key column", "gamedata/Datas/text/world.csv", "key,zh_TW", "id,zh_TW", `has no column "key"`},
		{"a text table without text", "gamedata/Datas/text/world.csv", "key,zh_TW,zh_CN,en,", "key,_zh_TW,_zh_CN,_en,", "has no column of text"},
		{"a column twice", "gamedata/Datas/text/world.csv", "key,zh_TW,zh_CN,en,", "key,zh_TW,zh_CN,zh_CN,", "has the column zh_CN twice"},
		{"a column that is neither a language nor a note", "gamedata/Datas/text/world.csv", "_en_依據,_上限\n", "_en_依據,_上限,status\n", `has the column "status", which is neither a language`},
		{"a basis that names no language", "gamedata/Datas/text/world.csv", "_en_依據,_上限\n", "_依據,_上限\n", "has the column _依據, which names no language"},
		{"the basis of a language the table lacks", "gamedata/Datas/text/world.csv", "_en_依據,_上限\n", "_ja_basis,_上限\n", "has the column _ja_basis, the basis of ja, and no column of ja"},
		{"text that stands in for text", "gamedata/Datas/text/world.csv", "grade.wood.name,木拍,,Wood,木拍,12", "grade.wood.name,待補,,TBD,待補,12", `the zh_TW text "待補" stands in for text not written yet`},
		{"a translation that stands in for text", "gamedata/Datas/text/world.csv", "grade.wood.name,木拍,,Wood,木拍,12", "grade.wood.name,木拍,,TODO: name,木拍,12", `the en text "TODO: name" stands in`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			project := testProject()
			before := string(project[c.file].Data)
			if strings.Count(before, c.from) != 1 {
				t.Fatalf("%q is in %s %d times", c.from, c.file, strings.Count(before, c.from))
			}
			project[c.file] = &fstest.MapFile{Data: []byte(strings.Replace(before, c.from, c.to, 1))}
			_, findings := check(t, project)
			hit := false
			for _, f := range findings {
				if strings.Contains(f.String(), c.found) {
					hit = true
				}
			}
			if !hit {
				t.Errorf("not found: %q; found:\n%s", c.found, listFindings(findings))
			}
		})
	}
}

func listFindings(findings []Finding) string {
	var lines []string
	for _, f := range findings {
		lines = append(lines, "  "+f.String())
	}
	return strings.Join(lines, "\n")
}

// What is allowed is not found: an empty string, an empty list, null where a
// number may have none, markup that stands alone, a language without a basis
// left empty.
func TestCheck_WhatIsAllowedIsNotFound(t *testing.T) {
	project := testProject()
	project["gamedata/Datas/grade.csv"] = &fstest.MapFile{Data: []byte(strings.Replace(testGrades, ",steel,grade.steel.name,40,5,fast|loud,30", ",steel,grade.steel.name,40,null,,30", 1))}
	project["gamedata/Datas/text/world.csv"] = &fstest.MapFile{Data: []byte(testText + "ui.title.line,一[br]二,,One[br]Two,一[br]二,\n" +
		"ui.title.beast,乳齒象,,Mastodon of Todoroki,乳齒象,\n")}
	_, findings := check(t, project)
	for _, f := range findings {
		t.Errorf("found: %s", f)
	}
	// Languages written with their country or script are languages; a basis
	// written in English is a basis.
	project = testProject()
	project["gamedata/Datas/text/world.csv"] = &fstest.MapFile{Data: []byte("key,zh_TW,pt-BR,en,_en_basis,_limit\nui.title.go,開始,Iniciar,Start,開始,9\n")}
	d, findings := check(t, project)
	for _, f := range findings {
		if !strings.Contains(f.Message, MissingKey) {
			t.Errorf("found: %s", f)
		}
	}
	if text := d.Texts[0]; strings.Join(text.Languages, ",") != "zh_TW,pt-BR,en" || strings.Join(text.Translated, ",") != "en" || text.Rows[0].Limit != 9 {
		t.Errorf("a table with English notes: %v, translated %v, limit %d", text.Languages, text.Translated, text.Rows[0].Limit)
	}
}

// A table that reads a file that is not there names the file: writing it puts
// the finding right.
func TestCheck_AFileATableNeeds(t *testing.T) {
	project := testProject()
	delete(project, "gamedata/Datas/rule.csv")
	_, findings := check(t, project)
	if len(findings) != 1 || findings[0].Needs != "gamedata/Datas/rule.csv" || findings[0].File != "gamedata/Defines/world.xml" {
		t.Fatalf("findings:\n%s", listFindings(findings))
	}
	if (Finding{Message: "power is blank"}).Needs != "" {
		t.Error("a finding names a file it does not need")
	}
}

func TestCheck_AKeyInTwoTextTables(t *testing.T) {
	project := testProject()
	project["gamedata/Datas/text/ui.csv"] = &fstest.MapFile{Data: []byte("key,zh_TW,zh_CN,en,_en_依據,_上限\nui.title.start,開始 {name},,Start {name},開始 {name},\n")}
	_, findings := check(t, project)
	if len(findings) != 1 || !strings.Contains(findings[0].String(), "the key ui.title.start is in gamedata/Datas/text/ui.csv:2 as well") ||
		findings[0].File != "gamedata/Datas/text/world.csv" || findings[0].Line != 4 {
		t.Errorf("findings:\n%s", listFindings(findings))
	}
}

func TestFinding(t *testing.T) {
	f := Finding{File: "gamedata/Datas/grade.csv", Line: 3, Message: "x"}
	if f.String() != "gamedata/Datas/grade.csv:3: x" {
		t.Error(f.String())
	}
	if (Finding{File: "a.xml", Message: "x"}).String() != "a.xml: x" {
		t.Error("a finding about a whole file names no line")
	}
	missing := Finding{Message: "name holds a.b, and " + MissingKey + "a.b: write its text in x"}
	if !missing.IsMissingKey() || (Finding{Message: "power is blank"}).IsMissingKey() {
		t.Error("IsMissingKey")
	}
}

func TestCopyReview(t *testing.T) {
	project := testProject()
	project["gamedata/Datas/text/world.csv"] = &fstest.MapFile{Data: []byte(testText + "ui.title.help,\"a|b\nc\",,\"A|B\nC\",x,\n")}
	d, findings := check(t, project)
	page := d.CopyReview(nil, findings)
	for _, want := range []string{
		"## gamedata/Datas/text/world.csv",
		"Written in zh_TW; translated into en.",
		"### grade\n",
		"### ui.title\n",
		"| Key | zh_TW | zh_CN | en | Limit | Used in | To look at |",
		"| `grade.wood.name` | 木拍 |  | Wood | 12 | TbGrade.name (grade.csv:3) |  |",
		"| `ui.title.help` | a\\|b<br>c |  | A\\|B<br>C |",
		"the en of ui.title.help is out of date",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the copy review lacks %q:\n%s", want, page)
		}
	}
	if other := d.CopyReview([]string{"gamedata/Datas/text/other.csv"}, findings); other != "" {
		t.Errorf("a review of other text tables shows this one:\n%s", other)
	}
}

func TestDataReview(t *testing.T) {
	project := testProject()
	project["gamedata/Datas/grade.csv"] = &fstest.MapFile{Data: []byte(strings.Replace(testGrades, ",wood,grade.wood.name,10,", ",wood,grade.wood.name,,", 1))}
	d, findings := check(t, project)
	page := d.DataReview([]string{"gamedata/Datas/grade.csv", "gamedata/Datas/rule.csv"}, findings)
	for _, want := range []string{
		"## TbGrade\n\nthe grades of a racket",
		"Schema `gamedata/Defines/world.xml` (Grade) · data `gamedata/Datas/grade.csv` · group c,s · mode map",
		"| `id`<br>code | `name`<br>name (text key) |",
		"| steel | `grade.steel.name`<br>鋼拍 | 40 | 5 | fast\\|loud | 30 |",
		"- gamedata/Datas/grade.csv:3: power is blank",
		"## TbRule",
		"| Field | What it holds | Type | Group | Value |",
		"| `tiebreak` | tiebreak | `bool` | s | true |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the data review lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "TbSmash") {
		t.Errorf("a table that is not asked for is shown:\n%s", page)
	}
	if all := d.DataReview(nil, nil); !strings.Contains(all, "## TbSmash") {
		t.Errorf("a review of all the tables leaves one out:\n%s", all)
	}
	if bySchema := d.DataReview([]string{"gamedata/Defines/world.xml"}, nil); !strings.Contains(bySchema, "## TbRule") {
		t.Errorf("a table whose schema is asked for is not shown:\n%s", bySchema)
	}
}

func TestLayout_Files(t *testing.T) {
	layout, _ := Find(testProject())
	files := []string{"docs/a.md", "gamedata/Datas/text/world.csv", "gamedata/Datas/grade.csv", "gamedata/Defines/world.xml", "gamedata/Datas/grade.csv"}
	if got := strings.Join(layout.TablesOf(files), ","); got != "gamedata/Datas/grade.csv,gamedata/Defines/world.xml" {
		t.Errorf("TablesOf: %s", got)
	}
	if got := strings.Join(layout.TextsOf(files), ","); got != "gamedata/Datas/text/world.csv" {
		t.Errorf("TextsOf: %s", got)
	}
}

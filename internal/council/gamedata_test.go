package council

import (
	"fmt"
	"strings"
	"testing"
)

// A project that keeps game data: Luban's configuration, a text table that is
// there already, and a roster in which the numbers and the text are written
// by two roles, the text's folder inside the numbers'.
const (
	lubanConf = `{
	"groups": [{"names":["c"], "default":true}, {"names":["s"], "default":true}],
	"schemaFiles": [{"fileName":"Defines", "type":""}],
	"dataDir": "Datas",
	"targets": [{"name":"all", "manager":"Tables", "groups":["c","s"], "topModule":"cfg"}]
}
`
	textHeader = "key,zh_TW,zh_CN,en,_en_依據,_上限\n"
	worldText  = textHeader + "ui.title.start,開始,,Start,開始,\n"

	gameRoster = `
limits:
  calls: 60
  parallel: 1
roles:
  - id: owner
    title: Product Owner
    decides: true
    stance: You answer for what is built.
    owns: [docs/product/]
  - id: numbers
    title: Numerical Designer
    stance: You answer for the numbers.
    owns: [docs/balance/, gamedata/Defines/, gamedata/Datas/]
  - id: writer
    title: Writer
    stance: You answer for every word the player reads.
    owns: [gamedata/Datas/text/]
  - id: builder
    title: Builder
    plans: true
    stance: You answer for how it is built.
    owns: [backend/docs/]
`

	racketSchema = `<module name="">
	<bean name="Racket" comment="the rackets">
		<var name="id" type="string" group="c,s" comment="the racket's code"/>
		<var name="name" type="string" group="c,s" comment="文字鍵 of its name"/>
		<var name="power" type="int" group="c,s" comment="how hard it hits"/>
	</bean>
	<table name="TbRacket" value="Racket" input="racket.csv" mode="map" group="c,s"/>
</module>`

	gameAgenda = `{"topics": [
  {"id": "rackets", "title": "The rackets", "question": "Which rackets are there?", "lead": "numbers", "speakers": ["writer", "owner"], "reads": [],
   "writes": ["gamedata/Defines/racket.xml", "gamedata/Datas/racket.csv", "gamedata/Datas/text/world.csv", "docs/balance/rackets.md"]}
], "questions": []}`
)

func gameProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	putFile(t, root, "gamedata/luban.conf", lubanConf)
	putFile(t, root, "gamedata/Datas/text/world.csv", worldText)
	return root
}

func dataFile(path, content string) string {
	return fmt.Sprintf("<<<DOCUMENT %s>>>\n%s\n<<<END>>>\n", path, content)
}

// The tables are written first, then the text of the keys they hold; what the
// check of the game data finds goes back to the owner of each file, a key the
// text tables lack to the role that writes the text. A person reviews what
// was written on two pages of the plan.
func TestRun_GameData(t *testing.T) {
	root := gameProject(t)
	roster := mustRoster(t, gameRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(gameAgenda), nil
		case c.Step == "documents" && c.Role == "numbers":
			// A power left blank, which Luban reads as 0.
			return dataFile("gamedata/Defines/racket.xml", racketSchema) +
				dataFile("gamedata/Datas/racket.csv", "##var,id,name,power\n##,code,name,power\n,wood,racket.wood.name,10\n,steel,racket.steel.name,") +
				docs("numbers", "docs/balance/rackets.md"), nil
		case c.Step == "documents" && c.Role == "writer":
			// The text of one key of the two.
			return dataFile("gamedata/Datas/text/world.csv", strings.TrimSuffix(worldText, "\n")+"\nracket.wood.name,木拍,,Wood racket,木拍,16"), nil
		case c.Step == "redata" && c.Role == "numbers":
			return dataFile("gamedata/Datas/racket.csv", "##var,id,name,power\n##,code,name,power\n,wood,racket.wood.name,10\n,steel,racket.steel.name,40") +
				"<<<UNCHANGED gamedata/Defines/racket.xml>>>\n", nil
		case c.Step == "redata" && c.Role == "writer":
			return dataFile("gamedata/Datas/text/world.csv", strings.TrimSuffix(worldText, "\n")+"\nracket.wood.name,木拍,,Wood racket,木拍,16\nracket.steel.name,鋼拍,,Steel racket,鋼拍,16"), nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "A tennis game.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The tables first, then the text.
	steps := strings.Join(model.steps(), "\n")
	if a, b := strings.Index(steps, "numbers documents"), strings.Index(steps, "writer documents"); a < 0 || b < 0 || a > b {
		t.Errorf("the text is written before the tables:\n%s", steps)
	}
	writer := model.promptOf("writer", "documents")
	for _, want := range []string{
		"The tables of the game data this topic wrote before your turn:\n===== gamedata/Datas/racket.csv (numbers's)\n",
		"- racket.wood.name (gamedata/Datas/racket.csv:3)\n- racket.steel.name (gamedata/Datas/racket.csv:4)\n",
		"A table of the game data (.csv) and its schema (.xml) are not documents",
	} {
		if !strings.Contains(writer, want) {
			t.Errorf("the writer is not told %q:\n%s", want, writer)
		}
	}
	if numbers := model.promptOf("numbers", "documents"); strings.Contains(numbers, "this topic wrote before your turn") {
		t.Errorf("the role that writes the tables is shown tables written before its turn:\n%s", numbers)
	}

	// Each finding goes to the owner of its file.
	if prompt := model.promptOf("numbers", "redata"); !strings.Contains(prompt, "gamedata/Datas/racket.csv:4: power is blank") || strings.Contains(prompt, "racket.steel.name, and the text tables") {
		t.Errorf("the role that writes the tables is not given what is its to put right:\n%s", prompt)
	}
	prompt := model.promptOf("writer", "redata")
	for _, want := range []string{
		"gamedata/Datas/racket.csv:4: name holds racket.steel.name, and the text tables have no key racket.steel.name",
		"The tables that hold the keys:\n===== gamedata/Datas/racket.csv",
		"Your files of the game data as they stand:\n",
		"===== gamedata/Datas/text/world.csv",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the writer is not told %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "power is blank") {
		t.Errorf("the writer is given what is not its to put right:\n%s", prompt)
	}

	// What was put right is on disk, and nothing is found any more.
	if got := readFile(t, root, "gamedata/Datas/racket.csv"); !strings.HasSuffix(got, ",steel,racket.steel.name,40\n") {
		t.Errorf("the table was not put right:\n%s", got)
	}
	if got := readFile(t, root, "gamedata/Datas/text/world.csv"); !strings.Contains(got, "racket.steel.name,鋼拍,,Steel racket,鋼拍,16") {
		t.Errorf("the text was not written:\n%s", got)
	}
	check := plan.Topics[0].DataCheck
	if check == nil || len(check.Found) != 2 || len(check.Open) != 0 || !check.Settled {
		t.Fatalf("the check of the game data: %+v", check)
	}
	if len(plan.DataOpen) != 0 {
		t.Errorf("still found: %v", plan.DataOpen)
	}
	if want := []string{"gamedata/Datas/racket.csv", "gamedata/Datas/text/world.csv", "gamedata/Defines/racket.xml"}; strings.Join(plan.GameData, ",") != strings.Join(want, ",") {
		t.Errorf("the game data the plan wrote: %v", plan.GameData)
	}

	// The record, and the pages a person reviews it on.
	topic := readFile(t, root, plan.Dir()+"/topics/01-rackets.md")
	for _, want := range []string{"### The check of the game data\n\n2 point(s) found and put right.\n",
		"- Found: gamedata/Datas/racket.csv:4: power is blank"} {
		if !strings.Contains(topic, want) {
			t.Errorf("the minutes of the topic lack %q:\n%s", want, topic)
		}
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	for _, want := range []string{"## Game data\n",
		"To review it: [copy.md](copy.md), [tables.md](tables.md).",
		"The check of the game data finds nothing in it.",
		"- `gamedata/Datas/racket.csv` (numbers)\n"} {
		if !strings.Contains(minutes, want) {
			t.Errorf("the minutes lack %q:\n%s", want, minutes)
		}
	}
	copyPage := readFile(t, root, plan.Dir()+"/"+CopyFile)
	for _, want := range []string{"# Plan " + plan.ID + ": the copy",
		"## gamedata/Datas/text/world.csv", "### racket\n",
		"| `racket.steel.name` | 鋼拍 |  | Steel racket | 16 | TbRacket.name (racket.csv:4) |  |"} {
		if !strings.Contains(copyPage, want) {
			t.Errorf("the copy page lacks %q:\n%s", want, copyPage)
		}
	}
	tablesPage := readFile(t, root, plan.Dir()+"/"+TablesFile)
	for _, want := range []string{"## TbRacket", "| steel | `racket.steel.name`<br>鋼拍 | 40 |"} {
		if !strings.Contains(tablesPage, want) {
			t.Errorf("the tables page lacks %q:\n%s", want, tablesPage)
		}
	}
	if !contains(plan.Reviews, plan.Dir()+"/"+CopyFile) || !contains(plan.Reviews, plan.Dir()+"/"+TablesFile) {
		t.Errorf("the pages to review: %v", plan.Reviews)
	}

	// Every role is told where the game data is kept, and the folder of the
	// text is the writer's, not the numbers'.
	speak := model.promptIn(1, "owner", "speak")
	for _, want := range []string{"The game data is kept in gamedata/, as Luban reads it (gamedata/luban.conf): the schema of the tables in gamedata/Defines/, one table per CSV file in gamedata/Datas/, and the text the player reads in gamedata/Datas/text/",
		"- gamedata/Datas/text/ (writer: any document in this folder)\n",
		"- gamedata/Datas/text/world.csv (writer)\n"} {
		if !strings.Contains(speak, want) {
			t.Errorf("a role is not told %q:\n%s", want, speak)
		}
	}
	if strings.Contains(speak, "- gamedata/Datas/text/world.csv (numbers)") {
		t.Errorf("the text table is shown as the numbers':\n%s", speak)
	}

	// The moderator is told that numbers end in tables and words in text
	// tables, and that a new file there is a schema or a table.
	agenda := model.promptOf("moderator", "agenda")
	for _, want := range []string{
		"The game data is kept in gamedata/, as Luban reads it",
		"a new file in a folder of that role (a Markdown document; in the folders of the game data, a schema (.xml) or a table (.csv))",
		"- A topic that settles numbers the game reads (stats, costs, odds, timings, curves) ends, besides the document that says why, in the tables that hold them: a schema in gamedata/Defines/ and a table in gamedata/Datas/, named for what they hold (gamedata/Defines/match.xml and gamedata/Datas/match_rule.csv, say). A topic that settles words the player reads (names, the words on a screen, messages) ends in a text table in gamedata/Datas/text/.",
		"- gamedata/Datas/text/world.csv (writer)\n",
	} {
		if !strings.Contains(agenda, want) {
			t.Errorf("the moderator is not told %q:\n%s", want, agenda)
		}
	}
}

// A schema defines tables the topic did not name: the role that writes it
// writes their files too, in its folders of the game data, and what it does
// not write it is given back to write. A document the topic did not name
// stays as it is.
func TestRun_GameDataATableTheTopicDidNotName(t *testing.T) {
	root := gameProject(t)
	roster := mustRoster(t, gameRoster)
	base := agreeable(roster)
	schema := strings.Replace(racketSchema, "</module>", `	<bean name="Grip" comment="the grips">
		<var name="id" type="string" group="c,s" comment="code"/>
		<var name="size" type="int" group="c,s" comment="how wide"/>
	</bean>
	<table name="TbGrip" value="Grip" input="grip.csv" mode="map" group="c,s"/>
	<bean name="Ball" comment="the balls">
		<var name="id" type="string" group="c,s" comment="code"/>
	</bean>
	<table name="TbBall" value="Ball" input="ball.csv" mode="map" group="c,s"/>
</module>`, 1)
	rackets := "##var,id,name,power\n,wood,ui.title.start,10"
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(gameAgenda), nil
		case c.Step == "documents" && c.Role == "numbers":
			// The grips are written with the schema; the balls are not, and
			// a document the topic did not name is handed in.
			return dataFile("gamedata/Defines/racket.xml", schema) + dataFile("gamedata/Datas/racket.csv", rackets) +
				dataFile("gamedata/Datas/grip.csv", "##var,id,size\n,small,2") +
				docs("numbers", "docs/balance/rackets.md", "docs/balance/extra.md"), nil
		case c.Step == "documents" && c.Role == "writer":
			return "<<<UNCHANGED gamedata/Datas/text/world.csv>>>\n", nil
		case c.Step == "redata" && c.Role == "numbers":
			return dataFile("gamedata/Datas/ball.csv", "##var,id\n,soft") + "<<<UNCHANGED gamedata/Defines/racket.xml>>>\n", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "A tennis game.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, root, "gamedata/Datas/grip.csv"); !strings.Contains(got, ",small,2") {
		t.Errorf("a table the schema defines was not written with it:\n%s", got)
	}
	if exists(root, "docs/balance/extra.md") {
		t.Error("a document the topic did not name was written")
	}
	if prompt := model.promptOf("numbers", "documents"); !strings.Contains(prompt, "Every table a schema of yours defines reads a file that is there") {
		t.Errorf("the role that writes the schema is not told to write its tables:\n%s", prompt)
	}
	prompt := model.promptOf("numbers", "redata")
	if !strings.Contains(prompt, "the table TbBall reads gamedata/Datas/ball.csv, which is not there") || !strings.Contains(prompt, "- gamedata/Datas/ball.csv: not written yet\n") {
		t.Errorf("the role is not given the table to write:\n%s", prompt)
	}
	if got := readFile(t, root, "gamedata/Datas/ball.csv"); !strings.Contains(got, ",soft") {
		t.Errorf("the table was not written when it was given back:\n%s", got)
	}
	check := plan.Topics[0].DataCheck
	if check == nil || len(check.Found) != 1 || len(check.Open) != 0 || len(plan.DataOpen) != 0 {
		t.Errorf("the check of the game data: %+v; still %v", check, plan.DataOpen)
	}
	for _, want := range []string{"gamedata/Datas/ball.csv", "gamedata/Datas/grip.csv"} {
		if !contains(plan.GameData, want) {
			t.Errorf("%s is not among the game data the plan wrote: %v", want, plan.GameData)
		}
	}
}

// What the owners leave is on record, and the plan says so.
func TestRun_GameDataLeftAsItWasFound(t *testing.T) {
	root := gameProject(t)
	roster := mustRoster(t, gameRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(gameAgenda), nil
		case c.Step == "documents" && c.Role == "numbers":
			return dataFile("gamedata/Defines/racket.xml", racketSchema) +
				dataFile("gamedata/Datas/racket.csv", "##var,id,name,power\n,wood,racket.wood.name,") +
				docs("numbers", "docs/balance/rackets.md"), nil
		case c.Step == "documents" && c.Role == "writer":
			return "<<<UNCHANGED gamedata/Datas/text/world.csv>>>\n", nil
		case c.Step == "redata":
			if c.Role == "numbers" {
				return "<<<UNCHANGED gamedata/Datas/racket.csv>>>\n", nil
			}
			return "<<<UNCHANGED gamedata/Datas/text/world.csv>>>\n", nil
		}
		return base(c)
	}}

	plan, err := runPlan(t, root, roster, model, "A tennis game.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	check := plan.Topics[0].DataCheck
	if check == nil || len(check.Found) != 2 || len(check.Open) != 2 || !check.Settled {
		t.Fatalf("the check of the game data: %+v", check)
	}
	if len(plan.DataOpen) != 2 {
		t.Errorf("still found: %v", plan.DataOpen)
	}
	topic := readFile(t, root, plan.Dir()+"/topics/01-rackets.md")
	if !strings.Contains(topic, "2 point(s) found; **2 still found** after the owners' turn.") || !strings.Contains(topic, "- **Still**: gamedata/Datas/racket.csv:2: power is blank") {
		t.Errorf("the minutes of the topic do not say what still stands:\n%s", topic)
	}
	minutes := readFile(t, root, plan.Dir()+"/minutes.md")
	if !strings.Contains(minutes, "The check of the game data still finds 2 point(s) in it:\n\n- gamedata/Datas/racket.csv:2: power is blank") ||
		!strings.Contains(minutes, "- gamedata/Datas/racket.csv:2: name holds racket.wood.name") {
		t.Errorf("the minutes do not say what the check still finds:\n%s", minutes)
	}
	// What was handed in again and left as it was is not on record twice.
	if strings.Count(minutes, "- `gamedata/Datas/racket.csv` (numbers)") != 1 || strings.Contains(minutes, "- `gamedata/Datas/racket.csv` (numbers): left as it was") {
		t.Errorf("the minutes do not say once what became of the table:\n%s", minutes)
	}
	// The copy page shows no text table the plan did not write.
	if exists(root, plan.Dir()+"/"+CopyFile) {
		t.Errorf("a copy page was written, and the plan wrote no text")
	}
	if page := readFile(t, root, plan.Dir()+"/"+TablesFile); !strings.Contains(page, "To look at:\n- gamedata/Datas/racket.csv:2: power is blank") {
		t.Errorf("the tables page does not say what to look at:\n%s", page)
	}
}

// A project without game data is told nothing of it, and nothing is checked.
func TestRun_NoGameData(t *testing.T) {
	root := t.TempDir()
	roster := mustRoster(t, gameRoster)
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(gameAgenda), nil
		case c.Step == "documents" && c.Role == "numbers":
			return dataFile("gamedata/Defines/racket.xml", racketSchema) +
				dataFile("gamedata/Datas/racket.csv", "##var,id,name,power\n,wood,racket.wood.name,") +
				docs("numbers", "docs/balance/rackets.md"), nil
		case c.Step == "documents" && c.Role == "writer":
			return dataFile("gamedata/Datas/text/world.csv", worldText), nil
		}
		return base(c)
	}}
	plan, err := runPlan(t, root, roster, model, "A tennis game.")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plan.Topics[0].DataCheck != nil || len(plan.GameData) != 0 || len(plan.Reviews) != 0 || model.count("numbers", "redata") != 0 {
		t.Errorf("game data was checked in a project that keeps none: %+v", plan.Topics[0].DataCheck)
	}
	if prompt := model.promptOf("owner", "speak"); strings.Contains(prompt, "The game data is kept in") {
		t.Errorf("a project without game data is told of it:\n%s", prompt)
	}
	if agenda := model.promptOf("moderator", "agenda"); strings.Contains(agenda, "game data") || !strings.Contains(agenda, "or a new Markdown file in a folder of that role, named for what it holds.") {
		t.Errorf("the moderator of a project without game data is told of it:\n%s", agenda)
	}
	// Both write at once: without game data nothing waits for the tables.
	if writer := model.promptOf("writer", "documents"); strings.Contains(writer, "this topic wrote before your turn") {
		t.Errorf("the writer is shown the tables in a project without game data:\n%s", writer)
	}
	if readFile(t, root, "gamedata/Datas/racket.csv") == "" {
		t.Error("the table was not written")
	}
}

// A luban.conf that cannot be read does not stop the meeting: it says so, and
// checks nothing.
func TestRun_GameDataThatCannotBeRead(t *testing.T) {
	root := t.TempDir()
	putFile(t, root, "gamedata/luban.conf", "{")
	roster := mustRoster(t, gameRoster)
	var said []string
	base := agreeable(roster)
	model := &table{answer: func(c call) (string, error) {
		switch {
		case c.Step == "agenda":
			return block(gameAgenda), nil
		case c.Step == "documents":
			return dataFile("gamedata/Datas/text/world.csv", worldText) + dataFile("gamedata/Datas/racket.csv", "##var,id\n,wood") +
				dataFile("gamedata/Defines/racket.xml", racketSchema) + docs(c.Role, "docs/balance/rackets.md"), nil
		}
		return base(c)
	}}
	plan, err := Run(t.Context(), Options{Root: root, Brief: "A tennis game.", Roster: roster, Model: model, Repos: testRepos, Now: fixedNow,
		Progress: func(line string) { said = append(said, line) }})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(strings.Join(said, "\n"), "game data: gamedata/luban.conf:") || len(plan.GameData) != 0 {
		t.Errorf("the meeting does not say the game data cannot be read:\n%s", strings.Join(said, "\n"))
	}
}

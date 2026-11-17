package install

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
	"gopkg.in/yaml.v3"
)

// ruleFile reads a rule file of a project, or returns "" when there is none.
func ruleFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(layout.Rules(dir), name+".md"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// commonRules are the rules every project starts with, whatever its stack.
var commonRules = []string{
	"agents", "audit-checklist", "coding-style", "design-before-implementation", "docs-source-of-truth",
	"new-system-owner-boundary", "readiness-update-policy", "security", "testing",
}

// The rules a project starts with are the rule set of the reference project,
// one for one; its git rule is the kit's own git rule, which is generated
// from the configuration. Its rule on architecture is one rule per repo where
// the preset knows the repos (backend, frontend), and one for the whole
// repository where it does not (architecture). A game keeps its numbers and
// text in gamedata/, by a rule of their own.
func TestStarterRules_PerPreset(t *testing.T) {
	godot := []string{
		"agents", "audit-checklist", "backend", "coding-style", "design-before-implementation",
		"docs-source-of-truth", "frontend", "gamedata", "new-system-owner-boundary", "readiness-update-policy",
		"security", "testing",
	}
	if got := StarterRules(PresetGodotGo); !reflect.DeepEqual(got, godot) {
		t.Errorf("StarterRules(godot-go) = %v, want %v", got, godot)
	}
	generic := []string{
		"agents", "architecture", "audit-checklist", "coding-style", "design-before-implementation",
		"docs-source-of-truth", "new-system-owner-boundary", "readiness-update-policy", "security", "testing",
	}
	for _, preset := range []string{PresetGeneric, ""} {
		if got := StarterRules(preset); !reflect.DeepEqual(got, generic) {
			t.Errorf("StarterRules(%q) = %v, want %v", preset, got, generic)
		}
	}
	if got := StarterRules("no-such-preset"); !reflect.DeepEqual(got, commonRules) {
		t.Errorf("StarterRules of a preset that is not known = %v, want the rules every project gets %v", got, commonRules)
	}
	// Every repo of every preset has the rule that holds what its owner
	// decided about it: an answer that is asked for has somewhere to go.
	for _, preset := range Presets {
		for _, r := range PresetRepos(preset) {
			holder := r.Name
			if r.Path == "./" {
				holder = "architecture"
			}
			ctx := answeredContext(t, preset, Answers{Decided: map[string]string{r.Name: "DECIDED-" + r.Name}})
			if text := rendered(t, preset, holder, ctx); !strings.Contains(text, "：「DECIDED-"+r.Name+"」") {
				t.Errorf("%s: no rule carries what the owner decided about %s (looked in %s.md)", preset, r.Name, holder)
			}
		}
	}
}

// presetContext is the context the rules of a preset are written from when
// the owner answered nothing.
func presetContext(t *testing.T, preset string) RuleContext {
	t.Helper()
	return answeredContext(t, preset, Answers{})
}

func answeredContext(t *testing.T, preset string, answers Answers) RuleContext {
	t.Helper()
	var cfg config.Config
	if err := yaml.Unmarshal(presetConfigs[preset]("demo", presetSettings{Branches: DefaultBranches()}), &cfg); err != nil {
		t.Fatalf("preset %q: %v", preset, err)
	}
	return NewRuleContext(&cfg, answers, "2026-11-16")
}

func rendered(t *testing.T, preset, name string, ctx RuleContext) string {
	t.Helper()
	data, err := RenderStarterRule(preset, name, ctx)
	if err != nil {
		t.Fatalf("%s/%s: %v", preset, name, err)
	}
	return string(data)
}

// frontmatterPaths returns the paths a rule says it is about, and its body.
// A rule without frontmatter is about everything.
func frontmatterPaths(t *testing.T, label, text string) (paths []string, body string) {
	t.Helper()
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return nil, text
	}
	head, body, closed := strings.Cut(rest, "\n---\n")
	if !closed {
		t.Fatalf("%s: the frontmatter is not closed", label)
	}
	var meta struct {
		Paths []string `yaml:"paths"`
	}
	if err := yaml.Unmarshal([]byte(head), &meta); err != nil {
		t.Fatalf("%s: the frontmatter is not valid YAML: %v", label, err)
	}
	if len(meta.Paths) == 0 {
		t.Errorf("%s: a frontmatter block that names no path", label)
	}
	return meta.Paths, body
}

// Every rule of every preset can be written, with and without answers, and
// what comes out is a rule file: a title, nothing of the template left in it,
// and no gap where a condition left a section out.
func TestStarterRules_RenderForEveryPreset(t *testing.T) {
	answers := Answers{
		Contract: "backend/api/proto/",
		Decided:  map[string]string{"backend": "HTTP + JSON", "frontend": "plain C#", "root": "a single binary"},
	}
	for _, preset := range Presets {
		for _, ctx := range []RuleContext{presetContext(t, preset), answeredContext(t, preset, answers)} {
			for _, name := range StarterRules(preset) {
				label := preset + "/" + name
				text := rendered(t, preset, name, ctx)
				_, body := frontmatterPaths(t, label, text)
				if !strings.HasPrefix(strings.TrimLeft(body, "\n"), "# ") {
					t.Errorf("%s: the rule does not start with a title", label)
				}
				for _, left := range []string{"{{", "}}", "<no value>", "\r", "\t\n", " \n", "\n\n\n"} {
					if strings.Contains(text, left) {
						t.Errorf("%s: the rule contains %q", label, left)
					}
				}
				if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
					t.Errorf("%s: the rule does not end with exactly one newline", label)
				}
			}
		}
	}
}

// The rules were taken from another project. What was that project's own is
// not in them: its transport, its libraries, its names. Its game data is the
// exception the owner of the kit chose (2026-11-17): a game project keeps
// its numbers and text in gamedata/, for Luban, as that project does. A
// project that is not a game is told nothing of it.
func TestStarterRules_NameNothingOfTheProjectTheyCameFrom(t *testing.T) {
	its := []string{
		"Twirp", "twirp", "Nakama", "nakama", "Spine", "pgx", "River", "wire.go", "wire_gen", "Wire DI",
		"make generate", "make build", "buf ", "immortal", "Cultivation", "cultivation", "修仙", "MatchRPG", "Ollama",
		"Redis", "aether", "combat/", "Albion", "JWT", "RS256", "liveops", "gen.py", "I18nBuild",
	}
	game := []string{"Luban", "gamedata"}
	for _, preset := range Presets {
		ctx := presetContext(t, preset)
		words := its
		if preset != PresetGodotGo {
			words = append(append([]string(nil), its...), game...)
		}
		for _, name := range StarterRules(preset) {
			text := rendered(t, preset, name, ctx)
			for _, word := range words {
				if strings.Contains(text, word) {
					t.Errorf("%s/%s names %q, which belongs to the project the rule came from", preset, name, word)
				}
			}
		}
	}
}

// A rule is loaded when a file it is about is worked on. The rules about code
// are about the directories of the project's repos; a project whose repo is
// the whole repository has no such directory, and they hold everywhere. The
// rule about documents is about documents, in every project. The two that
// are about how agents work are always in force.
func TestStarterRules_AreAboutTheRightFiles(t *testing.T) {
	docs := []string{"docs/**", "**/docs/**", "**/*.md"}
	code := []string{"coding-style", "design-before-implementation", "new-system-owner-boundary", "readiness-update-policy", "security", "testing"}

	godot := presetContext(t, PresetGodotGo)
	generic := presetContext(t, PresetGeneric)
	want := map[string][2][]string{ // rule: {godot-go, generic}
		"agents":               {nil, nil},
		"architecture":         {nil, nil}, // generic only: the whole repository
		"audit-checklist":      {nil, nil},
		"docs-source-of-truth": {docs, docs},
		"backend":              {{"backend/**"}, nil},
		"frontend":             {{"frontend/**"}, nil},
		"gamedata":             {{"gamedata/**"}, nil},
	}
	for _, name := range code {
		want[name] = [2][]string{{"backend/**", "frontend/**"}, nil}
	}

	for _, name := range StarterRules(PresetGodotGo) {
		expected, known := want[name]
		if !known {
			t.Errorf("no expectation for the rule %s: say what it is about", name)
			continue
		}
		if got, _ := frontmatterPaths(t, "godot-go/"+name, rendered(t, PresetGodotGo, name, godot)); !reflect.DeepEqual(got, expected[0]) {
			t.Errorf("godot-go/%s is about %v, want %v", name, got, expected[0])
		}
	}
	for _, name := range StarterRules(PresetGeneric) {
		if got, _ := frontmatterPaths(t, "generic/"+name, rendered(t, PresetGeneric, name, generic)); !reflect.DeepEqual(got, want[name][1]) {
			t.Errorf("generic/%s is about %v, want %v", name, got, want[name][1])
		}
	}

	// A project with one repo in a directory of its own and one that is the
	// whole repository: the code is everywhere.
	mixed := RuleContext{Repos: []RuleRepo{{Name: "tools", Path: "tools/", Language: "go"}, {Name: "root", Path: "./", Language: "go"}}}
	if got := mixed.CodeFrontmatter(); got != "" {
		t.Errorf("CodeFrontmatter with a root repo = %q, want none", got)
	}
	if got := (RuleContext{}).CodeFrontmatter(); got != "" {
		t.Errorf("CodeFrontmatter without repos = %q, want none", got)
	}
}

// What the owner said is decided goes into the rule of that repo in the
// owner's words, with the day; a repo the owner said nothing about says that
// nothing is decided yet, and where the decision will come from.
func TestStarterRules_SayWhatTheOwnerDecided(t *testing.T) {
	ctx := answeredContext(t, PresetGodotGo, Answers{
		Contract: ".\\backend\\api\\proto\\",
		Decided:  map[string]string{"backend": "  Twirp + buf、\n pgx，  Wire  "},
	})

	backend := rendered(t, PresetGodotGo, "backend", ctx)
	if want := "- owner 定（2026-11-16）：「Twirp + buf、 pgx， Wire」"; !strings.Contains(backend, want) {
		t.Errorf("backend.md does not carry the owner's decision as %q:\n%s", want, section(backend, "## 0)"))
	}
	if strings.Contains(backend, "還沒有 owner 定的技術選擇") {
		t.Error("backend.md says nothing is decided, and the owner decided something")
	}
	if want := "契約的唯一來源是 `backend/api/proto/`"; !strings.Contains(backend, want) {
		t.Errorf("backend.md does not name the contract (%q)", want)
	}

	frontend := rendered(t, PresetGodotGo, "frontend", ctx)
	if !strings.Contains(frontend, "還沒有 owner 定的技術選擇") || strings.Contains(frontend, "owner 定（") {
		t.Errorf("frontend.md should say nothing is decided yet:\n%s", section(frontend, "## 0)"))
	}
	if want := "以 `backend/api/proto/` 為唯一來源"; !strings.Contains(frontend, want) {
		t.Errorf("frontend.md does not name the contract (%q)", want)
	}

	for _, name := range []string{"docs-source-of-truth", "new-system-owner-boundary"} {
		with := rendered(t, PresetGodotGo, name, ctx)
		if !strings.Contains(with, "`backend/api/proto/`") {
			t.Errorf("%s.md does not name the contract the owner gave", name)
		}
		without := rendered(t, PresetGodotGo, name, presetContext(t, PresetGodotGo))
		if strings.Contains(without, "backend/api/proto") {
			t.Errorf("%s.md names a contract nobody gave", name)
		}
	}

	// Nothing decided anywhere: neither rule invents a decision.
	for _, name := range []string{"backend", "frontend"} {
		text := rendered(t, PresetGodotGo, name, presetContext(t, PresetGodotGo))
		if !strings.Contains(text, "還沒有 owner 定的技術選擇") || strings.Contains(text, "owner 定（") {
			t.Errorf("%s.md without an answer:\n%s", name, section(text, "## 0)"))
		}
	}

	// A project of one repo: its architecture rule holds the decision, names
	// the repo's architecture document, and the contract when one is named.
	one := rendered(t, PresetGeneric, "architecture", answeredContext(t, PresetGeneric, Answers{Contract: "api/openapi.yaml", Decided: map[string]string{"root": "Rust, axum"}}))
	for _, want := range []string{
		"- owner 定（2026-11-16）：「Rust, axum」",
		"**權威文件：`docs/engineering/architecture.md`**",
		"對外的契約以 `api/openapi.yaml` 為唯一來源",
	} {
		if !strings.Contains(one, want) {
			t.Errorf("architecture.md lacks %q:\n%s", want, one)
		}
	}
	bare := rendered(t, PresetGeneric, "architecture", presetContext(t, PresetGeneric))
	if !strings.Contains(bare, "還沒有 owner 定的技術選擇") || strings.Contains(bare, "owner 定（") || !strings.Contains(bare, "對外的契約只有一個來源，位置寫在權威文件裡") {
		t.Errorf("architecture.md without an answer:\n%s", bare)
	}
}

// A project has rules of its own once a rule file is under its rules
// directory, wherever: Claude Code reads the ones in subdirectories too. The
// kit's folder is the kit's.
func TestHasOwnRules(t *testing.T) {
	write := func(t *testing.T, file string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	if HasOwnRules(dir) {
		t.Error("a project without a rules directory has rules of its own")
	}
	// What is no rule of the project's own: the kit's rules, wherever in the
	// kit's folder; a file that is no rule; a directory with a rule's name.
	write(t, filepath.Join(layout.KitRules(dir), "git-workflow.md"))
	write(t, filepath.Join(layout.KitRules(dir), "more", "security.md"))
	write(t, filepath.Join(layout.Rules(dir), "notes.txt"))
	if err := os.MkdirAll(filepath.Join(layout.Rules(dir), "folder.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if HasOwnRules(dir) {
		t.Error("the kit's rules, a file that is no rule or a directory count as a rule of the project's own")
	}

	flat := t.TempDir()
	write(t, filepath.Join(layout.Rules(flat), "backend.md"))
	if !HasOwnRules(flat) {
		t.Error("a rule file beside the kit's does not count as the project's own")
	}

	nested := t.TempDir()
	write(t, filepath.Join(layout.KitRules(nested), "git-workflow.md"))
	write(t, filepath.Join(layout.Rules(nested), "backend", "api.md"))
	if !HasOwnRules(nested) {
		t.Error("a rule file in a subdirectory does not count as the project's own")
	}
}

// section returns a rule from a heading to the next heading of its level.
func section(text, heading string) string {
	_, rest, found := strings.Cut(text, heading)
	if !found {
		return "(no section " + heading + ")"
	}
	body, _, _ := strings.Cut(rest, "\n## ")
	return heading + body
}

// The rule about documents says where the documents of each repo are: beside
// the repo's code, or, for a repo that is the whole repository, apart from
// the product documents under docs/.
func TestStarterRules_DocumentsOfEveryRepo(t *testing.T) {
	godot := rendered(t, PresetGodotGo, "docs-source-of-truth", presetContext(t, PresetGodotGo))
	for _, want := range []string{
		"| `backend/docs/architecture.md` | `backend` 的工程**唯一權威** |",
		"| `frontend/docs/architecture.md` | `frontend` 的工程**唯一權威** |",
		"| `backend/docs/`（其他） |", "| `frontend/docs/`（其他） |",
		"`backend` → `backend/docs/research/`", "`frontend` → `frontend/docs/research/`",
		"| 工程文件、工程現況 |",
	} {
		if !strings.Contains(godot, want) {
			t.Errorf("godot-go: the rule about documents lacks %q", want)
		}
	}
	if strings.Contains(godot, "docs/engineering") {
		t.Error("godot-go: the rule about documents names docs/engineering, and no repo is the whole repository")
	}

	generic := rendered(t, PresetGeneric, "docs-source-of-truth", presetContext(t, PresetGeneric))
	for _, want := range []string{
		"| `docs/engineering/architecture.md` | `root` 的工程**唯一權威** |",
		"| 工程現況（工程文件放 `docs/engineering/`） |",
		"工程文件不放這裡，放 `docs/engineering/`。",
		"`root` → `docs/engineering/research/`",
	} {
		if !strings.Contains(generic, want) {
			t.Errorf("generic: the rule about documents lacks %q", want)
		}
	}

	// The rows of the table stay one table: no empty line between them.
	for _, text := range []string{godot, generic} {
		table := section(text, "## 文件擁有權")
		rows := strings.Split(strings.TrimSpace(strings.TrimPrefix(table, "## 文件擁有權")), "\n")
		for i, row := range rows {
			if !strings.HasPrefix(row, "|") {
				t.Errorf("line %d of the ownership table is not a row: %q", i+1, row)
			}
		}
	}

	audit := rendered(t, PresetGodotGo, "audit-checklist", presetContext(t, PresetGodotGo))
	if want := "`backend/docs/architecture.md`, `frontend/docs/architecture.md`"; !strings.Contains(audit, want) {
		t.Errorf("the audit checklist does not name the architecture documents (%q)", want)
	}
}

// What a rule says about a language is there when the project has a repo in
// that language, and the numbering of what follows stays whole.
func TestStarterRules_FollowTheLanguagesOfTheProject(t *testing.T) {
	both := presetContext(t, PresetGodotGo)
	none := presetContext(t, PresetGeneric)
	onlyGo := RuleContext{Repos: []RuleRepo{{Name: "server", Path: "server/", Language: "golang"}}}
	onlyCS := RuleContext{Repos: []RuleRepo{{Name: "client", Path: "client/", Language: "c#"}}}

	cases := []struct {
		rule    string
		ctx     RuleContext
		has     []string
		lacks   []string
		comment string
	}{
		{"coding-style", both, []string{"## Go Mutation Rules", "Go code under `backend/`", "neither written into C#"}, []string{"## Mutation\n"}, "Go and C#"},
		{"coding-style", none, []string{"## Mutation\n", "(server: constants or configuration)"}, []string{"Go Mutation Rules", "C#"}, "no known language"},
		{"coding-style", onlyGo, []string{"Go code under `server/`", "(server: constants or configuration)"}, []string{"C#"}, "Go only"},
		{"testing", both, []string{"## 3) Go", "## 4) C#", "## 契約驗收"}, nil, "Go and C#"},
		{"testing", none, []string{"## 2) 測試品質三道 gate", "## Workflow"}, []string{"## 3)", "## 4)", "## 契約驗收", "xUnit"}, "no known language"},
		{"testing", onlyGo, []string{"## 3) Go"}, []string{"## 4)", "C#", "## 契約驗收"}, "Go only"},
		{"testing", onlyCS, []string{"## 3) C#"}, []string{"## 4)", "## 3) Go", "table-driven"}, "C# only"},
		{"new-system-owner-boundary", both, []string{"3. **客戶端 owner 在哪一層？**", "4. **部署與運維是否受影響？**", "5. **文件放哪？**", "、客戶端的層"}, nil, "with a client"},
		{"new-system-owner-boundary", onlyGo, []string{"3. **部署與運維是否受影響？**", "4. **文件放哪？**"}, []string{"客戶端", "5. "}, "without a client"},
		{"security", both, []string{"## 客戶端"}, nil, "with a client"},
		{"security", onlyGo, []string{"## 資料保護"}, []string{"## 客戶端"}, "without a client"},
	}
	for _, c := range cases {
		text := rendered(t, PresetGodotGo, c.rule, c.ctx)
		for _, want := range c.has {
			if !strings.Contains(text, want) {
				t.Errorf("%s (%s) lacks %q", c.rule, c.comment, want)
			}
		}
		for _, gone := range c.lacks {
			if strings.Contains(text, gone) {
				t.Errorf("%s (%s) contains %q", c.rule, c.comment, gone)
			}
		}
	}

	// A contract that is named makes the acceptance of a contract a rule also
	// in a project with one repo.
	single := RuleContext{Repos: []RuleRepo{{Name: "server", Path: "server/", Language: "go"}}, Contract: "api/"}
	if text := rendered(t, PresetGodotGo, "testing", single); !strings.Contains(text, "## 契約驗收") {
		t.Error("testing.md of a project that names a contract has no section on its acceptance")
	}
	if got := onlyGo.PathsOf("go"); got != "`server/`" {
		t.Errorf("PathsOf(go) = %q", got)
	}
	if got := (RuleContext{Repos: []RuleRepo{{Name: "root", Path: "./", Language: "go"}}}).PathsOf("go"); got != "this repository" {
		t.Errorf("PathsOf(go) for a root repo = %q", got)
	}
}

// A repo's directory is written one way in the rules, whatever way the
// configuration writes it.
func TestNewRuleContext_WritesAReposDirectoryOneWay(t *testing.T) {
	cfg := &config.Config{Repos: []config.Repo{
		{Name: "a", Type: "directory", Path: "backend"},
		{Name: "b", Type: "directory", Path: "./frontend/"},
		{Name: "c", Type: "directory", Path: "tools\\gen\\"},
		{Name: "d", Type: "root", Path: "./"},
		{Name: "e", Type: "root", Path: "anything"},
		{Name: "f", Type: "directory", Path: "."},
		{Name: "g", Type: "directory", Path: ""},
	}}
	ctx := NewRuleContext(cfg, Answers{Decided: map[string]string{"a": "x"}}, "2026-01-02")
	want := []string{"backend/", "frontend/", "tools/gen/", "./", "./", "./", "./"}
	for i, r := range ctx.Repos {
		if r.Path != want[i] {
			t.Errorf("repo %s: path %q, want %q", r.Name, r.Path, want[i])
		}
	}
	if ctx.Repos[0].Decided != "x" || ctx.Repos[1].Decided != "" || ctx.Date != "2026-01-02" {
		t.Errorf("the answers did not reach the context: %+v", ctx)
	}
	if got, want := ctx.Repos[0].Docs(), "backend/docs/"; got != want {
		t.Errorf("Docs() = %q, want %q", got, want)
	}
	if got, want := ctx.Repos[3].Docs(), "docs/engineering/"; got != want {
		t.Errorf("Docs() of the root repo = %q, want %q", got, want)
	}
	if root := ctx.RootRepo(); root == nil || root.Name != "d" {
		t.Errorf("RootRepo() = %+v, want the repo d", root)
	}
	if (RuleContext{Repos: ctx.Repos[:3]}).RootRepo() != nil {
		t.Error("RootRepo() found a root among repos in directories")
	}
	if !ctx.Multi() || (RuleContext{Repos: ctx.Repos[:1]}).Multi() {
		t.Error("Multi() does not count the repos")
	}
	if got := ctx.Repo("b").Path; got != "frontend/" {
		t.Errorf("Repo(b).Path = %q", got)
	}
	if got := ctx.Repo("nobody"); got.Name != "" || got.Decided != "" {
		t.Errorf("Repo(nobody) = %+v, want nothing", got)
	}
}

// A new project gets the rule files, written from its own configuration and
// the owner's answers, and its configuration lists exactly those: a listed
// rule that is not there stops the Worker, and a rule that is there and not
// listed is never shown to it.
func TestInstall_ANewProjectStartsWithTheRulesOfItsPreset(t *testing.T) {
	for _, preset := range Presets {
		dir := t.TempDir()
		answers := Answers{Contract: "backend/api/proto/", Decided: map[string]string{"backend": "HTTP + JSON", "root": "one binary"}}
		opts := Options{Preset: preset, ProjectName: "demo", NoGenerate: true, Answers: answers, Today: "2026-11-16"}
		if _, err := Install(createMinimalMockFS(), dir, opts); err != nil {
			t.Fatalf("Install(%s): %v", preset, err)
		}

		ctx := answeredContext(t, preset, answers)
		for _, name := range StarterRules(preset) {
			if got, want := ruleFile(t, dir, name), rendered(t, preset, name, ctx); got != want {
				t.Errorf("%s: .claude/rules/%s.md is not the rule written for this project (%d bytes, want %d)", preset, name, len(got), len(want))
			}
		}
		cfg, err := config.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := cfg.Rules.Custom, StarterRules(preset); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: rules.custom = %v, want the starter rules %v", preset, got, want)
		}
		entries, err := os.ReadDir(layout.Rules(dir))
		if err != nil {
			t.Fatal(err)
		}
		var files []string
		for _, entry := range entries {
			if !entry.IsDir() {
				files = append(files, strings.TrimSuffix(entry.Name(), ".md"))
			}
		}
		if want := StarterRules(preset); !reflect.DeepEqual(files, want) {
			t.Errorf("%s: the rule files are %v, want %v", preset, files, want)
		}
	}

	// The day of the owner's answers is today when the caller names none.
	dir := t.TempDir()
	opts := Options{Preset: PresetGodotGo, NoGenerate: true, Answers: Answers{Decided: map[string]string{"backend": "HTTP"}}}
	if _, err := Install(createMinimalMockFS(), dir, opts); err != nil {
		t.Fatal(err)
	}
	if text := ruleFile(t, dir, "backend"); !strings.Contains(text, "- owner 定（20") || strings.Contains(text, "owner 定（）") {
		t.Errorf("the owner's decision carries no day:\n%s", section(text, "## 0)"))
	}
}

// A rule file is the project's own from the moment it is written. Another
// init keeps it, forced or not; so does an upgrade, which writes none.
func TestInstall_AStarterRuleIsNeverReplaced(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(layout.Rules(dir), "backend.md")
	if err := os.MkdirAll(filepath.Dir(own), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("the project's rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, opts := range []Options{
		{Preset: PresetGodotGo, NoGenerate: true},
		{Preset: PresetGodotGo, NoGenerate: true, Force: true},
		{Preset: PresetGodotGo, NoGenerate: true, ForceConfig: true},
	} {
		if _, err := Install(createMinimalMockFS(), dir, opts); err != nil {
			t.Fatalf("Install(%+v): %v", opts, err)
		}
		if got := ruleFile(t, dir, "backend"); got != "the project's rule" {
			t.Fatalf("Install(%+v) replaced the project's rule: %q", opts, got)
		}
	}
	// The ones the project did not have were written beside it.
	if ruleFile(t, dir, "frontend") == "" || ruleFile(t, dir, "testing") == "" || ruleFile(t, dir, "docs-source-of-truth") == "" {
		t.Error("the rules the project did not have were not written")
	}
}

// The rules go with the configuration that lists them. A project that keeps
// the configuration it has (an upgrade, or an init over an existing project)
// gets no rule file: its configuration would not name it.
func TestInstall_NoStarterRulesWithoutThePresetsConfiguration(t *testing.T) {
	upgraded := t.TempDir()
	if _, err := Install(createMinimalMockFS(), upgraded, Options{Preset: PresetGodotGo, NoGenerate: true, SkipConfig: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := ruleFile(t, upgraded, "backend") + ruleFile(t, upgraded, "security"); got != "" {
		t.Errorf("an upgrade wrote a starter rule: %q", got)
	}

	existing := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(layout.Config(existing)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.Config(existing), []byte("project: {name: demo}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(createMinimalMockFS(), existing, Options{Preset: PresetGodotGo, NoGenerate: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := ruleFile(t, existing, "backend") + ruleFile(t, existing, "security"); got != "" {
		t.Errorf("an init that kept the project's configuration wrote a starter rule: %q", got)
	}
}

// The configuration of a new project names the Worker the owner chose, and
// codex when the owner chose none.
func TestInstall_TheWorkerTheOwnerChose(t *testing.T) {
	for _, preset := range Presets {
		for worker, want := range map[string]string{"": WorkerCodex, WorkerCodex: WorkerCodex, WorkerClaudeCode: WorkerClaudeCode} {
			dir := t.TempDir()
			if _, err := Install(createMinimalMockFS(), dir, Options{Preset: preset, NoGenerate: true, Worker: worker}); err != nil {
				t.Fatalf("Install: %v", err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Worker.Backend != want {
				t.Errorf("%s with the Worker %q: worker.backend = %q, want %q", preset, worker, cfg.Worker.Backend, want)
			}
			if problems := cfg.Validate(); len(problems) != 0 {
				t.Errorf("%s with the Worker %q: the configuration is not valid: %v", preset, worker, problems)
			}
		}
	}
	if !reflect.DeepEqual(WorkerBackends, []string{WorkerCodex, WorkerClaudeCode}) {
		t.Errorf("WorkerBackends = %v", WorkerBackends)
	}
}

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/install"
)

// startChoices is what init starts from when no flag was given, in a new
// repository with one trunk.
func startChoices() initChoices {
	return initChoices{
		Preset:   install.PresetGeneric,
		Worker:   install.WorkerCodex,
		Branches: install.Branches{Integration: "main", Release: "main"},
		Answers:  install.Answers{Decided: map[string]string{}},
	}
}

func asked(t *testing.T, dir, input string, choices initChoices, given initGiven) (initChoices, string) {
	t.Helper()
	var out bytes.Buffer
	got := askInit(strings.NewReader(input), &out, dir, "the project", choices, given)
	return got, out.String()
}

// A person who answers every question decides everything: what the project
// is built with, the skeleton, the Worker, the branches, where the contract
// is, and what is decided about each repo.
func TestAskInit_EveryQuestion(t *testing.T) {
	input := strings.Join([]string{
		"2",                       // the second preset: godot-go
		"y",                       // the skeleton
		"claude-code",             // the Worker
		"develop,main",            // the branches
		"backend/api/proto/",      // the contract
		"Twirp + buf, PostgreSQL", // backend
		"",                        // frontend: nothing decided
	}, "\n") + "\n"
	got, out := asked(t, t.TempDir(), input, startChoices(), initGiven{})

	want := initChoices{
		Preset: install.PresetGodotGo, Scaffold: true, Worker: install.WorkerClaudeCode,
		Branches: install.Branches{Integration: "develop", Release: "main"},
		Answers: install.Answers{
			Contract: "backend/api/proto/",
			Decided:  map[string]string{"backend": "Twirp + buf, PostgreSQL"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("askInit =\n%+v\nwant\n%+v", got, want)
	}

	for _, said := range []string{
		"Setting up AWK in the project.\n",
		"Press Enter to take what is in [brackets]; awkit init --yes asks nothing.",
		"What is the project built with?\n  1  generic ",
		"  2  godot-go ",
		"Its number or its name [generic] > ",
		"A skeleton of the project that builds and passes its tests can be written (backend/, frontend/).\n",
		"Write it, y or n [y] > ",
		"Who is it, codex or claude-code [codex] > ",
		"Pull requests are merged into main, and releases are cut from it too.\n",
		"Keep that, or name the branches as integration,release [main,main] > ",
		"The contract between backend and frontend is the files both sides are generated from (backend/api/proto/, for instance).\n",
		"Where is it [] > ",
		"What is already decided about how each part is built",
		"  backend [] > ",
		"  frontend [] > ",
	} {
		if !strings.Contains(out, said) {
			t.Errorf("the questions lack %q:\n%s", said, out)
		}
	}
}

// Enter takes what is in the brackets. A project of one repo is not asked
// for a skeleton it has none of, nor for a contract between repos.
func TestAskInit_EnterTakesTheDefault(t *testing.T) {
	start := startChoices()
	start.Branches = install.Branches{Integration: "develop", Release: "main"}
	got, out := asked(t, t.TempDir(), "\n\n\n\n\n\n\n\n", start, initGiven{})

	if !reflect.DeepEqual(got, start) {
		t.Errorf("askInit with empty answers =\n%+v\nwant what it started from\n%+v", got, start)
	}
	for _, unasked := range []string{"A skeleton of the project", "The contract between", "  frontend ["} {
		if strings.Contains(out, unasked) {
			t.Errorf("a generic project was asked a question that is not its own (%q):\n%s", unasked, out)
		}
	}
	if !strings.Contains(out, "Pull requests are merged into develop, and releases are cut from main.\n") || !strings.Contains(out, "[develop,main] > ") {
		t.Errorf("the branches of the repository are not shown:\n%s", out)
	}
	if !strings.Contains(out, "  root [] > ") {
		t.Errorf("the one repo is not asked about:\n%s", out)
	}
}

// What a flag decided is not asked: its question is not put, and the answers
// that are typed go to the questions that are.
func TestAskInit_AFlagAnswersItsQuestion(t *testing.T) {
	start := startChoices()
	start.Preset, start.Scaffold, start.Worker = install.PresetGodotGo, true, install.WorkerClaudeCode
	start.Answers = install.Answers{Contract: "api/", Decided: map[string]string{"backend": "from the flag"}}
	given := initGiven{Preset: true, Scaffold: true, Worker: true, Contract: true, Decided: map[string]bool{"backend": true}}

	got, out := asked(t, t.TempDir(), "next\nplain C#\n", start, given)
	want := start
	want.Branches = install.Branches{Integration: "next", Release: "next"}
	want.Answers = install.Answers{Contract: "api/", Decided: map[string]string{"backend": "from the flag", "frontend": "plain C#"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("askInit =\n%+v\nwant\n%+v", got, want)
	}
	for _, unasked := range []string{"What is the project built with", "skeleton", "Who is it", "Where is it", "  backend ["} {
		if strings.Contains(out, unasked) {
			t.Errorf("a question a flag answered was put (%q):\n%s", unasked, out)
		}
	}
	if !strings.Contains(out, "  frontend [] > ") {
		t.Errorf("the repo no flag spoke for was not asked about:\n%s", out)
	}

	// Every repo spoken for: the question about what is decided is not put.
	given.Decided["frontend"] = true
	if _, out := asked(t, t.TempDir(), "\n", start, given); strings.Contains(out, "What is already decided") {
		t.Errorf("every repo had a --decided, and the question was put:\n%s", out)
	}
}

// The skeleton is offered with "y" where there is room for it, and with "n"
// where the project already has one of the directories it would write.
func TestAskInit_TheSkeletonFitsOrNot(t *testing.T) {
	start := startChoices()
	start.Preset = install.PresetGodotGo
	given := initGiven{Preset: true}

	empty := t.TempDir()
	if got, out := asked(t, empty, "\n", start, given); !got.Scaffold || !strings.Contains(out, "Write it, y or n [y] > ") {
		t.Errorf("an empty project: scaffold %v\n%s", got.Scaffold, out)
	}
	if got, _ := asked(t, empty, "n\n", start, given); got.Scaffold {
		t.Error("the person said n, and the skeleton is written")
	}
	if got, _ := asked(t, empty, "YES\n", start, given); !got.Scaffold {
		t.Error("the person said YES, and the skeleton is not written")
	}

	taken := t.TempDir()
	if err := os.Mkdir(filepath.Join(taken, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, out := asked(t, taken, "\n", start, given); got.Scaffold || !strings.Contains(out, "Write it, y or n [n] > ") {
		t.Errorf("a project that has a backend directory: scaffold %v\n%s", got.Scaffold, out)
	}
	if !skeletonFits(empty, install.PresetGodotGo) || skeletonFits(taken, install.PresetGodotGo) || skeletonFits(empty, install.PresetGeneric) {
		t.Error("skeletonFits: an empty godot-go project has room, one with a backend directory and a generic one have not")
	}
}

// An answer that cannot be is said to be wrong and asked for again; after
// three, what is in the brackets stands, and the person is told.
func TestAskInit_AnAnswerThatCannotBe(t *testing.T) {
	got, out := asked(t, t.TempDir(), "unity\ngodot-go\n\n\n\n\n\n\n", startChoices(), initGiven{})
	if got.Preset != install.PresetGodotGo {
		t.Errorf("the second answer was not taken: preset %q", got.Preset)
	}
	if !strings.Contains(out, "\"unity\" is none of them: generic, godot-go\n") || strings.Count(out, "Its number or its name [generic] > ") != 2 {
		t.Errorf("a wrong answer is not asked for again:\n%s", out)
	}

	given := initGiven{Preset: true, Scaffold: true}
	got, out = asked(t, t.TempDir(), "cursor\ncopilot\nhuman\n\n\n", startChoices(), given)
	if got.Worker != install.WorkerCodex {
		t.Errorf("after three wrong answers the Worker is %q, want the default", got.Worker)
	}
	if strings.Count(out, "is neither: codex, claude-code") != 3 || !strings.Contains(out, "  Taking codex.\n") {
		t.Errorf("three wrong answers:\n%s", out)
	}

	got, out = asked(t, t.TempDir(), "a,b,c\nfeature..x\ndevelop main\n\n", startChoices(), initGiven{Preset: true, Worker: true})
	if got.Branches != (install.Branches{Integration: "develop", Release: "main"}) {
		t.Errorf("the branches are %+v", got.Branches)
	}
	if !strings.Contains(out, "name one branch, or two as integration,release") || !strings.Contains(out, "\"feature..x\" is not the name of a branch") {
		t.Errorf("wrong branches:\n%s", out)
	}
}

// When the input ends, nobody is there to ask: what is left takes its
// default, and init goes on.
func TestAskInit_TheInputEnds(t *testing.T) {
	got, out := asked(t, t.TempDir(), "godot-go", startChoices(), initGiven{})
	want := startChoices()
	want.Preset = install.PresetGodotGo
	want.Scaffold = true // the default in an empty project
	if !reflect.DeepEqual(got, want) {
		t.Errorf("askInit with input that ends =\n%+v\nwant\n%+v", got, want)
	}
	if strings.Contains(out, "is none of them") || strings.Contains(out, "Taking") {
		t.Errorf("the end of the input was taken for a wrong answer:\n%s", out)
	}

	// A wrong answer as the last thing typed: the default, said once.
	got, out = asked(t, t.TempDir(), "unity", startChoices(), initGiven{})
	if got.Preset != install.PresetGeneric || strings.Count(out, "is none of them") != 1 || !strings.Contains(out, "  Taking generic.\n") {
		t.Errorf("a wrong answer and then nothing: preset %q\n%s", got.Preset, out)
	}
}

func TestParseBranches(t *testing.T) {
	for text, want := range map[string]install.Branches{
		"main":           {Integration: "main", Release: "main"},
		"develop,main":   {Integration: "develop", Release: "main"},
		"develop, main":  {Integration: "develop", Release: "main"},
		"develop main":   {Integration: "develop", Release: "main"},
		"feat/x,release": {Integration: "feat/x", Release: "release"},
	} {
		if got, err := parseBranches(text); err != nil || got != want {
			t.Errorf("parseBranches(%q) = %+v, %v, want %+v", text, got, err, want)
		}
	}
	for _, text := range []string{"", " , ", "a,b,c", "-x", "a..b", "a~1", "a:b", "x*", "a\\b", "a^", "a?", "a[1"} {
		if got, err := parseBranches(text); err == nil {
			t.Errorf("parseBranches(%q) = %+v, want an error", text, got)
		}
	}
}

func TestDecidedFlag(t *testing.T) {
	decided := decidedFlag{}
	if err := decided.Set("backend= PostgreSQL = the only store "); err != nil {
		t.Fatal(err)
	}
	if err := decided.Set(" frontend =plain C#"); err != nil {
		t.Fatal(err)
	}
	if want := (decidedFlag{"backend": "PostgreSQL = the only store", "frontend": "plain C#"}); !reflect.DeepEqual(decided, want) {
		t.Errorf("decided = %v, want %v", decided, want)
	}
	for _, wrong := range []string{"backend", "=x", "backend=", " = "} {
		if err := (decidedFlag{}).Set(wrong); err == nil || !strings.Contains(err.Error(), "<repo>=<what is decided>") {
			t.Errorf("Set(%q) = %v, want it refused with how to write it", wrong, err)
		}
	}
	if decided.String() != "" {
		t.Errorf("String() = %q", decided.String())
	}

	if err := checkDecided(install.PresetGodotGo, map[string]string{"backend": "x", "frontend": "y"}); err != nil {
		t.Errorf("checkDecided of the preset's repos: %v", err)
	}
	if err := checkDecided(install.PresetGeneric, nil); err != nil {
		t.Errorf("checkDecided of nothing: %v", err)
	}
	err := checkDecided(install.PresetGodotGo, map[string]string{"server": "x"})
	if err == nil || !strings.Contains(err.Error(), "--decided server=") || !strings.Contains(err.Error(), "it has backend, frontend") {
		t.Errorf("checkDecided of a repo the preset has not: %v", err)
	}
	if err := checkDecided(install.PresetGeneric, map[string]string{"backend": "x"}); err == nil || !strings.Contains(err.Error(), "it has root") {
		t.Errorf("checkDecided against the generic preset: %v", err)
	}
}

// The Worker nobody chose is codex, unless only claude is installed here.
func TestDefaultWorker(t *testing.T) {
	previous := initLookPath
	t.Cleanup(func() { initLookPath = previous })
	installed := func(names ...string) {
		initLookPath = func(name string) (string, error) {
			for _, have := range names {
				if have == name {
					return "/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	for _, c := range []struct {
		have []string
		want string
	}{
		{[]string{"codex", "claude"}, install.WorkerCodex},
		{[]string{"codex"}, install.WorkerCodex},
		{[]string{"claude"}, install.WorkerClaudeCode},
		{nil, install.WorkerCodex},
	} {
		installed(c.have...)
		if got := defaultWorker(); got != c.want {
			t.Errorf("with %v installed the Worker is %q, want %q", c.have, got, c.want)
		}
	}
}

// atTerminal makes init believe a person is at the terminal, who types input.
func atTerminal(t *testing.T, input string) {
	t.Helper()
	previousInput, previousTerminal, previousLook := initInput, initAtTerminal, initLookPath
	t.Cleanup(func() { initInput, initAtTerminal, initLookPath = previousInput, previousTerminal, previousLook })
	initInput, initAtTerminal = io.Reader(strings.NewReader(input)), func() bool { return true }
	initLookPath = func(string) (string, error) { return "", errors.New("not found") }
}

func rule(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "rules", name+".md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// At a terminal `awkit init` asks, and what the person answers is what the
// project gets: its configuration, its skeleton, and its rules.
func TestCmdInit_AsksAtATerminal(t *testing.T) {
	dir := inNewDir(t)
	atTerminal(t, "godot-go\ny\nclaude-code\n\nbackend/api/proto/\nTwirp + buf、pgx\n\n")

	out := captureStdout(t, func() {
		if code := cmdInit(nil); code != 0 {
			t.Errorf("cmdInit = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "A few questions first.") || !strings.Contains(out, "AWK is set up in this directory (preset godot-go)") {
		t.Errorf("init at a terminal:\n%s", out)
	}
	if strings.Index(out, "A few questions first.") > strings.Index(out, "AWK is set up") {
		t.Errorf("the questions come after the summary:\n%s", out)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worker.Backend != install.WorkerClaudeCode || len(cfg.Repos) != 2 || cfg.Git.IntegrationBranch != "main" {
		t.Errorf("the configuration does not follow the answers: worker %q, %d repos, integration %q", cfg.Worker.Backend, len(cfg.Repos), cfg.Git.IntegrationBranch)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err != nil {
		t.Errorf("the skeleton the person asked for is not there: %v", err)
	}
	if backend := rule(t, dir, "backend"); !strings.Contains(backend, "：「Twirp + buf、pgx」") || !strings.Contains(backend, "`backend/api/proto/`") {
		t.Errorf("backend.md does not carry the answers:\n%s", backend)
	}
	if frontend := rule(t, dir, "frontend"); !strings.Contains(frontend, "還沒有 owner 定的技術選擇") {
		t.Errorf("frontend.md, about which nothing was answered:\n%s", frontend)
	}

	// A project that keeps its configuration has nowhere to put an answer:
	// nothing is asked.
	atTerminal(t, "generic\n")
	if out := captureStdout(t, func() { cmdInit(nil) }); strings.Contains(out, "A few questions first.") {
		t.Errorf("a second init asked again:\n%s", out)
	}
}

// Nothing is asked where nobody is there to answer, where the person said
// --yes, and in a dry run.
func TestCmdInit_AsksNothing(t *testing.T) {
	for name, args := range map[string][]string{
		"--yes":     {"--yes"},
		"-y":        {"-y"},
		"--dry-run": {"--dry-run"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := inNewDir(t)
			atTerminal(t, "godot-go\ny\nclaude-code\n")
			out := captureStdout(t, func() {
				if code := cmdInit(args); code != 0 {
					t.Errorf("cmdInit = %d, want 0", code)
				}
			})
			if strings.Contains(out, "A few questions first.") {
				t.Errorf("init asked:\n%s", out)
			}
			if name == "--dry-run" {
				if _, err := os.Stat(filepath.Join(dir, ".ai")); err == nil {
					t.Error("a dry run wrote the project")
				}
				return
			}
			if cfg, err := config.Load(dir); err != nil || len(cfg.Repos) != 1 || cfg.Worker.Backend != install.WorkerCodex {
				t.Errorf("init did not take the defaults: %+v, %v", cfg, err)
			}
		})
	}

	// No terminal: a script gets the defaults, as it always did, also when
	// there is input it could read.
	t.Run("no terminal", func(t *testing.T) {
		dir := inNewDir(t)
		atTerminal(t, "godot-go\ny\nclaude-code\n")
		initAtTerminal = func() bool { return false }
		out := captureStdout(t, func() {
			if code := cmdInit(nil); code != 0 {
				t.Errorf("cmdInit = %d, want 0", code)
			}
		})
		if strings.Contains(out, "A few questions first.") {
			t.Errorf("init without a terminal asked:\n%s", out)
		}
		if cfg, err := config.Load(dir); err != nil || len(cfg.Repos) != 1 {
			t.Errorf("init without a terminal: %+v, %v", cfg, err)
		}
	})
}

// The flags say what the questions ask, for a script: the Worker, the
// contract, and what is decided about a repo.
func TestCmdInit_TheAnswersAsFlags(t *testing.T) {
	dir := inNewDir(t)
	out := captureStdout(t, func() {
		args := []string{"--preset", "godot-go", "--worker", "claude-code", "--contract", "backend\\api\\proto\\",
			"--decided", "backend=PostgreSQL, gRPC", "--decided=frontend=plain C#", "--yes"}
		if code := cmdInit(args); code != 0 {
			t.Errorf("cmdInit = %d, want 0", code)
		}
	})
	if strings.Contains(out, "A few questions first.") {
		t.Errorf("init --yes asked:\n%s", out)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worker.Backend != install.WorkerClaudeCode {
		t.Errorf("worker.backend = %q", cfg.Worker.Backend)
	}
	if backend := rule(t, dir, "backend"); !strings.Contains(backend, "：「PostgreSQL, gRPC」") || !strings.Contains(backend, "契約的唯一來源是 `backend/api/proto/`") {
		t.Errorf("backend.md does not carry what the flags said:\n%s", backend)
	}
	if frontend := rule(t, dir, "frontend"); !strings.Contains(frontend, "：「plain C#」") {
		t.Errorf("frontend.md does not carry what --decided=frontend=... said:\n%s", frontend)
	}
}

// A flag that cannot be is refused before anything is written.
func TestCmdInit_AFlagThatCannotBe(t *testing.T) {
	for name, args := range map[string][]string{
		"an unknown Worker":            {"--worker", "cursor"},
		"a repo the preset has not":    {"--preset", "godot-go", "--decided", "server=x", "--yes"},
		"--decided without what":       {"--decided", "backend"},
		"a repo of the generic preset": {"--decided", "backend=x", "--yes"},
	} {
		dir := inNewDir(t)
		var code int
		errOut := captureStderr(func() {
			captureStdout(t, func() { code = cmdInit(args) })
		})
		if code != 2 {
			t.Errorf("%s: cmdInit = %d, want 2\n%s", name, code, errOut)
		}
		if _, err := os.Stat(filepath.Join(dir, ".ai")); err == nil {
			t.Errorf("%s: the project was written all the same", name)
		}
	}
}

// At a terminal, a flag still answers its question: what the person types
// goes to the questions that are left, and all of it reaches the project.
func TestCmdInit_AtATerminalAFlagIsNotAsked(t *testing.T) {
	dir := inNewDir(t)
	atTerminal(t, "n\ndevelop,main\n\nGo modules\n\n")
	out := captureStdout(t, func() {
		if code := cmdInit([]string{"--preset", "godot-go", "--worker", "claude-code"}); code != 0 {
			t.Errorf("cmdInit = %d, want 0", code)
		}
	})
	for _, unasked := range []string{"What is the project built with", "Who is it"} {
		if strings.Contains(out, unasked) {
			t.Errorf("a question a flag answered was put (%q):\n%s", unasked, out)
		}
	}
	if !strings.Contains(out, "Write it, y or n") || !strings.Contains(out, "  backend [] > ") {
		t.Errorf("the questions no flag answered were not put:\n%s", out)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worker.Backend != install.WorkerClaudeCode || cfg.Git.IntegrationBranch != "develop" || cfg.Git.ReleaseBranch != "main" {
		t.Errorf("the configuration: worker %q, branches %s/%s, want claude-code and develop/main", cfg.Worker.Backend, cfg.Git.IntegrationBranch, cfg.Git.ReleaseBranch)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend")); err == nil {
		t.Error("the person said n to the skeleton, and it was written")
	}
	if backend := rule(t, dir, "backend"); !strings.Contains(backend, "：「Go modules」") {
		t.Errorf("backend.md does not carry the answer:\n%s", backend)
	}

	// The configuration is there now. Writing the preset's over it is a
	// reason to ask again: there is a configuration to put the answers into.
	// The rules are the project's by now and stay as they are, so what would
	// go into them is not asked; the person is told where it goes.
	for _, flag := range []string{"--force-config", "--force"} {
		atTerminal(t, "\ncodex\n\nthis line is nobody's answer\n")
		out = captureStdout(t, func() {
			if code := cmdInit([]string{"--preset", "godot-go", flag}); code != 0 {
				t.Errorf("cmdInit %s = %d, want 0", flag, code)
			}
		})
		if !strings.Contains(out, "A few questions first.") || !strings.Contains(out, "Who is it") {
			t.Errorf("init %s did not ask:\n%s", flag, out)
		}
		if strings.Contains(out, "What is already decided") || strings.Contains(out, "Where is it") {
			t.Errorf("init %s asked for what goes into rules it does not write:\n%s", flag, out)
		}
		if !strings.Contains(out, "The project has its rules already (.claude/rules/).") {
			t.Errorf("init %s does not say where what is decided goes:\n%s", flag, out)
		}
		if cfg, err := config.Load(dir); err != nil || cfg.Worker.Backend != install.WorkerCodex {
			t.Errorf("init %s: the answer did not reach the configuration: %+v, %v", flag, cfg.Worker.Backend, err)
		}
		if backend := rule(t, dir, "backend"); !strings.Contains(backend, "：「Go modules」") || strings.Contains(backend, "nobody's answer") {
			t.Errorf("init %s changed the project's rule:\n%s", flag, backend)
		}
	}
}

// The questions whose answers go into the rules are not put to a project
// that has its rules: the rest are.
func TestAskInit_AProjectThatHasItsRules(t *testing.T) {
	start := startChoices()
	start.Preset = install.PresetGodotGo
	got, out := asked(t, t.TempDir(), "n\nclaude-code\nnext\nnot an answer\n", start, initGiven{Preset: true, OwnRules: true})
	want := start
	want.Worker, want.Branches = install.WorkerClaudeCode, install.Branches{Integration: "next", Release: "next"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("askInit =\n%+v\nwant\n%+v", got, want)
	}
	if strings.Contains(out, "The contract between") || strings.Contains(out, "What is already decided") {
		t.Errorf("a project that has its rules was asked for what goes into them:\n%s", out)
	}
	if !strings.Contains(out, "The project has its rules already (.claude/rules/). They stay as they are") {
		t.Errorf("the person is not told where what is decided goes:\n%s", out)
	}
}

// The Worker nobody named is the one installed on this machine.
func TestCmdInit_TheWorkerThatIsInstalled(t *testing.T) {
	dir := inNewDir(t)
	atTerminal(t, "")
	initAtTerminal = func() bool { return false }
	initLookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/bin/claude", nil
		}
		return "", errors.New("not found")
	}
	captureStdout(t, func() {
		if code := cmdInit(nil); code != 0 {
			t.Errorf("cmdInit = %d, want 0", code)
		}
	})
	if cfg, err := config.Load(dir); err != nil || cfg.Worker.Backend != install.WorkerClaudeCode {
		t.Errorf("only claude is installed, and the Worker is %+v (%v)", cfg.Worker.Backend, err)
	}
}

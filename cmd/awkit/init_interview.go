package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silver2dream/ai-workflow-kit/internal/install"
	"golang.org/x/term"
)

// initChoices is what `awkit init` sets a project up from. A flag decides a
// choice; what no flag decided, the person is asked at the terminal; what the
// person is not asked, or leaves open, takes its default.
type initChoices struct {
	Preset   string
	Scaffold bool
	Worker   string
	Branches install.Branches
	Answers  install.Answers
}

// initGiven names the choices a flag made: the person is not asked those.
type initGiven struct {
	Preset, Scaffold, Worker, Contract bool
	// Decided is the repos a --decided flag spoke for.
	Decided map[string]bool
	// OwnRules says the project has rule files of its own already. They are
	// not written over, so the questions whose answers go into the rules are
	// not put.
	OwnRules bool
}

// initInput is where the answers of `awkit init` are read from, and
// initAtTerminal says whether a person is there to give them. Replaced in
// tests.
var (
	initInput      io.Reader = os.Stdin
	initAtTerminal           = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	// initLookPath finds a Worker's command. Replaced in tests.
	initLookPath = exec.LookPath
)

// defaultWorker is the Worker a new project gets when nobody chooses: codex,
// unless only claude is installed here.
func defaultWorker() string {
	if _, err := initLookPath("codex"); err == nil {
		return install.WorkerCodex
	}
	if _, err := initLookPath("claude"); err == nil {
		return install.WorkerClaudeCode
	}
	return install.WorkerCodex
}

// skeletonFits reports whether a preset's skeleton has room in the project:
// the preset has one, and none of its repos' directories is there yet.
func skeletonFits(targetDir, preset string) bool {
	if !install.HasScaffold(preset) {
		return false
	}
	for _, repo := range install.PresetRepos(preset) {
		if _, err := os.Stat(filepath.Join(targetDir, filepath.FromSlash(repo.Path))); err == nil {
			return false
		}
	}
	return true
}

// interview asks a person what no flag decided.
type interview struct {
	in  *bufio.Reader
	out io.Writer
	// ended is set once the input has ended: nobody is there to ask any more,
	// and what is left takes its default.
	ended bool
}

// line reads one answer. An empty line, and the end of the input, are "the
// default".
func (iv *interview) line() string {
	if iv.ended {
		return ""
	}
	text, err := iv.in.ReadString('\n')
	if err != nil {
		iv.ended = true
		fmt.Fprintln(iv.out)
	}
	return strings.TrimSpace(text)
}

// ask puts a question and returns the answer, or the default for an empty
// one. check says what is wrong with an answer; a wrong answer is asked for
// again, twice, and then the default stands.
func (iv *interview) ask(question, fallback string, check func(string) (string, error)) string {
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprintf(iv.out, "%s [%s] > ", question, fallback)
		answer := iv.line()
		if answer == "" {
			return fallback
		}
		if check == nil {
			return answer
		}
		value, err := check(answer)
		if err == nil {
			return value
		}
		fmt.Fprintf(iv.out, "  %v\n", err)
		if iv.ended {
			break
		}
	}
	fmt.Fprintf(iv.out, "  Taking %s.\n", fallback)
	return fallback
}

// askInit asks the person at the terminal what the flags left open, in the
// order the answers depend on each other: the preset decides whether there is
// a skeleton to offer and which repos there are to ask about.
func askInit(in io.Reader, out io.Writer, targetDir, target string, choices initChoices, given initGiven) initChoices {
	iv := &interview{in: bufio.NewReader(in), out: out}

	fmt.Fprintf(out, "\nSetting up AWK in %s.\n", target)
	fmt.Fprintln(out, "A few questions first. Press Enter to take what is in [brackets]; awkit init --yes asks nothing.")

	if !given.Preset {
		fmt.Fprintln(out, "\nWhat is the project built with?")
		for i, preset := range presets {
			fmt.Fprintf(out, "  %d  %-10s %s\n", i+1, preset.Name, preset.Description)
		}
		choices.Preset = iv.ask("Its number or its name", choices.Preset, func(answer string) (string, error) {
			for i, preset := range presets {
				if answer == preset.Name || answer == fmt.Sprint(i+1) {
					return preset.Name, nil
				}
			}
			return "", fmt.Errorf("%q is none of them: %s", answer, availablePresetNames())
		})
	}

	if !given.Scaffold && install.HasScaffold(choices.Preset) {
		fallback := "n"
		if skeletonFits(targetDir, choices.Preset) {
			fallback = "y"
		}
		fmt.Fprintf(out, "\nA skeleton of the project that builds and passes its tests can be written (%s).\n", strings.Join(repoPaths(choices.Preset), ", "))
		choices.Scaffold = iv.ask("Write it, y or n", fallback, yesOrNo) == "y"
	}

	if !given.Worker {
		fmt.Fprintln(out, "\nA Worker writes the code of each ticket.")
		choices.Worker = iv.ask("Who is it, "+strings.Join(install.WorkerBackends, " or "), choices.Worker, func(answer string) (string, error) {
			for _, backend := range install.WorkerBackends {
				if answer == backend {
					return backend, nil
				}
			}
			return "", fmt.Errorf("%q is neither: %s", answer, strings.Join(install.WorkerBackends, ", "))
		})
	}

	if choices.Branches.Integration == choices.Branches.Release {
		fmt.Fprintf(out, "\nPull requests are merged into %s, and releases are cut from it too.\n", choices.Branches.Integration)
	} else {
		fmt.Fprintf(out, "\nPull requests are merged into %s, and releases are cut from %s.\n", choices.Branches.Integration, choices.Branches.Release)
	}
	branches := iv.ask("Keep that, or name the branches as integration,release", choices.Branches.Integration+","+choices.Branches.Release, func(answer string) (string, error) {
		parsed, err := parseBranches(answer)
		if err != nil {
			return "", err
		}
		return parsed.Integration + "," + parsed.Release, nil
	})
	choices.Branches, _ = parseBranches(branches)

	if given.OwnRules {
		fmt.Fprintln(out, "\nThe project has its rules already (.claude/rules/). They stay as they are: what is decided about how it")
		fmt.Fprintln(out, "is built goes into them by hand.")
		fmt.Fprintln(out)
		return choices
	}

	repos := install.PresetRepos(choices.Preset)
	if !given.Contract && len(repos) > 1 {
		fmt.Fprintf(out, "\nThe contract between %s is the files both sides are generated from (%sapi/proto/, for instance).\n", repoNames(repos, " and "), repos[0].Path)
		fmt.Fprintln(out, "It is named in the rules. Leave it empty when that is not decided yet.")
		choices.Answers.Contract = iv.ask("Where is it", choices.Answers.Contract, nil)
	}

	var open []install.PresetRepo
	for _, repo := range repos {
		if !given.Decided[repo.Name] {
			open = append(open, repo)
		}
	}
	if len(open) > 0 {
		fmt.Fprintln(out, "\nWhat is already decided about how each part is built: its libraries, its protocols, its storage?")
		fmt.Fprintln(out, "One line each, in your own words; it goes into that part's rules as your decision. Leave it empty to decide later.")
		if choices.Answers.Decided == nil {
			choices.Answers.Decided = map[string]string{}
		}
		for _, repo := range open {
			if decided := iv.ask("  "+repo.Name, "", nil); decided != "" {
				choices.Answers.Decided[repo.Name] = decided
			}
		}
	}

	fmt.Fprintln(out)
	return choices
}

func yesOrNo(answer string) (string, error) {
	switch strings.ToLower(answer) {
	case "y", "yes":
		return "y", nil
	case "n", "no":
		return "n", nil
	}
	return "", fmt.Errorf("%q is neither y nor n", answer)
}

// parseBranches reads "integration,release", or one branch for both.
func parseBranches(text string) (install.Branches, error) {
	names := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' })
	for _, name := range names {
		if strings.ContainsAny(name, "~^:?*[\\") || strings.HasPrefix(name, "-") || strings.Contains(name, "..") {
			return install.Branches{}, fmt.Errorf("%q is not the name of a branch", name)
		}
	}
	switch len(names) {
	case 1:
		return install.Branches{Integration: names[0], Release: names[0]}, nil
	case 2:
		return install.Branches{Integration: names[0], Release: names[1]}, nil
	}
	return install.Branches{}, fmt.Errorf("name one branch, or two as integration,release")
}

func repoPaths(preset string) []string {
	var paths []string
	for _, repo := range install.PresetRepos(preset) {
		paths = append(paths, repo.Path)
	}
	return paths
}

func repoNames(repos []install.PresetRepo, joint string) string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return strings.Join(names, joint)
}

// decidedFlag collects --decided <repo>=<what is decided>, which may be given
// once for every repo.
type decidedFlag map[string]string

func (d decidedFlag) String() string { return "" }

func (d decidedFlag) Set(value string) error {
	name, text, found := strings.Cut(value, "=")
	name, text = strings.TrimSpace(name), strings.TrimSpace(text)
	if !found || name == "" || text == "" {
		return fmt.Errorf("write it as <repo>=<what is decided>, for instance backend=\"PostgreSQL, gRPC\"")
	}
	d[name] = text
	return nil
}

// checkDecided reports a --decided for a repo the preset does not have.
func checkDecided(preset string, decided map[string]string) error {
	repos := install.PresetRepos(preset)
	for name := range decided {
		known := false
		for _, repo := range repos {
			known = known || repo.Name == name
		}
		if !known {
			return fmt.Errorf("--decided %s=...: the preset %s has no repo of that name (it has %s)", name, preset, repoNames(repos, ", "))
		}
	}
	return nil
}

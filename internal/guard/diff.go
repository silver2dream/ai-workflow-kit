package guard

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A Worker's change can be fine by every check and test and still do what a
// person would want to look at: reach another host, run what it builds at run
// time, change how the project is built or what it depends on. Those are
// signals, not faults; they are recorded with the change, for the reviewer
// and the record. From P2 they feed the risk of a lane.

// DiffVersion names the signals below in every verdict they give.
const DiffVersion = "diff-signals/1"

// MassDeletion is how many deleted lines, outweighing the added ones four to
// one, make a change a mass deletion.
const MassDeletion = 500

type diffSignal struct {
	id      string
	reason  string
	pattern *regexp.Regexp
}

// contentSignals are looked for in the lines a change adds.
var contentSignals = []diffSignal{
	{"network", "it adds a call to another host",
		regexp.MustCompile(`\b(http\.(Get|Post|PostForm|Head|NewRequest(WithContext)?)\(|net\.Dial(Timeout|Context)?\(|tls\.Dial\(|fetch\(\s*['"\x60]https?:|axios(\.(get|post|put|request))?\(|requests\.(get|post|put|patch|request)\(|urllib(\.request)?\.urlopen\(|XMLHttpRequest\(|new WebSocket\(|socket\.(create_connection|connect)\(|HttpClient\(|Invoke-(WebRequest|RestMethod)\b|\b(curl|wget)\s+(-\S+\s+)*https?://)`)},
	{"dynamic-exec", "it adds a command or code run at run time",
		regexp.MustCompile(`(\bexec\.Command(Context)?\(|\bsyscall\.Exec\(|\bsubprocess\.(run|call|Popen|check_output|check_call)\(|\bos\.(system|popen|exec[lv]p?e?)\(|\beval\(|\bnew Function\(|\bchild_process\b|Runtime\.getRuntime\(\)\.exec\(|\bProcessBuilder\(|\bInvoke-Expression\b|\biex\s*\(|\b(ba)?sh\s+-c\s|\bpowershell(\.exe)?\s+-(e|enc|encodedcommand)\b)`)},
	{"install-script", "it adds a script a package manager runs on install",
		regexp.MustCompile(`"(pre|post)?install"\s*:`)},
}

// pathSignal says why a changed file is a signal, "" when it is none.
func pathSignal(file string) (id, reason string) {
	lower := strings.ToLower(file)
	base := path.Base(lower)
	switch {
	case strings.HasPrefix(lower, ".github/workflows/"), strings.HasPrefix(lower, ".github/actions/"),
		strings.HasPrefix(lower, ".circleci/"), base == ".gitlab-ci.yml", base == "jenkinsfile",
		base == "azure-pipelines.yml", base == ".travis.yml", base == "bitbucket-pipelines.yml",
		base == ".pre-commit-config.yaml", base == "makefile", base == "dockerfile",
		strings.HasPrefix(base, "dockerfile."), base == "docker-compose.yml", base == "docker-compose.yaml",
		base == ".goreleaser.yml", base == ".goreleaser.yaml":
		return "ci-build", "it changes how the project is built, tested or released"
	case base == "go.mod", base == "package.json", base == "requirements.txt",
		strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"),
		base == "pyproject.toml", base == "pipfile", base == "setup.py", base == "setup.cfg",
		base == "cargo.toml", base == "gemfile", base == "pom.xml", base == "build.gradle",
		base == "build.gradle.kts", base == "composer.json", strings.HasSuffix(base, ".csproj"),
		base == "packages.config", base == "pubspec.yaml", base == "mix.exs":
		return "dependency", "it changes what the project depends on"
	}
	return "", ""
}

// proseFile is a file whose text is about code, not code: what it says is no
// call.
func proseFile(file string) bool {
	switch strings.ToLower(path.Ext(file)) {
	case ".md", ".markdown", ".txt", ".rst", ".adoc":
		return true
	}
	return false
}

// ScanDiff reads a change (git diff of it, and its --numstat) for the
// signals above. It flags; it never blocks. A verdict names the files, not
// the lines.
func ScanDiff(diff, numstat string) []Verdict {
	found := map[string]map[string]bool{} // signal → files
	reasons := map[string]string{}
	note := func(id, reason, file string) {
		if found[id] == nil {
			found[id] = map[string]bool{}
		}
		found[id][file] = true
		reasons[id] = reason
	}

	file := ""
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if file == "/dev/null" {
				file = ""
			}
			continue
		case strings.HasPrefix(line, "diff --git "):
			file = ""
			if i := strings.LastIndex(line, " b/"); i >= 0 {
				file = line[i+3:]
			}
			continue
		}
		if file == "" || !strings.HasPrefix(line, "+") || proseFile(file) {
			continue
		}
		added := line[1:]
		for _, s := range contentSignals {
			if s.pattern.MatchString(added) {
				note(s.id, s.reason, file)
			}
		}
	}

	adds, dels := 0, 0
	for _, line := range strings.Split(numstat, "\n") {
		fields := strings.SplitN(strings.TrimSuffix(line, "\r"), "\t", 3)
		if len(fields) != 3 {
			continue
		}
		file := numstatPath(fields[2])
		if fields[0] == "-" && fields[1] == "-" {
			note("binary", "it adds or changes a binary file", file)
		} else {
			a, _ := strconv.Atoi(fields[0])
			d, _ := strconv.Atoi(fields[1])
			adds += a
			dels += d
		}
		if id, reason := pathSignal(file); id != "" {
			note(id, reason, file)
		}
	}
	if dels >= MassDeletion && dels > 4*adds {
		note("mass-deletion", "it deletes "+strconv.Itoa(dels)+" lines and adds "+strconv.Itoa(adds), "")
	}

	ids := make([]string, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var verdicts []Verdict
	for _, id := range ids {
		var files []string
		for f := range found[id] {
			if f != "" {
				files = append(files, f)
			}
		}
		sort.Strings(files)
		evidence := strings.Join(files, ", ")
		if len(files) > 5 {
			evidence = strings.Join(files[:5], ", ") + " and " + strconv.Itoa(len(files)-5) + " more"
		}
		verdicts = append(verdicts, Verdict{
			Result: Flag, Class: Security, Stage: StageOutput,
			Rule: DiffVersion + "#" + id, Source: "the Worker's change",
			Reason: reasons[id], Evidence: evidence,
		})
	}
	return verdicts
}

// numstatPath is the path of a numstat line, the new one of a rename
// ("dir/{old => new}/f" or "old => new").
func numstatPath(p string) string {
	if i := strings.Index(p, "{"); i >= 0 {
		if j := strings.Index(p[i:], "}"); j >= 0 {
			inner := p[i+1 : i+j]
			if k := strings.Index(inner, " => "); k >= 0 {
				return strings.TrimPrefix(path.Clean(p[:i]+inner[k+4:]+p[i+j+1:]), "/")
			}
		}
	}
	if k := strings.Index(p, " => "); k >= 0 {
		return p[k+4:]
	}
	return p
}

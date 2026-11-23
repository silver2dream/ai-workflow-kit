package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/silver2dream/ai-workflow-kit/internal/guard"
)

func TestRecordDiffSignals(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	wt := t.TempDir()
	state := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", wt, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(wt, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("main.go", "package main\n\nfunc main() {}\n")
	git("add", ".")
	git("commit", "-q", "-m", "init")

	write("main.go", "package main\n\nimport \"net/http\"\n\nfunc main() { http.Get(\"https://collector.example.net/x\") }\n")
	write(".github/workflows/ci.yml", "on: push\n")
	git("add", ".")

	var logged []string
	recordDiffSignals(context.Background(), wt, state, 9, func(f string, a ...interface{}) {
		logged = append(logged, fmt.Sprintf(f, a...))
	})

	data, err := os.ReadFile(filepath.Join(state, ".ai", "state", "events", guard.Session+".jsonl"))
	if err != nil {
		t.Fatalf("nothing recorded: %v", err)
	}
	record := string(data)
	for _, want := range []string{guard.DiffVersion + "#network", guard.DiffVersion + "#ci-build", `"issue_id":9`, `"result":"flag"`} {
		if !strings.Contains(record, want) {
			t.Errorf("record lacks %s:\n%s", want, record)
		}
	}
	if strings.Contains(record, "collector.example.net") {
		t.Errorf("the record holds the line itself:\n%s", record)
	}
	if len(logged) != 2 {
		t.Errorf("logged %q", logged)
	}

	// An ordinary change: nothing recorded.
	git("commit", "-q", "-m", "change")
	write("main.go", "package main\n\nfunc main() { println(1) }\n")
	git("add", ".")
	quiet := t.TempDir()
	recordDiffSignals(context.Background(), wt, quiet, 9, func(string, ...interface{}) {})
	if _, err := os.Stat(filepath.Join(quiet, ".ai", "state", "events", guard.Session+".jsonl")); err == nil {
		t.Error("an ordinary change was recorded")
	}
}

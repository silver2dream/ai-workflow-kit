package settings

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/landing"
)

// project is a clone of a bare origin whose develop has the settings, as a
// project of AWK has them.
type project struct {
	t      *testing.T
	origin string
	root   string
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func configure(t *testing.T, dir string) {
	for _, kv := range [][2]string{{"user.name", "test"}, {"user.email", "test@example.invalid"}, {"commit.gpgsign", "false"}, {"core.autocrlf", "false"}} {
		run(t, dir, "config", kv[0], kv[1])
	}
}

func put(t *testing.T, dir, path, text string) {
	t.Helper()
	file := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newProject(t *testing.T) *project {
	t.Helper()
	base := t.TempDir()
	p := &project{t: t, origin: filepath.Join(base, "origin.git"), root: filepath.Join(base, "project")}
	run(t, base, "init", "--quiet", "--bare", "--initial-branch=develop", p.origin)
	run(t, base, "clone", "--quiet", p.origin, p.root)
	configure(t, p.root)
	run(t, p.root, "checkout", "--quiet", "-B", "develop")
	put(t, p.root, ".gitignore", ".ai/state/\n")
	put(t, p.root, ".ai/config.yaml", configFile)
	put(t, p.root, ".ai/roster.yaml", rosterFile)
	put(t, p.root, "GENERATED.md", "tennis\n")
	run(t, p.root, "add", "-A")
	run(t, p.root, "commit", "--quiet", "-m", "start")
	run(t, p.root, "push", "--quiet", "-u", "origin", "develop")
	return p
}

// onOrigin is a file as develop on origin has it.
func (p *project) onOrigin(path string) string {
	out, err := exec.Command("git", "--git-dir", p.origin, "show", "develop:"+path).Output()
	if err != nil {
		p.t.Fatalf("%s is not on develop: %v", path, err)
	}
	return string(out)
}

// github merges on the bare origin as GitHub would.
type github struct {
	p      *project
	open   *landing.PR
	opened int
	checks []landing.Check
}

func (g *github) OpenPR(ctx context.Context, root, branch string) (*landing.PR, error) {
	return g.open, nil
}

func (g *github) CreatePR(ctx context.Context, root, base, branch, title, body string) (*landing.PR, error) {
	if base != "develop" || branch != Branch || !strings.HasPrefix(title, "[chore] settings: ") {
		return nil, errors.New("unexpected pull request: " + base + " " + branch + " " + title)
	}
	g.opened++
	g.open = &landing.PR{Number: 60 + g.opened, URL: "https://github.com/acme/tennis/pull/61"}
	return g.open, nil
}

func (g *github) Checks(ctx context.Context, root string, pr int) ([]landing.Check, error) {
	return g.checks, nil
}

func (g *github) Merge(ctx context.Context, root string, pr int, strategy string) error {
	t := g.p.t
	clone := filepath.Join(t.TempDir(), "merge")
	run(t, filepath.Dir(clone), "clone", "--quiet", "-c", "core.autocrlf=false", "--branch", "develop", g.p.origin, clone)
	configure(t, clone)
	run(t, clone, "fetch", "--quiet", "origin", Branch)
	run(t, clone, "merge", "--quiet", "--squash", "FETCH_HEAD")
	run(t, clone, "commit", "--quiet", "-m", "[chore] squashed")
	run(t, clone, "push", "--quiet", "origin", "develop")
	run(t, clone, "push", "--quiet", "origin", "--delete", Branch)
	g.open = nil
	return nil
}

// generated stands in for awkit generate: it writes the project's name.
func generated(root string) error {
	cfg, err := ReadConfig(mustRead(filepath.Join(root, ".ai", "config.yaml")))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "GENERATED.md"), []byte(strings.ToLower(cfg.Name)+"\n"), 0o644)
}

func mustRead(file string) []byte {
	data, _ := os.ReadFile(file)
	return data
}

func (p *project) options(request Request, gh *github) ApplyOptions {
	return ApplyOptions{
		Root: p.root, Request: request, GitHub: gh,
		RunGoing: func(string) bool { return false },
		Generate: generated,
		Local:    func(root string) error { return os.WriteFile(filepath.Join(root, ".ai", "state", "local"), []byte(root), 0o644) },
		Landing: func(o *landing.Options) {
			o.Poll, o.Grace, o.Absent, o.Wait = time.Millisecond, time.Millisecond, time.Millisecond, time.Second
			o.Sleep = func(time.Duration) {}
		},
	}
}

// A change of both files goes to develop through a pull request, with what
// generate writes from the configuration, and the working tree follows.
func TestApply_LandsTheChangeAndBringsTheWorkingTreeUp(t *testing.T) {
	p := newProject(t)
	roster := readRoster(t, rosterFile)
	roster.Model = "opus"
	cfg := readConfig(t, configFile)
	cfg.Name = "Arena"
	gh := &github{p: p, checks: []landing.Check{{Name: "build", Bucket: "pass"}}}
	var saved []string
	opts := p.options(Request{Roster: roster, Config: cfg}, gh)
	opts.Save = func(r *Record) { saved = append(saved, r.Step) }

	record := Apply(context.Background(), opts)
	if record.State != Done || record.Error != "" || record.Left != "" || record.PR != 61 || record.Unchanged {
		t.Fatalf("record = %+v", record)
	}
	if strings.Join(record.What, ",") != "roster,config" {
		t.Errorf("what = %v", record.What)
	}
	if !strings.Contains(p.onOrigin(".ai/roster.yaml"), "model: opus # the default for every role") {
		t.Errorf("roster on develop:\n%s", p.onOrigin(".ai/roster.yaml"))
	}
	if !strings.Contains(p.onOrigin(".ai/config.yaml"), `name: "Arena"`) || p.onOrigin("GENERATED.md") != "arena\n" {
		t.Errorf("config on develop:\n%s\ngenerated: %q", p.onOrigin(".ai/config.yaml"), p.onOrigin("GENERATED.md"))
	}
	if read(t, p.root, ".ai/roster.yaml") != p.onOrigin(".ai/roster.yaml") || read(t, p.root, "GENERATED.md") != "arena\n" {
		t.Error("the working tree was not brought up to develop")
	}
	// What git does not keep is written here; what it keeps came with the
	// branch, and is not written again.
	if read(t, p.root, ".ai/state/local") != p.root {
		t.Error("the local settings were not written here")
	}
	if status := run(t, p.root, "status", "--porcelain"); status != "" {
		t.Errorf("the working tree is not clean:\n%s", status)
	}
	for _, want := range []string{"check", "change", "checks", "sync", "done"} {
		if !contains(saved, want) {
			t.Errorf("step %s was not saved: %v", want, saved)
		}
	}
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// The change is made to the settings as develop has them: a change made
// there meanwhile is kept.
func TestApply_ChangesTheSettingsAsDevelopHasThem(t *testing.T) {
	p := newProject(t)
	other := filepath.Join(t.TempDir(), "other")
	run(t, filepath.Dir(other), "clone", "--quiet", "-c", "core.autocrlf=false", "--branch", "develop", p.origin, other)
	configure(t, other)
	put(t, other, ".ai/config.yaml", strings.Replace(configFile, "merge_strategy: squash", "merge_strategy: rebase", 1))
	run(t, other, "commit", "--quiet", "-am", "elsewhere")
	run(t, other, "push", "--quiet", "origin", "develop")

	// The page showed the settings this working tree has, with squash, and
	// sends them back whole with the test command changed.
	cfg := readConfig(t, configFile)
	cfg.Repos[0].Test = "go test ./..."
	gh := &github{p: p, checks: []landing.Check{{Name: "build", Bucket: "pass"}}}
	record := Apply(context.Background(), p.options(Request{Config: cfg, WasConfig: readConfig(t, configFile)}, gh))
	if record.State != Done {
		t.Fatalf("record = %+v", record)
	}
	there := p.onOrigin(".ai/config.yaml")
	if !strings.Contains(there, "test: go test ./...") || !strings.Contains(there, "merge_strategy: rebase") {
		t.Errorf("develop:\n%s", there)
	}
	// And the working tree, behind, was brought up to it.
	if read(t, p.root, ".ai/config.yaml") != there {
		t.Error("the working tree was not brought up")
	}
}

// Roles changed on develop meanwhile, and here too: nothing is changed.
func TestApply_RolesChangedOnBothSidesAreAConflict(t *testing.T) {
	p := newProject(t)
	other := filepath.Join(t.TempDir(), "other")
	run(t, filepath.Dir(other), "clone", "--quiet", "-c", "core.autocrlf=false", "--branch", "develop", p.origin, other)
	configure(t, other)
	put(t, other, ".ai/roster.yaml", strings.Replace(rosterFile, "title: Skeptic", "title: Doubter", 1))
	run(t, other, "commit", "--quiet", "-am", "elsewhere")
	run(t, other, "push", "--quiet", "origin", "develop")

	roster := readRoster(t, rosterFile)
	roster.Roles[2].Title = "Critic"
	gh := &github{p: p, checks: []landing.Check{{Name: "build", Bucket: "pass"}}}
	record := Apply(context.Background(), p.options(Request{Roster: roster, WasRoster: readRoster(t, rosterFile)}, gh))
	if record.State != Failed || record.Why != "conflict" || gh.opened != 0 {
		t.Errorf("record = %+v, %d opened", record, gh.opened)
	}

	// A change of something else lands, and keeps theirs.
	roster = readRoster(t, rosterFile)
	roster.Model = "opus"
	record = Apply(context.Background(), p.options(Request{Roster: roster, WasRoster: readRoster(t, rosterFile)}, gh))
	if record.State != Done || !strings.Contains(p.onOrigin(".ai/roster.yaml"), "title: Doubter") || !strings.Contains(p.onOrigin(".ai/roster.yaml"), "model: opus") {
		t.Errorf("record = %+v\n%s", record, p.onOrigin(".ai/roster.yaml"))
	}
}

func TestApply_RefusesBeforeAnythingIsPushed(t *testing.T) {
	p := newProject(t)
	head := run(t, p.root, "rev-parse", "HEAD")

	bad := readRoster(t, rosterFile)
	bad.Roles[0].Decides = false
	gh := &github{p: p}
	record := Apply(context.Background(), p.options(Request{Roster: bad}, gh))
	if record.State != Failed || record.Why != "invalid" || !strings.Contains(record.Error, "decide") {
		t.Errorf("invalid: %+v", record)
	}

	opts := p.options(Request{Roster: readRoster(t, rosterFile)}, gh)
	opts.RunGoing = func(string) bool { return true }
	if record := Apply(context.Background(), opts); record.State != Failed || record.Why != "run" {
		t.Errorf("run going: %+v", record)
	}

	if record := Apply(context.Background(), p.options(Request{}, gh)); record.State != Failed || record.Why != "invalid" {
		t.Errorf("nothing asked: %+v", record)
	}

	if gh.opened != 0 {
		t.Error("a pull request was opened")
	}
	if out, _ := exec.Command("git", "--git-dir", p.origin, "branch", "--list", Branch).Output(); strings.TrimSpace(string(out)) != "" {
		t.Error("the branch was pushed")
	}
	if run(t, p.root, "rev-parse", "HEAD") != head {
		t.Error("the working tree moved")
	}
}

// Settings develop has already land nothing: no pull request.
func TestApply_WhatDevelopHasAlreadyLandsNothing(t *testing.T) {
	p := newProject(t)
	gh := &github{p: p}
	record := Apply(context.Background(), p.options(Request{Roster: readRoster(t, rosterFile), Config: readConfig(t, configFile)}, gh))
	if record.State != Done || !record.Unchanged || record.PR != 0 || gh.opened != 0 {
		t.Errorf("record = %+v, %d opened", record, gh.opened)
	}
}

// Checks that fail leave the change on its pull request, not on develop,
// and the working tree as it was.
func TestApply_FailedChecksLeaveDevelopAsItWas(t *testing.T) {
	p := newProject(t)
	roster := readRoster(t, rosterFile)
	roster.Model = "opus"
	gh := &github{p: p, checks: []landing.Check{{Name: "build", Bucket: "fail"}}}
	record := Apply(context.Background(), p.options(Request{Roster: roster}, gh))
	if record.State != Failed || record.Why != "checks" || record.PR != 61 || record.URL == "" {
		t.Errorf("record = %+v", record)
	}
	if strings.Contains(p.onOrigin(".ai/roster.yaml"), "opus") || strings.Contains(read(t, p.root, ".ai/roster.yaml"), "opus") {
		t.Error("the change reached develop or the working tree")
	}
}

// A project without a roster gets the one it would start with, changed.
func TestApply_AProjectWithoutARosterGetsOne(t *testing.T) {
	p := newProject(t)
	run(t, p.root, "rm", "--quiet", ".ai/roster.yaml")
	run(t, p.root, "commit", "--quiet", "-m", "no roster")
	run(t, p.root, "push", "--quiet", "origin", "develop")

	files, err := ReadFiles(p.root)
	if err != nil || !files.RosterMissing {
		t.Fatalf("files = %+v, %v", files, err)
	}
	roster := readRoster(t, string(files.Roster))
	roster.Effort = "low"
	gh := &github{p: p, checks: []landing.Check{{Name: "build", Bucket: "pass"}}}
	record := Apply(context.Background(), p.options(Request{Roster: roster}, gh))
	if record.State != Done {
		t.Fatalf("record = %+v", record)
	}
	if !strings.Contains(p.onOrigin(".ai/roster.yaml"), "effort: low") {
		t.Errorf("develop:\n%s", p.onOrigin(".ai/roster.yaml"))
	}
}

func TestRecordAndRequestFiles(t *testing.T) {
	root := t.TempDir()
	file, err := WriteRequest(root, Request{Roster: &Roster{Model: "opus"}})
	if err != nil {
		t.Fatal(err)
	}
	request, err := ReadRequest(file)
	if err != nil || request.Roster == nil || request.Roster.Model != "opus" || request.Config != nil {
		t.Fatalf("request = %+v, %v", request, err)
	}
	record := &Record{State: Working, Step: "check"}
	for i := 0; i < maxLines+5; i++ {
		record.Say("line")
	}
	if err := WriteRecord(root, record); err != nil {
		t.Fatal(err)
	}
	back, err := ReadRecord(root)
	if err != nil || back.State != Working || len(back.Lines) != maxLines || back.What == nil {
		t.Errorf("record = %+v, %v", back, err)
	}
}

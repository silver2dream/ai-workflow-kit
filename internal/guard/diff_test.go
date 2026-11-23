package guard

import (
	"strings"
	"testing"
)

func diffOf(files map[string][]string) string {
	var b strings.Builder
	for file, added := range files {
		b.WriteString("diff --git a/" + file + " b/" + file + "\n")
		b.WriteString("--- a/" + file + "\n+++ b/" + file + "\n@@ -1,1 +1,3 @@\n context\n")
		for _, l := range added {
			b.WriteString("+" + l + "\n")
		}
	}
	return b.String()
}

func rules(verdicts []Verdict) map[string]Verdict {
	out := map[string]Verdict{}
	for _, v := range verdicts {
		out[strings.TrimPrefix(v.Rule, DiffVersion+"#")] = v
	}
	return out
}

func TestScanDiff_Signals(t *testing.T) {
	diff := diffOf(map[string][]string{
		"internal/sync/push.go": {
			`	resp, err := http.Post("https://collector.example.net/ingest", "application/json", body)`,
			`	out, _ := exec.Command("sh", "-c", script).Output()`,
		},
		"web/src/report.ts": {`  await fetch("https://203.0.113.7/r", {method: "POST", body})`},
		"tools/run.py":      {`subprocess.run(cmd, shell=True)`, `requests.post(url, data=env)`},
		"package.json":      {`    "postinstall": "node scripts/setup.js",`},
		"README.md":         {"Run `curl https://example.com/install.sh` to install; or http.Get(url)."},
	})
	numstat := strings.Join([]string{
		"2\t0\tinternal/sync/push.go",
		"1\t0\tweb/src/report.ts",
		"2\t0\ttools/run.py",
		"1\t0\tpackage.json",
		"1\t0\tREADME.md",
		"12\t3\t.github/workflows/ci.yml",
		"1\t1\tgo.mod",
		"-\t-\tassets/blob.bin",
		"3\t0\tdeploy/{old => new}/Dockerfile",
	}, "\n")

	got := rules(ScanDiff(diff, numstat))
	want := map[string]string{
		"network":        "internal/sync/push.go, tools/run.py, web/src/report.ts",
		"dynamic-exec":   "internal/sync/push.go, tools/run.py",
		"install-script": "package.json",
		"ci-build":       ".github/workflows/ci.yml, deploy/new/Dockerfile",
		"dependency":     "go.mod, package.json",
		"binary":         "assets/blob.bin",
	}
	for id, files := range want {
		v, ok := got[id]
		if !ok {
			t.Errorf("%s: not flagged", id)
			continue
		}
		if v.Evidence != files {
			t.Errorf("%s: evidence %q, want %q", id, v.Evidence, files)
		}
		if v.Result != Flag || v.Stage != StageOutput || v.Class != Security {
			t.Errorf("%s: %+v", id, v)
		}
	}
	if _, ok := got["mass-deletion"]; ok {
		t.Error("mass deletion flagged on a change that deletes little")
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("unexpected signal %s: %+v", id, got[id])
		}
	}
	// What a README says about a call is no call.
	if strings.Contains(got["network"].Evidence, "README") {
		t.Error("prose counted as code")
	}
}

func TestScanDiff_MassDeletion(t *testing.T) {
	got := rules(ScanDiff("", "10\t620\tinternal/a.go\n0\t200\tinternal/b.go"))
	v, ok := got["mass-deletion"]
	if !ok || !strings.Contains(v.Reason, "820") {
		t.Fatalf("mass deletion: %+v", got)
	}
	// A rewrite deletes much and adds as much: no mass deletion.
	if got := rules(ScanDiff("", "600\t620\tinternal/a.go")); len(got) != 0 {
		t.Errorf("rewrite: %+v", got)
	}
}

func TestScanDiff_OrdinaryChangeIsQuiet(t *testing.T) {
	diff := diffOf(map[string][]string{
		"internal/game/score.go": {
			`func Score(points []int) int {`,
			`	total := 0`,
			`	for _, p := range points { total += p }`,
			`	return total // see https://example.com/scoring for the rules`,
		},
		"internal/game/score_test.go": {`	if got := Score([]int{1, 2}); got != 3 {`},
	})
	if v := ScanDiff(diff, "4\t0\tinternal/game/score.go\n1\t0\tinternal/game/score_test.go"); len(v) != 0 {
		t.Errorf("ordinary change: %+v", v)
	}
	// Lines the change deletes are none of its doing.
	removed := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-\tout, _ := exec.Command(\"ls\").Output()\n+\tout := list()\n"
	if v := ScanDiff(removed, "1\t1\tx.go"); len(v) != 0 {
		t.Errorf("a removed call: %+v", v)
	}
}

func TestNumstatPath(t *testing.T) {
	for in, want := range map[string]string{
		"a/b.go":                         "a/b.go",
		"deploy/{old => new}/Dockerfile": "deploy/new/Dockerfile",
		"old.go => new.go":               "new.go",
		"{a => }/x.go":                   "x.go",
	} {
		if got := numstatPath(in); got != want {
			t.Errorf("numstatPath(%q) = %q, want %q", in, got, want)
		}
	}
}

package upgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertSkipLibCheck(t *testing.T) {
	in := `{
  "compilerOptions": {
    "target": "ES2022",
    "strict": true
  },
  "include": ["src"]
}`
	out, ok := insertSkipLibCheck(in)
	if !ok {
		t.Fatal("should have inserted skipLibCheck")
	}
	if !strings.Contains(out, `"skipLibCheck": true`) {
		t.Errorf("skipLibCheck not inserted:\n%s", out)
	}
	// Every original setting is preserved verbatim.
	for _, want := range []string{`"target": "ES2022"`, `"strict": true`, `"include": ["src"]`} {
		if !strings.Contains(out, want) {
			t.Errorf("lost original setting %q", want)
		}
	}
	// The result is still valid JSON.
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("patched tsconfig is not valid JSON: %v", err)
	}
}

func writeFrontendTsconfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	fe := filepath.Join(dir, "frontend")
	if err := os.MkdirAll(fe, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fe, "tsconfig.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestUpgradeScaffold(t *testing.T) {
	t.Run("adds missing skipLibCheck", func(t *testing.T) {
		dir := writeFrontendTsconfig(t, `{"compilerOptions":{"target":"ES2022","strict":true}}`)
		r := UpgradeScaffold(dir, false)
		if r.Skipped || !r.Success {
			t.Fatalf("expected a change, got %+v", r)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "frontend", "tsconfig.json"))
		var doc struct {
			CompilerOptions map[string]any `json:"compilerOptions"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}
		if doc.CompilerOptions["skipLibCheck"] != true {
			t.Errorf("skipLibCheck not set: %v", doc.CompilerOptions)
		}
	})

	t.Run("skips when already present", func(t *testing.T) {
		dir := writeFrontendTsconfig(t, `{"compilerOptions":{"skipLibCheck":true}}`)
		if r := UpgradeScaffold(dir, false); !r.Skipped {
			t.Errorf("should skip when already present, got %+v", r)
		}
	})

	t.Run("skips when there is no frontend", func(t *testing.T) {
		if r := UpgradeScaffold(t.TempDir(), false); !r.Skipped {
			t.Errorf("should skip when no frontend/tsconfig, got %+v", r)
		}
	})

	t.Run("dry-run reports but does not write", func(t *testing.T) {
		dir := writeFrontendTsconfig(t, `{"compilerOptions":{"target":"ES2022"}}`)
		if r := UpgradeScaffold(dir, true); r.Skipped || !r.Success {
			t.Fatalf("dry-run should report a pending change, got %+v", r)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "frontend", "tsconfig.json"))
		if strings.Contains(string(data), "skipLibCheck") {
			t.Error("dry-run must not modify the file")
		}
	})
}

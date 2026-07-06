package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScaffoldResult represents the result of aligning an existing project's
// scaffold with the current templates.
type ScaffoldResult struct {
	Success bool
	Skipped bool
	Changed []string
	Message string
}

// UpgradeScaffold patches known scaffold defects in an existing project in
// place, WITHOUT touching the user's own code — the safe middle ground that
// `upgrade --scaffold` lacked (skip-everything vs. overwrite-everything).
//
// Currently it fixes a frontend tsconfig.json missing skipLibCheck: without it
// `tsc` type-checks build tooling's .d.ts (vite/rollup) and fails, blocking CI
// on a brand-new-but-outdated scaffold. Only the missing key is added; every
// other setting and the file's formatting are preserved.
func UpgradeScaffold(stateRoot string, dryRun bool) ScaffoldResult {
	path := filepath.Join(stateRoot, "frontend", "tsconfig.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// No frontend scaffold in this project — nothing to align.
		return ScaffoldResult{Skipped: true, Message: "no frontend/tsconfig.json to align"}
	}

	// Detect the defect via a real parse (robust to formatting differences).
	var doc struct {
		CompilerOptions map[string]json.RawMessage `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ScaffoldResult{Message: fmt.Sprintf("frontend/tsconfig.json is not valid JSON: %v", err)}
	}
	if _, ok := doc.CompilerOptions["skipLibCheck"]; ok {
		return ScaffoldResult{Skipped: true, Message: "frontend/tsconfig.json already has skipLibCheck"}
	}

	if dryRun {
		return ScaffoldResult{Success: true, Changed: []string{"frontend/tsconfig.json"}, Message: "would add skipLibCheck to frontend/tsconfig.json"}
	}

	patched, ok := insertSkipLibCheck(string(data))
	if !ok {
		return ScaffoldResult{Message: "could not locate compilerOptions in frontend/tsconfig.json"}
	}
	if err := os.WriteFile(path, []byte(patched), 0644); err != nil {
		return ScaffoldResult{Message: fmt.Sprintf("failed to write frontend/tsconfig.json: %v", err)}
	}
	return ScaffoldResult{Success: true, Changed: []string{"frontend/tsconfig.json"}, Message: "added skipLibCheck to frontend/tsconfig.json"}
}

// insertSkipLibCheck adds `"skipLibCheck": true,` immediately after the
// compilerOptions opening brace, matching the indentation of the first existing
// option so the file stays clean and every other line is preserved verbatim.
func insertSkipLibCheck(content string) (string, bool) {
	mi := strings.Index(content, "\"compilerOptions\"")
	if mi < 0 {
		return "", false
	}
	brace := strings.IndexByte(content[mi:], '{')
	if brace < 0 {
		return "", false
	}
	pos := mi + brace + 1 // just past '{'

	// Indentation = leading whitespace of the next non-empty line.
	indent := "    "
	if nl := strings.IndexByte(content[pos:], '\n'); nl >= 0 {
		after := content[pos+nl+1:]
		i := 0
		for i < len(after) && (after[i] == ' ' || after[i] == '\t') {
			i++
		}
		if i > 0 {
			indent = after[:i]
		}
	}

	return content[:pos] + "\n" + indent + "\"skipLibCheck\": true," + content[pos:], true
}

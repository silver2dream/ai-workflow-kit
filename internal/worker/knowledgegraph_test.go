package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testGraphJSON = `{
  "nodes": [
    {"id": "n1", "path": "backend/internal/game/engine.go", "summary": "Core game loop: tick, collision, scoring.", "layer": "domain"},
    {"id": "n2", "path": "backend/internal/server/room.go", "summary": "Room lifecycle and player sessions.", "layer": "application"},
    {"id": "n3", "path": "frontend/src/ui/Hud.tsx", "summary": "Heads-up display component.", "layer": "presentation"},
    {"id": "n4", "path": "backend/internal/game/engine_test.go", "summary": "Engine unit tests."}
  ],
  "edges": [
    {"source": "n2", "target": "n1"},
    {"source": "n4", "target": "n1"}
  ]
}`

func writeTestGraph(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".understand-anything")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "knowledge-graph.json"), []byte(content), 0644); err != nil {
		t.Fatalf("write graph: %v", err)
	}
}

func TestLoadKnowledgeGraphContext_MatchesScopeAndListsDependents(t *testing.T) {
	root := t.TempDir()
	writeTestGraph(t, root, testGraphJSON)

	ticket := "# [feat] improve collision\n\n- Repo: backend\n\n## Scope\n- `backend/internal/game/engine.go` — adjust collision rules\n"
	got := loadKnowledgeGraphContext(root, 42, ticket, nil)

	if !strings.Contains(got, "backend/internal/game/engine.go") {
		t.Errorf("expected matched node path in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Core game loop") {
		t.Errorf("expected node summary in output, got:\n%s", got)
	}
	if !strings.Contains(got, "depended on by:") || !strings.Contains(got, "backend/internal/server/room.go") {
		t.Errorf("expected dependents listing, got:\n%s", got)
	}
}

func TestLoadKnowledgeGraphContext_RepoTokenMatchesWholeRepo(t *testing.T) {
	root := t.TempDir()
	writeTestGraph(t, root, testGraphJSON)

	ticket := "# [fix] backend cleanup\n\n- Repo: backend\n\n## Scope\n(refactor internals)\n"
	got := loadKnowledgeGraphContext(root, 1, ticket, nil)

	if !strings.Contains(got, "backend/internal/game/engine.go") {
		t.Errorf("expected backend nodes matched via Repo token, got:\n%s", got)
	}
	if strings.Contains(got, "Hud.tsx") {
		t.Errorf("frontend node should not match a backend ticket, got:\n%s", got)
	}
}

func TestLoadKnowledgeGraphContext_AbsentFileReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	if got := loadKnowledgeGraphContext(root, 1, "## Scope\n- `a/b.go`", nil); got != "" {
		t.Errorf("expected empty output without graph file, got %q", got)
	}
}

func TestLoadKnowledgeGraphContext_MalformedJSONReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	writeTestGraph(t, root, "{not json")
	if got := loadKnowledgeGraphContext(root, 1, "## Scope\n- `a/b.go`", nil); got != "" {
		t.Errorf("expected empty output for malformed graph, got %q", got)
	}
}

func TestLoadKnowledgeGraphContext_NoPathTokensReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	writeTestGraph(t, root, testGraphJSON)
	if got := loadKnowledgeGraphContext(root, 1, "just prose with no paths at all", nil); got != "" {
		t.Errorf("expected empty output without path tokens, got %q", got)
	}
}

func TestLoadKnowledgeGraphContext_ConfigOff(t *testing.T) {
	root := t.TempDir()
	writeTestGraph(t, root, testGraphJSON)
	cfg := &workflowWorker{KnowledgeGraph: "off"}
	ticket := "## Scope\n- `backend/internal/game/engine.go`"
	if got := loadKnowledgeGraphContext(root, 1, ticket, cfg); got != "" {
		t.Errorf("expected empty output when disabled, got %q", got)
	}
}

func TestLoadKnowledgeGraphContext_PrefersWorktreeCopy(t *testing.T) {
	root := t.TempDir()
	// Root copy has one summary; worktree copy has a different one.
	writeTestGraph(t, root, testGraphJSON)
	wt := filepath.Join(root, ".worktrees", "issue-7")
	writeTestGraph(t, wt, strings.Replace(testGraphJSON, "Core game loop", "Worktree version of engine", 1))

	ticket := "## Scope\n- `backend/internal/game/engine.go`"
	got := loadKnowledgeGraphContext(root, 7, ticket, nil)
	if !strings.Contains(got, "Worktree version of engine") {
		t.Errorf("expected worktree copy to take precedence, got:\n%s", got)
	}
}

func TestLoadKnowledgeGraphContext_OutputCapped(t *testing.T) {
	root := t.TempDir()

	// Build a large graph where every node matches the scope token.
	var sb strings.Builder
	sb.WriteString(`{"nodes": [`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"path": "pkg/mod` + strings.Repeat("x", i%7) + `/file` + string(rune('a'+i%26)) + `.go", "summary": "` + strings.Repeat("long summary segment ", 10) + `"}`)
	}
	sb.WriteString(`]}`)
	writeTestGraph(t, root, sb.String())

	got := loadKnowledgeGraphContext(root, 1, "## Scope\n- `pkg/mod`", nil)
	if len(got) > maxKnowledgeGraphChars+200 {
		t.Errorf("output exceeds cap: %d chars", len(got))
	}
}

func TestParseKnowledgeGraph_LinksAndAltFieldNames(t *testing.T) {
	data := []byte(`{
	  "nodes": [
	    {"name": "a", "file": "src/a.ts", "description": "module a"},
	    {"name": "b", "filePath": "src/b.ts"}
	  ],
	  "links": [{"from": "b", "to": "a"}]
	}`)
	g := parseKnowledgeGraph(data)
	if g == nil || len(g.nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %+v", g)
	}
	if g.nodes[0].Path != "src/a.ts" || g.nodes[0].Summary != "module a" {
		t.Errorf("alt field names not parsed: %+v", g.nodes[0])
	}
	if deps := g.dependents["a"]; len(deps) != 1 || deps[0] != "b" {
		t.Errorf("links not parsed into dependents: %+v", g.dependents)
	}
}

func TestTicketPathTokens_ExtractsFromScopeBackticksAndRepo(t *testing.T) {
	ticket := "# title\n\n- Repo: backend\n\n## Objective\nmentions stray/path.go here (ignored: not scope)\n\n## Scope\n- modify internal/game/engine.go\n- also `cmd/awkit/main.go`\n"
	tokens := ticketPathTokens(ticket)

	joined := strings.Join(tokens, "|")
	for _, want := range []string{"backend/", "internal/game/engine.go", "cmd/awkit/main.go"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing token %q in %v", want, tokens)
		}
	}
	if strings.Contains(joined, "stray/path.go") {
		t.Errorf("objective-section path should not be extracted: %v", tokens)
	}
}

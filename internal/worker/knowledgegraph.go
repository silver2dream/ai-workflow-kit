package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// knowledgeGraphRelPath is where Understand-Anything (and compatible tools)
// commit their codebase knowledge graph.
const knowledgeGraphRelPath = ".understand-anything/knowledge-graph.json"

// maxKnowledgeGraphChars caps the injected CODEBASE MAP section so the graph
// can never crowd out the ticket in the Worker prompt.
const maxKnowledgeGraphChars = 3000

// maxKnowledgeGraphNodes caps how many matched nodes are injected.
const maxKnowledgeGraphNodes = 12

// kgNode is the tolerant view of a knowledge-graph node. The on-disk schema
// is tool-defined ("just JSON"), so every field is optional.
type kgNode struct {
	ID      string
	Path    string
	Summary string
	Layer   string
}

// kgGraph holds parsed nodes plus a reverse dependency index (target -> sources).
type kgGraph struct {
	nodes      []kgNode
	byID       map[string]int
	dependents map[string][]string // node ID -> IDs of nodes that depend on it
}

// loadKnowledgeGraphContext returns a compact, ticket-relevant slice of the
// project's knowledge graph for Worker prompt injection, or "" when the graph
// is absent, malformed, disabled, or nothing matches the ticket. It never
// fails the dispatch: this context is best-effort grounding only.
func loadKnowledgeGraphContext(stateRoot string, issueID int, ticket string, workerCfg *workflowWorker) string {
	if workerCfg != nil && strings.EqualFold(workerCfg.KnowledgeGraph, "off") {
		return ""
	}

	// Prefer the worktree copy (matches the branch being worked on), then
	// fall back to the repo root copy.
	candidates := []string{
		filepath.Join(stateRoot, ".worktrees", fmt.Sprintf("issue-%d", issueID), knowledgeGraphRelPath),
		filepath.Join(stateRoot, knowledgeGraphRelPath),
	}
	var data []byte
	for _, p := range candidates {
		if b, err := os.ReadFile(p); err == nil {
			data = b
			break
		}
	}
	if len(data) == 0 {
		return ""
	}

	graph := parseKnowledgeGraph(data)
	if graph == nil || len(graph.nodes) == 0 {
		return ""
	}

	tokens := ticketPathTokens(ticket)
	if len(tokens) == 0 {
		return ""
	}

	matched := matchNodes(graph, tokens)
	if len(matched) == 0 {
		return ""
	}
	if len(matched) > maxKnowledgeGraphNodes {
		matched = matched[:maxKnowledgeGraphNodes]
	}

	return formatKnowledgeGraphSection(graph, matched)
}

// parseKnowledgeGraph decodes the graph JSON defensively: unknown fields are
// ignored and missing fields tolerated. Returns nil on malformed input.
func parseKnowledgeGraph(data []byte) *kgGraph {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	g := &kgGraph{
		byID:       make(map[string]int),
		dependents: make(map[string][]string),
	}

	rawNodes, _ := raw["nodes"].([]any)
	for _, rn := range rawNodes {
		m, ok := rn.(map[string]any)
		if !ok {
			continue
		}
		node := kgNode{
			ID:      firstString(m, "id", "name"),
			Path:    firstString(m, "path", "file", "filePath", "file_path"),
			Summary: firstString(m, "summary", "description"),
			Layer:   firstString(m, "layer", "type", "kind"),
		}
		if node.Path == "" && node.ID == "" {
			continue
		}
		if node.ID == "" {
			node.ID = node.Path
		}
		g.byID[node.ID] = len(g.nodes)
		g.nodes = append(g.nodes, node)
	}

	rawEdges, _ := raw["edges"].([]any)
	if rawEdges == nil {
		rawEdges, _ = raw["links"].([]any)
	}
	for _, re := range rawEdges {
		m, ok := re.(map[string]any)
		if !ok {
			continue
		}
		source := firstString(m, "source", "from")
		target := firstString(m, "target", "to")
		if source == "" || target == "" {
			continue
		}
		g.dependents[target] = append(g.dependents[target], source)
	}

	return g
}

// firstString returns the first non-empty string value among the given keys.
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// pathTokenRe matches path-like tokens: contains a slash or a filename with
// an extension (e.g. "runner.go", "src/game/engine.ts").
var pathTokenRe = regexp.MustCompile(`[\w./\\-]*[/\\][\w./\\-]+|\b[\w-]+\.[a-zA-Z]{1,10}\b`)

// ticketPathTokens extracts lowercase path-ish tokens from a ticket body,
// favoring backtick-quoted spans and Scope/Repo lines.
func ticketPathTokens(ticket string) []string {
	seen := make(map[string]bool)
	var tokens []string
	add := func(tok string) {
		tok = strings.ToLower(filepath.ToSlash(strings.TrimSpace(tok)))
		tok = strings.TrimPrefix(tok, "./")
		tok = strings.TrimRight(tok, ".,;:") // sentence punctuation; keep "/" so "backend/" stays a prefix token
		// Ignore bare extensions and too-short fragments that would match everything.
		if len(tok) < 4 || !strings.ContainsAny(tok, "/.") {
			return
		}
		if !seen[tok] {
			seen[tok] = true
			tokens = append(tokens, tok)
		}
	}

	// Backtick-quoted spans anywhere in the ticket.
	for _, m := range regexp.MustCompile("`([^`\n]+)`").FindAllStringSubmatch(ticket, -1) {
		for _, tok := range pathTokenRe.FindAllString(m[1], -1) {
			add(tok)
		}
	}

	// Path-like tokens in Scope/Plan sections and Repo metadata lines.
	inRelevantSection := false
	for _, line := range strings.Split(ticket, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			lower := strings.ToLower(trimmed)
			inRelevantSection = strings.Contains(lower, "scope") || strings.Contains(lower, "plan")
			continue
		}
		if lower := strings.ToLower(trimmed); strings.HasPrefix(lower, "- repo:") || strings.HasPrefix(lower, "repo:") {
			repo := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
			if repo != "" {
				add(repo + "/")
			}
			continue
		}
		if inRelevantSection {
			for _, tok := range pathTokenRe.FindAllString(trimmed, -1) {
				add(tok)
			}
		}
	}

	return tokens
}

// matchNodes returns indices of nodes whose path or ID contains any token.
// Results are ordered by path for deterministic output.
func matchNodes(g *kgGraph, tokens []string) []int {
	var matched []int
	for i, node := range g.nodes {
		haystack := strings.ToLower(filepath.ToSlash(node.Path))
		if haystack == "" {
			haystack = strings.ToLower(node.ID)
		}
		for _, tok := range tokens {
			if strings.Contains(haystack, tok) {
				matched = append(matched, i)
				break
			}
		}
	}
	sort.Slice(matched, func(a, b int) bool {
		return g.nodes[matched[a]].Path < g.nodes[matched[b]].Path
	})
	return matched
}

// formatKnowledgeGraphSection renders matched nodes (with their dependents)
// as a compact bullet list capped at maxKnowledgeGraphChars.
func formatKnowledgeGraphSection(g *kgGraph, matched []int) string {
	var b strings.Builder
	b.WriteString("Codebase knowledge graph slice relevant to this ticket (source: " + knowledgeGraphRelPath + ").\n")
	b.WriteString("Use it to locate code and to see what depends on the files you change.\n")
	b.WriteString("It may be stale — always verify against the actual code before relying on it.\n\n")

	for _, idx := range matched {
		node := g.nodes[idx]
		line := "- " + displayName(node)
		if node.Layer != "" {
			line += " [" + node.Layer + "]"
		}
		if node.Summary != "" {
			line += " — " + oneLine(node.Summary, 160)
		}
		line += "\n"

		if deps := g.dependents[node.ID]; len(deps) > 0 {
			names := make([]string, 0, len(deps))
			for _, id := range deps {
				if i, ok := g.byID[id]; ok {
					names = append(names, displayName(g.nodes[i]))
				} else {
					names = append(names, id)
				}
			}
			sort.Strings(names)
			if len(names) > 6 {
				names = append(names[:6], fmt.Sprintf("+%d more", len(names)-6))
			}
			line += "  depended on by: " + strings.Join(names, ", ") + "\n"
		}

		if b.Len()+len(line) > maxKnowledgeGraphChars {
			break
		}
		b.WriteString(line)
	}

	return b.String()
}

// displayName prefers a node's path over its ID for human-readable output.
func displayName(n kgNode) string {
	if n.Path != "" {
		return filepath.ToSlash(n.Path)
	}
	return n.ID
}

// oneLine collapses whitespace and truncates to maxLen.
func oneLine(s string, maxLen int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxLen {
		s = s[:maxLen-3] + "..."
	}
	return s
}

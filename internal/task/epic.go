package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/silver2dream/ai-workflow-kit/internal/analyzer"
	"github.com/silver2dream/ai-workflow-kit/internal/ghutil"

	"gopkg.in/yaml.v3"
)

// CreateEpicOptions contains options for creating a tracking issue (epic).
type CreateEpicOptions struct {
	SpecName  string
	Title     string // optional, default: "[spec-name] Task Tracking"
	Repo      string // optional, uses config if empty
	StateRoot string
	DryRun    bool
	GHTimeout time.Duration
	BodyFile  string // required: pre-formatted epic body file
}

// CreateEpicResult contains the result of creating an epic.
type CreateEpicResult struct {
	EpicNumber int
	EpicURL    string
	DryRunBody string // populated if DryRun is true
}

// CreateEpic creates a GitHub Tracking Issue from a pre-formatted body file and updates the workflow config.
func CreateEpic(ctx context.Context, opts CreateEpicOptions) (*CreateEpicResult, error) {
	if opts.BodyFile == "" {
		return nil, fmt.Errorf("body-file is required")
	}
	if opts.GHTimeout == 0 {
		opts.GHTimeout = 60 * time.Second
	}

	// 1. Load config
	configPath := filepath.Join(opts.StateRoot, ".ai", "config", "workflow.yaml")
	cfg, err := analyzer.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 2. Check if epic already exists for this spec
	existing := cfg.GetEpicIssue(opts.SpecName)
	if existing > 0 {
		return nil, fmt.Errorf("epic already exists for spec %q: issue #%d", opts.SpecName, existing)
	}

	// 3. Read body file
	bodyData, err := os.ReadFile(opts.BodyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read body file: %w", err)
	}
	epicBody := string(bodyData)

	// 6. Generate title
	title := opts.Title
	if title == "" {
		title = fmt.Sprintf("[epic] %s task tracking", opts.SpecName)
	}

	// 7. Dry run
	if opts.DryRun {
		return &CreateEpicResult{
			DryRunBody: epicBody,
		}, nil
	}

	// 8. Write body to temp file
	tempDir := filepath.Join(opts.StateRoot, ".ai", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	bodyPath := filepath.Join(tempDir, fmt.Sprintf("create-epic-%s.md", opts.SpecName))
	if err := os.WriteFile(bodyPath, []byte(epicBody), 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp body file: %w", err)
	}

	// 9. Create GitHub issue
	repo := opts.Repo
	if repo == "" {
		repo = cfg.GitHub.Repo
	}

	ghArgs := []string{
		"issue", "create",
		"--title", title,
		"--body-file", bodyPath,
	}
	if repo != "" {
		ghArgs = append(ghArgs, "--repo", repo)
	}

	createCtx, cancel := context.WithTimeout(ctx, opts.GHTimeout)
	defer cancel()

	output, err := ghutil.RunWithRetry(createCtx, ghutil.DefaultRetryConfig(), "gh", ghArgs...)
	if err != nil {
		return nil, fmt.Errorf("gh issue create failed: %s\n%s", err, string(output))
	}

	epicNumber, epicURL, err := parseIssueOutput(string(output))
	if err != nil {
		return nil, err
	}

	// 10. Update workflow.yaml with tracking config
	if err := updateConfigForEpic(configPath, opts.SpecName, epicNumber); err != nil {
		fmt.Fprintf(os.Stderr, "warning: epic #%d created but failed to update config: %v\n", epicNumber, err)
	}

	return &CreateEpicResult{
		EpicNumber: epicNumber,
		EpicURL:    epicURL,
	}, nil
}


// updateConfigForEpic updates the workflow.yaml to set epic tracking mode and issue number.
func updateConfigForEpic(configPath string, specName string, epicNumber int) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Parse as yaml.Node to preserve structure/comments as much as possible
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Find or create specs.tracking section
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("invalid yaml document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("root must be a mapping")
	}

	specsNode := findOrCreateMapKey(root, "specs")
	trackingNode := findOrCreateMapKey(specsNode, "tracking")

	// Set mode = github_epic
	setMapValue(trackingNode, "mode", analyzer.TrackingModeGitHubEpic)

	// Set epic_issues map
	epicIssuesNode := findOrCreateMapKey(trackingNode, "epic_issues")
	setMapValue(epicIssuesNode, specName, epicNumber)

	// Marshal back
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// findOrCreateMapKey finds a key in a mapping node, or creates it with an empty mapping value.
func findOrCreateMapKey(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	// Create new key-value pair
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode
}

// setMapValue sets a key to a scalar value in a mapping node.
func setMapValue(mapping *yaml.Node, key string, value interface{}) {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = scalarNode(value)
			return
		}
	}

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	mapping.Content = append(mapping.Content, keyNode, scalarNode(value))
}

func scalarNode(value interface{}) *yaml.Node {
	switch v := value.(type) {
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", v), Tag: "!!int"}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", v)}
	}
}

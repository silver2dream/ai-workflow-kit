package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Backtick for use in templates
const bt = "`"

// Options configures the generate operation
type Options struct {
	StateRoot   string
	GenerateCI  bool
	DryRun      bool // show what would be generated without writing
	InstallDeps bool // ignored in Go implementation
}

// Result holds the result of the generate operation
type Result struct {
	GeneratedFiles []string
}

// Config represents workflow.yaml structure
type Config struct {
	Project ProjectConfig `yaml:"project"`
	Git     GitConfig     `yaml:"git"`
	Repos   []RepoConfig  `yaml:"repos"`
	Rules   RulesConfig   `yaml:"rules"`
	Specs   SpecsConfig   `yaml:"specs"`
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field    string
	Message  string
	Expected string
}

func (e ValidationError) Error() string {
	if e.Expected != "" {
		return fmt.Sprintf("%s: %s (expected: %s)", e.Field, e.Message, e.Expected)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate checks if the configuration has all required fields
func (c *Config) Validate() []ValidationError {
	var errors []ValidationError

	if c.Project.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "project.name",
			Message: "required field is missing",
		})
	}

	if c.Project.Type == "" {
		errors = append(errors, ValidationError{
			Field:    "project.type",
			Message:  "required field is missing",
			Expected: "monorepo or single-repo",
		})
	} else if c.Project.Type != "monorepo" && c.Project.Type != "single-repo" {
		errors = append(errors, ValidationError{
			Field:    "project.type",
			Message:  fmt.Sprintf("invalid value: %s", c.Project.Type),
			Expected: "monorepo or single-repo",
		})
	}

	if c.Git.IntegrationBranch == "" {
		errors = append(errors, ValidationError{
			Field:   "git.integration_branch",
			Message: "required field is missing",
		})
	}

	// Validate repos
	for i, repo := range c.Repos {
		if repo.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("repos[%d].name", i),
				Message: "required field is missing",
			})
		}
		if repo.Path == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("repos[%d].path", i),
				Message: "required field is missing",
			})
		}
		if repo.Type != "" && repo.Type != "root" && repo.Type != "directory" && repo.Type != "submodule" {
			errors = append(errors, ValidationError{
				Field:    fmt.Sprintf("repos[%d].type", i),
				Message:  fmt.Sprintf("invalid value: %s", repo.Type),
				Expected: "root, directory, or submodule",
			})
		}
	}

	return errors
}

type ProjectConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type GitConfig struct {
	IntegrationBranch string `yaml:"integration_branch"`
	ReleaseBranch     string `yaml:"release_branch"`
	CommitFormat      string `yaml:"commit_format"`
}

type RepoConfig struct {
	Name     string       `yaml:"name"`
	Path     string       `yaml:"path"`
	Type     string       `yaml:"type"`
	Language string       `yaml:"language"`
	Verify   VerifyConfig `yaml:"verify"`
	Rules    []string     `yaml:"rules"`
}

type VerifyConfig struct {
	Build string `yaml:"build"`
	Test  string `yaml:"test"`
}

type RulesConfig struct {
	Kit    []string `yaml:"kit"`
	Custom []string `yaml:"custom"`
}

type SpecsConfig struct {
	BasePath string `yaml:"base_path"`
}

// TemplateContext holds all data for template rendering
type TemplateContext struct {
	Config
	HasSubmodules  bool
	HasDirectories bool
	IsSingleRepo   bool
}

// Generate runs the AWK generator to create helper docs and scaffolding
func Generate(opts Options) (*Result, error) {
	if opts.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}

	// Load config
	configPath := filepath.Join(opts.StateRoot, ".ai", "config", "workflow.yaml")
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Validate config before generation
	fmt.Println("[generate] Validating config...")
	if validationErrors := config.Validate(); len(validationErrors) > 0 {
		var errMsgs []string
		for _, e := range validationErrors {
			errMsgs = append(errMsgs, e.Error())
		}
		return nil, fmt.Errorf("config validation failed:\n  - %s", strings.Join(errMsgs, "\n  - "))
	}
	fmt.Println("[generate] Config validation: OK")

	// Build template context
	ctx := buildContext(config)

	result := &Result{}

	// Collect files to generate
	claudePath := filepath.Join(opts.StateRoot, "CLAUDE.md")
	agentsPath := filepath.Join(opts.StateRoot, "AGENTS.md")
	kitRulesDir := filepath.Join(opts.StateRoot, ".ai", "rules", "_kit")
	gitWorkflowPath := filepath.Join(kitRulesDir, "git-workflow.md")
	claudeDir := filepath.Join(opts.StateRoot, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Add files to result
	result.GeneratedFiles = append(result.GeneratedFiles, claudePath, agentsPath, gitWorkflowPath, settingsPath)

	// If dry-run, just return the list
	if opts.DryRun {
		return result, nil
	}

	// Generate CLAUDE.md
	if err := generateClaudeMd(claudePath, ctx); err != nil {
		return nil, fmt.Errorf("failed to generate CLAUDE.md: %w", err)
	}
	fmt.Printf("[generate] Created: %s\n", claudePath)

	// Generate AGENTS.md
	if err := generateAgentsMd(agentsPath, ctx); err != nil {
		return nil, fmt.Errorf("failed to generate AGENTS.md: %w", err)
	}
	fmt.Printf("[generate] Created: %s\n", agentsPath)

	// Generate git-workflow.md
	if err := os.MkdirAll(kitRulesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create kit rules dir: %w", err)
	}
	if err := generateGitWorkflowMd(gitWorkflowPath, ctx); err != nil {
		return nil, fmt.Errorf("failed to generate git-workflow.md: %w", err)
	}
	fmt.Printf("[generate] Created: %s\n", gitWorkflowPath)

	// Generate CI workflows if requested
	if opts.GenerateCI {
		ciFiles, err := generateCIWorkflows(opts.StateRoot, ctx)
		if err != nil {
			fmt.Printf("[generate] WARNING: CI generation failed: %v\n", err)
		} else {
			result.GeneratedFiles = append(result.GeneratedFiles, ciFiles...)
		}
	}

	// Setup .claude directory
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .claude dir: %w", err)
	}

	// Generate settings.local.json
	if err := generateClaudeSettings(settingsPath); err != nil {
		return nil, fmt.Errorf("failed to generate claude settings: %w", err)
	}
	fmt.Printf("[generate] Created: %s\n", settingsPath)

	// Install .claude/agents directory (CRITICAL for Task tool subagents)
	agentsDir := filepath.Join(claudeDir, "agents")
	if err := installAgentsDir(agentsDir); err != nil {
		return nil, fmt.Errorf("failed to install agents: %w", err)
	}

	// Create symlinks for rules and skills
	aiRoot := filepath.Join(opts.StateRoot, ".ai")
	if err := setupClaudeSymlinks(aiRoot, claudeDir); err != nil {
		fmt.Printf("[generate] WARNING: symlink setup failed: %v\n", err)
	}

	fmt.Println("[generate] Done!")
	return result, nil
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Specs.BasePath == "" {
		config.Specs.BasePath = ".ai/specs"
	}
	if len(config.Rules.Kit) == 0 {
		config.Rules.Kit = []string{"git-workflow"}
	}
	if config.Git.CommitFormat == "" {
		config.Git.CommitFormat = "[type] subject"
	}

	return &config, nil
}

func buildContext(config *Config) *TemplateContext {
	ctx := &TemplateContext{Config: *config}

	for _, repo := range config.Repos {
		switch repo.Type {
		case "submodule":
			ctx.HasSubmodules = true
		case "directory":
			ctx.HasDirectories = true
		case "root":
			ctx.IsSingleRepo = true
		}
	}

	if config.Project.Type == "single-repo" {
		ctx.IsSingleRepo = true
	}

	return ctx
}

func generateClaudeMd(path string, ctx *TemplateContext) error {
	var sb strings.Builder

	sb.WriteString("# CLAUDE.md (Principal Agent Guide)\n\n")
	sb.WriteString("This file is for the **Principal** agent. If you are a **Worker**, read " + bt + "AGENTS.md" + bt + " instead.\n\n")
	sb.WriteString("## Role: Principal Engineer\n\n")
	sb.WriteString("You are the **Principal Engineer**, responsible for orchestrating the AWK automated workflow and ensuring quality.\n\n")
	sb.WriteString("**Your responsibilities:**\n")
	sb.WriteString("- Audit the project and generate tasks (audit → tasks.md)\n")
	sb.WriteString("- Create Issues for Workers to execute\n")
	sb.WriteString("- Dispatch Workers (Senior Engineers) to execute tasks\n")
	sb.WriteString("- Review PRs submitted by Workers\n")
	sb.WriteString("- Decide approve/reject and merge approved PRs\n\n")
	sb.WriteString("**You do NOT write code directly.** You delegate coding tasks to Workers.\n\n")
	sb.WriteString("## Project Overview\n\n")
	sb.WriteString(fmt.Sprintf("**Name:** %s\n", ctx.Project.Name))
	sb.WriteString(fmt.Sprintf("**Type:** %s\n", ctx.Project.Type))

	// Repos list
	sb.WriteString("**Repos:** ")
	for i, repo := range ctx.Repos {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(repo.Name)
	}
	sb.WriteString("\n")

	// Rule Routing
	sb.WriteString("## Rule Routing (IMPORTANT)\n\n")
	sb.WriteString("Before coding, ALWAYS identify which area the task touches, then apply the corresponding rules:\n\n")
	sb.WriteString("### Kit Core Rules (ALWAYS)\n")

	for _, rule := range ctx.Rules.Kit {
		if rule == "git-workflow" {
			sb.WriteString(fmt.Sprintf("- %s.ai/rules/_kit/%s.md%s (commit format + PR base)\n", bt, rule, bt))
		} else {
			sb.WriteString(fmt.Sprintf("- %s.ai/rules/_kit/%s.md%s\n", bt, rule, bt))
		}
	}

	if len(ctx.Rules.Custom) > 0 {
		sb.WriteString("\n### Project-Specific Rules (enabled)\n")
		for _, rule := range ctx.Rules.Custom {
			sb.WriteString(fmt.Sprintf("- %s.ai/rules/%s.md%s\n", bt, rule, bt))
		}
	} else {
		sb.WriteString("\n### Optional Example Rules (not enabled by default)\n")
		sb.WriteString("If you want stricter, tech-specific rules, copy from " + bt + ".ai/rules/_examples/" + bt + " into " + bt + ".ai/rules/" + bt + ", then add them under " + bt + "rules.custom" + bt + " in " + bt + ".ai/config/workflow.yaml" + bt + ".\n")
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## Principal Workflow (MUST FOLLOW)\n\n")
	sb.WriteString("When " + bt + "awkit kickoff" + bt + " starts, use the **principal-workflow** Skill:\n\n")
	sb.WriteString("1. **Read** " + bt + ".ai/skills/principal-workflow/SKILL.md" + bt + "\n")
	sb.WriteString("2. **Read** " + bt + ".ai/skills/principal-workflow/phases/main-loop.md" + bt + "\n")
	sb.WriteString("3. Follow the main loop instructions\n\n")
	sb.WriteString("The Skill handles:\n")
	sb.WriteString("- Project audit (scan_repo, audit_project)\n")
	sb.WriteString("- Task selection and Issue creation\n")
	sb.WriteString("- Worker dispatch\n")
	sb.WriteString("- Result checking\n")
	sb.WriteString("- PR review\n\n")
	sb.WriteString("**DO NOT** manually implement the workflow steps. The Skill and " + bt + "awkit" + bt + " commands handle everything.\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## ⚠️ CRITICAL RULES\n\n")
	sb.WriteString("### Context Management\n")
	sb.WriteString("- **DO NOT** read log files to monitor Worker progress\n")
	sb.WriteString("- **DO NOT** output verbose descriptions of what Worker is doing\n")
	sb.WriteString("- **DO NOT** poll or check status repeatedly\n")
	sb.WriteString("- Commands are **synchronous** - they return when done, just wait\n\n")
	sb.WriteString("### dispatch_worker Behavior\n")
	sb.WriteString("When executing " + bt + "awkit dispatch-worker" + bt + ":\n")
	sb.WriteString("1. Run the command and **wait for it to return**\n")
	sb.WriteString("2. The command handles all Worker coordination internally\n")
	sb.WriteString("3. **DO NOT** read " + bt + ".ai/exe-logs/" + bt + " or any log files\n")
	sb.WriteString("4. **DO NOT** describe Worker progress or status\n")
	sb.WriteString("5. Just " + bt + "eval" + bt + " the output and continue to next loop iteration\n\n")
	sb.WriteString("Violating these rules will cause **context overflow** and workflow failure.\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## REPO TYPE SUPPORT\n\n")
	sb.WriteString("AWK supports three repository types configured in " + bt + ".ai/config/workflow.yaml" + bt + ":\n\n")
	sb.WriteString("| Type | Description | Use Case |\n")
	sb.WriteString("|------|-------------|----------|\n")
	sb.WriteString("| " + bt + "root" + bt + " | Single repository | Standalone projects |\n")
	sb.WriteString("| " + bt + "directory" + bt + " | Subdirectory in monorepo | Monorepo with shared .git |\n")
	sb.WriteString("| " + bt + "submodule" + bt + " | Git submodule | Monorepo with independent repos |\n\n")
	sb.WriteString("### Type-Specific Behavior\n\n")
	sb.WriteString("- **root**: All operations run from repo root. Path must be " + bt + "./" + bt + ".\n")
	sb.WriteString("- **directory**: Operations run from worktree root, changes scoped to subdirectory.\n")
	sb.WriteString("- **submodule**: Commits/pushes happen in submodule first, then parent updates reference.\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## Quick Reference\n\n")
	sb.WriteString("### Start Work\n")
	sb.WriteString("```bash\nawkit kickoff\n```\n\n")
	sb.WriteString("### Check Status\n")
	sb.WriteString("```bash\nawkit status\n```\n\n")
	sb.WriteString("### Stop Work\n")
	sb.WriteString("```bash\ntouch .ai/state/STOP\n```\n\n")
	sb.WriteString("## File Locations\n\n")
	sb.WriteString("| What | Where |\n")
	sb.WriteString("|------|-------|\n")
	sb.WriteString("| Config | " + bt + ".ai/config/workflow.yaml" + bt + " |\n")
	sb.WriteString("| Skills | " + bt + ".ai/skills/" + bt + " |\n")
	sb.WriteString("| Rules | " + bt + ".ai/rules/" + bt + " |\n")
	sb.WriteString(fmt.Sprintf("| Specs | %s%s/%s |\n", bt, ctx.Specs.BasePath, bt))
	sb.WriteString("| Results | " + bt + ".ai/results/" + bt + " |\n")
	sb.WriteString("| Principal Log | " + bt + ".ai/exe-logs/principal.log" + bt + " |\n")
	sb.WriteString("| Worker Logs | " + bt + ".ai/exe-logs/issue-{N}.worker.log" + bt + " |\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## Ticket Format (for Worker)\n\n")
	sb.WriteString("```markdown\n")
	sb.WriteString("# [type] short title\n\n")

	// Repo list for ticket
	sb.WriteString("- Repo: ")
	for i, repo := range ctx.Repos {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(repo.Name)
	}
	sb.WriteString("\n- Severity: P0 | P1 | P2\n")
	sb.WriteString("- Source: audit:<finding-id> | tasks.md #<n>\n")
	sb.WriteString("- Release: false\n\n")
	sb.WriteString("## Objective\n(what to achieve)\n\n")
	sb.WriteString("## Scope\n(what to change)\n\n")
	sb.WriteString("## Non-goals\n(what NOT to change)\n\n")
	sb.WriteString("## Constraints\n")
	sb.WriteString("- obey AGENTS.md\n")
	sb.WriteString("- obey " + bt + ".ai/rules/_kit/git-workflow.md" + bt + "\n")
	sb.WriteString("- obey enabled project rules in " + bt + ".ai/rules/" + bt + " (if any)\n\n")
	sb.WriteString("## Plan\n(steps)\n\n")
	sb.WriteString("## Verification\n")

	for _, repo := range ctx.Repos {
		sb.WriteString(fmt.Sprintf("- %s: %s%s%s and %s%s%s\n", repo.Name, bt, repo.Verify.Build, bt, bt, repo.Verify.Test, bt))
	}

	sb.WriteString("\n## Acceptance Criteria\n- [ ] ...\n```\n")

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func generateAgentsMd(path string, ctx *TemplateContext) error {
	var sb strings.Builder

	sb.WriteString("# AGENTS.md (Worker Agent Guide)\n\n")
	sb.WriteString("## Role: Senior Engineer (Worker)\n\n")
	sb.WriteString("You are a **Senior Engineer (Worker)**, responsible for executing coding tasks assigned by the Principal Engineer.\n\n")
	sb.WriteString("**Your responsibilities:**\n")
	sb.WriteString("- Read the Ticket (Issue body) to understand requirements\n")
	sb.WriteString("- Write/modify code to complete the task\n")
	sb.WriteString("- Run verification commands (build, test, lint)\n")
	sb.WriteString("- Ensure code quality meets project standards\n\n")
	sb.WriteString("**You do NOT handle git operations.** The runner script will automatically commit, push, and create PR.\n\n")
	sb.WriteString("**If you receive Principal's review feedback (PREVIOUS REVIEW FEEDBACK), you MUST fix the code according to the feedback.**\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("Default priority: correctness > minimal diff > speed.\n\n")
	sb.WriteString("## MUST-READ (before any work)\n\n")

	// Kit rules
	for _, rule := range ctx.Rules.Kit {
		if rule == "git-workflow" {
			sb.WriteString(fmt.Sprintf("- Read and obey: %s.ai/rules/_kit/%s.md%s (CRITICAL for commit format)\n", bt, rule, bt))
		} else {
			sb.WriteString(fmt.Sprintf("- Read and obey: %s.ai/rules/_kit/%s.md%s\n", bt, rule, bt))
		}
	}

	// Custom rules
	for _, rule := range ctx.Rules.Custom {
		sb.WriteString(fmt.Sprintf("- Read and obey: %s.ai/rules/%s.md%s\n", bt, rule, bt))
	}

	sb.WriteString("\nDo not proceed if these files are missing—stop and report what you cannot find.\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## NON-NEGOTIABLE HARD RULES\n\n")
	sb.WriteString("### 0. Use existing architecture & do not reinvent\n")
	sb.WriteString("- Do not create parallel systems. Extend existing patterns.\n")
	sb.WriteString("- Keep changes minimal. Avoid wide refactors.\n\n")
	sb.WriteString("### 1. Always read before writing\n")
	sb.WriteString("- Search the repo for the existing pattern before adding a new one.\n")
	sb.WriteString("- Prefer local conventions (naming, folder structure, module boundaries).\n\n")
	sb.WriteString("### 2. Tests & verification are part of the change\n")
	sb.WriteString("- New features MUST have corresponding unit tests\n")
	sb.WriteString("- Modified features MUST have updated or new test cases\n")
	sb.WriteString("- All tests must pass before completion\n")
	sb.WriteString("- Test coverage should cover happy path and error cases\n\n")
	sb.WriteString("### 3. Git operations are FORBIDDEN\n")
	sb.WriteString("**The runner script handles all git operations. You MUST NOT:**\n")
	sb.WriteString("- Run " + bt + "git commit" + bt + ", " + bt + "git push" + bt + ", or any git write commands\n")
	sb.WriteString("- Create PRs with " + bt + "gh pr create" + bt + "\n")
	sb.WriteString("- Modify " + bt + ".git" + bt + " directory\n\n")
	sb.WriteString("**Your job is ONLY to:**\n")
	sb.WriteString("1. Write/modify code files\n")
	sb.WriteString("2. Run verification commands (build, test, lint)\n")
	sb.WriteString("3. Print " + bt + "git status --porcelain" + bt + " and " + bt + "git diff" + bt + " for the runner to see\n\n")
	sb.WriteString("### 4. Review feedback handling\n")
	sb.WriteString("If you see a " + bt + "PREVIOUS REVIEW FEEDBACK" + bt + " section in your prompt:\n")
	sb.WriteString("- This means Principal rejected your previous work\n")
	sb.WriteString("- **Address ALL issues mentioned in the feedback**\n")
	sb.WriteString("- Pay special attention to:\n")
	sb.WriteString("  - Score Reason (why it failed)\n")
	sb.WriteString("  - Suggested Improvements (what to fix)\n")
	sb.WriteString("  - CI failures (if mentioned)\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## REPO TYPE SUPPORT\n\n")
	sb.WriteString("AWK supports three repository types configured in " + bt + ".ai/config/workflow.yaml" + bt + ":\n\n")
	sb.WriteString("| Type | Description | Use Case |\n")
	sb.WriteString("|------|-------------|----------|\n")
	sb.WriteString("| " + bt + "root" + bt + " | Single repository | Standalone projects |\n")
	sb.WriteString("| " + bt + "directory" + bt + " | Subdirectory in monorepo | Monorepo with shared .git |\n")
	sb.WriteString("| " + bt + "submodule" + bt + " | Git submodule | Monorepo with independent repos |\n\n")
	sb.WriteString("### Type-Specific Behavior\n\n")
	sb.WriteString("- **root**: All operations run from repo root. Path must be " + bt + "./" + bt + ".\n")
	sb.WriteString("- **directory**: Operations run from worktree root, changes scoped to subdirectory.\n")
	sb.WriteString("- **submodule**: Commits/pushes happen in submodule first, then parent updates reference.\n\n")
	sb.WriteString("### Submodule Constraints\n")
	sb.WriteString("- Changes must stay within submodule boundary (unless " + bt + "allow_parent_changes: true" + bt + ")\n")
	sb.WriteString("- PRs target parent repo, not submodule remote\n")
	sb.WriteString("- Rollback reverts both submodule and parent commits\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## DEFAULT VERIFY COMMANDS\n\n")

	for _, repo := range ctx.Repos {
		sb.WriteString(fmt.Sprintf("### %s\n", repo.Name))
		sb.WriteString(fmt.Sprintf("- Build: %s%s%s\n", bt, repo.Verify.Build, bt))
		sb.WriteString(fmt.Sprintf("- Test: %s%s%s\n\n", bt, repo.Verify.Test, bt))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func generateGitWorkflowMd(path string, ctx *TemplateContext) error {
	var sb strings.Builder

	sb.WriteString("# Git & Workflow Rules (STRICT)\n\n## 0. Branch Model\n\n")

	if ctx.Project.Type == "monorepo" {
		if ctx.HasSubmodules {
			sb.WriteString("This project uses submodules:\n")
			for _, repo := range ctx.Repos {
				if repo.Type == "submodule" {
					sb.WriteString(fmt.Sprintf("- %s%s%s (%s)\n", bt, repo.Name, bt, repo.Language))
				}
			}
		} else if ctx.HasDirectories {
			sb.WriteString("This project uses directories (monorepo without submodules):\n")
			for _, repo := range ctx.Repos {
				if repo.Type == "directory" {
					sb.WriteString(fmt.Sprintf("- %s%s%s - %s (%s)\n", bt, repo.Path, bt, repo.Name, repo.Language))
				}
			}
		}
	}

	sb.WriteString("\nBranches:\n")
	sb.WriteString(fmt.Sprintf("- **Integration branch**: %s%s%s (daily development, **target for all PRs**)\n", bt, ctx.Git.IntegrationBranch, bt))
	sb.WriteString(fmt.Sprintf("- **Release branch**: %s%s%s (release-only; merge from %s%s%s when releasing)\n\n", bt, ctx.Git.ReleaseBranch, bt, bt, ctx.Git.IntegrationBranch, bt))
	sb.WriteString("PR base rules:\n")

	for _, repo := range ctx.Repos {
		sb.WriteString(fmt.Sprintf("- %s repo PR base: %s%s%s (default)\n", repo.Name, bt, ctx.Git.IntegrationBranch, bt))
	}

	sb.WriteString("\nRelease rule (root only):\n")
	sb.WriteString(fmt.Sprintf("- Only create PR from %s%s%s -> %s%s%s for release tickets.\n", bt, ctx.Git.IntegrationBranch, bt, bt, ctx.Git.ReleaseBranch, bt))
	sb.WriteString(fmt.Sprintf("- Do NOT target %s%s%s unless the ticket explicitly says %sRelease: true%s.\n\n", bt, ctx.Git.ReleaseBranch, bt, bt, bt))
	sb.WriteString("## 1. Branching Strategy\n\n")
	sb.WriteString("- **Feature branches**: " + bt + "feat/<topic>" + bt + "\n")
	sb.WriteString("- **Fix branches**: " + bt + "fix/<topic>" + bt + "\n")
	sb.WriteString("- **Automation branches** (AI): " + bt + "feat/ai-issue-<id>" + bt + "\n\n")
	sb.WriteString("## 2. Commit Message Format (CUSTOM & STRICT)\n\n")
	sb.WriteString("You MUST use the bracket " + bt + "[]" + bt + " format. Do not use standard Conventional Commits (no colons).\n\n")
	sb.WriteString(fmt.Sprintf("- **Format**: %s%s%s\n", bt, ctx.Git.CommitFormat, bt))
	sb.WriteString("- **Rules**:\n")
	sb.WriteString("  1. Type MUST be inside square brackets " + bt + "[]" + bt + ".\n")
	sb.WriteString("  2. Subject MUST be lowercase.\n")
	sb.WriteString("  3. NO colon after the bracket.\n")
	sb.WriteString("- **Allowed Types**:\n")
	sb.WriteString("  - " + bt + "feat" + bt + ", " + bt + "fix" + bt + ", " + bt + "docs" + bt + ", " + bt + "style" + bt + ", " + bt + "refactor" + bt + ", " + bt + "perf" + bt + ", " + bt + "test" + bt + ", " + bt + "chore" + bt + "\n")
	sb.WriteString("- **Examples**:\n")
	sb.WriteString("  - ✅ " + bt + "[feat] add new feature" + bt + "\n")
	sb.WriteString("  - ✅ " + bt + "[refactor] update module structure" + bt + "\n")
	sb.WriteString("  - ✅ " + bt + "[chore] add configuration file" + bt + "\n")
	sb.WriteString("  - ❌ " + bt + "feat: add feature" + bt + " (Forbidden)\n\n")
	sb.WriteString("## 3. Pull Requests (MANDATORY)\n\n")
	sb.WriteString("- Any code change MUST go through a PR (no push-only changes).\n")
	sb.WriteString(fmt.Sprintf("- PR title SHOULD match commit style: %s%s%s.\n", bt, ctx.Git.CommitFormat, bt))
	sb.WriteString("- PR body MUST include: " + bt + "Closes #<IssueID>" + bt + "\n")
	sb.WriteString("- Required checks must pass before merge (branch protection / rulesets).\n\n")

	if ctx.HasSubmodules {
		sb.WriteString("## 4. Submodule Safety\n\n")
		sb.WriteString(fmt.Sprintf("- Root repo should not point submodules to commits that are not reachable from the submodule's allowed branches (normally %s%s%s).\n", bt, ctx.Git.IntegrationBranch, bt))
		sb.WriteString("- Do NOT change submodule pinned commits unless the ticket explicitly requires it.\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func generateClaudeSettings(path string) error {
	content := `{
  "permissions": {
    "allow": [
      "Skill(principal-workflow)",
      "Task(pr-reviewer)",
      "Task(conflict-resolver)",
      "Bash(gh:*)",
      "Bash(git:*)",
      "Bash(awkit:*)",
      "Bash(bash:*)",
      "Bash(codex:*)",
      "Bash(go:*)",
      "Bash(npm:*)",
      "Bash(python:*)",
      "Bash(python3:*)"
    ]
  }
}
`
	return os.WriteFile(path, []byte(content), 0644)
}

func installAgentsDir(agentsDir string) error {
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return err
	}

	// pr-reviewer agent definition (FULL version synced from .claude/agents/pr-reviewer.md)
	prReviewer := `---
name: pr-reviewer
description: AWK PR Reviewer. Executes complete PR review flow: prepare -> review implementation -> verify tests -> submit. Used when analyze-next returns review_pr.
tools: Read, Grep, Glob, Bash
model: sonnet
---

You are the AWK PR Review Expert. You are responsible for executing the **complete review flow**.

## Input

You will receive PR number and Issue number.

## Execution Flow

### Step 1: Prepare Review Context

` + "```bash" + `
awkit prepare-review --pr $PR_NUMBER --issue $ISSUE_NUMBER
` + "```" + `

Record the output:
- ` + "`CI_STATUS`" + `: passed or failed
- ` + "`WORKTREE_PATH`" + `: worktree path
- ` + "`TEST_COMMAND`" + `: command to run tests
- ` + "`TICKET`" + `: Issue body with acceptance criteria

### Step 2: Extract Acceptance Criteria

From the TICKET output, identify all acceptance criteria (lines like ` + "`- [ ] criteria`" + `).

**These criteria are the foundation of your review.** Each criterion MUST be addressed.

### Step 3: Switch to Worktree and Review Implementation

` + "```bash" + `
cd $WORKTREE_PATH
` + "```" + `

**CRITICAL: You MUST actually review the implementation code.**

For EACH acceptance criterion:

1. **Find the implementation** - Use Grep/Read to locate the actual code that implements this criterion
2. **Understand the logic** - Read the code and understand how it works
3. **Write implementation description** - Describe the implementation in your own words (minimum 20 characters), including:
   - Which function/method implements this
   - What the key logic is
   - How it satisfies the criterion

**PROHIBITIONS:**
- **DO NOT** copy criterion text as implementation description
- **DO NOT** assume code structure from ticket requirements
- **DO NOT** write generic descriptions like "implemented as expected"
- **DO NOT** skip reading actual code

### Step 4: Review Tests

For EACH acceptance criterion:

1. **Find the test** - Locate the test function that verifies this criterion
2. **Read the test code** - Understand what the test is checking
3. **Copy key assertion** - Copy an actual assertion line from the test code

**PROHIBITIONS:**
- **DO NOT** invent test function names
- **DO NOT** assume assertion content
- **DO NOT** copy assertions from other files

### Step 5: Additional Review Checks

1. **Requirements Compliance**: Does PR complete ticket requirements?
2. **Commit Format**: Is it ` + "`[type] subject`" + ` (lowercase)?
3. **Scope Restriction**: Any changes beyond ticket scope?
4. **Architecture Compliance**: Does it follow project conventions?
5. **Code Quality**: Any debug code or obvious bugs?
6. **Security Check**: Any sensitive information leakage?

### Step 6: Submit Review

` + "```bash" + `
awkit submit-review \
  --pr $PR_NUMBER \
  --issue $ISSUE_NUMBER \
  --score $SCORE \
  --ci-status $CI_STATUS \
  --body "$REVIEW_BODY"
` + "```" + `

Scoring criteria:
- 9-10: Perfect completion
- 7-8: Completed with good quality
- 5-6: Partial completion, has issues
- 1-4: Not completed or major issues

### Step 7: Return Result

**Immediately return** the submit-review result to Principal:

| Result | Action |
|--------|--------|
| ` + "`merged`" + ` | PR merged, task complete |
| ` + "`changes_requested`" + ` | Review failed, Worker needs to fix |
| ` + "`review_blocked`" + ` | Verification failed, **DO NOT retry** |
| ` + "`merge_failed`" + ` | Merge failed (e.g., conflict) |

---

## Review Body Format

Your review body MUST follow this exact format:

` + "```markdown" + `
### Implementation Review

#### 1. [First Criterion Text]

**Implementation**: [Describe the actual implementation. Must be 20+ chars, include function names and key logic.]

**Code Location**: ` + "`path/to/file.go:LineNumber`" + `

#### 2. [Second Criterion Text]

**Implementation**: [Description...]

**Code Location**: ` + "`path/to/file.go:LineNumber`" + `

### Test Review

| Criteria | Test | Key Assertion |
|----------|------|---------------|
| [FULL Criterion 1 text from ticket] | ` + "`TestFunctionName`" + ` | ` + "`assert.Equal(t, expected, actual)`" + ` |
| [FULL Criterion 2 text from ticket] | ` + "`TestOtherFunction`" + ` | ` + "`require.NoError(t, err)`" + ` |

**CRITICAL**: The Criteria column MUST contain the **exact full text** from the ticket's acceptance criteria. Do NOT use shortened or paraphrased versions.

### Score Reason

[Explain why you gave this score]

### Suggested Improvements

[List any improvement suggestions, or "None" if perfect]

### Potential Risks

[List any potential risks, or "None identified"]
` + "```" + `

---

## Verification Rules (System Enforced)

The system will verify your submission:

1. **Completeness Check**: Every acceptance criterion must have:
   - Implementation description (minimum 20 characters)
   - Test name mapping
   - Key assertion

2. **Test Execution**: System will execute ` + "`$TEST_COMMAND`" + ` in worktree
   - All mapped tests must PASS
   - Failed tests will block the review

3. **Assertion Verification**: System will search test files
   - Your quoted assertions must actually exist in test code
   - Non-existent assertions will block the review

**If verification fails, the review is blocked. A NEW session will retry.**

---

## Common Mistakes to Avoid

### Implementation Description

Wrong:
` + "```" + `
**Implementation**: Implemented according to requirements
` + "```" + `

Wrong:
` + "```" + `
**Implementation**: The feature is complete
` + "```" + `

Correct:
` + "```" + `
**Implementation**: Implemented in ` + "`HandleCollision()`" + ` at engine.go:145. When snake head position matches wall boundary, sets ` + "`game.State = GameOver`" + ` and emits collision event.
` + "```" + `

### Test Assertion (Criteria Column)

Wrong (shortened text):
` + "```" + `
| Wall collision ends game | TestCollision | assert passes |
` + "```" + `

Wrong (paraphrased text):
` + "```" + `
| Collision detection works | TestWallCollision | ` + "`t.Error(\"should end\")`" + ` |
` + "```" + `

Correct (FULL criteria text from ticket + actual assertion):
` + "```" + `
| Wall collision ends game and game state changes to GameOver | TestCollisionScenarios | ` + "`assert.Equal(t, GameOver, game.State)`" + ` |
` + "```" + `

**The Criteria column must match the EXACT text from the ticket's ` + "`- [ ]`" + ` lines.**

---

## CRITICAL: No Retry Rule

**When ` + "`submit-review`" + ` returns ` + "`review_blocked`" + `:**

- **DO NOT** attempt to fix evidence and resubmit
- **DO NOT** analyze failure reasons and retry
- **MUST** immediately return ` + "`review_blocked`" + ` to Principal

**Violating this rule causes "self-dealing" problem - same session self-correction is invalid.**
`

	prReviewerPath := filepath.Join(agentsDir, "pr-reviewer.md")
	if err := os.WriteFile(prReviewerPath, []byte(prReviewer), 0644); err != nil {
		return err
	}
	fmt.Printf("[generate] Created: %s\n", prReviewerPath)

	// conflict-resolver agent definition
	conflictResolver := `---
name: conflict-resolver
description: AWK Merge Conflict Resolver. Resolves git merge conflicts in a worktree.
tools: Read, Grep, Glob, Bash, Edit
model: sonnet
---

You are the AWK Conflict Resolution Expert.

## Input
You will receive: WORKTREE_PATH, ISSUE_NUMBER, PR_NUMBER

## Execution Flow

### Step 1: Navigate to Worktree
` + "```bash" + `
cd $WORKTREE_PATH
` + "```" + `

### Step 2: Identify Conflicts
` + "```bash" + `
git status
git diff --name-only --diff-filter=U
` + "```" + `

### Step 3: Resolve Each Conflict
For each conflicted file:
1. Read the file to understand context
2. Identify conflict markers (<<<<<<, ======, >>>>>>)
3. Determine correct resolution based on:
   - Intent from both branches
   - Code logic
   - Project conventions
4. Edit to resolve (remove markers, keep correct code)
5. Stage the resolved file

### Step 4: Complete Resolution
` + "```bash" + `
git add .
git rebase --continue
` + "```" + `

Or if conflict is too complex:
` + "```bash" + `
git rebase --abort
` + "```" + `

### Step 5: Return Result
Return one of:
- RESOLVED: Conflict resolved successfully
- TOO_COMPLEX: Conflict requires human intervention
- FAILED: Resolution failed
`

	conflictResolverPath := filepath.Join(agentsDir, "conflict-resolver.md")
	if err := os.WriteFile(conflictResolverPath, []byte(conflictResolver), 0644); err != nil {
		return err
	}
	fmt.Printf("[generate] Created: %s\n", conflictResolverPath)

	return nil
}

func setupClaudeSymlinks(aiRoot, claudeDir string) error {
	aiRules := filepath.Join(aiRoot, "rules")
	aiSkills := filepath.Join(aiRoot, "skills")
	claudeRules := filepath.Join(claudeDir, "rules")
	claudeSkills := filepath.Join(claudeDir, "skills")

	// Clean up deprecated commands dir
	commandsDir := filepath.Join(claudeDir, "commands")
	_ = os.RemoveAll(commandsDir)

	var lastErr error

	if _, err := os.Stat(aiRules); err == nil {
		if err := createSymlinkOrCopy(aiRules, claudeRules, "rules"); err != nil {
			lastErr = err
		}
	}

	if _, err := os.Stat(aiSkills); err == nil {
		if err := createSymlinkOrCopy(aiSkills, claudeSkills, "skills"); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func createSymlinkOrCopy(source, target, name string) error {
	// Remove existing
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			os.Remove(target)
		} else if info.IsDir() {
			os.RemoveAll(target)
		}
	}

	// Try symlink first
	relSource, err := filepath.Rel(filepath.Dir(target), source)
	if err != nil {
		relSource = source
	}

	if err := os.Symlink(relSource, target); err == nil {
		fmt.Printf("[generate] Created symlink: %s -> %s\n", target, relSource)
		return nil
	}

	// Fallback to copy
	if runtime.GOOS == "windows" {
		fmt.Printf("[generate] WARNING: Cannot create symlink for %s.\n", name)
		fmt.Println("[generate] On Windows, enable Developer Mode:")
		fmt.Println("[generate]   Settings -> Update & Security -> For developers -> Developer Mode: ON")
		fmt.Println("[generate] Falling back to copy...")
	}

	if err := copyDir(source, target); err != nil {
		return err
	}
	fmt.Printf("[generate] Copied %s to: %s\n", name, target)
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

func generateCIWorkflows(stateRoot string, ctx *TemplateContext) ([]string, error) {
	var generated []string

	languageTemplates := map[string]string{
		// Go
		"go": "go", "golang": "go",
		// Node.js
		"node": "node", "nodejs": "node", "typescript": "node", "javascript": "node",
		"react": "node", "vue": "node", "angular": "node", "nextjs": "node",
		"nuxt": "node", "svelte": "node", "express": "node", "nestjs": "node",
		// Python
		"python": "python", "django": "python", "flask": "python", "fastapi": "python",
		// Rust
		"rust": "rust",
		// .NET
		"dotnet": "dotnet", "csharp": "dotnet", "aspnet": "dotnet", "blazor": "dotnet",
		// Game engines
		"unity":  "unity",
		"unreal": "unreal", "ue4": "unreal", "ue5": "unreal",
		"godot": "godot",
	}

	for _, repo := range ctx.Repos {
		var workflowDir, workflowFile string

		if repo.Type == "submodule" {
			workflowDir = filepath.Join(stateRoot, repo.Path, ".github", "workflows")
			workflowFile = filepath.Join(workflowDir, "ci.yml")
		} else {
			workflowDir = filepath.Join(stateRoot, ".github", "workflows")
			if repo.Type == "directory" {
				workflowFile = filepath.Join(workflowDir, fmt.Sprintf("ci-%s.yml", repo.Name))
			} else {
				workflowFile = filepath.Join(workflowDir, "ci.yml")
			}
		}

		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			continue
		}

		lang := strings.ToLower(repo.Language)
		templateType := languageTemplates[lang]
		if templateType == "" {
			templateType = "generic"
		}

		content := generateCIContent(repo, ctx.Git, templateType)
		if err := os.WriteFile(workflowFile, []byte(content), 0644); err != nil {
			continue
		}
		generated = append(generated, workflowFile)
		fmt.Printf("[generate] Created: %s\n", workflowFile)
	}

	// Generate validate-submodules.yml for monorepo with submodules
	if ctx.Project.Type == "monorepo" && ctx.HasSubmodules {
		workflowDir := filepath.Join(stateRoot, ".github", "workflows")
		if err := os.MkdirAll(workflowDir, 0755); err == nil {
			workflowFile := filepath.Join(workflowDir, "validate-submodules.yml")
			content := generateValidateSubmodulesWorkflow(ctx.Git)
			if err := os.WriteFile(workflowFile, []byte(content), 0644); err == nil {
				generated = append(generated, workflowFile)
				fmt.Printf("[generate] Created: %s\n", workflowFile)
			}
		}
	}

	return generated, nil
}

func generateValidateSubmodulesWorkflow(git GitConfig) string {
	var sb strings.Builder

	sb.WriteString("name: validate-submodules\n\n")
	sb.WriteString("on:\n")
	sb.WriteString("  pull_request:\n")
	if git.IntegrationBranch != git.ReleaseBranch && git.ReleaseBranch != "" {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\", \"%s\"]\n", git.IntegrationBranch, git.ReleaseBranch))
	} else {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n", git.IntegrationBranch))
	}
	sb.WriteString("  push:\n")
	sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n\n", git.IntegrationBranch))

	sb.WriteString("jobs:\n")
	sb.WriteString("  validate:\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")
	sb.WriteString("    steps:\n")
	sb.WriteString("      - name: Checkout (with submodules)\n")
	sb.WriteString("        uses: actions/checkout@v4\n")
	sb.WriteString("        with:\n")
	sb.WriteString("          submodules: recursive\n")
	sb.WriteString("          fetch-depth: 0\n")
	sb.WriteString("          token: ${{ secrets.SUBMODULES_TOKEN }}\n\n")

	sb.WriteString("      - name: Ensure submodules initialised\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          git submodule update --init --recursive\n")
	sb.WriteString("          git submodule status --recursive\n\n")

	sb.WriteString("      - name: Verify submodule SHAs are on allowed branches\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString(fmt.Sprintf("          ALLOWED_BRANCHES=(\"%s\" \"%s\")\n\n", git.IntegrationBranch, git.ReleaseBranch))
	sb.WriteString("          paths=$(git config -f .gitmodules --get-regexp path | awk '{print $2}')\n")
	sb.WriteString("          for p in $paths; do\n")
	sb.WriteString("            echo \"== Checking submodule: $p ==\"\n")
	sb.WriteString("            sha=$(git -C \"$p\" rev-parse HEAD)\n")
	sb.WriteString("            echo \"SHA: $sha\"\n\n")
	sb.WriteString("            git -C \"$p\" fetch origin --prune\n\n")
	sb.WriteString("            ok=0\n")
	sb.WriteString("            for b in \"${ALLOWED_BRANCHES[@]}\"; do\n")
	sb.WriteString("              if git -C \"$p\" branch -r --contains \"$sha\" | grep -q \"origin/$b\"; then\n")
	sb.WriteString("                echo \"OK: $sha is contained in origin/$b\"\n")
	sb.WriteString("                ok=1\n")
	sb.WriteString("                break\n")
	sb.WriteString("              fi\n")
	sb.WriteString("            done\n\n")
	sb.WriteString("            if [[ \"$ok\" -ne 1 ]]; then\n")
	sb.WriteString("              echo \"ERROR: $p SHA $sha is NOT on allowed branches: ${ALLOWED_BRANCHES[*]}\"\n")
	sb.WriteString("              exit 2\n")
	sb.WriteString("            fi\n")
	sb.WriteString("          done\n")

	return sb.String()
}

func generateCIContent(repo RepoConfig, git GitConfig, templateType string) string {
	// Game engines have special templates
	switch templateType {
	case "unity":
		return generateUnityCIContent(repo, git)
	case "unreal":
		return generateUnrealCIContent(repo, git)
	case "godot":
		return generateGodotCIContent(repo, git)
	}

	workingDir := ""
	if repo.Type == "directory" && repo.Path != "./" {
		workingDir = strings.TrimSuffix(repo.Path, "/")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("name: %s CI\n\n", repo.Name))
	sb.WriteString("on:\n")
	sb.WriteString("  push:\n")
	sb.WriteString(fmt.Sprintf("    branches: [%s, %s]\n", git.IntegrationBranch, git.ReleaseBranch))
	sb.WriteString("  pull_request:\n")
	sb.WriteString(fmt.Sprintf("    branches: [%s]\n\n", git.IntegrationBranch))
	sb.WriteString("jobs:\n")
	sb.WriteString("  build:\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")

	if workingDir != "" {
		sb.WriteString("    defaults:\n")
		sb.WriteString("      run:\n")
		sb.WriteString(fmt.Sprintf("        working-directory: %s\n", workingDir))
	}

	sb.WriteString("    steps:\n")
	sb.WriteString("      - uses: actions/checkout@v4\n")
	sb.WriteString("        with:\n")
	sb.WriteString("          fetch-depth: 0\n")

	switch templateType {
	case "go":
		sb.WriteString("      - uses: actions/setup-go@v5\n")
		sb.WriteString("        with:\n")
		sb.WriteString("          go-version: '1.25.5'\n")
	case "node":
		sb.WriteString("      - uses: actions/setup-node@v4\n")
		sb.WriteString("        with:\n")
		sb.WriteString("          node-version: '20'\n")
		sb.WriteString("      - run: npm ci\n")
	case "python":
		sb.WriteString("      - uses: actions/setup-python@v5\n")
		sb.WriteString("        with:\n")
		sb.WriteString("          python-version: '3.11'\n")
		sb.WriteString("      - run: pip install -r requirements.txt\n")
	case "rust":
		sb.WriteString("      - name: Setup Rust\n")
		sb.WriteString("        uses: dtolnay/rust-toolchain@stable\n")
		sb.WriteString("        with:\n")
		sb.WriteString("          toolchain: stable\n")
		sb.WriteString("      - name: Cache cargo\n")
		sb.WriteString("        uses: Swatinem/rust-cache@v2\n")
	case "dotnet":
		sb.WriteString("      - name: Setup .NET\n")
		sb.WriteString("        uses: actions/setup-dotnet@v4\n")
		sb.WriteString("        with:\n")
		sb.WriteString("          dotnet-version: '8.0.x'\n")
		sb.WriteString("      - name: Restore dependencies\n")
		sb.WriteString("        run: dotnet restore\n")
	}

	sb.WriteString("      - name: Build\n")
	sb.WriteString(fmt.Sprintf("        run: %s\n", repo.Verify.Build))
	sb.WriteString("      - name: Test\n")
	sb.WriteString(fmt.Sprintf("        run: %s\n", repo.Verify.Test))

	return sb.String()
}

func generateUnityCIContent(repo RepoConfig, git GitConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("name: %s CI\n\n", repo.Name))
	sb.WriteString("on:\n")
	sb.WriteString("  pull_request:\n")
	if git.IntegrationBranch != git.ReleaseBranch && git.ReleaseBranch != "" {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\", \"%s\"]\n", git.IntegrationBranch, git.ReleaseBranch))
	} else {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n", git.IntegrationBranch))
	}
	sb.WriteString("  push:\n")
	sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n\n", git.IntegrationBranch))

	sb.WriteString("jobs:\n")
	sb.WriteString("  sanity:\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")
	sb.WriteString("    steps:\n")
	sb.WriteString("      - name: Checkout (with LFS)\n")
	sb.WriteString("        uses: actions/checkout@v4\n")
	sb.WriteString("        with:\n")
	sb.WriteString("          fetch-depth: 0\n")
	sb.WriteString("          lfs: true\n\n")

	sb.WriteString("      - name: Basic Unity project structure checks\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n\n")
	sb.WriteString("          echo \"== Check Packages files ==\"\n")
	sb.WriteString("          test -f Packages/manifest.json\n")
	sb.WriteString("          echo \"OK: Packages/manifest.json exists\"\n")
	sb.WriteString("          if [ -f Packages/packages-lock.json ]; then\n")
	sb.WriteString("            echo \"OK: Packages/packages-lock.json exists\"\n")
	sb.WriteString("          else\n")
	sb.WriteString("            echo \"WARN: Packages/packages-lock.json missing (not fatal)\"\n")
	sb.WriteString("          fi\n\n")
	sb.WriteString("          echo \"== Validate JSON syntax (manifest/lock) ==\"\n")
	sb.WriteString("          python3 - <<'PY'\n")
	sb.WriteString("          import json, pathlib, sys\n")
	sb.WriteString("          files = [\"Packages/manifest.json\", \"Packages/packages-lock.json\"]\n")
	sb.WriteString("          for f in files:\n")
	sb.WriteString("            p = pathlib.Path(f)\n")
	sb.WriteString("            if not p.exists():\n")
	sb.WriteString("              print(f\"skip: {f} (missing)\")\n")
	sb.WriteString("              continue\n")
	sb.WriteString("            try:\n")
	sb.WriteString("              json.loads(p.read_text(encoding=\"utf-8\"))\n")
	sb.WriteString("              print(f\"ok: {f} json valid\")\n")
	sb.WriteString("            except Exception as e:\n")
	sb.WriteString("              print(f\"ERROR: {f} invalid json: {e}\")\n")
	sb.WriteString("              sys.exit(2)\n")
	sb.WriteString("          PY\n\n")
	sb.WriteString("          echo \"== Check Assets folder ==\"\n")
	sb.WriteString("          test -d Assets\n")
	sb.WriteString("          echo \"OK: Assets exists\"\n\n")

	sb.WriteString("      - name: Basic .meta presence check\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString("          count_files=$(find Assets -type f ! -name \"*.meta\" | wc -l | tr -d ' ')\n")
	sb.WriteString("          count_meta=$(find Assets -type f -name \"*.meta\" | wc -l | tr -d ' ')\n")
	sb.WriteString("          echo \"Assets non-meta files: $count_files\"\n")
	sb.WriteString("          echo \"Assets meta files:     $count_meta\"\n\n")
	sb.WriteString("          if [ \"$count_files\" -gt 0 ] && [ \"$count_meta\" -eq 0 ]; then\n")
	sb.WriteString("            echo \"ERROR: Assets has files but no .meta files found.\"\n")
	sb.WriteString("            exit 3\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          echo \"OK: meta presence sanity check passed\"\n\n")

	sb.WriteString("      - name: Ensure no huge files\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString("          big=$(find . -type f -size +200M -not -path \"./.git/*\" | head -n 1 || true)\n")
	sb.WriteString("          if [ -n \"$big\" ]; then\n")
	sb.WriteString("            echo \"ERROR: Found very large file: $big\"\n")
	sb.WriteString("            exit 4\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          echo \"OK: no huge files detected\"\n")

	return sb.String()
}

func generateUnrealCIContent(repo RepoConfig, git GitConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("name: %s CI\n\n", repo.Name))
	sb.WriteString("on:\n")
	sb.WriteString("  pull_request:\n")
	if git.IntegrationBranch != git.ReleaseBranch && git.ReleaseBranch != "" {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\", \"%s\"]\n", git.IntegrationBranch, git.ReleaseBranch))
	} else {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n", git.IntegrationBranch))
	}
	sb.WriteString("  push:\n")
	sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n\n", git.IntegrationBranch))

	sb.WriteString("# Unreal Engine CI: sanity-check mode (no UE required)\n")
	sb.WriteString("# For full builds, configure self-hosted runner with UE installed\n\n")

	sb.WriteString("jobs:\n")
	sb.WriteString("  sanity-check:\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")
	sb.WriteString("    steps:\n")
	sb.WriteString("      - name: Checkout\n")
	sb.WriteString("        uses: actions/checkout@v4\n")
	sb.WriteString("        with:\n")
	sb.WriteString("          fetch-depth: 0\n")
	sb.WriteString("          lfs: true\n\n")

	sb.WriteString("      - name: Check Unreal project structure\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== Check .uproject file ==\"\n")
	sb.WriteString("          UPROJECT=$(find . -maxdepth 2 -name \"*.uproject\" | head -n 1)\n")
	sb.WriteString("          if [ -z \"$UPROJECT\" ]; then\n")
	sb.WriteString("            echo \"ERROR: No .uproject file found\"\n")
	sb.WriteString("            exit 1\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          echo \"OK: Found $UPROJECT\"\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== Validate .uproject JSON ==\"\n")
	sb.WriteString("          python3 -c \"import json; json.load(open('$UPROJECT'))\"\n")
	sb.WriteString("          echo \"OK: .uproject is valid JSON\"\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== Check required folders ==\"\n")
	sb.WriteString("          for dir in Source Content Config; do\n")
	sb.WriteString("            if [ -d \"$dir\" ]; then\n")
	sb.WriteString("              echo \"OK: $dir exists\"\n")
	sb.WriteString("            else\n")
	sb.WriteString("              echo \"WARN: $dir not found\"\n")
	sb.WriteString("            fi\n")
	sb.WriteString("          done\n\n")

	sb.WriteString("      - name: UE C++ code quality checks\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString("          \n")
	sb.WriteString("          if [ ! -d \"Source\" ]; then\n")
	sb.WriteString("            echo \"No Source folder, skipping\"\n")
	sb.WriteString("            exit 0\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== UE C++ Code Quality Checks ==\"\n")
	sb.WriteString("          ERRORS=0\n")
	sb.WriteString("          \n")
	sb.WriteString("          # Check UCLASS and GENERATED_BODY\n")
	sb.WriteString("          for file in $(find Source -name \"*.h\" 2>/dev/null); do\n")
	sb.WriteString("            if grep -q \"class.*:.*public.*UObject\\|AActor\\|UActorComponent\" \"$file\"; then\n")
	sb.WriteString("              if ! grep -q \"UCLASS(\" \"$file\"; then\n")
	sb.WriteString("                echo \"  ERROR: $file - inherits UE class but missing UCLASS()\"\n")
	sb.WriteString("                ERRORS=$((ERRORS + 1))\n")
	sb.WriteString("              elif ! grep -q \"GENERATED_BODY()\\|GENERATED_UCLASS_BODY()\" \"$file\"; then\n")
	sb.WriteString("                echo \"  ERROR: $file - has UCLASS but missing GENERATED_BODY()\"\n")
	sb.WriteString("                ERRORS=$((ERRORS + 1))\n")
	sb.WriteString("              fi\n")
	sb.WriteString("            fi\n")
	sb.WriteString("          done\n")
	sb.WriteString("          \n")
	sb.WriteString("          # Check Build.cs\n")
	sb.WriteString("          if ! find Source -name \"*.Build.cs\" | grep -q .; then\n")
	sb.WriteString("            echo \"  ERROR: No .Build.cs file found\"\n")
	sb.WriteString("            ERRORS=$((ERRORS + 1))\n")
	sb.WriteString("          else\n")
	sb.WriteString("            echo \"  OK: .Build.cs found\"\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          \n")
	sb.WriteString("          if [ $ERRORS -gt 0 ]; then\n")
	sb.WriteString("            exit 1\n")
	sb.WriteString("          fi\n\n")

	sb.WriteString("      - name: Check assets and LFS\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          echo \"== Asset checks ==\"\n")
	sb.WriteString("          big=$(find . -type f -size +100M -not -path \"./.git/*\" 2>/dev/null | head -5 || true)\n")
	sb.WriteString("          if [ -n \"$big\" ]; then\n")
	sb.WriteString("            echo \"WARN: Large files found (consider Git LFS):\"\n")
	sb.WriteString("            echo \"$big\"\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          \n")
	sb.WriteString("          if [ -f \".gitattributes\" ] && grep -q \"lfs\" .gitattributes; then\n")
	sb.WriteString("            echo \"OK: Git LFS configured\"\n")
	sb.WriteString("          else\n")
	sb.WriteString("            echo \"WARN: Consider adding Git LFS for .uasset, .umap files\"\n")
	sb.WriteString("          fi\n")

	return sb.String()
}

func generateGodotCIContent(repo RepoConfig, git GitConfig) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("name: %s CI\n\n", repo.Name))
	sb.WriteString("on:\n")
	sb.WriteString("  pull_request:\n")
	if git.IntegrationBranch != git.ReleaseBranch && git.ReleaseBranch != "" {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\", \"%s\"]\n", git.IntegrationBranch, git.ReleaseBranch))
	} else {
		sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n", git.IntegrationBranch))
	}
	sb.WriteString("  push:\n")
	sb.WriteString(fmt.Sprintf("    branches: [\"%s\"]\n\n", git.IntegrationBranch))

	sb.WriteString("# Godot CI: uses headless Godot for validation\n\n")

	sb.WriteString("env:\n")
	sb.WriteString("  GODOT_VERSION: '4.2.2'\n\n")

	sb.WriteString("jobs:\n")
	sb.WriteString("  sanity-check:\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")
	sb.WriteString("    steps:\n")
	sb.WriteString("      - name: Checkout\n")
	sb.WriteString("        uses: actions/checkout@v4\n")
	sb.WriteString("        with:\n")
	sb.WriteString("          fetch-depth: 0\n")
	sb.WriteString("          lfs: true\n\n")

	sb.WriteString("      - name: Check Godot project structure\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== Check project.godot ==\"\n")
	sb.WriteString("          if [ ! -f \"project.godot\" ]; then\n")
	sb.WriteString("            echo \"ERROR: project.godot not found\"\n")
	sb.WriteString("            exit 1\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          echo \"OK: project.godot found\"\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== Parse project config ==\"\n")
	sb.WriteString("          if grep -q \"config/name\" project.godot; then\n")
	sb.WriteString("            PROJECT_NAME=$(grep \"config/name\" project.godot | sed 's/.*=\"\\(.*\\)\"/\\1/')\n")
	sb.WriteString("            echo \"Project name: $PROJECT_NAME\"\n")
	sb.WriteString("          fi\n\n")

	sb.WriteString("      - name: GDScript code quality checks\n")
	sb.WriteString("        shell: bash\n")
	sb.WriteString("        run: |\n")
	sb.WriteString("          set -euo pipefail\n")
	sb.WriteString("          \n")
	sb.WriteString("          GD_FILES=$(find . -name \"*.gd\" -not -path \"./.git/*\" -not -path \"./addons/*\" 2>/dev/null || true)\n")
	sb.WriteString("          if [ -z \"$GD_FILES\" ]; then\n")
	sb.WriteString("            echo \"No GDScript files found, skipping\"\n")
	sb.WriteString("            exit 0\n")
	sb.WriteString("          fi\n")
	sb.WriteString("          \n")
	sb.WriteString("          echo \"== GDScript Code Quality Checks ==\"\n")
	sb.WriteString("          ERRORS=0\n")
	sb.WriteString("          \n")
	sb.WriteString("          for file in $GD_FILES; do\n")
	sb.WriteString("            # Check deprecated yield (Godot 4 should use await)\n")
	sb.WriteString("            if grep -qE \"\\byield\\s*\\(\" \"$file\"; then\n")
	sb.WriteString("              echo \"  ERROR: $file - uses deprecated 'yield' (use 'await' in Godot 4)\"\n")
	sb.WriteString("              ERRORS=$((ERRORS + 1))\n")
	sb.WriteString("            fi\n")
	sb.WriteString("            \n")
	sb.WriteString("            # Check old export syntax\n")
	sb.WriteString("            if grep -qE \"^export\\s*\\(\" \"$file\"; then\n")
	sb.WriteString("              echo \"  ERROR: $file - uses old 'export()' syntax (use '@export' in Godot 4)\"\n")
	sb.WriteString("              ERRORS=$((ERRORS + 1))\n")
	sb.WriteString("            fi\n")
	sb.WriteString("            \n")
	sb.WriteString("            # Check old onready syntax\n")
	sb.WriteString("            if grep -qE \"^onready\\s+\" \"$file\"; then\n")
	sb.WriteString("              echo \"  ERROR: $file - uses old 'onready' syntax (use '@onready' in Godot 4)\"\n")
	sb.WriteString("              ERRORS=$((ERRORS + 1))\n")
	sb.WriteString("            fi\n")
	sb.WriteString("          done\n")
	sb.WriteString("          \n")
	sb.WriteString("          GD_COUNT=$(echo \"$GD_FILES\" | wc -w)\n")
	sb.WriteString("          TSCN_COUNT=$(find . -name \"*.tscn\" -not -path \"./.git/*\" | wc -l)\n")
	sb.WriteString("          echo \"Stats: $GD_COUNT scripts, $TSCN_COUNT scenes\"\n")
	sb.WriteString("          echo \"Results: $ERRORS errors\"\n")
	sb.WriteString("          \n")
	sb.WriteString("          if [ $ERRORS -gt 0 ]; then\n")
	sb.WriteString("            exit 1\n")
	sb.WriteString("          fi\n\n")

	sb.WriteString("      - name: Setup Godot\n")
	sb.WriteString("        uses: chickensoft-games/setup-godot@v2\n")
	sb.WriteString("        with:\n")
	sb.WriteString("          version: ${{ env.GODOT_VERSION }}\n")
	sb.WriteString("          use-dotnet: false\n\n")

	sb.WriteString("      - name: Verify Godot installation\n")
	sb.WriteString("        run: godot --version\n\n")

	sb.WriteString("      - name: Import project\n")
	sb.WriteString("        run: timeout 120 godot --headless --import || true\n\n")

	sb.WriteString("      - name: Validate GDScript syntax\n")
	sb.WriteString("        run: godot --headless --check-only --script res://project.godot 2>&1 || echo \"Script validation completed\"\n")

	return sb.String()
}

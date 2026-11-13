package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	awkit "github.com/silver2dream/ai-workflow-kit"
	projectconfig "github.com/silver2dream/ai-workflow-kit/internal/config"
	"github.com/silver2dream/ai-workflow-kit/internal/layout"
)

// Backtick for use in templates
const bt = "`"

// Options configures the generate operation
type Options struct {
	StateRoot string
	CI        CIPolicy // what to do about the CI workflows; the zero value leaves them alone
	DryRun    bool     // show what would be generated without writing
}

// Result holds the result of the generate operation
type Result struct {
	// RemovedFiles are the CI workflows awkit owned that no repo of the
	// config accounts for any more.
	RemovedFiles   []string
	GeneratedFiles []string
}

// ValidateRepoPath validates repo path based on type.
// Property 15: Path Traversal Prevention - rejects paths containing ".." or resolving outside worktree.
func ValidateRepoPath(repoType, repoPath string) (bool, string) {
	// Check for path traversal (Req 22.1, 22.2)
	if strings.Contains(repoPath, "..") {
		return false, fmt.Sprintf("Path traversal detected: '%s' contains '..'", repoPath)
	}

	// Normalize path
	normalized := strings.ReplaceAll(repoPath, "\\", "/")
	normalized = strings.TrimSuffix(normalized, "/")

	// Root type must be ./ or . (Req 9.3)
	if repoType == "root" {
		if normalized != "." && normalized != "./" && normalized != "" {
			return false, fmt.Sprintf("Root type path must be './' or '.', got '%s'", repoPath)
		}
	}

	return true, ""
}

// ValidateSubmoduleConfig validates submodule configuration.
// Property 7: Config Validation Completeness - validates submodule-type repos.
func ValidateSubmoduleConfig(repoPath, gitmodulesContent string, hasGit bool) (bool, []string) {
	var errors []string

	// Check .gitmodules exists (Req 1.5)
	if gitmodulesContent == "" {
		errors = append(errors, "Missing .gitmodules file for submodule type repo")
		return false, errors
	}

	// Check path is in .gitmodules (Req 9.1)
	normalizedPath := strings.TrimSuffix(repoPath, "/")
	if !strings.Contains(gitmodulesContent, fmt.Sprintf("path = %s", normalizedPath)) {
		errors = append(errors, fmt.Sprintf("Submodule path '%s' not found in .gitmodules", repoPath))
	}

	// Check .git exists (Req 9.2)
	if !hasGit {
		errors = append(errors, fmt.Sprintf("Submodule path '%s' has no .git file/directory", repoPath))
	}

	return len(errors) == 0, errors
}

// CheckDirectoryHasGitFile checks if directory type path has .git file (might be submodule).
func CheckDirectoryHasGitFile(repoPath string, hasGitFile bool) (bool, string) {
	if hasGitFile {
		return true, fmt.Sprintf("WARNING: Directory '%s' has .git file - might be a submodule", repoPath)
	}
	return false, ""
}

// ValidateSubmoduleRemote validates submodule remote URL matches .gitmodules.
// Property 18: Submodule Remote Validation
func ValidateSubmoduleRemote(repoPath, gitmodulesURL, actualRemoteURL string) (bool, string) {
	if gitmodulesURL == "" {
		return false, "No URL found in .gitmodules for submodule"
	}

	if actualRemoteURL == "" {
		return false, "No remote URL configured for submodule"
	}

	// Normalize URLs for comparison
	normalizedGitmodules := strings.TrimSuffix(strings.TrimSuffix(gitmodulesURL, "/"), ".git")
	normalizedActual := strings.TrimSuffix(strings.TrimSuffix(actualRemoteURL, "/"), ".git")

	if normalizedGitmodules != normalizedActual {
		return false, fmt.Sprintf("Remote URL mismatch: .gitmodules has '%s', actual is '%s'", gitmodulesURL, actualRemoteURL)
	}

	return true, ""
}

// The configuration is read by package config, for everyone. These are the
// names the generator and its templates have for it.
type (
	Config          = projectconfig.Config
	ProjectConfig   = projectconfig.Project
	GitConfig       = projectconfig.Git
	RepoConfig      = projectconfig.Repo
	VerifyConfig    = projectconfig.Verify
	RulesConfig     = projectconfig.Rules
	AgentsConfig    = projectconfig.Agents
	CustomAgentDef  = projectconfig.CustomAgent
	ValidationError = projectconfig.ValidationError
)

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
	configPath := projectconfig.Path(opts.StateRoot)
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
	kitRulesDir := layout.KitRules(opts.StateRoot)
	gitWorkflowPath := filepath.Join(kitRulesDir, "git-workflow.md")
	claudeDir := filepath.Join(opts.StateRoot, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// Add files to result
	result.GeneratedFiles = append(result.GeneratedFiles, claudePath, agentsPath, gitWorkflowPath, settingsPath)

	// If dry-run, just return the list
	if opts.DryRun {
		ciPlan := planCIWorkflows(opts.StateRoot, ctx, opts.CI)
		for _, file := range ciPlan.write {
			result.GeneratedFiles = append(result.GeneratedFiles, file.path)
		}
		result.RemovedFiles = ciPlan.remove
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

	// Setup .claude directory
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .claude dir: %w", err)
	}

	// Generate settings.local.json
	if err := generateClaudeSettings(settingsPath, config.Agents); err != nil {
		return nil, fmt.Errorf("failed to generate claude settings: %w", err)
	}
	fmt.Printf("[generate] Created: %s\n", settingsPath)

	// Validate custom rules (non-fatal warnings)
	if ruleWarnings := validateCustomRules(opts.StateRoot, config.Rules); len(ruleWarnings) > 0 {
		for _, w := range ruleWarnings {
			fmt.Printf("[generate] WARNING: %s\n", w)
		}
	}

	// The subagents the Principal hands work to: the kit's, and the ones the
	// configuration defines.
	if err := installAgentsDir(layout.Agents(opts.StateRoot), config.Agents); err != nil {
		return nil, fmt.Errorf("failed to install agents: %w", err)
	}

	// CI workflows, as the policy says. They come last: a workflow that cannot
	// be written fails the run, and everything else is in place by then.
	ciWritten, ciRemoved, err := generateCIWorkflows(opts.StateRoot, ctx, opts.CI)
	result.GeneratedFiles = append(result.GeneratedFiles, ciWritten...)
	result.RemovedFiles = ciRemoved
	if err != nil {
		return result, err
	}

	fmt.Println("[generate] Done!")
	return result, nil
}

// loadConfig reads the configuration file at path.
func loadConfig(path string) (*Config, error) {
	return projectconfig.LoadFile(path)
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
	code := func(s string) string { return bt + s + bt }

	sb.WriteString("# CLAUDE.md\n\n")
	sb.WriteString("AWK (AI Workflow Kit) works on this project: a ticket is a GitHub issue, a Principal hands each ticket to a Worker, a reviewer checks the pull request, and what passes is merged. This file is a map. What it points to is where each thing is defined.\n\n")

	sb.WriteString("## Who you are\n\n")
	sb.WriteString("- Your task is to run the AWK principal workflow (" + code("awkit run") + " starts a session with exactly that): you are the **Principal**. Use the " + code("principal-workflow") + " skill and follow its loop. You hand out work and act on what comes back. You do not write code, review a pull request or merge one yourself.\n")
	sb.WriteString("- You were given one ticket to implement: you are a **Worker**. Read " + code("AGENTS.md") + " instead of this file.\n")
	sb.WriteString("- You are a subagent (" + code("pr-reviewer") + ", " + code("conflict-resolver") + "), or were started for one task with instructions of its own: follow those.\n")
	sb.WriteString("- Otherwise you are working with a person, and none of these roles is yours. The rules in the table below still hold for what you change.\n\n")

	sb.WriteString("## Where things are defined\n\n")
	sb.WriteString("Each topic has one place of definition. When another file disagrees with it, that place is right and the other file is corrected.\n\n")
	sb.WriteString("| Topic | Defined in |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString("| Repos, branches, verify commands, labels, review and retry settings | " + code(layout.ConfigFile) + " |\n")
	sb.WriteString("| The workflow loop: what to do for each next action, and when to stop | " + code(layout.SkillsDir+"/principal-workflow/SKILL.md") + " |\n")
	sb.WriteString("| What a ticket must contain | " + code(layout.SkillsDir+"/principal-workflow/references/ticket.md") + " |\n")

	// The kit's git rule has a row of its own; every other rule file says how
	// code is written here.
	var codeRules []string
	for _, rule := range ctx.Rules.Kit {
		if rule == "git-workflow" {
			sb.WriteString("| Branches, commit format, pull requests | " + code(layout.KitRulesDir+"/git-workflow.md") + " |\n")
			continue
		}
		codeRules = append(codeRules, code(layout.KitRulesDir+"/"+rule+".md"))
	}
	for _, rule := range ctx.Rules.Custom {
		codeRules = append(codeRules, code(layout.RulesDir+"/"+rule+".md"))
	}
	if len(codeRules) > 0 {
		sb.WriteString("| How code is written here | " + strings.Join(codeRules, ", ") + " |\n")
	} else {
		sb.WriteString("| How code is written here | No rule file yet. One goes in " + code(layout.RulesDir+"/") + " and is listed under " + code("rules.custom") + " in the configuration |\n")
	}

	sb.WriteString("| What a Worker is told | " + code("AGENTS.md") + " |\n")
	sb.WriteString("| Plans, design documents and decisions | " + code("docs/README.md") + " (the index) and " + code("docs/decisions.md") + " (append-only), once " + code("awkit plan") + " has written them |\n")
	sb.WriteString("| Who sits on the planning council | " + code(layout.RosterFile) + " |\n")
	sb.WriteString("| What runs left behind: results, logs, events | " + code(layout.StateDir+"/") + "; read it with " + code("awkit status") + " and " + code("awkit events") + ", not file by file |\n\n")
	sb.WriteString(code("CLAUDE.md") + ", " + code("AGENTS.md") + " and the files under " + code(layout.KitRulesDir+"/") + " are written by " + code("awkit generate") + " from the configuration. Change the configuration and run it again: an edit made in them is overwritten.\n\n")

	sb.WriteString("## Project\n\n")
	sb.WriteString(fmt.Sprintf("- Name: %s (%s)\n", ctx.Project.Name, ctx.Project.Type))
	sb.WriteString("- Repos: ")
	for i, repo := range ctx.Repos {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(code(repo.Name))
		if repo.Path != "" || repo.Language != "" {
			var about []string
			if repo.Path != "" {
				about = append(about, code(repo.Path))
			}
			if repo.Language != "" {
				about = append(about, repo.Language)
			}
			sb.WriteString(" (" + strings.Join(about, ", ") + ")")
		}
	}
	sb.WriteString("\n")
	sb.WriteString("- Branches: pull requests are merged into " + code(ctx.Git.IntegrationBranch) + "; " + code(ctx.Git.ReleaseBranch) + " is the release branch\n\n")

	sb.WriteString("## While a command runs (Principal)\n\n")
	sb.WriteString("A run lasts many loop iterations, and everything you read or write stays in your context for all of them. " + code("awkit") + " commands do their own waiting: " + code("awkit dispatch-worker") + " returns when the Worker has finished, and its output carries the result. So while a command runs, wait for it: do not read " + code(layout.LogsDir+"/") + " to follow the Worker, do not poll, do not narrate progress. When it returns, act on what it printed, as the skill describes.\n")

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
	sb.WriteString("---\n\n")
	sb.WriteString("Default priority: correctness > minimal diff > speed.\n\n")
	sb.WriteString("## MUST-READ (before any work)\n\n")

	// Kit rules
	for _, rule := range ctx.Rules.Kit {
		if rule == "git-workflow" {
			sb.WriteString(fmt.Sprintf("- Read and obey: %s"+layout.KitRulesDir+"/%s.md%s (branch and PR conventions; the runner makes the commits, you do not)\n", bt, rule, bt))
		} else {
			sb.WriteString(fmt.Sprintf("- Read and obey: %s"+layout.KitRulesDir+"/%s.md%s\n", bt, rule, bt))
		}
	}

	// Custom rules
	for _, rule := range ctx.Rules.Custom {
		sb.WriteString(fmt.Sprintf("- Read and obey: %s"+layout.RulesDir+"/%s.md%s\n", bt, rule, bt))
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
	sb.WriteString("2. Run verification commands (build, test, lint)\n\n")
	sb.WriteString("### 4. Implementation plan (REQUIRED before coding)\n")
	sb.WriteString("Before making any code changes, you MUST create an implementation plan file at:\n")
	sb.WriteString(bt + layout.RunsDir + "/issue-{ISSUE_ID}/plan.md" + bt + "\n\n")
	sb.WriteString("The plan MUST contain:\n")
	sb.WriteString("- " + bt + "## Summary" + bt + " — brief description of the approach (1-3 sentences)\n")
	sb.WriteString("- " + bt + "## Files to modify" + bt + " — list of files and what changes\n")
	sb.WriteString("- " + bt + "## Key decisions" + bt + " — important decisions and rationale\n\n")
	sb.WriteString("Write the plan FIRST. The runner includes it in the PR description for reviewers.\n\n")
	sb.WriteString("### 5. Review feedback handling\n")
	sb.WriteString("If you see a " + bt + "PREVIOUS REVIEW FEEDBACK" + bt + " section in your prompt:\n")
	sb.WriteString("- This means Principal rejected your previous work\n")
	sb.WriteString("- **Address ALL issues mentioned in the feedback**\n")
	sb.WriteString("- Pay special attention to:\n")
	sb.WriteString("  - Score Reason (why it failed)\n")
	sb.WriteString("  - Suggested Improvements (what to fix)\n")
	sb.WriteString("  - CI failures (if mentioned)\n\n")
	sb.WriteString("---\n\n")
	writeWorkerGuardrails(&sb)
	sb.WriteString("## REPO TYPE SUPPORT\n\n")
	sb.WriteString("AWK supports three repository types configured in " + bt + layout.ConfigFile + bt + ":\n\n")
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

// writeWorkerGuardrails emits the anti-rationalization table, red flags, and
// severity-tagged feedback sections. Keep in sync with the same sections in
// the repo root AGENTS.md.
func writeWorkerGuardrails(sb *strings.Builder) {
	sb.WriteString("## Common Rationalizations (READ BEFORE SHORTCUTTING)\n\n")
	sb.WriteString("When you feel tempted to skip a step, the excuse is almost always in this table. The right column is your reality check.\n\n")
	sb.WriteString("| Rationalization | Reality |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString("| \"I'll add tests after the code works\" | You won't. Test coverage is part of the change — Acceptance Criteria require it, and reviewer rejects PRs without verifiable tests. Write the test first or alongside. |\n")
	sb.WriteString("| \"This is too simple to need tests\" | Simple code grows complicated. The next change to it will break silently without a test pinning behavior. |\n")
	sb.WriteString("| \"I tested it manually, it works\" | Manual checks don't survive the next refactor. Reviewer cannot verify \"trust me\". |\n")
	sb.WriteString("| \"The failing test is probably wrong, I'll skip it\" | A failing test is a signal. Either fix the test (and explain why in the PR), or fix the code. Never silently skip. |\n")
	sb.WriteString("| \"Existing code is messy, let me clean it up while I'm here\" | Out-of-scope refactors balloon the diff and break unrelated things. Stay within ticket scope; file a follow-up issue if cleanup is needed. |\n")
	sb.WriteString("| \"I'll just stub this part for now\" | Half-finished implementations get merged and forgotten. Either complete the slice or split into a smaller ticket. |\n")
	sb.WriteString("| \"The Acceptance Criterion is vague, I'll guess\" | Don't guess. Re-read the ticket and the documents it points to; if still unclear, document your interpretation in " + bt + "plan.md" + bt + " so reviewer can correct or accept it. |\n")
	sb.WriteString("| \"This is what existing code does, I'll mirror it even if it's wrong\" | Existing bugs aren't a license to repeat them. If you spot a real issue, note it in " + bt + "plan.md" + bt + " rather than propagating. |\n")
	sb.WriteString("| \"Reviewer feedback is just nitpicking, I'll push back\" | Critical/Important feedback blocks merge. Read severity prefixes (see \"Responding to severity-tagged feedback\" below) and address everything that isn't Nit/Optional/FYI. |\n\n")
	sb.WriteString("## Red Flags (signs your work is going off-rails)\n\n")
	sb.WriteString("If any of these are true, STOP and reconsider before reporting completion:\n\n")
	sb.WriteString("- You ran no tests, or only \"happy path\" tests, before reporting done.\n")
	sb.WriteString("- Your diff touches files unrelated to the ticket.\n")
	sb.WriteString("- You added a test that passes immediately without ever failing first (it may not be testing what you think).\n")
	sb.WriteString("- You silently changed behavior that wasn't asked for.\n")
	sb.WriteString("- You copy-pasted criterion text into " + bt + "plan.md" + bt + " instead of describing the actual approach.\n")
	sb.WriteString("- You couldn't run verify commands and decided \"it should work\".\n")
	sb.WriteString("- You created a new pattern instead of extending the existing one because the existing one \"felt complicated\".\n")
	sb.WriteString("- You introduced a new dependency without checking if a similar one already exists in the repo.\n\n")
	sb.WriteString("If any red flag fires, fix it before you finish. Reviewer enforces these and a " + bt + "changes_requested" + bt + " round-trip is far more expensive than slowing down now.\n\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## Responding to severity-tagged feedback\n\n")
	sb.WriteString("Reviewer comments may be prefixed with severity tags. Respond accordingly:\n\n")
	sb.WriteString("| Prefix | Meaning | Your action |\n")
	sb.WriteString("|--------|---------|-------------|\n")
	sb.WriteString("| **Critical:** | Blocks merge (security, data loss, broken functionality) | MUST fix before next push |\n")
	sb.WriteString("| **Important:** | Should fix before merge (missing test, wrong abstraction, poor error handling) | MUST fix unless you have a strong, written reason to defer |\n")
	sb.WriteString("| **Nit:** | Minor / style preference | OPTIONAL — fix if cheap, otherwise leave a one-line note |\n")
	sb.WriteString("| **Optional:** / **Consider:** | Suggestion worth thinking about | OPTIONAL — your discretion |\n")
	sb.WriteString("| **FYI:** | Informational, no action required | No fix needed; acknowledge if relevant |\n\n")
	sb.WriteString("If a review has zero " + bt + "Critical:" + bt + " or " + bt + "Important:" + bt + " items, it should be approval-track — push a small fix and re-request review rather than re-architecting.\n\n")
	sb.WriteString("---\n\n")
}

func generateGitWorkflowMd(path string, ctx *TemplateContext) error {
	var sb strings.Builder
	code := func(s string) string { return bt + s + bt }
	integration, release := code(ctx.Git.IntegrationBranch), code(ctx.Git.ReleaseBranch)

	sb.WriteString("# Git & Workflow Rules\n\n")
	sb.WriteString("Scope: every branch, commit and pull request of this project. " + code("awkit generate") + " writes this file from " + code(layout.ConfigFile) + ": change the configuration, not this file.\n\n")

	sb.WriteString("## 1. Branches\n\n")
	if ctx.Project.Type == "monorepo" {
		if ctx.HasSubmodules {
			sb.WriteString("This project uses submodules:\n")
			for _, repo := range ctx.Repos {
				if repo.Type == "submodule" {
					sb.WriteString(fmt.Sprintf("- %s (%s)\n", code(repo.Name), repo.Language))
				}
			}
			sb.WriteString("\n")
		} else if ctx.HasDirectories {
			sb.WriteString("This project keeps its repos as directories of one repository:\n")
			for _, repo := range ctx.Repos {
				if repo.Type == "directory" {
					sb.WriteString(fmt.Sprintf("- %s: %s (%s)\n", code(repo.Path), repo.Name, repo.Language))
				}
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("- Integration branch: " + integration + ". Every pull request targets it, whichever repo the change is in.\n")
	sb.WriteString("- Release branch: " + release + ". Nothing targets it except a release: a pull request from " + integration + ", or the pull request of a ticket whose " + code("awk-ticket") + " block sets " + code("release: true") + ".\n")
	sb.WriteString("- Working branches: " + code("feat/<topic>") + " or " + code("fix/<topic>") + ". The workflow names its own " + code("feat/ai-issue-<id>") + ".\n\n")

	sb.WriteString("## 2. Commit messages\n\n")
	sb.WriteString("- Format: " + code(ctx.Git.CommitFormat) + "\n")
	sb.WriteString("- " + code("type") + " is one of " + code("feat") + ", " + code("fix") + ", " + code("docs") + ", " + code("style") + ", " + code("refactor") + ", " + code("perf") + ", " + code("test") + ", " + code("chore") + ".\n")
	if ctx.Git.CommitFormat == "[type] subject" {
		sb.WriteString("- The type is in square brackets with no colon after them, and the subject is lowercase. Conventional Commits (" + code("feat: add feature") + ") is not the format here.\n")
		sb.WriteString("- Example: " + code("[feat] add login endpoint") + "\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## 3. Pull requests\n\n")
	sb.WriteString("- A change reaches " + integration + " or " + release + " through a pull request, never by a direct push.\n")
	sb.WriteString("- The title is in the commit format.\n")
	sb.WriteString("- The body contains " + code("Closes #<issue>") + ", which ties the pull request to its ticket.\n")
	sb.WriteString("- It is merged once the required checks have passed.\n\n")

	section := 4
	if ctx.HasSubmodules {
		sb.WriteString(fmt.Sprintf("## %d. Submodules\n\n", section))
		sb.WriteString("- The commit a submodule is pinned to is reachable from that submodule's " + integration + ".\n")
		sb.WriteString("- A pinned commit changes only when the ticket asks for it.\n\n")
		section++
	}

	sb.WriteString(fmt.Sprintf("## %d. Rule files\n\n", section))
	sb.WriteString("- The files under " + code(layout.KitRulesDir+"/") + " come with AWK and are rewritten when it is upgraded. The files beside that directory are the project's own.\n")
	sb.WriteString("- A rule file holds rules: what is required, what is forbidden, what to check. The state of the project is not a rule; point to where it is defined.\n")
	sb.WriteString("- When a rule disagrees with the code or the git log, the code and the log are right, and the rule file is corrected.\n")

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func generateClaudeSettings(path string, agents AgentsConfig) error {
	allowEntries := []string{
		`      "Skill(principal-workflow)"`,
		`      "Task(pr-reviewer)"`,
		`      "Task(conflict-resolver)"`,
	}

	// Add custom agent permissions
	for _, agent := range agents.Custom {
		allowEntries = append(allowEntries, fmt.Sprintf(`      "Task(%s)"`, agent.Name))
	}

	allowEntries = append(allowEntries,
		`      "Bash(gh:*)"`,
		`      "Bash(git:*)"`,
		`      "Bash(awkit:*)"`,
		`      "Bash(bash:*)"`,
		`      "Bash(codex:*)"`,
		`      "Bash(go:*)"`,
		`      "Bash(npm:*)"`,
		`      "Bash(python:*)"`,
		`      "Bash(python3:*)"`,
	)

	content := "{\n  \"permissions\": {\n    \"allow\": [\n" +
		strings.Join(allowEntries, ",\n") +
		"\n    ]\n  }\n}\n"

	return os.WriteFile(path, []byte(content), 0644)
}

// builtinAgentFiles lists the agent definitions installed from the kit.
var builtinAgentFiles = []string{"pr-reviewer.md", "conflict-resolver.md"}

func installAgentsDir(agentsDir string, agents AgentsConfig) error {
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return err
	}

	// Built-in agent definitions have a single source of truth: the files the
	// kit ships under .claude/agents/ (embedded in KitFS). Hardcoded copies in
	// this file drifted, and generate then overwrote the current definitions
	// with stale ones.
	for _, name := range builtinAgentFiles {
		content, err := awkit.KitFS.ReadFile(".claude/agents/" + name)
		if err != nil {
			return fmt.Errorf("built-in agent %s is missing from the kit: %w", name, err)
		}
		agentPath := filepath.Join(agentsDir, name)
		if err := os.WriteFile(agentPath, content, 0644); err != nil {
			return err
		}
		fmt.Printf("[generate] Created: %s\n", agentPath)
	}

	// Generate custom agent definitions
	for _, agent := range agents.Custom {
		body := generateCustomAgentBody(agent)
		agentPath := filepath.Join(agentsDir, agent.Name+".md")
		if err := os.WriteFile(agentPath, []byte(body), 0644); err != nil {
			return err
		}
		fmt.Printf("[generate] Created: %s\n", agentPath)
	}

	// Clean stale agent files not in config
	builtinNames := map[string]bool{"pr-reviewer": true, "conflict-resolver": true}
	customNames := make(map[string]bool)
	for _, agent := range agents.Custom {
		customNames[agent.Name] = true
	}
	if err := cleanStaleAgents(agentsDir, builtinNames, customNames); err != nil {
		fmt.Printf("[generate] WARNING: failed to clean stale agents: %v\n", err)
	}

	return nil
}

func generateCustomAgentBody(agent CustomAgentDef) string {
	var sb strings.Builder

	// YAML frontmatter
	tools := agent.Tools
	if tools == "" {
		tools = "Read, Grep, Glob, Bash"
	}
	model := agent.Model
	if model == "" {
		model = "opus"
	}

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", agent.Name))
	sb.WriteString(fmt.Sprintf("description: %s\n", agent.Description))
	sb.WriteString(fmt.Sprintf("tools: %s\n", tools))
	sb.WriteString(fmt.Sprintf("model: %s\n", model))
	sb.WriteString("---\n\n")
	sb.WriteString(GeneratedAgentMark + "\n\n")

	// Body
	sb.WriteString(fmt.Sprintf("You are the **%s** agent.\n\n", agent.Name))
	sb.WriteString(fmt.Sprintf("## Description\n\n%s\n\n", agent.Description))

	if agent.Trigger != "" {
		sb.WriteString(fmt.Sprintf("## Trigger\n\nThis agent is triggered on: `%s`\n\n", agent.Trigger))
	}
	if agent.Condition != "" {
		sb.WriteString(fmt.Sprintf("## Condition\n\n%s\n\n", agent.Condition))
	}

	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Follow the project conventions defined in `CLAUDE.md` and `AGENTS.md`.\n")
	sb.WriteString("Use the available tools to complete your assigned task.\n")

	return sb.String()
}

// GeneratedAgentMark is in every subagent file the generator writes for a
// custom agent of the configuration. A file in the agents directory without
// it is the project's own.
const GeneratedAgentMark = "<!-- Generated by awkit from agents.custom in the configuration. -->"

// cleanStaleAgents removes the subagents the generator wrote for custom
// agents the configuration no longer names. A file it did not write is the
// project's own and stays: the agents directory is Claude Code's, and the
// project may keep subagents of its own there.
func cleanStaleAgents(agentsDir string, builtinNames, customNames map[string]bool) error {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		agentName := strings.TrimSuffix(name, ".md")
		if builtinNames[agentName] || customNames[agentName] {
			continue
		}
		stalePath := filepath.Join(agentsDir, name)
		if content, err := os.ReadFile(stalePath); err != nil || !strings.Contains(string(content), GeneratedAgentMark) {
			continue
		}
		if err := os.Remove(stalePath); err != nil {
			return err
		}
		fmt.Printf("[generate] Removed stale agent: %s\n", stalePath)
	}
	return nil
}

func validateCustomRules(stateRoot string, rules RulesConfig) []string {
	var warnings []string
	for _, rule := range rules.Custom {
		rulePath := filepath.Join(layout.Rules(stateRoot), rule+".md")
		data, err := os.ReadFile(rulePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("custom rule '%s' not found: %s", rule, rulePath))
			continue
		}
		if strings.TrimSpace(string(data)) == "" {
			warnings = append(warnings, fmt.Sprintf("custom rule '%s' is empty: %s", rule, rulePath))
		}
	}
	return warnings
}

# Platform Evolution — Requirements

## Goal

Based on ECC (everything-claude-code) competitive analysis, strengthen AWKit's platform across 6 areas in 3 priority tiers while preserving the core moat: deterministic workflow engine + GitHub state machine.

## Requirements

### R1: Agent Role Diversity

- THE system SHALL support custom agent definitions in `workflow.yaml` under an `agents.custom` list.
- EACH custom agent definition SHALL include: `name`, `description`, `tools`, `model`, `trigger`, and optional `condition`.
- THE `model` field SHALL accept values `haiku`, `sonnet`, `opus` and default to `opus` if omitted.
- THE `trigger` field SHALL map to `analyze-next` action types: `review_pr`, `check_result`, `dispatch_worker`, `generate_tasks`.
- THE `condition` field SHALL support simple string matching against issue metadata (labels, status).
- THE `awkit generate` command SHALL produce `.claude/agents/{name}.md` files for each custom agent with YAML frontmatter.
- THE generator SHALL remove stale agent files that are no longer in config (built-in agents are never removed).
- THE config validator SHALL reject duplicate agent names and invalid model/trigger values.
- Repos: root (config + generator changes)

### R2: Rich Skill & Rule Library

- THE repository SHALL ship at least 13 new example rule packs under `.ai/rules/_examples/`.
- NEW language-specific rules SHALL cover: Python, Rust, Node.js, Vue, Svelte, Flutter (6 rules).
- NEW methodology rules SHALL cover: testing strategy, API design, database migrations, security checklist, performance budget, accessibility, documentation (7 rules).
- EACH rule file SHALL include at minimum: Role, Goal, and at least one numbered section with actionable patterns.
- THE repository SHALL include 2 new skills: `post-mortem` (failure analysis) and `release-checklist` (release verification).
- THE `post-mortem` skill SHALL produce structured output: root cause, what was tried, recommended next action.
- THE `release-checklist` skill SHALL verify: all specs complete, no open P0 issues, CI green.
- THE `awkit generate` command SHALL validate enabled rule files for required sections and warn on missing structure.
- THE `awkit status` command SHALL display which custom rules are currently active.
- Repos: root (rules, skills, generator, status command)

### R3: Strategic Compaction

- THE system SHALL support a compaction strategy skill phase at `.ai/skills/principal-workflow/phases/compaction-strategy.md`.
- THE compaction strategy SHALL define trigger points: post-generation, post-batch (every N dispatches), post-review, and on-demand.
- EACH compaction checkpoint SHALL produce a structured Context Snapshot with: timestamp, task progress, active issues, last action, key decisions, blockers.
- THE system SHALL provide an `awkit context-snapshot` command that reads current workflow state from GitHub and outputs JSON or markdown.
- THE snapshot output SHALL include: epic progress, open issues with labels, recent PR statuses, active Worker info, blockers list.
- THE compaction interval SHALL be configurable via `principal.compaction.interval` in `workflow.yaml` (default: 5 dispatch cycles).
- THE compaction feature SHALL default to disabled (`enabled: false`) to preserve backward compatibility.
- THE main-loop skill phase SHALL reference compaction checkpoints at configurable intervals.
- Repos: root (new package, command, skill phase, config)

### R4: Review Feedback Loop

- THE system SHALL maintain a structured feedback log at `.ai/state/review-feedback-log.jsonl`.
- EACH feedback entry SHALL contain: `timestamp`, `issue_number`, `pr_number`, `spec`, `score`, `category`, `summary`, `rejection_count`.
- THE system SHALL categorize rejections using a defined taxonomy: `test_coverage`, `logic_error`, `style_violation`, `scope_creep`, `build_failure`, `security`, `performance`, `incomplete`.
- THE reviewer SHALL append a JSONL entry after every PR rejection (`requestChangesPR` path).
- JSONL writes SHALL be atomic (write to temp file, then rename).
- THE Worker prompt generator SHALL inject historical feedback into the prompt file, including:
  - Per-issue previous rejection reasons (all prior rejections for the same issue).
  - Top 3 most common rejection categories across all issues (pattern awareness).
- THE feedback injection SHALL be formatted as a structured `## Historical Feedback Patterns` section in the prompt.
- THE system SHALL provide an `awkit feedback-stats` command showing: total rejections by category, trend analysis, per-spec breakdown.
- THE feedback log SHALL be cleaned up by `awkit reset --all`.
- Repos: root (reviewer, worker, command, config)

### R5: Event Hook System

- THE system SHALL support a `hooks:` section in `workflow.yaml` defining shell commands to run at lifecycle events.
- THE system SHALL support 6 lifecycle events: `pre_dispatch`, `post_dispatch`, `pre_review`, `post_review`, `on_merge`, `on_failure`.
- EACH hook definition SHALL include: `command` (required), `timeout` (default: 30s), `on_failure` (default: `warn`), `env` (optional key-value map).
- THE `on_failure` field SHALL accept values: `warn` (log and continue), `abort` (return error to caller, block workflow), `ignore` (silently continue).
- HOOKS SHALL receive event-specific environment variables prefixed with `AWK_` (e.g., `AWK_ISSUE`, `AWK_PR`, `AWK_SCORE`, `AWK_STATUS`).
- HOOKS within the same event SHALL execute sequentially in definition order.
- HOOK timeouts SHALL be enforced via `context.WithTimeout`; timed-out hooks follow their `on_failure` policy.
- THE hook runner SHALL be implemented in a new `internal/hooks/` package with a `HookRunner` struct.
- THE `hooks:` section SHALL be optional; an empty or missing section means no hooks are fired.
- THE system SHALL provide an `awkit hooks list` subcommand displaying configured hooks per event.
- Repos: root (new package, runner integration, reviewer integration, config, command)

### R6: Multi-Model Worker Backends

- THE system SHALL define a `WorkerBackend` interface with methods: `Name()`, `Execute()`, `Available()`.
- THE existing Codex execution logic SHALL be refactored into a `CodexBackend` struct implementing `WorkerBackend`.
- THE refactored Codex backend SHALL preserve all existing behavior: `--full-auto` flag, retry logic, per-attempt logging, exit code handling.
- THE system SHALL include a `ClaudeCodeBackend` implementation wrapping the `claude` CLI.
- THE system SHALL provide a `BackendRegistry` for looking up backends by name.
- THE active backend SHALL be configurable via `worker.backend` in `workflow.yaml` (default: `codex`).
- EACH backend SHALL have its own config subsection (e.g., `worker.codex`, `worker.claude_code`) for backend-specific options.
- THE `awkit preflight` command SHALL verify that the configured backend binary exists in PATH and report its version.
- THE `worker.backend` default value (`codex`) SHALL ensure zero behavior change for existing users.
- Repos: root (worker package refactor, config, preflight)

## Cross-Cutting Concerns

- ALL new config sections SHALL have JSON Schema validation entries.
- ALL new Go packages SHALL include unit tests achieving reasonable coverage.
- ALL new commands SHALL be registered in `cmd/awkit/main.go` following the existing subcommand pattern (no CLI framework).
- ALL changes SHALL pass `go build ./...`, `go test ./...`, and `go vet ./...`.
- NO existing `workflow.yaml` files SHALL break; all new sections are optional with safe defaults.

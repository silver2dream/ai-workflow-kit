# Platform Evolution — Design

## Overview

### Motivation

Competitive analysis of ECC (everything-claude-code, 51K GitHub stars) revealed 6 improvement areas where AWKit can strengthen its platform without abandoning its core moat — the deterministic workflow engine + GitHub state machine.

ECC operates at the **config/prompt collection** layer (Markdown agents, skills, hooks). AWKit operates at the **workflow engine** layer (Go binary, state machine, automated merge pipeline). These layers are complementary, not competing. This spec borrows ECC's strongest patterns and integrates them into AWKit's engine-first architecture.

### Phased Approach

| Phase | Priority | Areas | Theme |
|-------|----------|-------|-------|
| Phase 0 | P0 | 1, 2 | Content depth — agent diversity + rule/skill library |
| Phase 1 | P1 | 3, 4 | Intelligence — compaction strategy + feedback loop |
| Phase 2 | P2 | 5, 6 | Extensibility — hooks + multi-model backends |

### Dependencies

All 6 areas are independently implementable. Soft benefits exist:
- Area 4 (feedback loop) informs Area 1 (agent design — rejection data reveals which agent roles are most needed)
- Area 6 (multi-model backends) pairs with Area 5 (hooks — backend switch events fire hooks)

---

## Phase 0: Content Depth

### Area 1: Agent Role Diversity

**Problem:** AWKit generates only 2 agents (`pr-reviewer`, `conflict-resolver`). ECC ships 13 specialized agents with model tiers and trigger conditions. Users cannot define custom agents without modifying Go source.

**Proposed Design:**

#### 1.1 Custom Agent Configuration

Add `agents.custom` to `workflow.yaml`:

```yaml
agents:
  # Built-in agents (always generated)
  builtin:
    - pr-reviewer
    - conflict-resolver

  # User-defined custom agents
  custom:
    - name: security-reviewer
      description: "Reviews PRs for security vulnerabilities (OWASP top 10)"
      tools: Read, Grep, Glob, Bash
      model: sonnet           # haiku | sonnet | opus
      trigger: review_pr      # lifecycle event that activates this agent
      condition: "labels:security"  # optional: only when issue has this label

    - name: build-error-resolver
      description: "Diagnoses and suggests fixes for build failures"
      tools: Read, Grep, Glob, Bash
      model: haiku
      trigger: check_result
      condition: "status:crashed OR status:failed_will_retry"
```

#### 1.2 Config Struct Changes

**File:** `internal/analyzer/config.go`

```go
type AgentsConfig struct {
    Builtin []string          `yaml:"builtin"`
    Custom  []CustomAgentDef  `yaml:"custom"`
}

type CustomAgentDef struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Tools       string `yaml:"tools"`
    Model       string `yaml:"model"`       // haiku | sonnet | opus
    Trigger     string `yaml:"trigger"`     // lifecycle event name
    Condition   string `yaml:"condition"`   // optional filter expression
}
```

Add `Agents AgentsConfig` field to the root `Config` struct.

#### 1.3 Generator Extension

**File:** `internal/generate/generator.go`

Extend `installAgentsDir()` to:
1. Continue generating built-in agents as today (lines 708-1017)
2. Iterate `config.Agents.Custom` and generate additional `.claude/agents/{name}.md` files
3. Each custom agent file follows the same YAML frontmatter format:
   ```
   ---
   name: {name}
   description: {description}
   tools: {tools}
   model: {model}
   ---
   {auto-generated instruction body based on trigger type}
   ```
4. Clean up agent files that exist on disk but are not in config (stale agent removal)

#### 1.4 Model Tier Validation

Validate `model` field against allowed values: `haiku`, `sonnet`, `opus`. Default to `opus` if omitted.

#### 1.5 Trigger/Condition Integration

The `trigger` field maps to `analyze-next` action types:
- `review_pr` — agent activated during PR review phase
- `check_result` — agent activated when checking Worker results
- `dispatch_worker` — agent activated before/after dispatch
- `generate_tasks` — agent activated during task generation

The `condition` field is a simple expression evaluated against issue metadata (labels, status). Implementation uses string matching, not a full expression parser.

**Files changed:**
- `internal/analyzer/config.go` — new structs + validation
- `internal/generate/generator.go` — extend `installAgentsDir()`
- `.ai/config/workflow.yaml` — schema update for `agents:` section

---

### Area 2: Rich Skill & Rule Library

**Problem:** AWKit ships only 4 example rules (`backend-go.md`, `frontend-react.md`, `frontend-unity.md`, `ui-toolkit-react-to-uxml.md`) and 2 skills (principal-workflow, worker-execution). ECC ships 48+ skills and rules across 6+ languages.

**Proposed Design:**

#### 2.1 New Example Rules (13 total)

**Language-specific (6 new):**

| File | Description |
|------|-------------|
| `backend-python.md` | Python/FastAPI patterns, async, typing, pytest |
| `backend-rust.md` | Rust/Axum patterns, error handling, lifetimes |
| `backend-node.md` | Node.js/Express patterns, ESM, error middleware |
| `frontend-vue.md` | Vue 3 Composition API, Pinia, Vitest |
| `frontend-svelte.md` | SvelteKit patterns, stores, load functions |
| `mobile-flutter.md` | Flutter/Dart patterns, BLoC, widget testing |

**Methodology (7 new):**

| File | Description |
|------|-------------|
| `testing-strategy.md` | Test pyramid, coverage targets, mocking policy |
| `api-design.md` | REST/GraphQL conventions, versioning, error format |
| `database-migrations.md` | Migration safety, rollback, zero-downtime DDL |
| `security-checklist.md` | OWASP top 10, input validation, auth patterns |
| `performance-budget.md` | Bundle size, TTFB, Core Web Vitals targets |
| `accessibility.md` | WCAG 2.1 AA, ARIA, keyboard nav, screen reader |
| `documentation.md` | ADR format, API docs, inline comment policy |

#### 2.2 New Skills (2 total)

**`post-mortem` skill:**
- Location: `.ai/skills/post-mortem/`
- Purpose: after a Worker fails max retries, Principal generates a structured post-mortem
- Outputs: root cause, what was tried, recommended next action
- Files: `SKILL.md`, `phases/analyze-failure.md`

**`release-checklist` skill:**
- Location: `.ai/skills/release-checklist/`
- Purpose: before merging integration branch to main, run through release verification
- Checks: all specs complete, no open P0 issues, CI green, CHANGELOG updated
- Files: `SKILL.md`, `phases/verify-release.md`

#### 2.3 Rule Activation Pipeline

Enhance `awkit generate` to:
1. Scan `.ai/rules/` for enabled custom rules
2. Validate rule files have required sections (Role, Goal, at minimum)
3. Report which rules are active in `awkit status` output

**Files changed:**
- `.ai/rules/_examples/` — 13 new markdown files
- `.ai/skills/post-mortem/` — new skill directory
- `.ai/skills/release-checklist/` — new skill directory
- `internal/generate/generator.go` — rule validation in generate pipeline
- `cmd/awkit/main.go` — `status` subcommand shows active rules

---

## Phase 1: Intelligence

### Area 3: Strategic Compaction

**Problem:** Principal agent has no context management strategy. Long workflows (20+ tasks) cause context overflow, degrading decision quality. ECC uses strategic compaction at logical boundaries.

**Proposed Design:**

#### 3.1 Compaction Strategy Skill

Create `.ai/skills/principal-workflow/phases/compaction-strategy.md`:

Define compaction trigger points in the main loop:
1. **Post-generation**: after `generate_tasks` completes, compact research context
2. **Post-batch**: every N dispatched workers (configurable, default 5), summarize progress
3. **Post-review**: after reviewing a PR, compact review details to summary
4. **On-demand**: Principal can request compaction when context feels heavy

Each compaction produces a structured summary:
```markdown
## Context Snapshot — {timestamp}
- Tasks: {completed}/{total}
- Active issues: #{list}
- Last action: {action} on #{issue}
- Key decisions: {bullet list}
- Blockers: {bullet list or "none"}
```

#### 3.2 Context Snapshot Command

**New command:** `awkit context-snapshot`

Reads current workflow state and produces a machine-readable summary:
- Epic progress (from GitHub)
- Open issues with labels
- Recent PR statuses
- Active Worker (if any)

Output: JSON for programmatic use, or markdown for prompt injection.

**Implementation:**

**File:** `cmd/awkit/main.go` — register `context-snapshot` subcommand

**File:** `internal/snapshot/snapshot.go` (new package)

```go
type ContextSnapshot struct {
    Timestamp    time.Time         `json:"timestamp"`
    Spec         string            `json:"spec"`
    Progress     ProgressSummary   `json:"progress"`
    OpenIssues   []IssueSummary    `json:"open_issues"`
    RecentPRs    []PRSummary       `json:"recent_prs"`
    ActiveWorker *WorkerSummary    `json:"active_worker,omitempty"`
    Blockers     []string          `json:"blockers"`
}
```

#### 3.3 Main Loop Integration

Update `.ai/skills/principal-workflow/phases/main-loop.md` to include compaction checkpoints:
- After every 5th `dispatch_worker` cycle, insert `awkit context-snapshot` call
- Feed snapshot into next decision cycle as compressed context
- Configurable interval via `workflow.yaml`:

```yaml
principal:
  compaction:
    enabled: true
    interval: 5          # compact every N dispatch cycles
    snapshot_format: json # json | markdown
```

**Files changed:**
- `.ai/skills/principal-workflow/phases/compaction-strategy.md` — new skill phase
- `cmd/awkit/main.go` — new `context-snapshot` subcommand
- `internal/snapshot/snapshot.go` — new package
- `internal/analyzer/config.go` — `PrincipalConfig` struct with compaction settings
- `.ai/skills/principal-workflow/phases/main-loop.md` — compaction checkpoint references

---

### Area 4: Review Feedback Loop

**Problem:** When a PR is rejected, the rejection reason is lost after the current session. Workers repeat the same mistakes. ECC records rejection reasons and injects them into future Worker prompts.

**Proposed Design:**

#### 4.1 Structured Feedback Log

**File:** `.ai/state/review-feedback-log.jsonl`

Each line is a JSON object:
```json
{
  "timestamp": "2026-02-25T10:30:00Z",
  "issue_number": 42,
  "pr_number": 55,
  "spec": "platform-evolution",
  "score": 4,
  "category": "test_coverage",
  "summary": "Missing unit tests for error paths in handler",
  "rejection_count": 1
}
```

#### 4.2 Rejection Category Taxonomy

| Category | Description |
|----------|-------------|
| `test_coverage` | Missing or insufficient tests |
| `logic_error` | Incorrect business logic |
| `style_violation` | Rule/convention violations |
| `scope_creep` | Changes outside ticket scope |
| `build_failure` | Code doesn't compile/build |
| `security` | Security vulnerability introduced |
| `performance` | Performance regression |
| `incomplete` | Acceptance criteria not fully met |

#### 4.3 Feedback Recording

**File:** `internal/reviewer/submit.go`

After `requestChangesPR()` (score < threshold), append to feedback log:
1. Extract category from review body (keyword matching or structured section)
2. Write JSONL entry with atomic append
3. Increment rejection count for the issue

#### 4.4 Prompt Injection into Worker

**File:** `internal/worker/runner.go`

In `writePromptFile()` (around line 1060), before the existing `PREVIOUS REVIEW FEEDBACK` section:
1. Read `.ai/state/review-feedback-log.jsonl`
2. Filter entries matching current issue number
3. Also include top 3 most common rejection categories across all issues (pattern awareness)
4. Format as structured section in prompt:

```markdown
## Historical Feedback Patterns
- Most common rejection reasons: test_coverage (5x), style_violation (3x), logic_error (2x)
- This issue was previously rejected for: test_coverage
  - "Missing unit tests for error paths in handler"
```

#### 4.5 Feedback Statistics Command

**New command:** `awkit feedback-stats`

Outputs:
- Total rejections by category
- Top offending patterns
- Trend (improving/worsening over last N PRs)
- Per-spec breakdown

```bash
$ awkit feedback-stats
Review Feedback Summary (last 30 days):
  Total rejections: 12
  By category:
    test_coverage:    5 (42%)
    style_violation:  3 (25%)
    logic_error:      2 (17%)
    scope_creep:      2 (17%)
  Trend: improving (3 rejections last week vs 5 previous week)
```

**Files changed:**
- `internal/reviewer/submit.go` — feedback recording after rejection
- `internal/reviewer/feedback.go` — new file: JSONL read/write, category extraction
- `internal/worker/runner.go` — inject historical feedback into prompt
- `cmd/awkit/main.go` — new `feedback-stats` subcommand
- `internal/analyzer/config.go` — feedback config (enabled, max_history)

---

## Phase 2: Extensibility

### Area 5: Event Hook System

**Problem:** All workflow lifecycle events are hardcoded in Go. Users cannot run custom scripts on dispatch, review, merge, or failure. ECC has 8 hook event types.

**Proposed Design:**

#### 5.1 Hook Configuration

Add `hooks:` section to `workflow.yaml`:

```yaml
hooks:
  pre_dispatch:
    - command: "scripts/notify-slack.sh"
      timeout: 30s
      on_failure: warn    # warn | abort | ignore
      env:
        CHANNEL: "#dev-ops"

  post_review:
    - command: "scripts/update-dashboard.sh"
      timeout: 15s
      on_failure: ignore

  on_merge:
    - command: "scripts/deploy-staging.sh"
      timeout: 120s
      on_failure: warn

  on_failure:
    - command: "scripts/alert-team.sh"
      timeout: 30s
      on_failure: ignore
```

#### 5.2 Lifecycle Events (6 total)

| Event | Fires when | Env vars injected |
|-------|-----------|-------------------|
| `pre_dispatch` | Before `dispatch_worker` starts | `AWK_ISSUE`, `AWK_SPEC`, `AWK_REPO` |
| `post_dispatch` | After Worker completes (any status) | `AWK_ISSUE`, `AWK_STATUS`, `AWK_EXIT_CODE` |
| `pre_review` | Before PR review begins | `AWK_PR`, `AWK_ISSUE`, `AWK_REPO` |
| `post_review` | After review decision | `AWK_PR`, `AWK_SCORE`, `AWK_DECISION` |
| `on_merge` | After PR successfully merged | `AWK_PR`, `AWK_ISSUE`, `AWK_BRANCH` |
| `on_failure` | When Worker crashes or max retries | `AWK_ISSUE`, `AWK_FAILURE_REASON`, `AWK_ATTEMPTS` |

#### 5.3 Hook Execution Engine

**New package:** `internal/hooks/`

**File:** `internal/hooks/hooks.go`

```go
type HookConfig struct {
    Command   string            `yaml:"command"`
    Timeout   time.Duration     `yaml:"timeout"`
    OnFailure string            `yaml:"on_failure"` // warn | abort | ignore
    Env       map[string]string `yaml:"env"`
}

type HookRunner struct {
    hooks   map[string][]HookConfig
    workDir string
    logger  *log.Logger
}

func (r *HookRunner) Fire(event string, envVars map[string]string) error
```

Execution rules:
- Hooks run sequentially within an event (order matters for pre_ hooks)
- Environment variables: merge config `env` + event-specific `AWK_*` vars
- Timeout enforced via `context.WithTimeout`
- `on_failure: abort` returns error to caller (blocks workflow)
- `on_failure: warn` logs warning but continues
- `on_failure: ignore` silently continues

#### 5.4 Integration Points

Wire `HookRunner.Fire()` into existing code:

| Event | Integration file | Location |
|-------|-----------------|----------|
| `pre_dispatch` | `internal/worker/runner.go` | Before Codex execution |
| `post_dispatch` | `internal/worker/runner.go` | After Codex returns |
| `pre_review` | `internal/reviewer/submit.go` | Before review logic |
| `post_review` | `internal/reviewer/submit.go` | After review decision |
| `on_merge` | `internal/reviewer/submit.go` | After `mergePR()` succeeds |
| `on_failure` | `internal/worker/runner.go` | On crash/max-retry |

**Files changed:**
- `internal/hooks/hooks.go` — new package: HookRunner, HookConfig, Fire()
- `internal/analyzer/config.go` — `HooksConfig` struct
- `internal/worker/runner.go` — Fire pre/post_dispatch and on_failure
- `internal/reviewer/submit.go` — Fire pre/post_review and on_merge
- `cmd/awkit/main.go` — `hooks list` subcommand (shows configured hooks)

---

### Area 6: Multi-Model Worker Backends

**Problem:** AWKit is hardcoded to use Codex as the Worker backend (`internal/worker/codex.go`). Users cannot swap in Claude Code, Gemini CLI, or other AI coding tools without modifying Go source.

**Proposed Design:**

#### 6.1 WorkerBackend Interface

**File:** `internal/worker/backend.go` (new)

```go
type WorkerBackend interface {
    // Name returns the backend identifier (e.g., "codex", "claude-code")
    Name() string

    // Execute runs the Worker task and returns the result
    Execute(ctx context.Context, opts WorkerOptions) (*WorkerResult, error)

    // Available checks if the backend binary is installed and accessible
    Available() error
}

type WorkerOptions struct {
    WorkDir     string
    PromptFile  string
    SummaryFile string
    LogBase     string
    MaxAttempts int
    RetryDelay  time.Duration
    Timeout     time.Duration
    Trace       *TraceRecorder
}

type WorkerResult struct {
    ExitCode      int
    Attempts      int
    Retries       int
    FailureStage  string
    FailureReason string
    Summary       string
}
```

#### 6.2 Backend Implementations

**Codex backend** (refactor existing):
- **File:** `internal/worker/backend_codex.go`
- Refactor `internal/worker/codex.go` into a `CodexBackend` struct implementing `WorkerBackend`
- Preserves all existing behavior: `--full-auto`, retry logic, log parsing

**Claude Code backend** (new):
- **File:** `internal/worker/backend_claude.go`
- Wraps `claude` CLI with `--print` mode
- Maps prompt file to stdin, captures output
- Handles Claude Code-specific flags (`--model`, `--max-turns`)

#### 6.3 Worker Configuration

```yaml
worker:
  backend: codex              # codex | claude-code
  codex:
    full_auto: true
    timeout: 600s
    max_attempts: 1
  claude_code:
    model: sonnet
    max_turns: 50
    timeout: 600s
    dangerously_skip_permissions: false
```

#### 6.4 Backend Registry

**File:** `internal/worker/registry.go` (new)

```go
type BackendRegistry struct {
    backends map[string]WorkerBackend
}

func NewBackendRegistry() *BackendRegistry
func (r *BackendRegistry) Register(b WorkerBackend)
func (r *BackendRegistry) Get(name string) (WorkerBackend, error)
```

#### 6.5 Runner Integration

**File:** `internal/worker/runner.go`

Replace direct Codex calls with:
1. Load `worker.backend` from config
2. Look up backend in registry
3. Call `backend.Execute(ctx, opts)`

This is a refactor of existing Codex-specific code into the interface pattern. Behavior for `backend: codex` (default) is identical to current behavior.

#### 6.6 Preflight Check

Add backend availability check to `awkit preflight`:
- Verify the configured backend binary exists in PATH
- Report version if available
- Warn if backend is not installed

**Files changed:**
- `internal/worker/backend.go` — new: WorkerBackend interface + types
- `internal/worker/backend_codex.go` — refactored from codex.go
- `internal/worker/backend_claude.go` — new: Claude Code backend
- `internal/worker/registry.go` — new: BackendRegistry
- `internal/worker/runner.go` — use BackendRegistry instead of direct Codex calls
- `internal/analyzer/config.go` — `WorkerConfig` struct
- `cmd/awkit/main.go` — preflight check for backend availability

---

## Testing Strategy

### Per-Area Unit Tests

| Area | Test file | Key tests |
|------|-----------|-----------|
| 1 | `internal/generate/generator_test.go` | Custom agent generation, stale cleanup, model validation |
| 2 | `internal/generate/generator_test.go` | Rule validation (required sections) |
| 3 | `internal/snapshot/snapshot_test.go` | Snapshot generation, JSON/markdown output |
| 4 | `internal/reviewer/feedback_test.go` | JSONL write/read, category extraction, stats calculation |
| 5 | `internal/hooks/hooks_test.go` | Fire sequencing, timeout, on_failure modes, env injection |
| 6 | `internal/worker/backend_test.go` | Interface compliance, registry lookup, Codex refactor parity |

### Integration Verification

All areas:
```bash
go build ./...
go test ./...
go vet ./...
```

### Backward Compatibility

- Area 1: `agents.custom` defaults to empty; existing behavior unchanged
- Area 3: `principal.compaction.enabled` defaults to false initially
- Area 4: feedback log created on first rejection; no impact if no rejections occur
- Area 5: `hooks:` section is optional; empty = no hooks fired
- Area 6: `worker.backend` defaults to `codex`; existing Codex behavior preserved exactly

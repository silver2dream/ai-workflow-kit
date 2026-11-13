# AWK Evolution — Requirements

Status: draft (pending human approval)
Owner: Principal
Created: 2026-07-06

## Motivation

A full architecture review (2026-07-06) identified three structural limits in the
current engine and one product gap:

1. **The workflow is strictly serial.** `analyzer.Decide` short-circuits to
   `check_result` whenever *any* issue carries `in-progress`
   (`internal/analyzer/analyzer.go` Step 1), so a second worker can never be
   dispatched. Worker execution is the long pole (10–30 min per issue) and it
   is fully blocking (`dispatch.go` runs the worker via `cmd.Run()`).
2. **There is no single-ticket entry point.** `awkit dispatch-worker --issue N`
   exists but requires a pre-existing Principal session, the `ai-task` label,
   and pipeline-created tracking refs. A user cannot point AWK at one issue and
   say "just do this one" without a full `kickoff`.
3. **Ticket metadata is regex-scraped prose.** `ParseTicketMetadata` extracts
   `Repo:`/`Severity:` etc. from free-form issue bodies with regex patterns.
   There is no structured field for dependencies, so no dependency-aware
   scheduling is possible.
4. **Product gap:** users must hand-author `requirements.md`/`design.md`/`tasks.md`
   before AWK can do anything. There is no path from a natural-language product
   idea to the document suite (business analysis, PRD, tech designs, art bible,
   test strategy) that a commercial project needs.

## Engineering Bar (applies to every requirement below)

- **Production grade. No workarounds, no fast paths, no bypasses.** The serial
  case and the parallel case MUST run through the same code path (serial =
  `max_workers: 1`), never a separate legacy branch.
- **Fail closed.** Invalid config, unparseable metadata, or validation failure
  stops the operation with a precise, actionable error. No silent fallbacks.
  The single sanctioned compatibility path (legacy ticket parsing, R3) MUST
  emit an observable deprecation event every time it is taken.
- **Every state mutation is crash-safe and idempotent.** Re-running any command
  after a crash converges to the same state (reconcile philosophy, as
  established by `internal/selfheal`).
- **Observable.** Every new mechanism emits events to the unified event stream
  (`.ai/state/events/`).
- **Tested.** `go build ./...`, `go test ./...`, `GOOS=linux go vet ./...`
  green; git-touching features get hermetic integration tests against a local
  bare remote (pattern established by `internal/selfheal` tests); CI runs with
  `-race`.

---

## R1 — Single-Ticket Direct Run

**User story:** As a user, I can run one existing GitHub issue through the full
lifecycle (dispatch → result check → review → merge) with a single command,
without configuring a spec, running an audit, or starting a full kickoff.

Acceptance criteria:
- `awkit run --issue N` executes the complete lifecycle for issue N and only
  issue N, reusing the existing multi-session loop and decision engine with an
  issue scope — not a parallel implementation of the loop.
- `awkit analyze-next --issue N` restricts every step of the decision ladder to
  issue N. Spec-level actions (`generate_tasks`, `create_task`, `audit_epic`)
  are never returned in scoped mode.
- Scoped mode terminates with `all_complete` when issue N is closed with a
  merged PR, and with the existing failure exit reasons otherwise.
- Preflight for `awkit run` checks everything that is relevant to a single
  issue (gh auth, claude CLI, clean tree, lock, config, labels) and does NOT
  require a non-empty `specs.active`.
- The single-instance lock (`kickoff.lock`) is shared with `kickoff`: one
  orchestrator per state root, always.

## R2 — Issue Adoption

**User story:** As a user, I can adopt an issue that was created outside the
pipeline (by hand, by another tool) so that AWK treats it exactly like a
pipeline-created issue — including progress tracking and false-completion
detection.

Acceptance criteria:
- `awkit adopt --issue N --repo <name> [--spec S]` is idempotent and:
  - validates the issue exists and is open (fail closed otherwise),
  - ensures the `ai-task` label,
  - normalizes the issue body with a structured ticket block (R3) carrying at
    minimum the target repo,
  - when `--spec` is given, registers the issue in that spec's tracking source
    (tasks.md task line with issue ref, or epic-body checkbox) so progress and
    false-completion machinery account for it.
- An adopted issue without `--spec` is dispatchable (analyzer Step 3 already
  selects labeled issues) and is excluded from spec progress accounting by
  design, not by accident — documented behavior.
- Adopting an issue that is already fully adopted is a no-op with exit 0.

## R3 — Structured Ticket Metadata (schema v1)

**User story:** As the engine, I read ticket metadata from a versioned,
strictly-parsed structure instead of regex-scraping prose, so that scheduling
(R6) and safety flags have a reliable source of truth.

Acceptance criteria:
- Ticket metadata lives in a fenced YAML block in the issue body (info string
  `yaml awk-ticket`), with a `schema: 1` field and strict decoding (unknown
  fields are errors). Fields cover at least: `repo`, `severity`, `spec`,
  `task`, `depends_on_tasks`, `release`, and the `allow_*` safety flags.
- `create-task` (skill template + any Go-rendered body) emits the block for
  every new issue.
- Parsing precedence: structured block first; when absent, the existing regex
  parser runs as a **managed migration path** and emits a
  `ticket_legacy_format` deprecation event with the issue number. A malformed
  structured block is a hard error (fail closed), never a silent fallthrough
  to regex.
- All existing metadata consumers (dispatch, runner, boundary checks) read
  from the same parsed struct; no consumer keeps its own regex.

## R4 — Inception: Natural Language → Document Suite

**User story:** As a user, I describe a product in natural language and AWK
produces the commercial-grade document suite — business analysis, PRD,
backend/frontend technical designs, art bible, test strategy — converged into
the `requirements.md`/`design.md`/`tasks.md` triple that the existing pipeline
consumes, then STOPS for my review.

Acceptance criteria:
- `awkit inception --spec <name> "<requirement>"` runs a role council defined
  by a declarative roster config (role name, backend, model, owned output
  paths, required sections, prompt template). Roster is validated fail-closed
  at load.
- Execution model: rounds with barriers. Round 1: every role drafts its owned
  documents. Round 2: every role reads the other roles' documents, records
  disagreements/decisions, and revises its own. A synthesizer role then
  converges the suite into `requirements.md`, `design.md`, `tasks.md`.
- **Single-owner enforcement, not convention:** each role executes in its own
  isolated worktree; the coordinator copies back ONLY paths the role owns and
  fails the round if the role modified anything else. Decision fragments are
  per-role files assembled by the coordinator into an append-only
  `00-governance/DECISIONS.md`.
- The synthesizer's `tasks.md` is validated by the SAME parsing code the
  analyzer uses in production (every unindented `- [ ] N.` line must be
  recognized; every `Depends on:` reference and every `depends_on_tasks`
  entry must resolve). Validation failure → exit 2 with per-file, per-section
  errors; generated documents are left in place for inspection.
- **Human gate:** inception never creates issues, epics, or branches, and
  never modifies `specs.active`. It ends by reporting the generated files and
  the explicit next command (`awkit create-epic` / enabling the spec).
- Re-running inception on an existing spec directory is the revision
  mechanism: roles read the existing documents ("documents are the only
  reliable memory") and improve them; the coordinator requires a clean git
  working tree before starting so user edits are never silently mixed with
  generated changes.

## R5 — Bounded Parallel Execution

**User story:** As a user, I set `concurrency.max_workers: 3` and AWK runs up
to three workers concurrently on independent issues, while reviews and merges
into the integration branch remain strictly serial.

Acceptance criteria:
- `concurrency.max_workers` (default `1`, validated range 1–8). The default
  MUST reproduce current behavior exactly through the same scheduler code.
- Scheduler invariant — **parallel execution, serial integration**: at most
  `max_workers` issues carry `in-progress`; at most ONE review/merge is in
  flight at any time. Conflicts introduced into a PR by a preceding merge are
  handled by the existing mergeability check + conflict auto-resolution path
  (PR #234), which becomes the designed backstop for serialized merges.
- Worker dispatch becomes supervised-async: the dispatcher spawns the worker
  subprocess, records its PID (`.ai/state/pids/issue-N.json`), and can either
  wait or return; `check-result` multiplexes across all in-progress issues.
  Crash/timeout detection continues to work per issue.
- No process-global mutable state in the dispatch path: worker-specific values
  (`AI_SPEC_NAME`, `AI_TASK_LINE`, `WORKER_SESSION_ID`) are injected into the
  subprocess environment explicitly (`cmd.Env`), never via `os.Setenv`.
- Shared counters (`loop_count`, `consecutive_failures`) use crash-safe atomic
  file primitives (Windows-portable lock, no flock) with defined semantics
  under concurrency.

## R6 — Dependency-Aware Scheduling

**User story:** As a user, my spec's task dependencies are respected: a task is
dispatched only when everything it depends on has merged.

Acceptance criteria:
- Dependencies are declared in ticket metadata (`depends_on_tasks`, spec task
  numbers — R3) and sourced from the spec's `Depends on:` lines during task
  generation.
- The scheduler dispatches an issue only when all its dependencies resolve to
  merged PRs. Deterministic rules: no dependency field → no constraint
  (current behavior); a dependency that cannot be resolved to an existing
  issue → the task is not ready (fail closed), reported via a decision event
  with a precise reason.
- With `max_workers: 1` the dependency filter still applies (correct ordering
  is not a parallel-only concern).

## R7 — CI and Test Robustness

Acceptance criteria:
- CI runs build + test on push to `main` (currently only feat/example, develop
  pushes and PRs are gated).
- Integration tests that skip (missing claude CLI / gh auth) are surfaced: CI
  prints a skip summary so a green run cannot silently mean "nothing ran".
- `TestIntegration_PreflightToLockRelease` fixture includes `specs.active` so
  the full local suite is green on a developer machine.

## Non-goals (this spec)

- Multi-spec / multi-epic concurrent progression (per-spec tracking mode,
  per-spec counters) — deferred until R5 is proven in production.
- Multiple orchestrator processes per state root. One orchestrator, N workers.
- Web/chat transports for inception roles (claude-agent-farm's Discord/K8s
  substrate). Roles are local subprocesses reusing the worker backend
  abstraction.
- Auto-committing or auto-publishing inception output. The human gate is a
  feature, not a limitation.

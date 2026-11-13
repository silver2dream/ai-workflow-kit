# AWK Evolution — Design

Status: draft (pending human approval)
Requirements: `.ai/specs/awk-evolution/requirements.md`
Created: 2026-07-06

## Overview

This design turns the engine from a strictly serial, kickoff-only pipeline into
a system that is:

1. **Ticket-addressable** — any single issue can be run through the full
   lifecycle directly (`awkit run --issue N`), and out-of-band issues can be
   adopted as first-class citizens (`awkit adopt`).
2. **Boundedly parallel** — up to `max_workers` workers execute concurrently in
   isolated worktrees while reviews/merges into the integration branch remain
   strictly serial ("parallel execution, serial integration").
3. **Self-bootstrapping** — `awkit inception` converts a natural-language
   product description into the commercial document suite (business analysis,
   PRD, backend/frontend tech design, art bible, test strategy) and converges
   it into the `requirements/design/tasks` triple the existing pipeline
   already consumes.

### Current-state facts this design builds on

| Fact | Location |
|------|----------|
| Serialization gate: any `in-progress` issue → `check_result`, never a 2nd dispatch | `internal/analyzer/analyzer.go` Step 1 (~L88–103) |
| Dispatch blocks on the worker subprocess | `internal/worker/dispatch.go` (`runWorkerScript` → `cmd.Run()`) |
| Ticket body = GitHub issue body; metadata regex-scraped | `dispatch.go` (~L221–223), `internal/worker/ticket.go` (`ParseTicketMetadata`) |
| Worker-specific values passed via `os.Setenv` | `dispatch.go` (~L311–312), `runner.go` (~L320) |
| Per-issue PID files + crash/timeout detection already exist | `internal/worker/process.go`, `.ai/state/pids/issue-N.json` |
| Per-issue worktrees/branches already collision-free | `internal/git/worktree.go`, branch `feat/ai-issue-{N}` |
| Global counters with intra-process mutex only | `analyzer.go` (`loop_count`, `consecutive_failures`) |
| State machine is label-derived and stateless-resumable | `analyzer.Decide` reconstructs from GitHub each call |
| PR mergeability check + conflict auto-resolution | analyzer Step 2 (PR #234), `PRMergeable` |
| Single-instance lock | `internal/kickoff/lock.go` (`kickoff.lock`, O_CREATE\|O_EXCL + PID liveness) |

### What deliberately does NOT change

- The decision-engine contract: `analyze-next` still returns exactly ONE
  decision per call; the Principal skill loop (`main-loop.md`) keeps its
  route-table shape. Parallelism is expressed through the *sequence* of
  decisions (multiple `dispatch_worker` decisions while quota is free), not
  through a new multi-decision protocol.
- GitHub labels remain the source of truth. No new local state becomes
  load-bearing for resumption.
- One orchestrator process per state root (`kickoff.lock` stays global).

---

## Global Engineering Constraints (binding for every task)

1. **One code path.** Serial is `max_workers: 1` flowing through the same
   scheduler as parallel. `awkit run` is the same loop + decision engine with
   an issue scope, not a re-implementation. Any PR that introduces an `if
   legacy { oldPath() }` fork around new logic is rejectable on sight — the
   only sanctioned compatibility path is the ticket-metadata migration (below),
   and it must be observable.
2. **Fail closed.** Config validation errors, malformed structured metadata,
   roster errors, unresolvable dependencies → hard error with actionable
   message and non-zero exit. Never degrade silently.
3. **Managed migration, not fallback.** Legacy ticket parsing survives only as
   an explicit migration path that (a) is reached only when the structured
   block is *absent*, (b) emits a `ticket_legacy_format` event with issue
   number every time, (c) has a documented removal milestone (one minor
   release after `create-task` emits structured blocks).
4. **Crash-safe state.** All multi-writer files go through
   `internal/statefile` primitives (new, Task 3): write-to-temp + rename for
   documents; lock-file (O_CREATE|O_EXCL with stale-PID detection, the proven
   `kickoff.lock` pattern — Windows-portable, no flock) + read-modify-write
   for counters.
5. **Interface discipline.** `GitHubClientInterface` changes must update BOTH
   mocks (`MockGitHubClient` in `internal/analyzer/analyzer_decide_test.go`
   and `mockGHClient` in `internal/epicaudit/audit_extra_test.go`) in the same
   commit.
6. **Cross-platform proof.** Any change near PTY/process handling requires
   `GOOS=linux go vet ./...` locally (Windows builds skip `pty_unix.go`);
   git-touching features require hermetic integration tests against a local
   bare remote (the `internal/selfheal` test pattern); CI keeps `-race`.
7. **Events.** Every new decision branch, scheduler action, adoption,
   inception round, and migration hit emits a typed event to
   `.ai/state/events/` (extend `internal/trace` event types).

---

## Phase A — Structured Ticket Metadata (R3)

### A.1 Schema

The issue body carries one fenced YAML block, info string `yaml awk-ticket`:

````markdown
```yaml awk-ticket
schema: 1
repo: backend            # required; must match a configured repo name
severity: P1             # optional: P0 | P1 | P2 | P3
spec: tennis-arena       # optional: spec this task belongs to
task: 7                  # optional: task number within the spec
depends_on_tasks: [3, 5] # optional: spec task numbers that must be merged first
release: false
allow:
  parent_changes: false
  script_changes: false
  secrets: false
```
````

Rationale for a fenced block over `---` frontmatter: GitHub renders `---` in
issue bodies as a horizontal rule and the block would be invisible/mangled; a
fenced block renders as code, survives copy/paste, and the info string makes it
unambiguous to locate.

### A.2 Parser

New `internal/ticket` package (replaces the parsing half of
`internal/worker/ticket.go`; `TicketMetadata` moves here, `worker` re-exports
or migrates its consumers):

```go
type Metadata struct {
    Schema         int    `yaml:"schema"`
    Repo           string `yaml:"repo"`
    Severity       string `yaml:"severity"`
    Spec           string `yaml:"spec"`
    Task           int    `yaml:"task"`
    DependsOnTasks []int  `yaml:"depends_on_tasks"`
    Release        bool   `yaml:"release"`
    Allow          Allow  `yaml:"allow"`
}

// Parse extracts metadata from an issue body.
// Precedence: structured block > legacy regex.
//  - Block present + valid  -> (meta, SourceStructured, nil)
//  - Block present + invalid -> error (fail closed; NEVER falls through)
//  - Block absent            -> legacy regex + (meta, SourceLegacy, nil)
func Parse(body string) (*Metadata, Source, error)
```

- Strict decoding (`yaml.Decoder` with `KnownFields(true)`); `schema` must be
  `1`; `repo` must be non-empty and is validated against configured repos by
  the caller that has config access (dispatch).
- Locating the block: first fence whose info string is exactly
  `yaml awk-ticket` (whitespace-tolerant). A second `awk-ticket` block is an
  error (ambiguity is a defect, not a choice).
- Callers: `dispatch.go` (`ParseTicketMetadata` call site), `runner.go`
  boundary checks, adoption (A.3 below), scheduler (Phase D). All consume the
  one parsed struct.
- On `SourceLegacy`, the caller emits event `ticket_legacy_format`
  (`internal/trace`: new `TypeTicketMigration`).

### A.3 Emission

- `.ai/skills/principal-workflow/tasks/create-task.md` template gains the
  block (fields filled from the task line: repo, spec, task number,
  `depends_on_tasks` from the design's `Depends on:` extraction that
  `generate-tasks.md` §A.0 already performs).
- The Ticket Format section in `CLAUDE.md` / `AGENTS.md` documents the block
  as the canonical metadata carrier; the prose bullets (`- Repo: ...`) remain
  human-facing duplicates until the migration milestone, then become optional.
- `awkit adopt` (Phase B) writes/normalizes the block on adopted issues via
  `UpdateIssueBody` (already on `GitHubClientInterface`).

### A.4 Tests

- Unit: block parsing (valid, unknown field, bad schema, duplicate block,
  absent → legacy path + source flag), round-trip with template output.
- Regression: every field the legacy regex supported has a structured
  equivalent asserted equal on a corpus of real historical ticket bodies
  (fixtures from the existing test suite).

---

## Phase B — Single-Ticket Entry (R1, R2)

### B.1 Analyzer issue scope

`internal/analyzer`:

```go
type DecideOptions struct {
    // IssueScope restricts the decision ladder to a single issue.
    // 0 = unscoped (full workflow).
    IssueScope int
}

func (a *Analyzer) DecideScoped(ctx context.Context, opts DecideOptions) (*Decision, error)
// Decide(ctx) == DecideScoped(ctx, DecideOptions{}) — one ladder, one code path.
```

Scoped semantics per ladder step (same steps, filtered):

| Step | Scoped behavior |
|------|-----------------|
| 0 loop-safety | unchanged (global counters still apply) |
| 1 in-progress | only issue N considered |
| 2 / 2.3 / 2.5 / 2.6 pr-ready, review-failed, merge-conflict, needs-rebase | only issue N |
| 2.7 worker-failed / needs-human-review | only issue N |
| 3 pending ai-task | only issue N (must carry `ai-task`; if missing → `none` with `exit_reason: issue_not_adopted`, message points to `awkit adopt`) |
| 4 spec-level (`generate_tasks`/`create_task`/`audit_epic`) | **skipped entirely** — scoped mode operates on an existing issue |
| 5 all_complete | issue N closed AND merged PR found → `all_complete`; closed WITHOUT merged PR → `none` with `exit_reason: issue_closed_unmerged` (false-completion guard applies in scope too) |
| 6 fallthrough | `none` (`no_actionable_tasks`) |

CLI: `awkit analyze-next --issue N` (flag plumbed to `DecideScoped`). JSON
output unchanged in shape.

### B.2 `awkit run --issue N`

`cmd/awkit/run.go` — composition, not duplication:

1. **Preflight subset:** the same `PreflightChecker` with a declared check
   profile. Refactor: each check gets a profile tag (`full`, `single-issue`);
   `RunAll(profile)` executes the matching set. Single-issue profile includes
   gh auth + write access, claude CLI, clean tree, STOP marker, lock, config
   validity, label existence; excludes `specs.active` non-empty and per-spec
   file checks. (Profile membership is data, not scattered `if` statements.)
2. **Validation:** issue exists, is open, carries `ai-task` (else the error
   text tells the user to run `awkit adopt --issue N`).
3. **Loop:** the existing multi-session kickoff loop parameterized with the
   scope — every `analyze-next` invocation gets `--issue N`; the Principal
   prompt names the scoped issue. `kickoff.lock` acquired exactly as today.
4. **Exit:** `all_complete` → 0; escalation/failure exit reasons → non-zero
   with the reason on stderr and in the JSON summary.

Implementation note: `cmdKickoff`'s loop body is extracted into a shared
`runOrchestratorLoop(cfg LoopConfig)` used by both `kickoff` (unscoped) and
`run` (scoped). Neither command keeps a private copy of the loop.

### B.3 `awkit adopt`

`cmd/awkit/adopt.go` + `internal/adopt`:

```
awkit adopt --issue N --repo backend [--spec S] [--dry-run] [--json]
```

Idempotent pipeline (each step checks-then-acts):
1. Fetch issue; must exist and be open (fail closed).
2. Ensure `ai-task` label.
3. Normalize body: ensure a valid `awk-ticket` block exists; if absent,
   prepend one built from flags (`repo` required — no guessing); if present,
   verify `repo` consistency with the flag (mismatch = error, not overwrite).
4. With `--spec S`: register in the tracking source —
   - tasks_md mode: append `- [ ] K. <issue title> <!-- Issue #N -->` (K =
     next task number) to the spec's tasks.md, commit is left to the user
     (dirty-tree rules unchanged) — the command prints the diff it made;
   - epic mode: append `- [ ] #N <title>` to the epic body via the existing
     epic-body update path.
5. Emit `issue_adopted` event; `--json` prints what was ensured vs already-ok
   (reconcile-style output, like `awkit reconcile`).

### B.4 Tests

- Analyzer: scoped-mode table tests for every ladder step (mock GH client);
  explicitly: scoped issue closed-unmerged → `issue_closed_unmerged`;
  unlabeled → `issue_not_adopted`; spec steps never returned.
- `run`: integration test driving a fake issue through
  dispatch→result→review-ready transitions with the mock/hermetic harness.
- `adopt`: idempotency (run twice = one change set), mismatch failure,
  tracking-source registration in both modes.

---

## Phase C — Inception (R4)

### C.1 Package layout

```
internal/inception/
  config.go      # roster schema + strict loader/validator
  coordinator.go # rounds, worktree lifecycle, ownership enforcement
  role.go        # role subprocess execution (reuses worker backend abstraction)
  synthesize.go  # convergence + spec-format validation
  prompts/       # embedded default role prompt templates (go:embed)
cmd/awkit/inception.go
```

### C.2 Roster config

`.ai/config/inception.yaml` (installed by `awkit generate`/`install`; separate
file so `workflow.yaml` stays lean):

```yaml
version: 1
rounds: 2                # 1 = draft only; 2 = draft + cross-review (default)
max_concurrent: 3        # bounded role parallelism within a round
roles:
  - name: business-analyst
    backend: claude       # claude | codex — same backend abstraction as workers
    model: opus
    owns: ["01-business/"]
    outputs:
      - path: 01-business/business-analysis.md
        required_sections: ["Market", "Target Users", "Competitors", "Business Model", "Risks"]
    prompt: business-analyst   # embedded template name; users may override with a file path
  - name: product-manager
    owns: ["02-product/"]
    outputs:
      - path: 02-product/prd.md
        required_sections: ["Goals", "User Stories", "Functional Requirements", "Non-goals", "Milestones"]
  - name: backend-architect
    owns: ["03-tech/backend/"]
    outputs:
      - path: 03-tech/backend/design.md
        required_sections: ["Architecture", "Data Model", "APIs", "Testing Strategy"]
  - name: frontend-architect
    owns: ["03-tech/frontend/"]
    outputs:
      - path: 03-tech/frontend/design.md
        required_sections: ["Architecture", "State Management", "UI Structure", "Testing Strategy"]
  - name: art-director
    owns: ["04-art/"]
    outputs:
      - path: 04-art/art-bible.md
        required_sections: ["Art Direction", "Style References", "Color & Typography", "Asset Pipeline"]
  - name: qa-lead
    owns: ["05-quality/"]
    outputs:
      - path: 05-quality/test-strategy.md
        required_sections: ["Test Levels", "Acceptance Criteria Mapping", "Tooling", "Release Gates"]
synthesizer:
  backend: claude
  model: opus
  # outputs are fixed: requirements.md, design.md, tasks.md (spec root)
```

Validation (fail closed): version == 1; role names unique; `owns` paths
relative, non-overlapping across roles, confined to the spec directory;
every output path inside an owned prefix; backend/model from the allowed sets;
unknown fields rejected (`KnownFields(true)`).

### C.3 Coordinator

```
awkit inception --spec tennis-arena "商業級即時聯網 3D 網球遊戲,支援排名和回放" [--roster PATH] [--dry-run]
```

1. **Preconditions:** clean git working tree (generated changes must never mix
   with uncommitted user edits); roster valid; spec name valid (path-safe).
2. **Rounds with barriers.** For each round, for each role (bounded by
   `max_concurrent`, `errgroup`):
   - Create an isolated worktree from HEAD (existing worktree machinery,
     `.worktrees/inception-<spec>-<role>`).
   - Render the role prompt: NL requirement + round instructions + the
     CURRENT content of every other role's documents (round ≥ 2) + governance
     rules ("documents are your only reliable memory"; required sections; the
     document metadata header: spec/document/version/status/owner/updated).
   - Execute the role via the worker backend abstraction (same subprocess,
     timeout, PID and summary capture as workers — no new execution engine).
   - **Ownership enforcement (mechanical, per-role worktree = exact
     attribution):** `git status --porcelain` in the role's worktree; every
     changed path must be under the role's `owns` prefixes or its decision
     fragment `00-governance/decisions/<role>-round<K>.md`. Any other change
     → the role's round FAILS (fail closed; event `inception_ownership_violation`),
     nothing is copied back.
   - Copy owned paths back into the primary tree (write-temp + rename).
   - Remove the worktree.
3. **Barrier**, then next round (roles now see each other's round-1 output).
4. **DECISIONS.md assembly:** coordinator concatenates decision fragments in
   deterministic order (round, then roster order) into
   `00-governance/DECISIONS.md` with an append-only guarantee across
   inception runs (existing content is never rewritten; new runs append a
   dated section).
5. **Synthesizer round:** one role that owns the spec root files, reads the
   whole suite, writes `requirements.md`, `design.md` (with a Step
   Dependencies table: step, repo, depends-on, acceptance criteria), and
   `tasks.md` in the analyzer-consumable format (unindented `- [ ] N. Title`
   main tasks, `Repo:` / `Depends on:` sub-bullets — the format proven by the
   tennis-arena spec).
6. **Structural validation** (C.4). Failure → exit 2, documents left in
   place, precise error list.
7. **Human gate:** print the generated tree + next steps
   (`review/edit → git commit → awkit create-epic --spec X --body-file ...`
   or enable in `specs.active`). Inception NEVER creates issues/epics/
   branches and never edits `workflow.yaml`.

Crash safety: every copy-back is atomic per file; a crashed run leaves either
the previous or the new version of each document, and re-running inception
converges (roles read current state). Worktrees are cleaned on startup if a
previous run left them (`inception-<spec>-*` prefix scan) — reconcile
philosophy.

### C.4 Validation

`internal/inception/synthesize.go` validates:
- every roster output exists, is non-empty, has all `required_sections`
  (heading match, case-insensitive);
- `tasks.md` parses with the analyzer's production task parser (exported for
  reuse — the validator MUST call the same function `checkTasksFiles` uses,
  not a re-implementation);
- every `Depends on:` / `depends_on_tasks` reference resolves to an existing
  task number; the dependency graph is acyclic (topological check);
- every `design.md` Step Dependencies row maps 1:1 to a `tasks.md` main task.

### C.5 Events & observability

`inception_start`, `inception_role_done` (role, round, duration, files),
`inception_ownership_violation`, `inception_validation_failed`,
`inception_complete`. Progress lines to stderr; machine output via `--json`.

### C.6 Tests

- Roster validation table tests (overlapping owns, escaping paths, unknown
  fields, bad backend).
- Coordinator with a fake role backend (scripted subprocess writing files):
  ownership violation blocks copy-back; round barrier ordering; DECISIONS
  assembly determinism; crash-rerun convergence.
- Synthesizer validation against golden good/bad suites, including "tasks.md
  line not recognized by production parser" and "cyclic dependency".
- Hermetic end-to-end with the fake backend in a temp git repo.

---

## Phase D — Bounded Parallelism (R5, R6)

### D.1 Config

```yaml
concurrency:
  max_workers: 1   # default; validated 1..8
```

`internal/analyzer/config.go` + `internal/kickoff/config.go` gain the section;
validation fail-closed. Default 1 == exact current behavior through the same
scheduler code (assert by test, not by review).

### D.2 State primitives (foundation)

New `internal/statefile`:

```go
// Counter: crash-safe, multi-process-safe integer file.
// Lock protocol: <path>.lock created O_CREATE|O_EXCL with PID+timestamp,
// stale detection identical to kickoff.lock; bounded retry with backoff.
func IncrementCounter(path string) (int, error)
func ResetCounter(path string) error
func ReadCounter(path string) (int, error)

// AtomicWriteFile: temp file + rename in the target directory.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error
```

`loop_count`, `consecutive_failures`, and per-PR attempt counters migrate to
these primitives (their existing semantics preserved; the intra-process mutex
in `analyzer.go` is subsumed). This lands BEFORE any concurrency is enabled.

### D.3 Env-injection cleanup

`dispatch.go` / `runner.go`: all `os.Setenv` for worker-specific values are
replaced by explicit `cmd.Env = append(os.Environ(), "AI_SPEC_NAME="+..., ...)`
on the worker subprocess. Grep-gate in CI-reviewable form: the dispatch path
contains zero `os.Setenv` calls after this task.

### D.4 Supervised-async dispatch

`dispatch-worker` gains `--detach`:

- Common path (both modes): spawn worker subprocess → write
  `.ai/state/pids/issue-N.json` (exists today) → register start event.
- `--detach=false` (default): wait for the subprocess, then post-process as
  today. `--detach=true`: return immediately with `{detached: true, pid: …}`;
  post-processing (result JSON interpretation, label transitions, failure
  handling) moves into `check-result`, which becomes the single owner of
  "worker finished" handling for BOTH modes (the sync mode simply calls it
  inline after the wait — one code path).
- `check-result --issue N` semantics unchanged; new `check-result --any`
  scans all in-progress issues and reports the first completed result (or
  `not_found` after the existing 30s wait). Timeout/crash detection per issue
  keeps using PID files (`process.go`).

### D.5 Scheduler (Step 1 rewrite)

Replace the short-circuit with a quota policy — same ladder, same single
decision returned:

```
IP = issues labeled in-progress (scoped set in scoped mode)
1. Any issue in IP with a completed/crashed/timed-out result → check_result(it)
2. |IP| >= max_workers                                        → check_result(oldest IP)
3. Ready dispatchable issue exists (Step 2.x/3 candidates,
   dependency-filtered per D.6)                               → that decision
   (dispatch decisions carry detach=true when max_workers > 1)
4. IP non-empty (workers running, nothing else to do)         → check_result(oldest IP)
5. Fall through to Steps 4–6 as today
```

**Serial integration invariant:** review/merge actions (`review_pr` and the
merge inside the review flow) are never emitted while another review is in
flight (a `pr-review-in-progress` marker label is NOT added; instead the
review flow is synchronous within the Principal loop — the loop executes one
decision at a time, so one review at a time holds by construction; the
invariant is asserted by scheduler tests). Conflicts created in remaining PRs
by a merge are caught by the existing Step 2 mergeability check and routed to
conflict auto-resolution (PR #234) — that path is the designed backstop and
gets an explicit parallel-scenario test.

With `max_workers: 1`: rule 2 fires whenever IP is non-empty — byte-for-byte
the current behavior, via the same code.

### D.6 Dependency-aware readiness (R6)

Dispatch candidate filter (applies in Step 3 and Step 4's re-dispatch path):

1. Parse candidate issue's `awk-ticket` block (Phase A). No
   `depends_on_tasks` → ready (no constraint).
2. For each dep task number: resolve via the spec's tracking source
   (task→issue) and require a merged PR (`MergedIssueBranches` /
   `IsPRMerged`, already on the interface). All merged → ready.
3. Unresolvable dep (task not yet an issue, or lookup error) → NOT ready;
   decision event `dependency_not_ready` records issue, dep, reason. If no
   candidate is ready and no worker is running, the ladder falls through to
   Step 4 (create more tasks) or `none` with a precise exit reason — never a
   silent stall.

Cycle defense: task generation validates acyclicity (C.4 for inception; the
same validator runs in `generate-tasks` skill flow via
`awkit validate --spec`), and the scheduler additionally breaks unexpected
runtime deadlock (all candidates blocked, nothing in flight) by escalating
`none`/`dependency_deadlock` — fail closed with diagnosis, not spin.

### D.7 Orchestrator loop under parallelism

The Principal skill contract keeps working unchanged: the loop calls
`analyze-next`, gets ONE decision, executes it, repeats. Parallelism emerges
because consecutive iterations return `dispatch_worker(detach)` for different
issues until quota fills, then `check_result` drains completions.
`main-loop.md` gains a short "parallel mode" note (dispatch may return
immediately; check_result multiplexes) — the route table itself is unchanged.

### D.8 Tests

- Scheduler table tests: quota boundary (0/1/N in progress), result-ready
  preemption, oldest-first draining, `max_workers: 1` equivalence (golden
  decision sequences vs current implementation on identical mock states).
- Counter primitives: multi-process increment torture test (spawned
  subprocesses, Windows + Linux CI).
- Hermetic parallel integration test (local bare remote, fake worker script
  that sleeps and commits): 3 issues, `max_workers: 2` → assert max 2
  concurrent PIDs, serial merges, all merged, and the "merge invalidates
  sibling PR → conflict auto-resolution path engages" scenario.
- Race: CI `-race` on the new packages.

---

## Phase E — CI & Test Robustness (R7)

1. `.github/workflows/ci.yml`: add `main` to `push:` branches so merged
   release PRs are built and tested; add a job step printing `go test`'s
   skipped-test summary (`-v` filtered or `gotestsum` summary) so silently
   skipped integration tests are visible in the log.
2. `TestIntegration_PreflightToLockRelease` fixture gains `specs.active` (the
   check added in 2db2048 made the fixture stale).

---

## Step Dependencies

| Step | Title | Repo | Depends on | Acceptance criteria (summary) |
|------|-------|------|------------|-------------------------------|
| 1 | CI gates main + skip visibility | root | – | push to main runs build+test; skip summary in CI log |
| 2 | Fix preflight integration fixture | root | – | full local `go test ./...` green |
| 3 | `internal/statefile` primitives | root | – | counter torture test green on win+linux, `-race` |
| 4 | Ticket metadata schema v1 (`internal/ticket`) | root | – | strict parse; legacy path emits event; corpus regression green |
| 5 | Emit structured block in create-task + adopt normalization + docs | root | 4 | new issues carry block; docs updated |
| 6 | Analyzer issue scope (`DecideScoped`, `--issue`) | root | – | scoped table tests; spec steps never returned in scope |
| 7 | Preflight profiles | root | – | full/single-issue profiles are data; existing behavior unchanged |
| 8 | `awkit run --issue N` | root | 6,7 | shared loop extraction; hermetic single-issue lifecycle test |
| 9 | `awkit adopt` | root | 4 | idempotent; both tracking modes; mismatch fails closed |
| 10 | Inception roster config + validation | root | – | fail-closed loader table tests |
| 11 | Inception coordinator (worktrees, rounds, ownership, DECISIONS) | root | 10 | fake-backend tests incl. ownership violation + crash-rerun |
| 12 | Inception default roster + role prompts (embedded) | root | 10 | prompts embedded; roster installs via generate |
| 13 | Inception synthesizer + spec-format validation | root | 11,12 | validates via production task parser; golden good/bad suites |
| 14 | `awkit inception` CLI + human gate + events | root | 11,12,13 | e2e fake-backend run produces valid suite then stops |
| 15 | Env-injection cleanup in dispatch path | root | 3 | zero `os.Setenv` in dispatch path; behavior unchanged |
| 16 | Supervised-async dispatch (`--detach`) + `check-result --any` | root | 15 | one finish-handling path; detach returns pid; crash/timeout intact |
| 17 | Scheduler quota (`max_workers`) + Step 1 rewrite | root | 16 | `max_workers:1` golden-equivalence; quota table tests |
| 18 | Dependency-aware readiness | root | 4,17 | dep filter; deadlock → fail-closed escalation; events |
| 19 | Parallel hermetic integration test + skills/docs update | root | 17,18 | 3-issue/2-worker scenario green incl. conflict backstop |
| 20 | Checkpoint | root | 1–19 | `go build ./...`, `go test ./...`, `GOOS=linux go vet ./...`, `awkit evaluate --offline` all green |

Steps 1–5 and 6–9 and 10–14 and 15–19 are four largely independent tracks;
within a track the listed order is binding.

---

## Rollout & migration

- Phases land behind nothing: single-ticket and inception are new commands
  (additive); parallelism defaults to `max_workers: 1` with golden-equivalence
  tests proving no behavioral change until a user raises the number.
- Legacy ticket parsing removal milestone: one minor release after Task 5
  ships, gated on `ticket_legacy_format` event volume observed in real runs.
- `main-loop.md` / `CLAUDE.md` / `AGENTS.md` documentation updates ship in the
  same PR as the behavior they describe (Task 19 for parallel; Tasks 5, 8, 9,
  14 for their commands).

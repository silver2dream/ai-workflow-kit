# AWK Evolution — Tasks

Repo: root
Coordination: dependency-ordered (see design.md Step Dependencies)
Design: `.ai/specs/awk-evolution/design.md`

## Tasks

- [ ] 1. CI gates main + integration-test skip visibility
  - Repo: root
  - Depends on: -
  - [ ] 1.1 Add `main` to ci.yml push triggers so merged release PRs run build+test
    - _Requirements: R7_
  - [ ] 1.2 Surface skipped tests in CI output (skip summary step)
    - _Requirements: R7_

- [ ] 2. Fix stale preflight integration fixture
  - Repo: root
  - Depends on: -
  - [ ] 2.1 Add `specs.active` to the `TestIntegration_PreflightToLockRelease` fixture so the full local suite is green
    - _Requirements: R7_

- [ ] 3. internal/statefile: crash-safe state primitives
  - Repo: root
  - Depends on: -
  - [ ] 3.1 Implement `IncrementCounter`/`ResetCounter`/`ReadCounter` with lock-file protocol (kickoff.lock pattern, Windows-portable) and `AtomicWriteFile`
    - _Requirements: R5_
  - [ ] 3.2 Migrate `loop_count`, `consecutive_failures`, and per-PR attempt counters to the primitives (semantics preserved)
    - _Requirements: R5_
  - [ ] 3.3 Multi-process counter torture test (spawned subprocesses), green on Windows and Linux with `-race`
    - _Requirements: R5_

- [ ] 4. internal/ticket: structured ticket metadata schema v1
  - Repo: root
  - Depends on: -
  - [ ] 4.1 Implement `Parse` with fenced `yaml awk-ticket` block, strict decoding, fail-closed on malformed block, legacy-regex migration path returning a source flag
    - _Requirements: R3_
  - [ ] 4.2 Emit `ticket_legacy_format` event (new trace type) whenever the legacy path is taken
    - _Requirements: R3_
  - [ ] 4.3 Migrate all metadata consumers (dispatch, runner boundary checks) to the single parsed struct; delete duplicated regex use
    - _Requirements: R3_
  - [ ] 4.4 Corpus regression test: legacy bodies parse identically via the new package
    - _Requirements: R3_

- [ ] 5. Emit structured ticket block on creation + docs
  - Repo: root
  - Depends on: Step 4
  - [ ] 5.1 Update create-task skill template to emit the `awk-ticket` block (repo, spec, task, depends_on_tasks from design extraction)
    - _Requirements: R3_
  - [ ] 5.2 Update CLAUDE.md / AGENTS.md ticket format docs; document the legacy-removal milestone
    - _Requirements: R3_

- [ ] 6. Analyzer issue scope
  - Repo: root
  - Depends on: -
  - [ ] 6.1 Add `DecideScoped(ctx, DecideOptions{IssueScope})`; `Decide` delegates with empty options (one ladder, one code path)
    - _Requirements: R1_
  - [ ] 6.2 Scoped semantics per design table: spec-level steps skipped; `issue_not_adopted`, `issue_closed_unmerged` exit reasons
    - _Requirements: R1_
  - [ ] 6.3 `analyze-next --issue N` flag; scoped table tests for every ladder step
    - _Requirements: R1_

- [ ] 7. Preflight check profiles
  - Repo: root
  - Depends on: -
  - [ ] 7.1 Tag each preflight check with profiles (`full`, `single-issue`) as data; `RunAll(profile)`; existing kickoff behavior unchanged (assert by test)
    - _Requirements: R1_

- [ ] 8. awkit run --issue N
  - Repo: root
  - Depends on: Steps 6,7
  - [ ] 8.1 Extract the kickoff multi-session loop into a shared `runOrchestratorLoop`; `kickoff` and `run` both consume it (no duplicated loop)
    - _Requirements: R1_
  - [ ] 8.2 Implement `awkit run --issue N` (single-issue preflight profile, ai-task validation with adopt hint, shared lock, scoped loop)
    - _Requirements: R1_
  - [ ] 8.3 Hermetic single-issue lifecycle integration test
    - _Requirements: R1_

- [ ] 9. awkit adopt
  - Repo: root
  - Depends on: Step 4
  - [ ] 9.1 Implement idempotent adopt pipeline (label, ticket-block normalization with repo consistency check, optional tracking-source registration for both modes, `--dry-run`/`--json`)
    - _Requirements: R2_
  - [ ] 9.2 Emit `issue_adopted` event; reconcile-style ensured/already-ok output
    - _Requirements: R2_
  - [ ] 9.3 Tests: idempotency, repo mismatch fails closed, tasks_md and epic registration
    - _Requirements: R2_

- [ ] 10. Inception roster config + validation
  - Repo: root
  - Depends on: -
  - [ ] 10.1 `internal/inception/config.go`: strict loader (KnownFields), non-overlapping `owns`, outputs inside owned prefixes, backend/model whitelists
    - _Requirements: R4_
  - [ ] 10.2 Fail-closed validation table tests
    - _Requirements: R4_

- [ ] 11. Inception coordinator
  - Repo: root
  - Depends on: Step 10
  - [ ] 11.1 Round barriers with bounded role parallelism (errgroup, `max_concurrent`); per-role isolated worktrees via existing worktree machinery
    - _Requirements: R4_
  - [ ] 11.2 Mechanical ownership enforcement (porcelain diff in role worktree; violation fails the round, nothing copied back) + atomic copy-back
    - _Requirements: R4_
  - [ ] 11.3 DECISIONS.md assembly from per-role fragments (deterministic order, append-only across runs); startup cleanup of orphaned inception worktrees
    - _Requirements: R4_
  - [ ] 11.4 Fake-backend coordinator tests: ownership violation, barrier ordering, crash-rerun convergence
    - _Requirements: R4_

- [ ] 12. Inception default roster + role prompts
  - Repo: root
  - Depends on: Step 10
  - [ ] 12.1 Embed default prompt templates (business-analyst, product-manager, backend-architect, frontend-architect, art-director, qa-lead, synthesizer) via go:embed with governance rules (docs-as-memory, metadata header, required sections)
    - _Requirements: R4_
  - [ ] 12.2 Install `.ai/config/inception.yaml` default roster via awkit generate/install
    - _Requirements: R4_

- [ ] 13. Inception synthesizer + spec-format validation
  - Repo: root
  - Depends on: Steps 11,12
  - [ ] 13.1 Synthesizer converges suite into requirements.md / design.md (Step Dependencies table) / tasks.md
    - _Requirements: R4_
  - [ ] 13.2 Structural validator: required sections; tasks.md validated via the exported production task parser; dependency resolution + acyclicity; design-steps ↔ tasks 1:1
    - _Requirements: R4_
  - [ ] 13.3 Golden good/bad suite tests
    - _Requirements: R4_

- [ ] 14. awkit inception CLI + human gate
  - Repo: root
  - Depends on: Steps 11,12,13
  - [ ] 14.1 `awkit inception --spec <name> "<requirement>"` with clean-tree precondition, `--roster`, `--dry-run`, `--json`; exit 2 on validation failure with per-file errors
    - _Requirements: R4_
  - [ ] 14.2 Human gate: report generated tree + next commands; never create issues/epics/branches, never touch specs.active
    - _Requirements: R4_
  - [ ] 14.3 Inception event types + end-to-end fake-backend test in temp git repo
    - _Requirements: R4_

- [ ] 15. Env-injection cleanup in dispatch path
  - Repo: root
  - Depends on: Step 3
  - [ ] 15.1 Replace all worker-specific `os.Setenv` with explicit `cmd.Env` injection (AI_SPEC_NAME, AI_TASK_LINE, WORKER_SESSION_ID); zero `os.Setenv` remains in the dispatch path
    - _Requirements: R5_
  - [ ] 15.2 Behavior-unchanged tests for env propagation to worker subprocess
    - _Requirements: R5_

- [ ] 16. Supervised-async dispatch + result multiplexing
  - Repo: root
  - Depends on: Step 15
  - [ ] 16.1 `dispatch-worker --detach`: spawn + PID registration shared with sync mode; detach returns `{detached, pid}` immediately
    - _Requirements: R5_
  - [ ] 16.2 Move worker-finish handling (result interpretation, labels, failure path) into check-result as the single owner for both modes
    - _Requirements: R5_
  - [ ] 16.3 `check-result --any` scanning all in-progress issues; crash/timeout detection intact per issue
    - _Requirements: R5_

- [ ] 17. Scheduler quota (max_workers)
  - Repo: root
  - Depends on: Step 16
  - [ ] 17.1 `concurrency.max_workers` config (default 1, validated 1..8) in analyzer + kickoff config
    - _Requirements: R5_
  - [ ] 17.2 Step 1 quota rewrite per design D.5 (result-ready preemption, quota check, oldest-first drain); dispatch decisions carry detach when max_workers > 1
    - _Requirements: R5_
  - [ ] 17.3 Golden-equivalence test: `max_workers: 1` decision sequences identical to pre-change behavior on identical mock states; quota boundary table tests
    - _Requirements: R5_

- [ ] 18. Dependency-aware readiness
  - Repo: root
  - Depends on: Steps 4,17
  - [ ] 18.1 Dispatch candidate filter: resolve `depends_on_tasks` via tracking source to merged PRs; unresolvable → not ready with `dependency_not_ready` event
    - _Requirements: R6_
  - [ ] 18.2 Deadlock defense: all candidates blocked + nothing in flight → `none`/`dependency_deadlock` escalation with diagnosis
    - _Requirements: R6_
  - [ ] 18.3 Table tests incl. max_workers:1 ordering enforcement
    - _Requirements: R6_

- [ ] 19. Parallel hermetic integration test + docs
  - Repo: root
  - Depends on: Steps 17,18
  - [ ] 19.1 Hermetic scenario (local bare remote, fake worker): 3 issues, max_workers 2 → ≤2 concurrent PIDs, serial merges, all merged; sibling-PR conflict routed through auto-resolution backstop
    - _Requirements: R5, R6_
  - [ ] 19.2 Update main-loop.md (parallel-mode note), CLAUDE.md, AGENTS.md in the same PR as behavior
    - _Requirements: R5_

- [ ] 20. Checkpoint
  - Depends on: Steps 1-19
  - Ensure `go build ./...`, `go test ./...`, `GOOS=linux go vet ./...`, and `awkit evaluate --offline` pass.

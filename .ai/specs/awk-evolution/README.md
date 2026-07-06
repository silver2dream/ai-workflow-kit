# AWK Evolution Spec

Turns the engine from a serial, kickoff-only pipeline into a ticket-addressable,
boundedly parallel system with a natural-language → document-suite inception
phase. See `requirements.md` (what & why), `design.md` (how, with per-step
acceptance criteria), `tasks.md` (execution units).

## Tracks

Four largely independent tracks (binding order only within a track):

| Track | Steps | Outcome |
|-------|-------|---------|
| Robustness | 1–2 | CI gates `main`; local suite green |
| Foundations | 3–5 | crash-safe state primitives; structured ticket metadata v1 |
| Single-ticket | 6–9 | `awkit run --issue N`, `awkit adopt` |
| Inception | 10–14 | `awkit inception` (NL → business/PRD/tech/art/QA docs → spec triple) |
| Parallelism | 15–19 | `concurrency.max_workers`, supervised-async dispatch, dependency-aware scheduling |

Step 20 is the final checkpoint.

## Engineering bar

Production grade, binding for every task (full text in `design.md` "Global
Engineering Constraints"): one code path (serial = `max_workers: 1`, `run` =
scoped kickoff loop — never a parallel implementation), fail closed, managed
migrations only (observable + removal milestone), crash-safe state, both GH
mocks updated with interface changes, `GOOS=linux go vet` + hermetic
integration tests + `-race`.

## Dogfooding note

This repo's `.ai/config/workflow.yaml` is the shipped template (example
backend/frontend repos, `specs.active: []`). Running `awkit kickoff` against
this spec requires a dedicated dogfood branch whose config declares the repo
as `type: root, language: go` and activates `awk-evolution` — that config
change must not land in release branches. Principal-driven implementation
(reading `design.md` step by step) is equally valid; the acceptance criteria
are written to be verifiable either way.

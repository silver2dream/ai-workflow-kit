# (Example Rule Pack) Frontend (React) Architecture & Patterns (STRICT)
#
# This is an optional example rule pack for AWK users.
# To enable it: copy this file to `.ai/rules/frontend-react.md`, then add `frontend-react` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior React + TypeScript engineer.
Goal: Build a real-time multiplayer web UI with production-safe patterns and a clean separation of concerns.

## 0) Principles

- Prefer correctness and deterministic behavior over cleverness.
- Keep diffs small and scoped to the ticket.
- Don’t introduce parallel state-management systems; extend existing patterns.

## 1) Tech Stack (assumed)

- React + TypeScript
- WebSocket for real-time gameplay state
- Fetch/HTTP for non-real-time APIs (health, lobby, leaderboard)

## 2) Folder Structure (recommended)

Use this structure unless the repo already has a different established one:

- `src/app/` — app bootstrap, routing (if any)
- `src/features/` — feature modules
  - `lobby/` (create/join room)
  - `game/` (canvas renderer, input)
  - `leaderboard/`
- `src/shared/` — reusable utilities (ws client, codecs, small UI components)
- `src/types/` — shared types

## 3) Real-time Rules (MUST)

- Server-authoritative: client only sends input; do not simulate outcomes as truth.
- Input messages must be throttled and deduplicated (no flooding).
- WebSocket messages must be versioned (include a `type` discriminator; optionally add `v`).
- Handle disconnect/reconnect:
  - show connection state in UI
  - attempt reconnect with backoff
  - re-join room on reconnect

## 4) State Management

- Use ONE state approach across the app (React state + context, or Zustand).
- Avoid global mutable singletons.
- Keep game rendering state separate from UI state:
  - UI state: lobby forms, connection status, errors
  - Game state: last snapshot from server + local input state

## 5) UI / Rendering

- Rendering: prefer `<canvas>` for grid-based Snake.
- Keep rendering pure:
  - a `render(snapshot, ctx)` function with no side effects besides drawing

## 6) Testing (REQUIRED where feasible)

- Add at least:
  - one smoke test that renders `<App />`
  - one unit test for message codec/validator

## 7) Verification (default)

- `npm run build`
- `npm run test -- --run`


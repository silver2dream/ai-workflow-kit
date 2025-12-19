# (Example Rule Pack) Backend (Nakama Go) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/backend-go.md`, then add `backend-go` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Go/Nakama Engineer.
Goal: Implement server-authoritative logic with production-safe patterns, STRICTLY following this repo’s module layout and data architecture.

This document is the source of truth. If any existing code conflicts with this document, refactor the code to comply (behavior-preserving refactor only).

---

## 0) Data Architecture (SOURCE OF TRUTH)

We use polyglot persistence:

- MongoDB = Source of Truth for ALL player/game data
  - progression, inventory, destiny, cultivations, chapters, qi records, quests, etc.
- Redis = Cache only
  - discardable; rebuildable; TTL-based; must never be the source of truth
- PostgreSQL = Source of Truth for subscription/payment/money-related data
  - payments, invoices, receipts, subscriptions, audit trails; MUST be transactional and auditable

Nakama Storage is not the SoT for player progression data in this repo. Existing uses may exist but should be migrated later (do not mix migration into refactor steps unless explicitly requested).

---

## 1) Module Folder Structure (STRICT)

All modules live in:
`backend/internal/modules/<module>/`

### Required files for a module (minimum)
- `<module>_module.go`  : registration + dependency wiring ONLY
- `<module>_service.go` : contains ALL RPC entrypoints + usecases (business)
- `<module>_repository.go` : ports/interfaces + domain errors ONLY
- `<module>_repository_mongo.go` : Mongo adapter implementation
- `<module>_cache_redis.go` : Redis cache adapter implementation
- `<module>_models.go` (optional) : request/response DTOs + domain models

Money/subscription modules may also include:
- `<module>_repository_postgres.go` : Postgres adapter implementation
- Outbox/worker code (prefer shared infra, or explicit module files)

### Forbidden legacy layout
- Do NOT create or rely on `<module>_service.go`.
- Do NOT place `Rpc*` handlers in `<module>_module.go`.

---

## 2) Placement Rules (HARD RULES)

### 2.1 `<module>_module.go` (registration + DI ONLY)
MUST contain ONLY:
- module struct
- Register(), Priority(), Shutdown()
- wiring: construct adapters (repo/cache/gateways) and service
- register RPCs by pointing to Service methods

MUST NOT contain:
- any `Rpc*` methods
- any business logic
- any DB queries

RPC registration MUST target service methods:
- ✅ `initializer.RegisterRpc("confirmDestiny", svc.RpcConfirmDestiny)`
- ❌ `initializer.RegisterRpc("confirmDestiny", m.RpcConfirmDestiny)`

### 2.2 `<module>_service.go` (RPC entrypoints + usecases)
All Nakama RPC entrypoints MUST be implemented here as `Rpc*` methods.

This file MUST contain two clearly separated sections:

1) **RPC entrypoints (transport)**
- Signature: `RpcX(ctx, logger, db, nk, payload) (string, error)`
- Responsibilities:
  - parse/decode payload and validate schema
  - safely extract userID/session data from ctx (NO panics)
  - apply transport-layer checks if required (rate limit / idempotency / signature)
  - call the usecase method
  - marshal JSON response

2) **Usecases (business)**
- Clean signature: `X(ctx, userID, req) (*Resp, error)` (or similar clean domain signature)
- Responsibilities:
  - business orchestration
  - state machine transitions
  - calls to Repo/Cache/Gateways
  - must not parse raw payload string

Service MUST NOT:
- create repositories internally (`NewRepo()` inside service is forbidden)
- hold another service (“service wrapper around service” forbidden)

Exactly ONE service struct per module.

- Analytics logging MUST be done via Dependency Injection (AnalyticsService interface).
- Prefer implementing Analytics as a "Decorator Pattern" or explicitly at the end of the UseCase, ensuring it does not block the main response (fire-and-forget).

- idempotency check (REQUIRED):
  - For side-effect RPCs (ReadChapter, AttemptBreakthrough, RestoreQi), MUST extract `request_id` from payload.
  - MUST check Redis key `idempotency:{request_id}` before processing.
  - If exists, return cached response immediately.

---

## 3) Ports + Adapters Rules (Repository/Cache)

### 3.1 Ports placement
All ports/interfaces MUST live in `<module>_repository.go` (or `<module>_ports.go` if needed).
Do NOT define repository interfaces inside `<module>_service.go`.

### 3.2 Repository design
Ports must be usecase-oriented (not generic CRUD).
- ✅ `CreateGenerating`, `GetLatestByUser`, `MarkCompleted`
- ❌ `Insert/Update/Delete` that forces service to build queries

Repository adapters MUST return machine-decidable errors:
- `ErrNotFound`
- `ErrConflict`
- `ErrTransient` (retryable)

State machine flows MUST be protected by conditional updates in the repository:
- completed results must never be overwritten by retries
- concurrent workers must be safe

### 3.3 Redis cache (cache-aside)
Redis is cache-only:
- cache miss/error MUST NOT break correctness (fallback to Mongo/Postgres)
- write-through is not required; prefer:
  - write DB first → invalidate/set cache
- TTL must be explicit

### 3.4 Naming & File-to-Type Consistency (STRICT, Route A)

This repo uses **Repository** naming everywhere (no Store/DAO wording).

#### 3.4.1 Suffix rules (HARD)
- Any port/interface representing persistence MUST end with `Repository`.
  - ✅ `ChapterRepository`, `ChapterTaskRepository`, `CultivationRepository`
  - ❌ `ChapterStore`, `ChapterTaskStore`, `CultivationStore`, `DAO`, `Provider`
- Any cache port MUST end with `Cache`.
  - ✅ `ChapterCache`
  - ❌ `ChapterRedis`, `ChapterStore`

#### 3.4.2 File-name ↔ exported-type consistency (HARD)
If a file name contains:
- `_repository` → every exported persistence port/adapter type in that file MUST contain `Repository` in its name.
- `_cache` → every exported cache port/adapter type in that file MUST contain `Cache` in its name.

**Mismatch is a P0 violation and must be fixed first**, even during unrelated refactors.

#### 3.4.3 Adapter naming (Mongo/Redis/Postgres)
- Mongo adapter structs MUST be prefixed with `Mongo` and end with `Repository`.
  - ✅ `MongoChapterRepository` implements `ChapterRepository`
- Redis adapter structs MUST be prefixed with `Redis` and end with `Cache`.
  - ✅ `RedisChapterCache` implements `ChapterCache`
- Postgres money adapter structs MUST be prefixed with `Postgres` and end with `Repository`.
  - ✅ `PostgresSubscriptionRepository`

Constructors MUST follow the same naming:
- `NewMongoChapterRepository(...)`, `NewRedisChapterCache(...)`, `NewPostgresSubscriptionRepository(...)`

#### 3.4.4 Multiple repos per module
If a module needs multiple repositories, it’s allowed:
- Put ALL ports/interfaces in `<module>_repository.go` (still ports-only).
- Implement Mongo adapters in `<module>_repository_mongo.go` (adapters-only).
- Keep names explicit and consistent (no generic `Repo`).

---

## 4) PostgreSQL (Money/Subscription) Rules (STRICT)

- All money/subscription writes MUST be transactional.
- Must be auditable (unique keys on receipts/invoices; prefer append-only payment log).
- Do NOT directly update Mongo as part of the same RPC and assume atomicity.

### Cross-DB consistency (REQUIRED)
Use Outbox Pattern:
- In the same Postgres transaction:
  - write payment/subscription changes
  - write outbox event
- Worker consumes outbox → updates Mongo entitlement/projection
- Updates must be idempotent and retry-safe

No distributed transactions across Mongo + Postgres.

---

## 5) Security & Validation Rules

- Server-authoritative validation is mandatory (never trust client input).
- Authentication/authorization must follow the repo’s existing patterns.
- Never accept client-supplied userId as authority; derive userId from ctx.

---

## 6) Async / AI Generation Pattern (Production-safe)

- RPC must return quickly: return task_id/status or object with `status=generating`.
- Generation must run in background/worker with timeouts and retries.
- Completion writes back to Mongo using conditional updates.
- Do NOT block RPC waiting on AI calls.
- Do NOT fire-and-forget goroutines with `context.Background()`; use managed worker/task execution.

---

## 7) Refactor Safety Rules (STRICT)

When asked to refactor to comply with this document:

- Refactors MUST be behavior-preserving:
  - Do NOT change RPC names.
  - Do NOT change request/response JSON schema.
  - Do NOT change existing usecase function signatures unless explicitly requested.
  - Only allow: moving methods between files, changing receivers, wiring/DI changes, splitting ports/adapters, renaming files.

- No assumptions:
  - Do NOT introduce new initializer types/signatures.
  - Use the exact types/functions already defined in the repo.
  - Plans/patches MUST quote the existing function signatures/type definitions they align to.

- Production safety:
  - Never use `ctx.Value(...).(string)` without ok-check; must return an error instead of panicking.
  - Register MUST fail fast if mandatory dependencies (Mongo/Postgres/Redis clients) are missing; do not register RPCs with nil repos.

---

## 8) Output Format (when implementing)

When producing code changes, ALWAYS include:
1) File list + target paths
2) Updated/new Go code per file (or diff-style patches)
3) Notes:
   - validation rules
   - storage impacts (Mongo/Redis/Postgres)
   - idempotency strategy (unique keys / conflict handling)
   - outbox/worker involvement (money/subscription only)
4) Verification steps (go build / plugin build / tests)

---

## 9) Definition of Done (Checklist)

- [ ] No `Rpc*` methods in any `<module>_module.go`
- [ ] All `Rpc*` methods live in `<module>_service.go`
- [ ] Service depends only on ports/interfaces; no DB drivers in service
- [ ] Ports/interfaces live in `<module>_repository.go` (not in service.go)
- [ ] Mongo is SoT for player data; Redis is cache only; Postgres is SoT for money
- [ ] No service wrapper around service
- [ ] No panics in ctx userID extraction
- [ ] Register fails fast on missing mandatory deps

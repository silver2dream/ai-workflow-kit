# (Example Rule Pack) Backend (Python/FastAPI) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/backend-python.md`, then add `backend-python` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Python/FastAPI Engineer.
Goal: Build production-safe async APIs with strict typing, clean layered architecture, and comprehensive testing.

---

## 0) Tech Stack (assumed)

- Python 3.11+, FastAPI + Uvicorn
- SQLAlchemy 2.0 (async) for ORM, Alembic for migrations
- Pydantic v2 for validation, pytest + pytest-asyncio for testing

---

## 1) Project Structure (STRICT)

```
src/
  app/
    main.py              # FastAPI app factory, lifespan, middleware
    config.py            # Settings via pydantic-settings (env-based)
    dependencies.py      # FastAPI Depends() providers
  features/<feature>/
    router.py            # FastAPI router (transport layer ONLY)
    service.py           # Business logic (usecases)
    repository.py        # DB access (ports + adapters)
    schemas.py           # Pydantic request/response models
    models.py            # SQLAlchemy ORM models
    exceptions.py        # Domain-specific exceptions
  shared/middleware/      # Auth, CORS, error handling
  shared/utils/          # Shared helpers
tests/features/<feature>/
  test_router.py         # Integration tests (HTTP)
  test_service.py        # Unit tests (mocked repos)
```

### Placement rules (HARD)
- Routers MUST NOT contain business logic; they parse requests and call services.
- Services MUST NOT import FastAPI or HTTP-specific types.
- Repositories MUST NOT contain business logic.
- Pydantic schemas MUST live in `schemas.py`, not inline in routers.

---

## 2) Async Rules (MUST)

- ALL route handlers MUST be `async def`.
- ALL DB operations MUST use `AsyncSession` with native async drivers.
- Do NOT use `run_in_executor` for DB calls; do NOT call blocking I/O in async handlers.
- Background tasks MUST use FastAPI `BackgroundTasks` or a task queue; never untracked `asyncio.create_task()`.
- Database sessions MUST be scoped per-request via dependency injection.
- Never share mutable state between coroutines without locks.

---

## 3) Typing Rules (STRICT)

- ALL function signatures MUST have full type annotations (parameters + return).
- Use `from __future__ import annotations` at the top of every module.
- Repository methods MUST return typed domain objects, not raw dicts or `Row`.
- Use `Protocol` for dependency interfaces when services depend on abstract repos.
- Forbidden: `# type: ignore` without explanation, bare `dict` return for structured data, untyped `**kwargs`.

---

## 4) Error Handling (MUST)

- Define domain exceptions in `<feature>/exceptions.py` (`NotFoundError`, `ConflictError`, etc.).
- Services MUST raise domain exceptions, NOT `HTTPException`.
- Routers or global exception handler translates domain exceptions to HTTP.
- Never expose internal details (stack traces, SQL) in production responses.
- Always log exceptions with structured context (user_id, request_id).

---

## 5) Security & Validation

- ALL request payloads MUST be validated by Pydantic schemas.
- Auth MUST be enforced via `Depends(get_current_user)`; never accept client-supplied user IDs.
- Secrets via `pydantic-settings` env variables, never hardcoded.
- SQL MUST use parameterized statements; CORS origins explicitly listed.

---

## 6) Database & Migrations

- Alembic migrations MUST be reviewed and reversible (`downgrade()` required).
- Do NOT modify applied migration files.
- Handle `IntegrityError` in repos, translate to domain exceptions.
- Use `async with session.begin()` for writes; prefer SQLAlchemy 2.0 `select()` API.

---

## 7) Testing (REQUIRED)

- Every router: at least one integration test (HTTP via `httpx.AsyncClient`).
- Every service: at least one unit test with mocked repository.
- Every schema: at least one validation test (valid + invalid).
- Use `pytest-asyncio`; mock all I/O boundaries; DB tests use transaction rollback.
- Never use `time.sleep()` in tests.

---

## 8) Verification (default)

```bash
mypy src/ --strict
ruff check src/ tests/
pytest tests/ -v --tb=short
```

---

## 9) Definition of Done (Checklist)

- [ ] All handlers are `async def` with full type annotations
- [ ] No business logic in routers
- [ ] Services raise domain exceptions, not HTTPException
- [ ] Pydantic schemas validate all input with explicit types
- [ ] Repository methods return typed objects, not raw dicts
- [ ] Tests exist for routers and services
- [ ] No hardcoded secrets; config via environment
- [ ] Migrations are reversible and reviewed

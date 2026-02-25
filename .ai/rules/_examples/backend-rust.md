# (Example Rule Pack) Backend (Rust/Axum) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/backend-rust.md`, then add `backend-rust` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Rust Engineer.
Goal: Build safe, performant async APIs with proper error handling, strict ownership discipline, and comprehensive testing.

---

## 0) Tech Stack (assumed)

- Rust (latest stable), Axum + Tokio
- SQLx (compile-time checked queries preferred) or SeaORM
- serde + serde_json, thiserror, tracing, tower middleware

---

## 1) Project Structure (STRICT)

```
src/
  main.rs                 # Entry: runtime setup, server bind
  lib.rs                  # App factory, router composition
  config.rs               # Typed config from env
  routes/<feature>.rs     # Handler functions (transport only)
  services/<feature>.rs   # Business logic (usecases)
  repositories/<feature>.rs  # Database access layer
  models/<feature>.rs     # Domain types + DTOs
  errors.rs               # AppError enum + IntoResponse impl
  state.rs                # AppState (shared deps)
tests/<feature>_test.rs   # Integration tests
```

### Placement rules (HARD)
- Handlers MUST NOT contain business logic; extract request data and call services.
- Services MUST NOT import `axum` types; operate on domain types only.
- Repositories MUST NOT contain business logic.
- `AppState` holds shared deps (DB pool, config); passed via Axum state extraction.

---

## 2) Error Handling (MUST)

Define a single `AppError` enum using `thiserror` with variants: `NotFound`, `Conflict`, `Validation`, `Unauthorized`, `Internal`.

### Rules
- ALL handlers MUST return `Result<impl IntoResponse, AppError>`.
- Implement `IntoResponse` for `AppError` to map variants to HTTP status codes.
- Never use `.unwrap()` or `.expect()` in production code paths; always propagate with `?`.
- `.unwrap()` allowed ONLY in tests and `main()` startup.
- Never expose internal details (SQL errors, file paths) in HTTP responses.
- Forbidden: `panic!()` in handlers, `.unwrap()` on `Option`/`Result` in handler/service/repo code.

---

## 3) Lifetimes & Ownership (STRICT)

- Prefer owned types (`String`, `Vec<T>`) in DTOs and DB models.
- Use `&str` / `&[T]` in function params when ownership is not needed.
- Use `Arc<T>` for shared state; do NOT use `Rc<T>` in async code.
- Do NOT add lifetime parameters to structs unless genuinely borrowing.
- Prefer `String` over `&'a str` in request/response types (serde requires owned for deserialization).
- If `&self` returns a `Future`, clone needed fields into the future to avoid borrowing beyond await.

---

## 4) Async Patterns (MUST)

- ALL handlers MUST be `async fn`. DB queries MUST be async.
- Use `tokio::spawn` for background work with proper error logging.
- Never use `std::thread::sleep` in async; use `tokio::time::sleep`.
- Never use `block_on` inside async context.
- Spawned tasks MUST handle their own errors; do NOT ignore `JoinHandle`.
- Shared mutable state MUST use `tokio::sync::Mutex` (not `std::sync::Mutex`) if held across `.await`.

---

## 5) Serialization & Validation (STRICT)

- Request bodies via `axum::Json<T>` where `T: Deserialize`.
- Response bodies via `axum::Json<T>` where `T: Serialize`.
- Use `#[serde(deny_unknown_fields)]` on request DTOs.
- Path/query params MUST use typed extractors (`Path<(Uuid,)>`, `Query<Params>`).
- Naming: `<Action>Request`, `<Action>Response` in `models/<feature>.rs`.

---

## 6) Security Rules

- Auth via Axum middleware or `FromRequestParts` extractors.
- Never accept client-supplied user IDs; derive from JWT claims.
- Secrets from env, loaded at startup. SQL MUST use parameterized bind.
- Explicit CORS origins; no wildcard in production.

---

## 7) Testing (REQUIRED)

- Every handler: at least one integration test (HTTP round-trip).
- Every service: at least one unit test with mocked repo (trait + mock impl).
- Error paths MUST be tested. Use `#[tokio::test]` for async tests.
- Test DB with migrations; mock external services via trait objects.
- Unit tests: `#[cfg(test)] mod tests` in source files. Integration: `tests/` directory.

---

## 8) Verification (default)

```bash
cargo build
cargo clippy -- -D warnings
cargo test
cargo fmt -- --check
```

---

## 9) Definition of Done (Checklist)

- [ ] No `.unwrap()` / `.expect()` in handler/service/repo code
- [ ] All handlers return `Result<impl IntoResponse, AppError>`
- [ ] Services depend on trait abstractions, not concrete DB types
- [ ] No business logic in route handlers
- [ ] Request/response types have explicit serde derives
- [ ] Tests exist for handlers and services
- [ ] `cargo clippy` passes with zero warnings
- [ ] No `std::sync::Mutex` held across `.await`

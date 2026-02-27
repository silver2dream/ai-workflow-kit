# (Example Rule Pack) Testing Strategy & Coverage (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/testing-strategy.md`, then add `testing-strategy` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior QA/Testing Engineer.
Goal: Ensure comprehensive test coverage following the test pyramid, with clear mocking boundaries and CI-enforced quality gates.

This document is the source of truth for testing standards. All new code MUST include tests that comply with these rules.

---

## 0) Test Pyramid (STRICT)

Follow the test pyramid distribution:

- **Unit tests** (70%): fast, isolated, no I/O
- **Integration tests** (20%): verify component interactions, may use test databases or containers
- **E2E tests** (10%): full system tests, run against deployed or containerized environment

Every PR MUST include unit tests for new logic. Integration and E2E tests are REQUIRED for new endpoints, workflows, or cross-service interactions.

---

## 1) Unit Tests (MUST)

### 1.1 Scope
- Test a single function, method, or class in isolation.
- All external dependencies (DB, HTTP, filesystem, clock) MUST be mocked or stubbed.
- No network calls, no disk I/O, no database connections.

### 1.2 Naming Convention
- Test files: `*_test.go` (Go), `*.test.ts` / `*.spec.ts` (TypeScript), or equivalent.
- Test names MUST describe the behavior, not the implementation:
  - `TestCreateUser_DuplicateEmail_ReturnsConflict`
  - `TestCalculateDiscount_EmptyCart_ReturnsZero`
  - Do NOT use generic names like `TestFunc1` or `TestHappyPath`.

### 1.3 Structure (AAA Pattern)
Every test MUST follow Arrange-Act-Assert:
1. **Arrange**: set up inputs, mocks, and expected values
2. **Act**: call the function under test
3. **Assert**: verify the result and side effects

### 1.4 Table-Driven Tests (RECOMMENDED)
For functions with multiple input/output combinations, prefer table-driven tests to reduce duplication.

---

## 2) Integration Tests (REQUIRED for I/O boundaries)

### 2.1 Scope
- Test interactions between two or more components (service + repository, API + database).
- Use real dependencies where practical (testcontainers, in-memory databases, test servers).

### 2.2 Isolation
- Each integration test MUST set up and tear down its own state.
- Tests MUST NOT depend on execution order.
- Use transactions or truncation to reset database state between tests.

### 2.3 Tagging
- Integration tests MUST be tagged/labeled so they can be run separately from unit tests:
  - Go: `//go:build integration`
  - JS/TS: describe block with `[integration]` prefix, or separate test config

---

## 3) E2E Tests (REQUIRED for critical paths)

### 3.1 Scope
- Test complete user workflows from the external interface (API, UI).
- Run against a fully deployed or containerized system.

### 3.2 Required Coverage
At minimum, E2E tests MUST cover:
- Authentication flow (login, token refresh, logout)
- Primary CRUD operations for core entities
- Error handling for invalid requests (4xx responses)

### 3.3 Stability Rules
- E2E tests MUST be deterministic; no flaky tests allowed in CI.
- Use explicit waits (not sleep) for async operations.
- Retry logic for known transient failures MUST be documented.

---

## 4) Mocking Strategy (STRICT)

### 4.1 What to Mock
- External services (third-party APIs, payment gateways)
- Database connections (in unit tests only)
- Time/clock (use injectable clock interface)
- Random number generators (use seeded or injectable source)

### 4.2 What NOT to Mock
- The code under test itself
- Pure utility functions with no side effects
- Data structures and value objects

### 4.3 Mock Rules
- Prefer interface-based mocking over monkey-patching or reflection.
- Mocks MUST verify call expectations (called with correct args, called N times).
- Do NOT create mocks that return hardcoded values without assertions; that hides bugs.
- Mock implementations MUST live in `*_test.go` files or a dedicated `testutil/` package.

---

## 5) Coverage Requirements (ENFORCED)

### 5.1 Minimum Thresholds
- **Overall**: 80% line coverage minimum
- **New code in PR**: 90% line coverage minimum
- **Critical paths** (auth, payments, state machines): 95% line coverage minimum

### 5.2 Exclusions
The following MAY be excluded from coverage calculations:
- Generated code (protobuf, OpenAPI stubs)
- Main/entry point files (bootstrap/wiring only)
- Test utilities and helpers

### 5.3 Coverage Enforcement
- Coverage MUST be checked in CI; PRs below threshold MUST NOT merge.
- Coverage reports MUST be generated and accessible (e.g., Codecov, Coveralls, or CI artifacts).

---

## 6) CI Integration (MUST)

### 6.1 Pipeline Requirements
CI MUST run the following stages in order:
1. **Lint**: static analysis, formatting checks
2. **Unit tests**: fast, parallelized
3. **Integration tests**: with required services (DB, cache)
4. **Coverage report**: generate and enforce thresholds
5. **E2E tests**: (optional per-PR; required for release branches)

### 6.2 Failure Rules
- Any test failure MUST block the PR from merging.
- Flaky tests MUST be quarantined immediately (moved to a skip list with a tracking issue).
- Do NOT retry-until-green as a workaround for flaky tests.

### 6.3 Performance
- Unit tests MUST complete in under 2 minutes.
- Integration tests MUST complete in under 10 minutes.
- E2E tests MUST complete in under 20 minutes.
- If tests exceed these limits, optimize or parallelize before adding more.

---

## 7) Output Format (when implementing tests)

When producing test code, ALWAYS include:
1) File list + target paths
2) Test code per file
3) Notes:
   - which layer of the pyramid each test covers
   - mocking strategy used
   - any test fixtures or setup required
4) Verification commands (e.g., `go test ./...`, `npm test`)

---

## 8) Definition of Done (Checklist)

- [ ] Unit tests cover all new business logic
- [ ] Integration tests cover new I/O boundaries
- [ ] No test depends on execution order
- [ ] Mocks use interfaces, not reflection/monkey-patching
- [ ] Coverage meets or exceeds thresholds
- [ ] All tests pass in CI
- [ ] No flaky tests introduced
- [ ] Test names describe behavior, not implementation

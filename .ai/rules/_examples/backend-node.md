# (Example Rule Pack) Backend (Node.js/Express) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/backend-node.md`, then add `backend-node` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Node.js Engineer.
Goal: Build production-safe Express APIs with ESM modules, proper error handling, and comprehensive testing.

---

## 0) Tech Stack (assumed)

- Node.js 20+ (LTS), Express 5 (or 4 with async error handling)
- ESM modules (`"type": "module"`), TypeScript (strict mode)
- Prisma, Drizzle, or Knex for DB; Zod for validation
- Vitest or Jest for testing; pino for logging

---

## 1) Project Structure (STRICT)

```
src/
  app.ts                  # Express app factory, middleware registration
  server.ts               # Entry: listen + graceful shutdown
  config.ts               # Typed config from env (zod-validated)
  features/<feature>/
    router.ts             # Express Router (transport ONLY)
    service.ts            # Business logic (usecases)
    repository.ts         # Database access layer
    schemas.ts            # Zod request/response schemas
    errors.ts             # Domain error classes
  middleware/
    error-handler.ts      # Global error-to-HTTP mapping
    auth.ts               # Authentication middleware
    validate.ts           # Zod validation middleware factory
  shared/
    logger.ts             # pino instance
    db.ts                 # Database client singleton
tests/features/<feature>/
  router.test.ts          # Integration (supertest)
  service.test.ts         # Unit (mocked repos)
```

### Placement rules (HARD)
- Routers MUST NOT contain business logic; validate input and call services.
- Services MUST NOT import Express types (`Request`, `Response`).
- Repositories MUST NOT contain business logic.

---

## 2) Module System (ESM STRICT)

- `"type": "module"` in `package.json`. ALL imports use ESM syntax.
- File extensions MUST be included in relative imports.
- Forbidden: `require()`, `module.exports`, `__dirname`, `__filename`.
- Use `import.meta.url` + `fileURLToPath` if path resolution is needed.

---

## 3) Error Handling (MUST)

- Define domain errors in `<feature>/errors.ts` (extend a base `AppError` with `statusCode` + `code`).
- Services MUST throw domain errors, NOT set `res.status()`.
- Global error-handler middleware translates errors to HTTP responses (registered last).
- ALL async handlers MUST have errors caught (`express-async-errors` or try/catch).
- Never expose stack traces in production; log the full error with pino.

---

## 4) Middleware Rules (STRICT)

Registration order (REQUIRED):
1. Request ID (`x-request-id`)
2. Structured logging (pino-http)
3. CORS
4. Body parsing (`express.json()`)
5. Authentication
6. Feature routers
7. 404 handler
8. Global error handler (LAST)

- Middleware MUST call `next()` or send a response; never leave requests hanging.
- Auth middleware MUST attach typed user to `res.locals`.

---

## 5) Validation (MUST)

- ALL request bodies validated with Zod before reaching services.
- ALL path/query params validated (use `z.coerce` for numerics).
- Validation in middleware or router, NOT in services.
- Zod schemas MUST use `.strict()` to reject unknown fields.

---

## 6) Security Rules

- Auth enforced via middleware; never inline in handlers.
- Never accept client-supplied user IDs; derive from JWT/session.
- Secrets from env, validated at startup via Zod. Parameterized SQL only.
- Security headers via `helmet`. Rate limiting on public endpoints.

---

## 7) Graceful Shutdown (REQUIRED)

- `server.ts` MUST handle `SIGTERM` and `SIGINT`.
- Stop accepting connections, finish in-flight requests, close DB pool, exit.
- Force exit after timeout (e.g., 10 seconds).

---

## 8) Testing (REQUIRED)

- Every router: at least one integration test (supertest).
- Every service: at least one unit test with mocked repository.
- Every Zod schema: at least one validation test (valid + invalid).
- Mock DB and external services at repo boundary. No network in tests.

---

## 9) Verification (default)

```bash
npx tsc --noEmit
npx eslint src/ tests/
npx vitest run
npm run build
```

---

## 10) Definition of Done (Checklist)

- [ ] All imports use ESM syntax; no `require()` calls
- [ ] No business logic in routers
- [ ] Services throw domain errors, not HTTP responses
- [ ] All payloads validated with Zod
- [ ] Error handler registered last and logs all errors
- [ ] Graceful shutdown handles SIGTERM/SIGINT
- [ ] Tests exist for routers and services

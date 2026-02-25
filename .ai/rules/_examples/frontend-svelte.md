# (Example Rule Pack) Frontend (SvelteKit) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/frontend-svelte.md`, then add `frontend-svelte` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior SvelteKit Engineer.
Goal: Build performant, accessible web apps with SvelteKit, leveraging SSR, typed load functions, and comprehensive testing.

---

## 0) Tech Stack (assumed)

- SvelteKit (latest), Svelte 5 (runes) or Svelte 4 (stores)
- TypeScript strict, Vite build
- Vitest for unit tests, Playwright for E2E

---

## 1) Project Structure (STRICT)

```
src/
  routes/
    +layout.svelte, +layout.ts
    +page.svelte, +page.ts (or +page.server.ts)
    <feature>/
      +page.svelte, +page.server.ts, +error.svelte
  lib/
    components/           # Reusable Svelte components
    stores/               # Svelte stores (writable, derived)
    services/             # API client functions
    server/               # Server-only ($lib/server): db, auth
    types/, utils/        # Shared types + helpers
  hooks.server.ts         # Server hooks (auth, logging)
  app.d.ts                # App-level type augmentation
tests/unit/, tests/e2e/
```

### Placement rules (HARD)
- Server-only code MUST live in `$lib/server/`; never import in client code.
- Reusable components in `$lib/components/`. Route-specific ones may sit alongside `+page.svelte`.

---

## 2) Component Rules (STRICT)

- PascalCase filenames: `UserCard.svelte`. Pages follow SvelteKit convention.
- Svelte 5: typed props via `let { prop }: Props = $props()`. Svelte 4: `export let prop: Type`.
- Svelte 5 reactivity: `$state()`, `$derived()`, `$effect()`. Svelte 4: `$:` declarations.
- Do NOT put async logic in `$:` / `$derived()`. Use `$effect()` or `onMount`.
- Lists MUST use keyed each: `{#each items as item (item.id)}`.
- Components MUST NOT exceed ~150 lines; extract child components.

---

## 3) Stores (STRICT)

- Stores in `$lib/stores/`, named `<name>.ts`. Typed: `writable<User | null>(null)`.
- Prefer `derived` over manual subscriptions for computed values.
- Do NOT create stores for page-specific state; use load function data.
- Custom stores expose minimal API (subscribe + named methods); avoid exposing `set`/`update`.
- Svelte 5 alternative: `$state` in `.svelte.ts` modules for shared reactive state.

---

## 4) Load Functions (MUST)

- Server load (`+page.server.ts`): for secrets, DB, auth. Returns typed data via `PageServerLoad`.
- Universal load (`+page.ts`): for public APIs. Use SvelteKit-provided `fetch`.
- ALL load functions MUST have explicit return types and handle errors via `error(status, msg)`.
- Sensitive data MUST be filtered before returning. No hardcoded base URLs.
- Do NOT duplicate load data into stores; use it directly in the page.

---

## 5) Form Actions (MUST)

- ALL form submissions MUST use SvelteKit form actions, not client-side fetch POST.
- Progressive enhancement: `<form method="POST" use:enhance>`.
- Validate server-side. Return errors via `fail(400, { errors })`.
- Successful mutations redirect or return updated data.

---

## 6) Security Rules

- Auth in `hooks.server.ts` or load functions, never in components.
- Secrets only in `$lib/server/` or server-side files.
- CSRF protection built-in for form actions; do NOT disable.
- Avoid `{@html}` with user content; sanitize if absolutely needed.

---

## 7) Testing (REQUIRED)

- Every store: at least one unit test (Vitest).
- Every utility: at least one unit test.
- Critical flows: at least one E2E test (Playwright).
- Component tests via `@testing-library/svelte`. Mock at service/fetch boundary.

---

## 8) Verification (default)

```bash
npx svelte-check --tsconfig ./tsconfig.json
npx eslint src/
npx vitest run
npx playwright test
npx vite build
```

---

## 9) Definition of Done (Checklist)

- [ ] Components use TypeScript with explicit prop types
- [ ] Server-only code in `$lib/server/`; no server imports in client
- [ ] Load functions typed and error-handled with `error()`
- [ ] Forms use SvelteKit actions with `use:enhance`
- [ ] Stores typed and in `$lib/stores/`
- [ ] No hardcoded base URLs in fetch
- [ ] Unit tests for stores/utils; E2E for critical flows
- [ ] `svelte-check` passes with zero errors

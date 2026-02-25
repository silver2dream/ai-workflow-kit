# (Example Rule Pack) Frontend (Vue 3) Architecture & Patterns (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/frontend-vue.md`, then add `frontend-vue` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Vue.js + TypeScript Engineer.
Goal: Build reactive, type-safe UIs with Vue 3 Composition API, Pinia state management, and comprehensive testing.

---

## 0) Tech Stack (assumed)

- Vue 3 (`<script setup>` + Composition API), TypeScript strict
- Pinia for state, Vue Router 4, Vite build
- Vitest for testing, Playwright or Cypress for E2E (optional)

---

## 1) Project Structure (STRICT)

```
src/
  app/
    App.vue, main.ts, router.ts
  features/<feature>/
    components/           # Feature-scoped components
    composables/          # Feature-scoped composables (useX)
    stores/               # Pinia stores
    types.ts, index.ts    # Types + barrel export
  shared/
    components/           # Reusable UI components (2+ features)
    composables/          # Shared composables
    utils/, types/        # Helpers + shared types
tests/unit/, tests/e2e/
```

### Placement rules (HARD)
- Feature code MUST live under `features/<feature>/`.
- Shared components must be reusable across 2+ features.
- No `helpers/` or `lib/` folders; use `shared/utils/` or composables.

---

## 2) Composition API Rules (MUST)

- ALL new components MUST use `<script setup lang="ts">`. No Options API in new code.
- No mixins; use composables (`useX`) instead.
- Composables MUST return reactive refs/computed. Clean up side effects via `onUnmounted`.
- Use `ref()` for primitives, `reactive()` for objects. Do NOT destructure reactive objects.
- Prefer `computed()` over `watch()` for derived state.

---

## 3) State Management (Pinia) (STRICT)

- ALL shared state in Pinia stores. No `provide/inject` for app-wide state.
- Prefer Setup Store syntax (function-based with `defineStore`).
- One store per feature (split only if >200 lines). Stores MUST NOT import Vue components.
- State typed explicitly (no `any`). Async actions handle loading/error.
- Do NOT mutate store state from components directly; use actions.

---

## 4) Routing (Vue Router) (MUST)

- Lazy-loaded routes: `component: () => import(...)`.
- Use named routes, not hardcoded paths in `<router-link>`.
- Auth-protected routes use global `beforeEach` guard.
- Route params typed via `RouteMeta` augmentation.

---

## 5) Component Rules (STRICT)

- PascalCase filenames. Pages suffixed `Page`, layouts suffixed `Layout`.
- Props via `defineProps<T>()`, emits via `defineEmits<T>()`, defaults via `withDefaults()`.
- Lists MUST use `:key` with unique ID (never array index).
- Templates MUST NOT exceed ~100 lines; extract child components.
- No complex logic in templates; use `computed` or methods.

---

## 6) TypeScript Rules (STRICT)

- `strict: true` in tsconfig. ALL props, emits, store state, API responses typed.
- No `any`; use `unknown` + type guards. Prefer `interface` for objects, `type` for unions.

---

## 7) Testing (REQUIRED)

- Every Pinia store: at least one unit test.
- Every composable: at least one unit test.
- Every page component: at least one component test.
- Use `@vue/test-utils`, `createTestingPinia()`. Mock API at fetch boundary.
- Use `data-testid` for selectors, not CSS classes.

---

## 8) Verification (default)

```bash
npx vue-tsc --noEmit
npx eslint src/ --ext .ts,.vue
npx vitest run
npx vite build
```

---

## 9) Definition of Done (Checklist)

- [ ] All components use `<script setup lang="ts">`
- [ ] No mixins; reusable logic in composables
- [ ] Shared state in Pinia stores with explicit typing
- [ ] Props/emits defined with TypeScript generics
- [ ] Routes lazy-loaded and named
- [ ] No `any`; all state and responses typed
- [ ] Tests exist for stores, composables, and pages
- [ ] `vue-tsc --noEmit` passes with zero errors

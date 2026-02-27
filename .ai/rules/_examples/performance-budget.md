# (Example Rule Pack) Performance Budget & Core Web Vitals (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/performance-budget.md`, then add `performance-budget` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Performance Engineer.
Goal: Maintain performance budgets and Core Web Vitals targets, preventing regressions through measurable thresholds and CI enforcement.

This document is the source of truth for performance standards. Changes that exceed budgets MUST be justified and approved.

---

## 0) Core Web Vitals Targets (STRICT)

All pages MUST meet these thresholds at the 75th percentile:

| Metric | Target | Maximum |
|--------|--------|---------|
| LCP (Largest Contentful Paint) | < 2.0s | < 2.5s |
| INP (Interaction to Next Paint) | < 150ms | < 200ms |
| CLS (Cumulative Layout Shift) | < 0.05 | < 0.1 |
| TTFB (Time to First Byte) | < 400ms | < 800ms |
| FCP (First Contentful Paint) | < 1.5s | < 1.8s |

Rules:
- Exceeding "Target" requires justification in the PR description.
- Exceeding "Maximum" blocks merge; an exception requires tech lead approval.
- Measure on a mid-tier mobile device over simulated 4G (Lighthouse default).

---

## 1) Bundle Size Budget (MUST)

### 1.1 Thresholds (compressed, gzip)

| Asset | Budget | Hard Limit |
|-------|--------|------------|
| Initial JS bundle | < 150 KB | < 200 KB |
| Initial CSS | < 30 KB | < 50 KB |
| Per-route lazy chunk | < 50 KB | < 80 KB |
| Total page weight | < 500 KB | < 750 KB |

### 1.2 Enforcement
- Bundle size MUST be checked in CI using a size analysis tool (webpack-bundle-analyzer, `size-limit`, Vite rollup-plugin-visualizer).
- PRs that increase any bundle beyond budget MUST include:
  - Reason for the increase
  - Offsetting size reductions (if possible)
  - Approval from a performance-aware reviewer

### 1.3 Rules
- Import only what you use: prefer named imports over default full-library imports.
  - `import { debounce } from 'lodash-es'` (correct)
  - `import _ from 'lodash'` (forbidden)
- Tree-shaking MUST be enabled; verify with bundle analysis.
- Do NOT add dependencies that duplicate existing functionality.

---

## 2) Image Optimization (REQUIRED)

### 2.1 Formats
- Use modern formats: WebP (lossy), AVIF (preferred where supported), with fallback to JPEG/PNG.
- SVG for icons and simple vector graphics.
- Do NOT use uncompressed BMP or TIFF.

### 2.2 Sizing
- Serve responsive images with `srcset` and `sizes` attributes.
- Do NOT serve images larger than their display size (no 4000px images in a 400px container).
- Maximum image file size: 200 KB for hero images, 50 KB for thumbnails.

### 2.3 Loading
- Use `loading="lazy"` for below-the-fold images.
- Use `fetchpriority="high"` for LCP images.
- Include explicit `width` and `height` attributes to prevent CLS.

---

## 3) Lazy Loading & Code Splitting (MUST)

### 3.1 Route-Level Splitting
- Every route MUST be lazy-loaded (dynamic import).
- Use framework-provided lazy loading: `React.lazy()`, `defineAsyncComponent()`, or equivalent.
- Show a lightweight loading skeleton during chunk fetch (no spinner-only or blank screens).

### 3.2 Component-Level Splitting
- Heavy components (charts, editors, maps) MUST be lazy-loaded.
- Third-party integrations (analytics, chat widgets) MUST load asynchronously after initial render.

### 3.3 Preloading
- Preload critical route chunks on hover or when likely navigation is detected.
- Use `<link rel="preload">` for critical fonts and above-the-fold images.

---

## 4) Caching Strategy (STRICT)

### 4.1 Static Assets
- Hashed filenames for JS/CSS/images (e.g., `main.a1b2c3.js`).
- `Cache-Control: public, max-age=31536000, immutable` for hashed assets.
- Do NOT cache HTML documents aggressively; use `Cache-Control: no-cache` or short `max-age`.

### 4.2 API Responses
- GET responses MUST include appropriate `Cache-Control` and `ETag` headers.
- Cacheable responses: list endpoints, static config, public content.
- Non-cacheable: user-specific data, authenticated endpoints (unless `private` + `max-age`).

### 4.3 Service Worker (if applicable)
- Use a stale-while-revalidate strategy for non-critical assets.
- Cache-first for versioned static assets.
- Network-first for API calls and HTML.

---

## 5) Runtime Performance (MUST)

### 5.1 Rendering
- Avoid layout thrashing: batch DOM reads and writes.
- Minimize re-renders: use `React.memo`, `useMemo`, `useCallback` where measurable benefit exists.
- Do NOT use `React.memo` everywhere preemptively; profile first, optimize second.
- Long lists (>100 items): use virtualization (react-window, react-virtual).

### 5.2 JavaScript
- Avoid blocking the main thread for > 50ms (Long Tasks).
- Use `requestIdleCallback` or `setTimeout(fn, 0)` to defer non-critical work.
- Heavy computations MUST run in a Web Worker.

### 5.3 Network
- API calls MUST include timeout configuration (default: 10 seconds).
- Implement request deduplication for concurrent identical requests.
- Use AbortController to cancel abandoned requests (navigating away, component unmount).

---

## 6) Monitoring (REQUIRED)

### 6.1 Real User Monitoring (RUM)
- Collect Core Web Vitals from real users using the `web-vitals` library or equivalent.
- Report to an analytics/monitoring dashboard (e.g., Datadog, New Relic, custom).
- Alert on p75 regression exceeding targets for 24+ hours.

### 6.2 Synthetic Monitoring
- Run Lighthouse CI on every PR; fail if performance score drops below 90.
- Run scheduled Lighthouse audits on production (daily or weekly).
- Track trends over time; investigate any sustained regression.

---

## 7) Output Format (when implementing)

When producing performance-related changes, ALWAYS include:
1) File list + target paths
2) Code changes
3) Notes:
   - bundle size impact (before/after if measurable)
   - Core Web Vitals impact
   - caching implications
   - lazy loading strategy
4) Verification commands (Lighthouse, bundle analyzer, profiler steps)

---

## 8) Definition of Done (Checklist)

- [ ] Core Web Vitals within target thresholds
- [ ] Bundle size within budget
- [ ] Images optimized with responsive sizing and lazy loading
- [ ] Routes are lazy-loaded with code splitting
- [ ] Cache headers set correctly for static and dynamic assets
- [ ] No main thread blocking > 50ms without justification
- [ ] Lighthouse CI score >= 90
- [ ] No new uncompressed or oversized assets

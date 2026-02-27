# (Example Rule Pack) Accessibility & WCAG 2.1 AA Compliance (STRICT)

This is an optional example rule pack for AWK users.
To enable it: copy this file to `.ai/rules/accessibility.md`, then add `accessibility` under `rules.custom` in `.ai/config/workflow.yaml`.

Role: Senior Accessibility Engineer.
Goal: Ensure WCAG 2.1 AA compliance across all user-facing interfaces, providing an inclusive experience for users with disabilities.

This document is the source of truth for accessibility standards. Accessibility violations are P1 and block merge for user-facing changes.

---

## 0) Perceivable (STRICT)

### 0.1 Text Alternatives
- Every non-decorative `<img>` MUST have a descriptive `alt` attribute.
- Decorative images MUST use `alt=""` (empty alt) or be applied via CSS `background-image`.
- Complex images (charts, diagrams) MUST have extended descriptions via `aria-describedby` or adjacent text.
- Icon buttons MUST have `aria-label` or visually hidden text.

### 0.2 Time-Based Media
- Video content MUST have captions (closed or open).
- Audio content MUST have transcripts.
- Auto-playing media MUST be pausable and have controls.

### 0.3 Adaptable Content
- Use semantic HTML elements: `<header>`, `<nav>`, `<main>`, `<section>`, `<article>`, `<footer>`.
- Do NOT use `<div>` or `<span>` for interactive elements; use `<button>`, `<a>`, `<input>`.
- Reading order in the DOM MUST match visual order.
- Do NOT use CSS to reorder content in ways that break logical reading flow.

---

## 1) Operable (MUST)

### 1.1 Keyboard Accessibility
- ALL interactive elements MUST be reachable and operable via keyboard.
- Tab order MUST follow logical reading order.
- Do NOT use `tabindex` values > 0 (only `0` or `-1`).
- Custom interactive components MUST handle `Enter`, `Space`, `Escape`, and arrow keys as appropriate.

### 1.2 Focus Management
- Focus MUST be visible on all interactive elements (no `outline: none` without a replacement).
- Custom focus styles MUST have a contrast ratio of at least 3:1 against the background.
- When opening modals/dialogs: move focus into the dialog and trap it until dismissed.
- When closing modals: return focus to the trigger element.

### 1.3 Timing
- Do NOT impose time limits on user interactions without providing extension options.
- Auto-rotating carousels MUST have pause/stop controls.
- Animations MUST respect `prefers-reduced-motion` media query.

### 1.4 Navigation
- Provide skip links ("Skip to main content") at the top of every page.
- Page titles MUST be unique and descriptive.
- Breadcrumbs and consistent navigation MUST be present on multi-page applications.

---

## 2) Understandable (MUST)

### 2.1 Language
- Set the `lang` attribute on `<html>` (e.g., `lang="en"`).
- Sections in a different language MUST have their own `lang` attribute.

### 2.2 Predictable Behavior
- Interactive elements MUST behave consistently across the application.
- Form submissions MUST NOT trigger on focus or input change alone (no auto-submit without user action).
- Opening a new window/tab MUST be indicated (e.g., external link icon + `aria-label` mention).

### 2.3 Input Assistance
- Form fields MUST have associated `<label>` elements (via `for`/`id` or wrapping).
- Required fields MUST be indicated (visually AND via `aria-required="true"`).
- Validation errors MUST be announced to screen readers (use `aria-live="polite"` or `role="alert"`).
- Error messages MUST identify the field and describe how to fix the error.

---

## 3) Robust (STRICT)

### 3.1 Valid HTML
- HTML MUST be well-formed and pass validation.
- Do NOT use duplicate `id` attributes on a page.
- Custom components MUST use appropriate ARIA roles and states.

### 3.2 ARIA Guidelines
- **First rule of ARIA**: Do NOT use ARIA if a native HTML element provides the semantics.
  - Use `<button>` instead of `<div role="button">`.
  - Use `<nav>` instead of `<div role="navigation">`.
- Every ARIA role MUST include all required attributes:
  - `role="checkbox"` requires `aria-checked`
  - `role="tab"` requires `aria-selected` and association with `role="tabpanel"`
- `aria-hidden="true"` MUST NOT be set on focusable elements.
- Dynamic content updates MUST use `aria-live` regions appropriately:
  - `polite` for non-urgent updates (search results, status messages)
  - `assertive` for urgent updates (error alerts, time-critical notifications)

---

## 4) Color & Contrast (STRICT)

### 4.1 Contrast Ratios
- Normal text (< 18pt / 14pt bold): minimum 4.5:1 contrast ratio.
- Large text (>= 18pt / 14pt bold): minimum 3:1 contrast ratio.
- UI components and graphical objects: minimum 3:1 contrast ratio.

### 4.2 Color Independence
- Do NOT use color as the sole means of conveying information.
  - Error states: use color + icon + text (not just red border).
  - Charts: use patterns/shapes in addition to colors.
  - Links within text: use underline or other non-color indicator.

### 4.3 Dark Mode
- If the application supports dark mode, contrast requirements apply to BOTH themes.
- Test both themes with accessibility tools before release.

---

## 5) Forms & Interactive Patterns (MUST)

### 5.1 Form Design
- Group related fields with `<fieldset>` and `<legend>`.
- Use `autocomplete` attributes for common fields (name, email, address, credit card).
- Provide visible instructions before the form, not only inside placeholders.
- Do NOT use placeholder text as the only label.

### 5.2 Error Handling
- Display errors inline next to the relevant field.
- Provide an error summary at the top of the form with links to each field.
- Move focus to the first error on submission.

### 5.3 Custom Components
- Dropdowns, date pickers, and sliders MUST follow WAI-ARIA Authoring Practices.
- Test custom components with at least two screen readers (NVDA + VoiceOver or JAWS).

---

## 6) Testing Tools & Process (REQUIRED)

### 6.1 Automated Testing (CI)
- Run axe-core or similar automated accessibility scanner on every PR.
- Fail the build on any Critical or Serious violations.
- Automated tools catch ~30-40% of issues; manual testing is still REQUIRED.

### 6.2 Manual Testing (per feature)
- Tab through the entire feature using keyboard only.
- Test with a screen reader (NVDA on Windows, VoiceOver on macOS).
- Verify with browser zoom at 200% (content must not break or overlap).
- Use forced-colors mode (Windows High Contrast) to check visibility.

### 6.3 Color Contrast Verification
- Use a contrast checker tool (WebAIM Contrast Checker, Chrome DevTools) for all new color values.
- Document contrast ratios in design specifications.

---

## 7) Output Format (when implementing)

When producing UI changes, ALWAYS include:
1) File list + target paths
2) Code with accessibility attributes included
3) Notes:
   - ARIA roles and attributes used
   - keyboard interaction model
   - screen reader announcements for dynamic content
   - contrast ratios for new colors
4) Verification steps (axe scan, keyboard test, screen reader test)

---

## 8) Definition of Done (Checklist)

- [ ] All images have appropriate alt text
- [ ] Semantic HTML used (no div-button, no span-link)
- [ ] All interactive elements keyboard-accessible
- [ ] Focus visible and managed correctly (modals, dynamic content)
- [ ] Color contrast meets WCAG AA ratios
- [ ] Color not used as sole indicator
- [ ] Form fields labeled and errors announced to screen readers
- [ ] ARIA used correctly (no ARIA where native HTML suffices)
- [ ] Automated accessibility scan passes in CI
- [ ] Manual keyboard and screen reader test performed

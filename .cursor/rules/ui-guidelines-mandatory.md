---
description: Enforce UI design guidelines when implementing or changing frontend pages and components
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# UI Guidelines Are Mandatory

When adding or editing **pages, layouts, or shared UI components**, follow the UI rules in `.cursor/rules/ui-*.md` and `docs/patterns/frontend-guide.md`. Do not ship UI without applying these patterns.

## Before You Ship UI Changes

1. **Forms**: every text field has `autocomplete`; use `variant="outlined"`, `density="comfortable"`, `hide-details="auto"`, `class="mb-4"`. Cap numbers / durations / selects with `max-width` — full width only for freeform text (see `ui-form-inputs.md`).
2. **Cards / dialogs**: page-level cards → `StandardCard`; modals → `StandardDialog` (never raw `v-dialog` + `v-card`). Submit/Save in `#actions` as **elevated** primary; Cancel **`color="primary" variant="tonal"`**.
3. **Page shell**: interior pages use `v-container.page-content` + `header.page-content__intro`; compact breadcrumbs when needed. Hub tabs → flat panel (no card under tabs).
4. **Spacing**: Vuetify utilities first; `mb-*` not `mt-*` for section gaps; control clusters use `style="gap: 8px"` (not `ga-*`).
5. **Shell**: Material Blue app bar only (no drawer) — see `runner-ui-deltas.mdc`; square corners on shell controls.
6. **No** `alert()` / `confirm()` / `prompt()`.

## Quick Checklist

- [ ] Inputs have autocomplete + standard props
- [ ] Short values (numbers, durations, selects) use `max-width` — not full-bleed `flex-grow-1` alone
- [ ] StandardCard / StandardDialog used correctly
- [ ] `#actions` has elevated primary + tonal primary Cancel (not text/outline)
- [ ] Hub tabs use flat panel — not a full StandardCard under the strip
- [ ] Page shell classes (not heavy `py-8`)
- [ ] MD3 typography classes (Vuetify 4)

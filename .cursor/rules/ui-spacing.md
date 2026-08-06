---
description: Spacing utilities and gap conventions
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# Spacing

**Reference**: `docs/patterns/frontend-guide.md`

- Prefer Vuetify spacing classes (`pa-*`, `mb-*`, …) over custom theme padding.
- Create vertical gaps with `mb-*` on the upper element — avoid `mt-*` for section rhythm.
- Control clusters: `d-flex align-center flex-wrap` + `style="gap: 8px"`.
- Do **not** rely on `ga-*` for critical layout.
- Card/dialog bodies: use `.standard-card-body` (full `padding` shorthand in theme) — not split `pt-*/px-*` defaults.
- Nested card copy: theme resets `margin-block-start` only; keep bottom gaps on `mb-*`.
- Do **not** add theme.scss rules that only restate Vuetify spacing.
- Reserve `theme.scss` for brand tokens and structural chrome (page shell, cards, list-details, workspace tabs, sections).

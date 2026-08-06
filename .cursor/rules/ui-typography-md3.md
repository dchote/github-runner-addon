---
description: MD3 typography classes for Vuetify 4
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# Typography (MD3)

**Reference**: `docs/patterns/frontend-guide.md`

- Vuetify 4: use MD3 classes only — never `text-h1`…`text-h6`, `text-body-1`, `text-caption`.
- Interior page titles: `text-headline-small font-weight-bold`.
- Body: `text-body-large` / `text-body-medium` / `text-body-small`.
- **StandardCard / StandardDialog titles**: `text-title-medium font-weight-bold`.
- Larger emphasis (metrics, empty-state headings): `text-title-large`.
- Marketing: `.text-display`, `.text-headline`, `.eyebrow` (tight line-height), etc.
- Fonts: Inter (body), Plus Jakarta Sans (headings).
- Muted copy: `.brand-text-muted` → `var(--brand-muted)`.

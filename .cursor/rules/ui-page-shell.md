---
description: Interior page shell classes and breadcrumbs
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# Page Shell

**Reference**: `docs/patterns/frontend-guide.md`

- Interior `v-container`: class `page-content` (not `py-8`).
- Title block: `<header class="page-content__intro">` with `text-headline-small font-weight-bold mb-1` and muted subtitle `mb-0`.
- Intro → first card: `.page-content__intro` margin-bottom **12px / 16px** (≥960px).
- Breadcrumbs: `density="compact"` + `page-content-breadcrumbs px-0`, above intro.
- Sub-pages: breadcrumbs — **not** back buttons.
- Workspace width: `page-content--workspace` (960px) when appropriate.
- Marketing heroes: do **not** use `page-content`.

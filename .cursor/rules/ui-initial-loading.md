---
description: initialLoading vs loading and PageLoader
globs: github_runner/frontend/src/**/*.{vue,js}
alwaysApply: false
---

# Initial Loading

**Reference**: `docs/patterns/frontend-guide.md`

- `initialLoading`: first hydrate — card `:loading`, disable fields, PageLoader for panes.
- `loading`: subsequent updates — keep content visible; button `:loading`.
- Empty states only when `!initialLoading && items.length === 0`.
- Do not flash empty editable forms that suddenly populate.

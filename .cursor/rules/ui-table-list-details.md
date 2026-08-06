---
description: List + details pane responsive pattern
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# Table / List + Details

**Reference**: `docs/patterns/frontend-guide.md`

- **One** `StandardCard` with `content-class="pa-0"`; list + details inside (not two cards).
- Use `useListDetailsPane` for responsive behavior.
- Desktop: list `md="8"`, details `md="4"` when selected; detail chrome via `standard-card-header` (not a nested card).
- Detail actions: `.standard-card-actions` footer; primary buttons **default size** (not `size="small"`).
- Mobile: details in fullscreen `StandardDialog`.
- Tables that select rows: `hover` + `@click:row` toggle; `:row-props` → `selected-row`.
- Admin toolbars: `style="gap: 8px; width: 100%"`; search `min-width: 220px`.

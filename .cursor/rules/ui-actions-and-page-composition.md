---
description: Elevated primary actions in footers; no card-under-tabs composition
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# Actions & Page Composition

**Reference**: `docs/patterns/frontend-guide.md`

## Action footers (`#actions` / dialog actions)

- **Always** include a primary: `color="primary" variant="elevated"` (or `color="error" variant="elevated"` for destructive).
- Never make Save/Confirm/Continue `variant="text"`, `plain`, or flat in the footer.
- Secondary Cancel/Close: `color="primary" variant="tonal"` — filled color, not outlined, not text-only.
- Order: secondary first, primary last; `style="gap: 8px"` on the actions row.
- `variant="text"` is OK for header icon buttons (close, overflow menu) only.

## Page composition

- Hub tabs (account, admin, settings): page intro → `.admin-workspace-tabs` → flat `.admin-workspace-panel`.
- **Do not** place a full-width `StandardCard` under page-level tabs for the whole tab body (card-under-toolbar anti-pattern).
- OK: tabs inside one `StandardCard` via `#tabs` when the card *is* the workspace.
- Nested `StandardCard` only for dense sub-blocks inside a flat panel.

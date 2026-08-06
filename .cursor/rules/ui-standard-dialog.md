---
description: StandardDialog required for all modals
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# StandardDialog

**Reference**: `docs/patterns/frontend-guide.md`

- **ALWAYS** use `StandardDialog` — never raw `v-dialog` + `v-card`.
- **ALWAYS** pass `:fullscreen="dialogFullscreen"` from `useMobile()`.
- Title: `text-title-medium font-weight-bold`; header padding from theme (`12px 16px 8px`).
- Body default: `contentPadding="standard-card-body"` (same inset as StandardCard).
- Do **not** zero `padding-right` for the close button.
- Actions: Cancel (`color="primary" variant="tonal"`) then primary elevated; destructive uses `color="error" variant="elevated"`. Never text/outline primary or outline Cancel.
- Use `persistent` for unsaved forms / critical flows.
- Split pickers: `.dialog-content-split` / `.dialog-fixed-section` / `.dialog-scroll-section`.

---
description: StandardCard usage for page-level cards
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# StandardCard

**Reference**: `docs/patterns/frontend-guide.md`

- Page-level / parent cards → `StandardCard`.
- Slot order: **header → tabs → toolbar → alerts → body → actions**.
- Title default: `text-title-medium font-weight-bold`; header padding `12px 16px 8px`.
- `#tabs`: `v-tabs` with `density="compact" class="px-2 mt-2"` and a trailing `v-divider`.
- `titleAppend`: Add / Invite — class `standard-card-header-action` (not cramped `size="small"`).
- `toolbar`: search / filters (or page-level actions when tabs live in the body, e.g. App detail).
- `actions`: Save / Submit elevated primary + tonal primary Cancel (right-aligned; default button size; no `v-spacer`).
- Body: `.standard-card-body` by default (`12px 16px 16px`; `20px` top when first child is an input).
- Nested cards: raw `v-card` + `.card-nested` — **never** `variant="outlined"`.
- Card body copy: theme zeros `margin-block-start` on nested `p`/headings — use `mb-*` for bottom gaps (do not fight that with theme `margin-block-end: 0`).

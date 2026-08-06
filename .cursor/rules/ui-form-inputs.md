---
description: Form input and filter styling consistency
globs: github_runner/frontend/src/**/*.vue
alwaysApply: false
---

# Form Inputs

**Reference**: `docs/patterns/frontend-guide.md`

## Mandatory props

| Prop | Value |
|------|-------|
| `variant` | `"outlined"` |
| `density` | `"comfortable"` (compact only for OTP / toolbar search) |
| `hide-details` | `"auto"` (boolean for filters) |
| `autocomplete` | `"off"` or semantic token — never omit |
| `class` | `mb-4` for stacked fields |

## Layout

- Gaps with `mb-*` on the upper element — not `mt-*`.
- Section labels: `text-title-small mb-2` (or `mb-3` when more air is needed).
- Do **not** use `v-row`/`v-col` for form fields — use `d-flex flex-column flex-sm-row` with `style="column-gap: 16px"`.
- Do **not** rely on Vuetify `ga-*` for form layout.
- Submit/Save inside StandardCard goes in `#actions`.

## Field width

Default is **capped to content**, not full card width. Stretching short values across a settings card looks sparse and amateur.

| Kind | Width |
|------|-------|
| Numbers, ports, short durations (`15m`, `5s`), tight enums | `max-width: 12rem` |
| Medium selects / duration fields with longer labels | `max-width: 16rem`–`20rem` (or `280px`) |
| Freeform text (names, emails, domains, paths, PEM, search, multi-word strings) | full row / `flex-grow-1` OK |
| Side-by-side freeform pair | `flex-grow-1` each |

```vue
<!-- ✅ short value -->
<v-text-field
  v-model="idleTimeout"
  label="Idle timeout"
  placeholder="15m"
  class="mb-4"
  style="max-width: 20rem"
  ...
/>

<!-- ✅ freeform -->
<v-text-field
  v-model="domain"
  label="Domain"
  class="mb-4 flex-grow-1"
  ...
/>
```

- Do **not** use `flex-grow-1` alone on a lone number / duration / select — it fills the card.
- Do **not** use `min-width` to “size” short fields; that still lets them grow. Prefer `max-width`.
- Keep labels short enough for the cap; put examples in `placeholder` / `hint`, not in the label.

## Button groups

- Joined groups: `v-btn-toggle` / `v-btn-group` + `density="compact"` + `divided` + class **`btn-group-segmented`**.
- Outer corners only (`8px` on the group); adjoining button edges stay square.
- Separate buttons: `style="gap: 8px"`, not a btn-group.

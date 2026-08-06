# Frontend Guide

Vue 3 + Vuetify 4 (MD3) operator UI for the GitHub Runner Manager control plane. Follow `.cursor/rules/ui-*.md`, `runner-ui-deltas.mdc`, and the 8wi design guide page shell / spacing.

Product context and runner-image attribution: [repository README](../../README.md).

## Shell

Material Blue app bar only (no drawer / site-nav) — palette from go-mumble-server MD3 (`primary: #1976D2`). Body: `v-main.pa-0` → `v-container.page-content` at **max-width 1100px** (wide table).

- Intro: `<header class="page-content__intro">` with `text-headline-small font-weight-bold mb-1` + muted subtitle `mb-0`
- Intro → card spacing comes from `.page-content__intro` (12px / 16px) — do not add extra `mb-4` / `mb-8` on the intro
- Sub-pages: compact breadcrumbs above intro
- Cards: flat blue top border + blue gradient headers (mumble chrome)
## Forms

- `variant="outlined"` `density="comfortable"` `hide-details="auto"`
- `autocomplete="off"` except login
- Actions: elevated primary; Cancel `color="primary" variant="tonal"`

## Tables

List + details in one `StandardCard` (`content-class="pa-0"`). Create CTA in `#titleAppend`.

## Logs

`RunnerLogsDialog` from the runner actions (not a separate route). Dark `.log-viewport` panel; ANSI via `ansi_up`; Follow streams via WebSocket `/ws` (`container_logs`). Follow off loads a REST log snapshot. Download + auto-reconnect with backoff on disconnect.

## Settings

Read-only settings dialog from the app bar: labeled value rows (version, image, PAT chip, docker-sock chip), supporting copy under each row, then compact `border="start"` tonal alerts for security notes. Use `useMobile()` fullscreen. Edits stay in HA options / env.

## Ingress

Resolve API and docs URLs with `resolveURL` / `resolveWSURL` from `@/utils/api` so HA ingress base paths work.

## Health-driven UI

`/api/v1/health` drives the Docker chip, PAT-aware create/edit, settings fields, store-readable errors, and orphan warnings. Status summary chips are derived from the runners list (same `statusColor` helper) to avoid a second inspect round-trip.

## Edit

`EditRunnerDialog` PATCHes labels/image/limits/env/network/docker-sock; Save & apply sets `apply: true` (recreate). Shared fields live in `RunnerConfigFields`.

---
description: Operator overview — SummaryStrip, service health, sparklines, tiles, pies
globs: github_runner/frontend/src/**/*{Overview,Home,Dashboard,Network,Cloud}*.vue
alwaysApply: false
---

# Overview Dashboard

**Reference**: `docs/patterns/frontend-guide.md`

- Info bars under card headers: `SummaryStrip` (`.summary-strip` / `--summary-strip-bg`) — not a nested card.
- Service health: `ServiceHealthPanel` (prop-driven rows) rather than ad-hoc icon stacks.
- Sparklines: `InstrumentationTrendCard` → `GenericGraph` (Chart.js); health colors via `instrumentationTrendHealth.js`.
- Hardware / detail blocks: `OverviewTileCard` (bordered tile), not nested `StandardCard`.
- Storage: `StoragePieChart` + `formatBytes` / `chartPalette`.
- Prefer `style="gap: …"` over `ga-*` for flex clusters in this repo’s showcase.

---
description: Primary site-nav chrome — uppercase links, gold underline, overflow scroller
globs: github_runner/frontend/src/**/{Navigation,layouts/**,*Layout*}.vue
alwaysApply: false
---

# Primary Navigation

**Reference**: `docs/patterns/frontend-guide.md`

- Use one shared `Navigation` with an `items` prop (`{ path, label }[]` in the design-guide reference).
- App bar: `v-app-bar.site-nav`, brand navy, height ~72px. Keep navy in dark mode — do **not** bind the bar to dark `primary`.
- Desktop links: uppercase `router-link.site-nav__link` — **not** rounded `v-btn` pills (`border-radius: 0`).
- Active: gold inset underline (`box-shadow: inset 0 -2px 0 0 var(--brand-gold)`).
- No border-radius on nav controls (hamburger, theme icon, drawer list items) or tab bars (`.admin-workspace-tab`, in-card `v-tabs` / `v-tab`).
- Overflow: horizontal scroller + chevron arrows when links exceed available width; default bar inline padding `24px` / `16px` mobile.
- Icon controls (theme, menu): set `aria-label`.
- Product apps may add login / user menus; keep the same link chrome.

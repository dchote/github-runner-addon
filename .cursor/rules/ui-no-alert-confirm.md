---
description: Ban native alert/confirm/prompt
globs: github_runner/frontend/src/**/*.{vue,js,ts}
alwaysApply: false
---

# No Native Dialogs

Never use `alert()`, `confirm()`, or `prompt()`.

Use `StandardDialog`, `v-alert`, or snackbars/toasts instead.

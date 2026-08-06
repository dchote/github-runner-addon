# Changelog

## 0.2.0

- Optional GitHub PAT (`github_pat` / `GITHUB_PAT`) to mint registration tokens and deregister runners on delete
- Recreate from stored config (token/PAT required when data volume missing); PATCH labels and runtime limits
- Per-runner image override, CPU/memory limits, extra env, network mode, docker-sock override; scrub `RUNNER_TOKEN` after confirmed registration
- Startup/periodic reconcile with orphan reporting; enriched health (counts, store, PAT configured, version)
- Rate limits on create/recreate; sanitized client errors; compose binds `127.0.0.1:8099`
- UI: status summary, edit/apply, PAT-aware create, recreate, orphan warnings, settings (read-only), logs download + auto-reconnect
- Vendored Swagger UI (offline `/docs`); OpenAPI 0.2.0
- Documentation: product/README rewrite with clear purpose and attribution for myoung34/github-runner

## 0.1.1

- Fix startup: run under s6 `services.d` with `with-contenv` and read options from `/data/options.json` (no Supervisor API call at boot)

## 0.1.0

- Initial release: create / list / lifecycle / logs for Docker-based GitHub runners
- Home Assistant ingress UI (Vue + Vuetify)
- Persistent state in `/data/runners.json` (standard HA app data directory)
- Optional GHCR image pull; local Supervisor builds when `image:` is commented out
- WebSocket live logs with ingress-aware origin checks

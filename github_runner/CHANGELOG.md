# Changelog

## 0.3.2

- Fix reported `version=` in logs/health: bake `BUILD_VERSION` into image `APP_VERSION` and stop the s6 run script from forcing a stale `0.2.0` fallback

## 0.3.1

- **Automatic sibling-Docker workdir:** each runner gets `gha-runner-<name>-work`; manager same-path binds the volume Mountpoint as `RUNNER_WORKDIR` (no manual host path). Recreate remounts it and keeps registration
- Container stop grace is **120s** for all managed runners (recreate uses the same)
- UI/details show work volume and resolved Mountpoint (or an error if Inspect fails)
- Docs: [0004](../docs/features/0004-sibling-docker-workdir-host-bind.md); upgrade note — recreate existing runners after upgrade

## 0.3.0

- Persistent runner cache: named Docker volume or host bind (default `/cache`), optional shared volumes across runners
- Optional persist job workdir (`/work` named volume) and read-only cache mount
- Longer stop grace (120s) when cache/workdir persistence is enabled; recreate uses the same grace
- Apply-time cleanup of stale unreferenced work/cache volumes; shared cache delete uses store refcount
- UI Advanced fields for cache/workdir; details show volume names and read-only state
- Docs: [0003-persistent-runner-cache](../docs/features/0003-persistent-runner-cache.md), product/tech/runtime/DOCS updates
- Schema version 3 for `runners.json`

## 0.2.1

- First tagged GitHub release
- README screenshots reordered to lead with the empty fleet state
- Version bump across config, OpenAPI, and default `APP_VERSION`

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

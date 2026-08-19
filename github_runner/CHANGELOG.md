# Changelog

## 0.6.0

- **Reset-resilient recreate:** configure phase no longer uses `DEBUG_ONLY` (upstream skips `config.sh` when that flag is set and `.runner` is missing). Configure uses `RUNNER_TOKEN` + CMD `true` + `restart=no`, then a tokenless listener. Recreate works after a Docker wipe with a token/PAT — see [0007](../docs/features/0007-reset-resilient-recreate.md)
- Do not 409 `RUNNER_BUSY` when the managed container is missing or exited (leftover host `status.json` after a Docker reset); inspect failures fail closed as **503**; a **running** container with missing/unreadable/unknown `status.json` is **409**
- Create rollback keeps the store row and data volume when `.runner` or a managed container exists; recreate restores `.runner` from backup if start fails; Save & apply persists only after a successful recreate
- Create/recreate rate-limit quota is consumed only after validation (400/404 do not count); extra_env values, CPU/memory caps, and `network_mode` are validated
- Operator **Recreate missing** (`POST /api/v1/runners/recreate-missing`) restores a Docker-reset fleet; Start/restart/logs on a missing container return **400**
- Listen wait timeout while the container is still running fails create/recreate (never 201); leftover configure containers with `RUNNER_TOKEN` are not adopted as the listener
- `GET /runners/{id}` reads `.runner` via `CopyFromContainer` while running and skips volume helpers when missing; configure wait does not alpine-poll on file-not-found; `ReadVolumeFile` does not auto-create missing volumes and shares the helper semaphore
- List after a successful `ListManaged` marks absent names `missing` without `InspectByName`
- Clear error when image is missing (`:local` tags are not pulled, **400**); registry pull failures are **502 `IMAGE_PULL_ERROR`**
- Health `status` is `ok` or `degraded` (HTTP stays 200); empty HA `github_pat` unsets inherited `GITHUB_PAT`; host binaries bind `127.0.0.1` unless `LISTEN_ADDR` / Docker / Supervisor
- WebSocket Origin no longer trusts `X-Ingress-Path` alone; log lines and errors are token-redacted
- UI: list poll no longer clears recreate errors; failed mutations refresh the list; Recreate / Recreate missing / Edit apply always offer a token field and show the API error

## 0.5.3

- **Same-path build cache:** bind mounts always use `host_path` as the container target; inject `RUNNER_CACHE` / expose `cache_effective` for workflows and Buildx `type=local` (see [0006](../docs/features/0006-same-path-build-cache.md))
- Remove bind `host_path` ≠ `target` mismatch warning (coerced instead); named-volume sibling advisory retained
- UI: Persistent cache defaults to **host path (bind)**; single host-path field; prefer `$RUNNER_CACHE` in help copy
- `EnsureHostDir` binds the path’s top-level directory so any absolute host path works (USB, SSD, custom mounts) — no allowlist
- **Concurrent API scalability:** parallel List enrich (`errgroup`), batched `ListManaged` (skip N inspects), inspect singleflight + generation-gated TTL cache, capped helper concurrency, per-runner lifecycle locks (create holds id lock through start; prune idle `create:` keys), cheap Health `StatusCounts`, List job-status without host helpers, timeout-safe response writer with Unwrap (no superfluous WriteHeader after 504)
- API rejects recreate / delete / patch-apply while `job_state=busy` (**409 `RUNNER_BUSY`**) so Save & apply cannot kill in-flight builds
- Stop/restart remain available for emergency intervention; UI busy gating unchanged

## 0.5.2

- Soft advisories on runner API/UI as `warnings[]` (create/patch still succeed):
  - cache **bind** with `host_path` ≠ `target` (sibling Docker / Buildx `type=local` miss)
  - cache **named volume** (never visible to sibling host binds of the target path)
- Docs/UI: same-path cache bind recipe (prefer `host_path` = mount path, commonly both `/cache`)
- UI: selecting host-path cache prefills Host path from the mount path when empty; details use warning alerts

## 0.5.1

- Register runners with a configure-only (`DEBUG_ONLY`) container, then start the long-running listener without `RUNNER_TOKEN` — avoids killing a live GitHub session to scrub the token (fixes “session already exists” races)
- Recreate reuses volume credentials without minting a fresh token unless workdir/volume reconfigure is required
- UI: only the action that was clicked shows a loading spinner; other controls are disabled while it runs
- UI: disable Edit / Delete / Recreate while a runner is busy (`job_state=busy`); start/stop/restart/logs stay available (UI-only; API unchanged)

## 0.5.0

- Show runner **idle/busy** job activity (and current job fields while busy) from local Actions job hooks — no PAT required; see [0005](../docs/features/0005-runner-job-state.md)
- Status column shows idle/busy/unknown when the container is running; details pane lists current job metadata
- After upgrade, **Recreate** existing runners so `ACTIONS_RUNNER_HOOK_JOB_*` env is applied

## 0.4.4

- Harden Docker lifecycle against client/ingress cancel: detached create/recreate/scrub contexts; fail closed on workdir verify (return error, keep runner for recovery); do not clear `.runner` on transient read cancel
- List skips live `.runner` reads (cache-only); Get/details fetch live diagnostics; short-lived volume helper; longer PATCH/stop/delete API timeouts
- Docs: workdir diagnostics List vs Get; consuming-repo Actions `RUNNER_TOKEN` note

## 0.4.3

- Fix `.runner` workFolder reads: use `CopyFromContainer` instead of `cat` via container logs (avoids multiplex/BOM corruption that blocked create verify and hung the UI), and strip UTF-8 BOM before JSON parse

## 0.4.2

- Harden sibling-Docker workdir pipeline: stop container before clearing `.runner`; assert agent `workFolder` after start; narrow host `mkdir` bind roots; typed `ErrVolumeFileNotFound`; workdirHost test seam + pipeline unit tests; remove unused VolumeMountpoint; UI default path uses normalized runner name

## 0.4.1

- Create missing workdir (and cache bind) host directories before bind-mount so runner create no longer fails when `/srv/gha-work/<name>` does not exist yet

## 0.4.0

- **Fix sibling-Docker workdir:** use a real host directory same-path bind (default `/srv/gha-work/<name>`, optional `workdir_host_path`) instead of Docker volume Mountpoints
- Recreate/apply clears `.runner` and reconfigures when agent `workFolder` ≠ host bind (token/PAT required) — `RUNNER_WORKDIR` env alone is not enough
- UI/API diagnostics: `workdir_effective`, `workdir_agent`, `workdir_mismatch`; reject `/var/lib/docker/…` workdir paths
- Schema version 4; docs [0004](../docs/features/0004-sibling-docker-workdir-host-bind.md) updated

## 0.3.3

- Add `./scripts/bump-version.sh` and a Cursor release rule so tag/release bumps keep `config.yaml`, `DefaultVersion`, OpenAPI, Dockerfile `BUILD_VERSION`, and CHANGELOG in sync
- Docs: version bump workflow in build-and-test; stop pinning `APP_VERSION` examples to a literal release

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

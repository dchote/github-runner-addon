# Technical Overview

## Stack

| Area | Direction |
|------|-----------|
| Backend | Go, Chi router, single binary |
| Frontend | Vue 3 + Vuetify 4 (MD3) + Vite, **embedded** via `//go:embed` |
| API | REST `/api/v1` + OpenAPI at `/docs` (vendored Swagger UI) |
| Live logs | WebSocket `/ws` (`container_logs`) and REST follow |
| Runtime | Docker Engine via socket (`DOCKER_HOST` or default) |
| Runner image | Default [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner) (override via `RUNNER_IMAGE`) |
| GitHub | Optional PAT → registration-token + runner delete APIs (github.com + GHES) |
| Config | `DATA_DIR` / `/data` → `runners.json` (schema versioned) |
| Packaging | Home Assistant app in `github_runner/` (local build or GHCR pull) |
| Auth | Network trust only (HA ingress or private bind); no app login |

Architecture: Go Chi API with binary-embedded Vue SPA, HA app packaging (self-contained build context), JSON-backed runner fleet, Docker Engine for container lifecycle, optional GitHub API for token mint and deregister. Runner **agents** run inside the upstream container image — this codebase is the control plane only.

## Layout

```text
repository.yaml
github_runner/                 # HA app directory (= Docker build context)
  config.yaml                  # image: → GHCR pull; omit → local Supervisor build
  Dockerfile                   # multi-stage Node + Go + home-assistant/base
  rootfs/                      # s6 service (options.json → env)
  cmd/github-runner-addon/     # main, embed stubs, frontend-dist/
  internal/                    # rest, runner, store, github, container/docker
  api/                         # openapi.yaml + embed
  frontend/                    # Vue SPA source
docs/
.github/workflows/             # official HA BuildKit → GHCR
```

## Embed pipeline

```text
npm ci && npm run build  →  github_runner/frontend/dist/
  → sync → github_runner/cmd/github-runner-addon/frontend-dist/
  → go build -tags embed_frontend
```

Without the tag, `getFrontendFS()` returns nil (API-only / Vite proxy).

## HA packaging modes

1. **GHCR (default):** `image: ghcr.io/dchote/github-runner-addon` in `config.yaml` — Supervisor pulls the multi-arch manifest.
2. **Local build:** Comment out `image:` — Supervisor builds `github_runner/Dockerfile` on the host.

## Runner model

Creating a runner:

1. Validate name, URL; resolve registration token (request body or mint via PAT).
2. Derive `RUNNER_SCOPE` (`repo` vs `org`) from URL path segments.
3. Persist expected config (token not in `runners.json`).
4. Create named volume + containers from configured runner image (default **`myoung34/github-runner`**) with env understood by that image’s entrypoint:
   `REPO_URL` / `ORG_NAME`, `RUNNER_NAME`, `RUNNER_TOKEN` (configure phase only), `LABELS`,
   `CONFIGURED_ACTIONS_RUNNER_FILES_DIR`, `RUNNER_SCOPE`,
   `RUNNER_WORKDIR` (same-path host bind), `RUNNER_CACHE` (when cache enabled),
   `DISABLE_AUTO_UPDATE`, `DISABLE_AUTOMATIC_DEREGISTRATION`,
   `ACTIONS_RUNNER_HOOK_JOB_STARTED`, `ACTIONS_RUNNER_HOOK_JOB_COMPLETED`
   (the deregistration flag is required with reusage or the image entrypoint exits 1;
   hooks write `$RUNNER_WORKDIR/.gha-addon/status.json` for idle/busy — see [0005](features/0005-runner-job-state.md)).
5. Labels: `com.github-runner-addon.managed=true`, `com.github-runner-addon.id=<id>`.
6. Configure-only start (`DEBUG_ONLY=true` + token) until `.runner` is on the volume; on failure roll back.
7. Start the long-running listener **without** `RUNNER_TOKEN` (registration files remain on the volume).

List merges JSON records with a single batched `ListManaged` (label filter) plus parallel enrich — not N× `InspectByName`. Missing containers report `missing`; when Docker is unavailable, status stays `unknown`. Running containers get `job_state` / `current_job` from cached/`CopyFromContainer` `status.json` only (no alpine host-file helpers on List). Get/lifecycle may fall back to host reads. Health uses cheap `StatusCounts` (same ListManaged path, no job-status). Bind caches inject `RUNNER_CACHE`; views expose `cache_effective`. Startup/periodic reconcile does not mutate the store for missing containers; it inventories unmanaged labeled **orphan** containers (no matching store row) and exposes them on `/api/v1/health`.

Upstream image behavior (tools inside the container, registration edge cases, OS packages) is documented in [myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner). Prefer pinning `RUNNER_IMAGE` to a digest.

## Ports

Default listen: `:8099` (`HTTP_PORT` / `-http-port`). HA ingress uses the same port. Standalone compose binds `127.0.0.1:8099` by default.

## Persistence (Home Assistant)

Use the standard app data directory `/data` for `runners.json`. It is always available to HA apps, included in backups, and does not need an extra `map:` entry. App options (`log_level`, `mount_docker_sock`, `runner_image`, `github_pat`) are read from `/data/options.json` by the s6 service `rootfs/etc/services.d/github-runner/run` (via `with-contenv`).

Runner **registration** lives in Docker named volumes (`gha-runner-*-data`), not in `/data`. Job workdir is a **host directory** same-path bind (default `/srv/gha-work/<name>`, override `workdir_host_path`) as `RUNNER_WORKDIR`. The agent `workFolder` in `.runner` is set only at configure time — recreate clears `.runner` and reconfigures when it mismatches; create/recreate assert the match and return an error on failure. Optional **cache** mounts (schema v4) are host same-path binds (UI default) or named volumes — large volumes will not appear in HA addon backups. Bind caches inject `RUNNER_CACHE` at the host path; named volumes warn that siblings cannot see them. See [0003](features/0003-persistent-runner-cache.md), [0006](features/0006-same-path-build-cache.md), and [0004](features/0004-sibling-docker-workdir-host-bind.md).

## Home Assistant ingress

The SPA injects `<base href>` from `X-Ingress-Path` (and the UI resolves API/WS URLs from that base) so the operator UI works under `/api/hassio_ingress/<token>/`.

## Environment

| Variable | Default | Purpose |
|----------|---------|---------|
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `MOUNT_DOCKER_SOCK` | `false` | Bind host Docker socket into runner containers (opt-in; enabled in compose/app options) |
| `RUNNER_IMAGE` | `myoung34/github-runner:latest` | Default [myoung34](https://github.com/myoung34/docker-github-actions-runner) image for new runners (prefer a digest for pinning) |
| `DATA_DIR` | `./data` or `/data` | Persist `runners.json` (HA apps use `/data`, always mounted and backed up) |
| `GITHUB_PAT` | _(empty)_ | Optional PAT for minting registration tokens and deregistering runners |
| `APP_VERSION` | image `BUILD_VERSION` / `DefaultVersion` | Reported in logs and `/api/v1/health` (keep in sync via `./scripts/bump-version.sh`) |

## PAT scopes

Classic PAT minimums: `repo` for repository runners; `admin:org` (or org runner admin) for organization runners. Fine-grained PATs need Actions runner administration on the target org/repo. Never commit the PAT; store it in HA options or a private env file.

**Name collision:** container env `RUNNER_TOKEN` (registration, configure phase only) is not the same as a consuming repo’s Actions secret `RUNNER_TOKEN` (PAT used only to list runners for self-hosted-vs-hosted selection). See [DOCS — Consuming repositories](../github_runner/DOCS.md#consuming-repositories-actions-secrets).

## Credits

See [README — Credits and attribution](../README.md#credits-and-attribution). Default runner image: **Matt Young / myoung34** — [docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner).

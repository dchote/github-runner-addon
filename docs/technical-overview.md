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
4. Create named volume + container from configured runner image (default **`myoung34/github-runner`**) with env understood by that image’s entrypoint:
   `REPO_URL` / `ORG_NAME`, `RUNNER_NAME`, `RUNNER_TOKEN`, `LABELS`,
   `CONFIGURED_ACTIONS_RUNNER_FILES_DIR`, `RUNNER_SCOPE`,
   `DISABLE_AUTO_UPDATE`, `DISABLE_AUTOMATIC_DEREGISTRATION`
   (the last is required with reusage or the image entrypoint exits 1).
5. Labels: `com.github-runner-addon.managed=true`, `com.github-runner-addon.id=<id>`.
6. Wait briefly for registration success in container logs; on failure roll back.
7. Scrub `RUNNER_TOKEN` from container env by recreating the container against the same volume (registration files remain).

List merges JSON records with Docker inspect status (`missing` when the container is absent). Startup/periodic reconcile does not mutate the store for missing containers; it inventories unmanaged labeled **orphan** containers (no matching store row) and exposes them on `/api/v1/health`.

Upstream image behavior (tools inside the container, registration edge cases, OS packages) is documented in [myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner). Prefer pinning `RUNNER_IMAGE` to a digest.

## Ports

Default listen: `:8099` (`HTTP_PORT` / `-http-port`). HA ingress uses the same port. Standalone compose binds `127.0.0.1:8099` by default.

## Persistence (Home Assistant)

Use the standard app data directory `/data` for `runners.json`. It is always available to HA apps, included in backups, and does not need an extra `map:` entry. App options (`log_level`, `mount_docker_sock`, `runner_image`, `github_pat`) are read from `/data/options.json` by the s6 service `rootfs/etc/services.d/github-runner/run` (via `with-contenv`).

Runner **registration** lives in Docker named volumes (`gha-runner-*-data`), not in `/data`. Each runner also gets an automatic **work** volume (`gha-runner-*-work`) whose Docker Mountpoint is same-path bind-mounted as `RUNNER_WORKDIR` (sibling-Docker safe). Optional **cache** mounts (schema v3) are separate named volumes or host binds — large volumes will not appear in HA addon backups. Prefer named volumes on HAOS. The default runner image runs as root (Mountpoints OK); cache host binds and non-root images need absolute Docker-host paths writable by the runner uid (often `1000`) unless cache is read-only. Recreate runners after upgrading to automatic workdirs. See [0003](features/0003-persistent-runner-cache.md) and [0004](features/0004-sibling-docker-workdir-host-bind.md).

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

## Credits

See [README — Credits and attribution](../README.md#credits-and-attribution). Default runner image: **Matt Young / myoung34** — [docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner).

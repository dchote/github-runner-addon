# Container Runtime

Docker SDK usage is confined to `internal/container/docker`. The runner package calls this adapter for create/start/stop/remove/inspect/logs/list-managed.

## Upstream runner image

Managed runners are containers from the configured `RUNNER_IMAGE` / `runner_image` option. The **default** is [`myoung34/github-runner`](https://hub.docker.com/r/myoung34/github-runner) from [myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner) (Matt Young).

That image’s entrypoint understands env such as `REPO_URL` / `ORG_NAME`, `RUNNER_TOKEN`, `LABELS`, `RUNNER_SCOPE`, `CONFIGURED_ACTIONS_RUNNER_FILES_DIR`, and `DISABLE_AUTOMATIC_DEREGISTRATION`. This manager sets those variables; it does not vendor or modify the upstream image.

Prefer pinning to a digest. Report image/entrypoint issues upstream; report manager/orchestration issues in this repository. See [README credits](../../README.md#credits-and-attribution).

## Labels

Managed containers carry:

- `com.github-runner-addon.managed=true`
- `com.github-runner-addon.id=<runner-uuid>`

## Volumes

Each runner gets a named volume mounted at the path set in `CONFIGURED_ACTIONS_RUNNER_FILES_DIR` so registration survives container recreate. When that dir is set, the image also requires `DISABLE_AUTOMATIC_DEREGISTRATION=true` or its entrypoint exits 1 after registration.

After a successful first registration, the manager recreates the container **without** `RUNNER_TOKEN` in env (credentials remain on the volume) so `docker inspect` does not retain the short-lived token.

### Persistent cache and workdir

Optional per-runner mounts (see [0003-persistent-runner-cache](../features/0003-persistent-runner-cache.md)):

| Mount | When | Storage | Target | Notes |
|-------|------|---------|--------|-------|
| Cache | `cache.enabled` | Named volume or host bind | default `/cache` | Same `volume_name` / `host_path` across runners shares the cache |
| Workdir | `persist_workdir` | Named volume `*-work` | `/work` | Sets `RUNNER_WORKDIR=/work`; per-runner only |

Prefer **named volumes** on Home Assistant OS. Host binds use **Docker host** paths (not addon `/data` or `/share`). Cache Docker volumes are outside HA `/data` backups.

When cache or workdir persistence is enabled, container `StopTimeout` is **120s** (otherwise 30s) so the runner listener can release cleanly. Recreate/scrub use that same grace when stopping the old container. Stop/restart fall back to the container’s configured timeout, or **30s** if unset (older containers).

**Bind mounts:** ensure the host directory is writable by the runner user (often uid `1000`), e.g. `chown -R 1000:1000 /path/to/cache`, unless the cache is mounted read-only. Sensitive paths (`/`, `/etc`, `/proc`, `/sys`, docker.sock) are rejected as host sources and as container targets.

**Disk hygiene:** unused named volumes and large host cache trees are not pruned automatically — remove them with Docker/`rm` when cold. Applying a config that disables or renames an unreferenced cache/workdir volume deletes that volume after a successful recreate.

## Docker socket

`MOUNT_DOCKER_SOCK=true` bind-mounts the host Docker socket into runner containers (needed for many Actions workflows that build images). Default is `false` for safer standalone runs; docker-compose and the HA addon option enable it. Per-runner override is stored on the runner record.

Mounting the socket is root-equivalent on the Docker host — document and prefer the least privilege that still runs your workflows.

## Resources

Optional per-runner `cpu_limit` (cores → NanoCPUs) and `memory_limit_mb` are applied via Docker HostConfig. Image defaults come from `RUNNER_IMAGE`; prefer a digest for pinning in production.

## Reconcile

On startup and every ~2 minutes the manager lists Docker containers with the managed label and records orphans (containers without a matching store row). Orphans are surfaced in `/api/v1/health` and the UI, and are not deleted automatically.

## Create resilience

`ContainerCreate` can succeed on the daemon after the client context is canceled (request timeout or disconnect). `CreateAndStart` recovers by inspecting the named container with a detached timeout, adopting it when managed labels match, and starting it if needed. Failed create cleanup in the manager also uses a detached Docker context so rollback is not a no-op under cancel; if a matching managed container already exists, the store row is kept instead of becoming an orphan.

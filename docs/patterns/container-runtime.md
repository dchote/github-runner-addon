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

## Docker socket

`MOUNT_DOCKER_SOCK=true` bind-mounts the host Docker socket into runner containers (needed for many Actions workflows that build images). Default is `false` for safer standalone runs; docker-compose and the HA addon option enable it. Per-runner override is stored on the runner record.

Mounting the socket is root-equivalent on the Docker host — document and prefer the least privilege that still runs your workflows.

## Resources

Optional per-runner `cpu_limit` (cores → NanoCPUs) and `memory_limit_mb` are applied via Docker HostConfig. Image defaults come from `RUNNER_IMAGE`; prefer a digest for pinning in production.

## Reconcile

On startup and every ~2 minutes the manager lists Docker containers with the managed label and records orphans (containers without a matching store row). Orphans are surfaced in `/api/v1/health` and the UI, and are not deleted automatically.

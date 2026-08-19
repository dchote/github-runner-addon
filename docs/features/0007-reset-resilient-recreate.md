# 0007 Reset-resilient recreate

## Status

Implemented.

## Goal

After a Docker engine reset (or any other wipe of containers and named volumes), **Recreate** must be able to bring runners back from `runners.json` plus a registration token or PAT — regardless of leftover Docker state. Create on a new/empty volume must actually run `config.sh`. Diagnostic alpine helpers must not spin forever while the UI polls missing runners.

## Problem

1. **`DEBUG_ONLY` does not configure.** The manager used `DEBUG_ONLY=true` to mean “configure, don’t listen”. Upstream [`myoung34` `entrypoint.sh`](https://github.com/myoung34/docker-github-actions-runner/blob/master/entrypoint.sh) skips `config.sh` when `DEBUG_ONLY` is set **and** `.runner` is missing (the Docker-reset case). The configure container exits 0 without writing `.runner`. A pasted registration token never takes effect. Create of a new volume hits the same path.
2. **`GET /runners/{id}` live-reads `.runner` via alpine helpers** even when the container is missing. The UI polls that every 10s for the selected row, which looks like containers constantly creating and destroying.
3. **List poll clears `store.error`**, so recreate failures vanish from the UI.
4. **`409 RUNNER_BUSY`** consulted host `status.json` even when no managed container was running, which can block recreate after a Docker kill of a busy agent (until the 24h stale window).

## Contract

Recreate succeeds whenever the engine can create containers, given stored config plus a token or PAT, in these states:

- Container missing / volume missing
- Container missing / volume empty (no `.runner`) — same as missing
- Container missing / volume has reusable `.runner` — reuse credentials, no token
- Stale leftover named container (remove, then create)
- Host workdir and cache binds still on disk (do not require wipe)
- `status.json` busy but **no running managed container** — do not 409
- Image missing (including `:local` tags) — fail closed with **400**; do not pull `:local`. Registry/network pull failures are **502 `IMAGE_PULL_ERROR`**.

Reconcile stays read-only. Operators recover with Recreate. Start still does not create missing containers.

## Configure phase

Two-phase start is unchanged in intent (token never remains on the long-running container):

1. **Configure:** `RUNNER_TOKEN` set, **no** `DEBUG_ONLY`, `RestartPolicy=no`, container **CMD** overridden to `true` so the image entrypoint runs `config.sh --replace`, copies `.runner` onto the data volume, then exits without listening.
2. **Run:** remove the configure container; start `unless-stopped` **without** `RUNNER_TOKEN`.

Wait-for-configure and post-start verify use `CopyFromContainer` on the runner container. A missing `.runner` is treated as “not configured yet” — they do **not** fall back to alpine `ReadVolumeFile` (that storm was the original poll bug during a 90s configure wait). Get live-reads `.runner` the same way while the container is running.

Images that ignore Docker `CMD` (do not exec the configured command after `config.sh`) may start the listener during the configure phase. Prefer `myoung34/github-runner` or an image with the same entrypoint contract. Custom `:local` tags are never pulled — they must already exist on the host.

Registry/network pull failures return **502 `IMAGE_PULL_ERROR`**. A missing `:local` (or still-missing image after pull) returns **400 `VALIDATION_ERROR`**.

## Diagnostics / UI

- `GET /runners/{id}` does not spawn volume helpers when the container is not running. While running, it reads `.runner` via `CopyFromContainer` (not alpine).
- List after a successful `ListManaged` treats names absent from that batch as `missing` without `InspectByName`.
- `ReadVolumeFile` checks `VolumeExists` first (never auto-creates an empty volume) and shares the helper concurrency semaphore with other alpine helpers.
- Background list poll does not clear mutation errors.
- Recreate and Edit apply dialogs always offer an optional registration token and show the API error in the dialog.
- Inspect errors during the busy gate **fail closed** as **503 `DOCKER_UNAVAILABLE`**. A **running** container with missing/unreadable/**unknown** `status.json` is **409 `RUNNER_BUSY`**.
- Create rollback **keeps** the store row and data volume when `.runner` exists or a managed container is present (GitHub may already have the runner). Recreate backs up `.runner` and restores it if start fails.
- Save & apply persists the store **after** a successful recreate. Create/recreate rate-limit quota is not consumed by 400/404.
- Operator **Recreate missing** (`POST /api/v1/runners/recreate-missing`) restores a Docker-reset fleet; reconcile stays read-only.
- Start/restart/logs on a missing container return **400** (recreate it). Listen wait timeout while still running fails the create/recreate (never 201).

## Related

- [0002 Hardened persistent fleet](0002-hardened-persistent-fleet.md)
- [0005 Runner job state](0005-runner-job-state.md)
- [Container runtime](../patterns/container-runtime.md)

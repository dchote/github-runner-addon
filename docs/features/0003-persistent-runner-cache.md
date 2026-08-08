# 0003 Persistent Runner Cache

## Status

Cache implemented in 0.3.0. **Sibling-Docker / Buildx bind policy** is defined in [0006](0006-same-path-build-cache.md) (same-path binds + `RUNNER_CACHE`). Sibling-Docker workdir is a separate host-bind + reconfigure flow — see [0004](0004-sibling-docker-workdir-host-bind.md).

## Goal

Enable long-running / incremental CI on managed runners: a durable cache mount (named Docker volume or host same-path bind), plus a sibling-Docker-safe job workdir, while keeping registration on `/runner/data`.

## Scope

### In scope

- Per-runner **cache** mount: named volume or host bind
- Shared cache by using the same `volume_name` or `host_path` on multiple runners
- Optional **read-only** cache mount
- **Sibling-Docker workdir** via host same-path bind + agent reconfigure when `workFolder` drifts (see [0004](0004-sibling-docker-workdir-host-bind.md))
- Stop timeout **120s** for managed runners
- Delete refcount for shared cache volumes; never delete bind host paths
- Apply-time cleanup of stale unreferenced cache volumes
- UI Advanced fields for cache and optional workdir host path

### Out of scope

- Autoscaling / ephemeral pools / ARC
- Actions remote cache proxy (`ACTIONS_CACHE_*`)
- Automatic cache pruning / size quotas
- Resolving Home Assistant Supervisor `path_extern_share` into host binds
- Project-specific toolchain env defaults beyond `RUNNER_CACHE` (use `extra_env`)

## Persistence model

| Knob | Storage | Default path | Sharing | Delete with runner? |
|------|---------|--------------|---------|---------------------|
| Registration | Named volume `*-data` | `/runner/data` | Never | Always |
| Cache (bind) | Host same-path bind | operator `host_path` | Same path across runners | Never (host path left on disk) |
| Cache (volume) | Named volume | container `target` (default `/cache`) | Same `volume_name` | Volume only if unreferenced |
| Workdir | Host directory same-path bind | `/srv/gha-work/<name>` | Never | Never (host path left on disk) |

### Volume vs bind (cache)

- **Host bind (UI default):** same-path mount at `host_path` for dedicated disks and sibling Docker / Buildx `type=local`. See [0006](0006-same-path-build-cache.md).
- **Named volume:** portable; Docker manages location. Prefer on HAOS when sibling host binds are not required. Sibling containers that bind-mount a host path will **not** see a named volume (soft `warnings[]`).

Do **not** rely on HA `map: share` for runner caches — the Supervisor maps those trees only into the addon; sibling runners created via `docker_api` see the host filesystem.

## Contract notes

- Schema version **4** (adds `workdir_host_path`).
- Changing cache settings requires container recreate (`apply=true`).
- Changing workdir (or fixing a `workFolder` mismatch) requires recreate **and** agent reconfigure (token/PAT); env alone is not enough.
- Large caches live in Docker volumes / host disk — **not** in addon `/data` HA backups.
- **Ownership:** default runner image is root. For non-root images, `chown` work/cache host paths to the runner uid (often `1000`) unless cache `read_only`.
- **Disk:** prune unused Docker volumes and host cache/workdir trees periodically.
- Bind path policy, `RUNNER_CACHE`, and operator recipes: [0006](0006-same-path-build-cache.md).

## Operator recipe

See [0006](0006-same-path-build-cache.md) for per-project same-path binds (e.g. `/scratch/build-cache/<project>`) and HAOS named-volume guidance.

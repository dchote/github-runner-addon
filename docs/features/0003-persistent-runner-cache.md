# 0003 Persistent Runner Cache

## Status

Implemented — host-side cache and optional workdir persistence for managed runners.

## Goal

Enable long-running / incremental CI on managed runners: a durable cache mount (named Docker volume or host bind, default `/cache`), optional durable job workdir at `/work`, while keeping registration on the existing `/runner/data` volume. Multiple runners can share one cache by using the same volume name or host path.

## Scope

### In scope

- Per-runner **cache** mount: named volume or host bind, default target `/cache`
- Shared cache by using the same `volume_name` or `host_path` on multiple runners
- Optional **persist workdir**: per-runner named volume at `/work` (`RUNNER_WORKDIR=/work`)
- Optional **read-only** cache mount
- Stop timeout **120s** when cache or workdir persistence is enabled
- Delete refcount for shared cache volumes; never delete bind host paths
- Apply-time cleanup of stale unreferenced work/cache volumes
- UI Advanced fields, OpenAPI, docs (HAOS vs bind paths)

### Out of scope

- Autoscaling / ephemeral pools / ARC
- Actions remote cache proxy (`ACTIONS_CACHE_*`)
- Automatic cache pruning / size quotas
- Resolving Home Assistant Supervisor `path_extern_share` into host binds
- Project-specific toolchain env defaults (use `extra_env`)

## Persistence model

| Knob | Storage | Default target | Sharing | Delete with runner? |
|------|---------|----------------|---------|---------------------|
| Registration (existing) | Named volume `*-data` | `/runner/data` | Never | Always |
| Cache | Named volume or host bind | `/cache` | Same name/path across runners | Volume only if unreferenced; never host path |
| Workdir (optional) | Named volume | `/work` | Never | Always |

### Volume vs bind

- **Named volume (preferred on HAOS):** portable; Docker manages location.
- **Host bind:** for dedicated disks (e.g. `/srv/runner-cache`). Path is resolved by the **Docker host**, not paths inside the addon container. Addon `/share` / `/data` are not sibling-mountable without the host’s real path.

Do **not** rely on HA `map: share` for runner caches — the Supervisor maps those trees only into the addon; sibling runners created via `docker_api` see the host filesystem.

## Contract notes

- Schema version **3** (additive; older runners.json remains valid with cache disabled).
- Changing cache/workdir settings requires container recreate (`apply=true`).
- After a successful apply, stale **owned** work volumes and **unreferenced** cache volumes from the previous config are removed.
- Failed create rollback removes only volumes this create owned (auto-named); does not remove a pre-existing shared cache volume.
- Recreate / scrub keep still-referenced volume types; stop grace is **120s** when cache or workdir persistence is enabled (otherwise **30s**).
- Large caches live in Docker volumes / host disk — **not** in addon `/data` HA backups.
- Shared R/W cache concurrency is an operator responsibility (serialize writers / use flock as needed).
- **Bind ownership:** host directories keep host UIDs. The runner image typically runs as uid **1000** (or similar) — `chown` the host cache path so jobs can write (unless `read_only` is set).
- **Disk:** prune unused Docker volumes and host cache trees periodically; there is no in-app quota UI.

## Operator recipe (shared host cache)

Example for several runners sharing one host directory:

- `cache.enabled=true`, `cache.type=bind`, `cache.host_path=/srv/runner-cache`, `cache.target=/cache`
- `persist_workdir=true` (optional; per-runner job workspace)
- `extra_env` for workflow cache roots as needed by the job (download dirs, compiler caches, etc.)

On HAOS without a known host path, use `cache.type=volume` and the same `volume_name` on every runner that should share the cache.

Ensure the host directory is writable by the runner user when not using read-only mode:

```bash
sudo mkdir -p /srv/runner-cache
sudo chown -R 1000:1000 /srv/runner-cache
```

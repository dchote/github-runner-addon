# 0003 Persistent Runner Cache

## Status

Cache implemented in 0.3.0; automatic Mountpoint workdir in 0.3.1.

## Goal

Enable long-running / incremental CI on managed runners: a durable cache mount (named Docker volume or host bind, default `/cache`), plus an **automatic** job workdir that is sibling-Docker safe, while keeping registration on `/runner/data`.

## Scope

### In scope

- Per-runner **cache** mount: named volume or host bind, default target `/cache`
- Shared cache by using the same `volume_name` or `host_path` on multiple runners
- Optional **read-only** cache mount
- **Automatic workdir** via Docker volume Mountpoint same-path bind (see [0004](0004-sibling-docker-workdir-host-bind.md))
- Stop timeout **120s** for managed runners
- Delete refcount for shared cache volumes; never delete bind host paths
- Apply-time cleanup of stale unreferenced cache volumes
- UI Advanced fields for cache; workdir requires no operator input

### Out of scope

- Autoscaling / ephemeral pools / ARC
- Actions remote cache proxy (`ACTIONS_CACHE_*`)
- Automatic cache pruning / size quotas
- Resolving Home Assistant Supervisor `path_extern_share` into host binds
- Project-specific toolchain env defaults (use `extra_env`)

## Persistence model

| Knob | Storage | Default target | Sharing | Delete with runner? |
|------|---------|----------------|---------|---------------------|
| Registration | Named volume `*-data` | `/runner/data` | Never | Always |
| Cache | Named volume or host bind | `/cache` | Same name/path across runners | Volume only if unreferenced; never host path |
| Workdir (automatic) | Named volume `*-work` → host Mountpoint same-path bind | Mountpoint path | Never | Always (volume) |

### Volume vs bind (cache)

- **Named volume (preferred on HAOS):** portable; Docker manages location.
- **Host bind:** for dedicated disks (e.g. `/srv/runner-cache`). Path is resolved by the **Docker host**.

Do **not** rely on HA `map: share` for runner caches — the Supervisor maps those trees only into the addon; sibling runners created via `docker_api` see the host filesystem.

## Contract notes

- Schema version **3** (additive).
- Changing cache settings requires container recreate (`apply=true`).
- Recreate re-resolves the work volume Mountpoint and remounts it; registration files on `*-data` are reused (no re-registration).
- **Upgrade:** recreate runners after moving to automatic Mountpoint workdirs so sibling Docker jobs see the new bind.
- Large caches live in Docker volumes / host disk — **not** in addon `/data` HA backups.
- **Ownership:** default runner image is root (volume Mountpoints OK). For non-root images, `chown` work/cache host paths to the runner uid (often `1000`) unless cache `read_only`.
- **Disk:** prune unused Docker volumes and host cache trees periodically.

## Operator recipe (shared host cache)

- `cache.enabled=true`, `cache.type=bind`, `cache.host_path=/srv/runner-cache`, `cache.target=/cache`
- Workdir: no configuration — created automatically per runner
- `extra_env` for workflow cache roots as needed

```bash
sudo mkdir -p /srv/runner-cache
sudo chown -R 1000:1000 /srv/runner-cache
```

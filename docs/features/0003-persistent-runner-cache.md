# 0003 Persistent Runner Cache

## Status

Cache implemented in 0.3.0. Sibling-Docker workdir is a separate host-bind + reconfigure flow — see [0004](0004-sibling-docker-workdir-host-bind.md).

## Goal

Enable long-running / incremental CI on managed runners: a durable cache mount (named Docker volume or host bind, default `/cache`), plus a sibling-Docker-safe job workdir, while keeping registration on `/runner/data`.

## Scope

### In scope

- Per-runner **cache** mount: named volume or host bind, default target `/cache`
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
- Project-specific toolchain env defaults (use `extra_env`)

## Persistence model

| Knob | Storage | Default target | Sharing | Delete with runner? |
|------|---------|----------------|---------|---------------------|
| Registration | Named volume `*-data` | `/runner/data` | Never | Always |
| Cache | Named volume or host bind | `/cache` | Same name/path across runners | Volume only if unreferenced; never host path |
| Workdir | Host directory same-path bind | `/srv/gha-work/<name>` | Never | Never (host path left on disk) |

### Volume vs bind (cache)

- **Named volume (preferred on HAOS):** portable; Docker manages location. Sibling containers that bind-mount the cache **target** as a host path (e.g. `docker run -v /cache:…` or Buildx `type=local,dest=/cache/…`) will **not** see a named volume — use a host bind (below) when workflows need that.
- **Host bind:** for dedicated disks and for sibling-Docker/Buildx local export. Path is resolved by the **Docker host**.

Do **not** rely on HA `map: share` for runner caches — the Supervisor maps those trees only into the addon; sibling runners created via `docker_api` see the host filesystem.

### Same-path cache bind (sibling Docker / Buildx)

Jobs that call sibling Docker with an absolute cache path, or Buildx `type=local` under that path, resolve mounts on the **Docker host**, not inside the runner. The runner mount alone is not enough when `host_path` ≠ `target`:

| Runner sees | Sibling `docker run -v /cache:…` uses | Result |
|-------------|----------------------------------------|--------|
| bind `/srv/runner-cache` → `/cache` | host `/cache` | **Miss** unless host `/cache` is the same directory (bind/symlink) |
| bind `/cache` → `/cache` | host `/cache` | **Hit** (same-path rule) |
| named volume → `/cache` | host `/cache` | **Miss** (volume is not host `/cache`) |

**Prefer** `cache.host_path` equal to `cache.target` (commonly both `/cache`). A mismatched bind remains allowed; create/list/get surface a soft `warnings[]` advisory and the manager logs it — the request still succeeds.

If data must live on another disk, bind that tree onto the **target path on the host** and still set `host_path` to the target (so the advisory stays quiet and sibling Docker sees one path):

```bash
sudo mkdir -p /srv/runner-cache /cache
sudo mount --bind /srv/runner-cache /cache
# cache.host_path=/cache  cache.target=/cache
```

Setting `host_path=/srv/runner-cache` with `target=/cache` can work **only if** `/cache` on the host is already that same directory, but the API still warns (heuristic; it does not inspect host mounts). Named volumes also warn: they are never visible to sibling `docker run -v <target>:…` on the host.

## Contract notes

- Schema version **4** (adds `workdir_host_path`).
- Changing cache settings requires container recreate (`apply=true`).
- Changing workdir (or fixing a `workFolder` mismatch) requires recreate **and** agent reconfigure (token/PAT); env alone is not enough.
- Large caches live in Docker volumes / host disk — **not** in addon `/data` HA backups.
- **Ownership:** default runner image is root. For non-root images, `chown` work/cache host paths to the runner uid (often `1000`) unless cache `read_only`.
- **Disk:** prune unused Docker volumes and host cache/workdir trees periodically.

## Operator recipe (shared host cache)

For sibling Docker / Buildx local under `/cache` (recommended on dedicated builder hosts):

- `cache.enabled=true`, `cache.type=bind`, `cache.host_path=/cache`, `cache.target=/cache`
- Workdir: default `/srv/gha-work/<name>` or set `workdir_host_path`
- `extra_env` for workflow cache roots as needed

```bash
sudo mkdir -p /cache /srv/gha-work/my-runner
sudo chown -R 1000:1000 /cache /srv/gha-work/my-runner
```

On HAOS, prefer a **named volume** when workflows do not need sibling host binds of `/cache` (expect a soft `warnings[]` about sibling visibility).

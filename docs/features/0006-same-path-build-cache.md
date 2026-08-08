# 0006 Same-path per-project build cache

## Status

Implements sibling-Docker-safe cache binds (same-path + `RUNNER_CACHE`). Supersedes the soft mismatch-warning policy in [0003](0003-persistent-runner-cache.md) for bind mounts.

## Goal

Let operators place durable build caches on **any** absolute Docker-host path (SSD under `/scratch/…`, USB under `/media/…`, or whatever mount they prefer) so that path works inside the runner, sibling `docker run -v`, and Buildx `type=local` — without remapping onto a shared `/cache`.

## Contract

1. **Bind cache = same-path only.** `normalizeCache` sets `target = host_path` for `type=bind`. Mismatched API input is coerced.
2. **Single operator knob for binds:** host path. UI hides a separate mount-path field in bind mode.
3. **UI default storage = host bind** when Persistent cache is enabled in Create/Edit. Named volume remains available (HAOS / non-sibling).
4. **`RUNNER_CACHE=<absolute path>`** is injected when cache is enabled:
   - bind → host/same path
   - volume → container `target` (runner-local; siblings cannot see named volumes)
5. **No bind mismatch warning.** Named volumes still surface a soft sibling-visibility advisory.
6. **No host-level `/cache` bind/symlink** orchestration and **no default `/cache` alias mount** — workflows use `$RUNNER_CACHE`.
7. **API omit-`type` default** stays `volume` for backward compatibility; only the manager UI defaults new forms to `bind`.

## Persistence model

| Knob | Storage | Path | Sharing | Delete with runner? |
|------|---------|------|---------|---------------------|
| Registration | Named volume `*-data` | `/runner/data` | Never | Always |
| Cache (bind) | Host same-path bind | operator `host_path` | Same path across runners | Never (host path left on disk) |
| Cache (volume) | Named volume | container `target` (default `/cache`) | Same `volume_name` | Volume only if unreferenced |
| Workdir | Host same-path bind | default `/srv/gha-work/<name>` | Never | Never |

## Sibling Docker / Buildx

Jobs that call sibling Docker or Buildx `type=local` resolve paths on the **Docker host**.

| Runner mount | Workflow / Buildx uses | Result |
|--------------|------------------------|--------|
| bind `/scratch/build-cache/proj` → same | `$RUNNER_CACHE/...` or that absolute path | **Hit** |
| named volume → `/cache` | host `/cache` or `$RUNNER_CACHE` as host path | **Miss** (volume ≠ host path) |

Prefer bind + `$RUNNER_CACHE` for builder hosts. Prefer named volume on HAOS when sibling host binds are not required (expect soft `warnings[]`).

## Operator recipe (any host path)

Pick any absolute path on the Docker host — the manager same-path binds it and injects `RUNNER_CACHE`. Examples: `/scratch/build-cache/<project>`, `/media/usb0/ci-cache`, `/mnt/nvme/gha-cache`.

```text
cache.enabled=true, type=bind, host_path=<absolute-host-path>
# RUNNER_CACHE is set automatically to that path
# workflows: type=local,dest=$RUNNER_CACHE/buildx/...
#            docker run -v $RUNNER_CACHE/...:...
```

```bash
sudo mkdir -p /media/usb0/ci-cache   # or whatever path you chose
sudo chown -R 1000:1000 /media/usb0/ci-cache
```

Share a cache across runners by reusing the same `host_path` (or the same `volume_name` in volume mode). Missing host dirs are created via `EnsureHostDir` (binds the top-level directory of the path — not an allowlist).

**Removable drives:** mount the USB/SSD *before* create/recreate. `EnsureHostDir` runs `mkdir -p` under the top-level bind root; if the mount point is absent, that can create a real directory that later shadows the device when it is plugged in.

## API / UI

- Create/list/get expose `cache_effective` (resolved path jobs should use) and `RUNNER_CACHE` in the container env.
- Changing cache settings requires container recreate (`apply=true`).
- Busy runners block apply/recreate/delete ([0005](0005-runner-job-state.md)).

## Migration

Existing bind configs with `host_path ≠ target`: on normalize, `target` becomes `host_path`. Data on the host stays put; the **in-container path** changes from the old target (often `/cache`) to the host path. Recreate runners and update workflows that hardcoded `/cache` to `$RUNNER_CACHE`.

## Related

- [0003 Persistent Runner Cache](0003-persistent-runner-cache.md) — volume lifecycle, delete refcount
- [0004 Sibling-Docker workdir](0004-sibling-docker-workdir-host-bind.md) — same-path workdir pattern
- [Container runtime](../patterns/container-runtime.md)

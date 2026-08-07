# 0004 — Sibling Docker workdir (same-path host bind)

## Status

Implemented.

## Goal

Make Actions jobs that call `docker run -v $GITHUB_WORKSPACE:…` (goreleaser-cross, etc.) work when the runner container mounts the host Docker socket.

## Problem

Ephemeral workdir (`/tmp/runner/work`) and named-volume workdir (`/work`) exist only **inside** the runner container. Sibling containers started via the host `docker.sock` resolve bind mounts on the **host**, so `$GITHUB_WORKSPACE` is empty/missing → e.g. `open .goreleaser-amd64.yaml: no such file or directory`.

## Solution

Optional **`workdir_host_path`**: absolute path on the Docker host, bind-mounted at the **same absolute path** in the runner, and set as `RUNNER_WORKDIR`. Then `github.workspace` is a host-visible path and sibling `-v` mounts work.

| Mode | Storage | Sibling `docker run -v $GITHUB_WORKSPACE` |
|------|---------|-------------------------------------------|
| Default ephemeral | Container `/tmp/runner/work` | Broken |
| `persist_workdir` volume | Named volume → `/work` | Broken |
| **`workdir_host_path`** | Host bind → same path | Works |

## Operator recipe

On the Docker host (not inside the addon):

```bash
sudo mkdir -p /srv/gha-work/supervisor-builder
sudo chown -R 1000:1000 /srv/gha-work/supervisor-builder
```

In the runner UI (Advanced):

- Set **Workdir host path** to `/srv/gha-work/supervisor-builder` (or similar unique path per runner)
- Keep **Docker socket** mounted
- **Save & apply** (recreate container)

HAOS: use a host path the Docker engine can see (dedicated disk / known Supervisor path). Named volumes alone are not enough for sibling workspace mounts.

## API

- Create/Patch/Runner: `workdir_host_path` (string; empty clears on patch)
- When set, named work volume at `/work` is not mounted; `persist_workdir` is ignored for mounts

## Related

- [0003 Persistent Runner Cache](0003-persistent-runner-cache.md)
- [Container runtime](../patterns/container-runtime.md)

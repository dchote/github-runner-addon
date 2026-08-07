# 0004 — Automatic sibling-Docker workdir

## Status

Implemented in 0.3.1.

## Goal

Make Actions jobs that call `docker run -v $GITHUB_WORKSPACE:…` work when the runner mounts the host Docker socket — with no operator host-path configuration.

## Problem

Paths that exist only inside the runner are not visible on the Docker host. Sibling containers started via `docker.sock` resolve bind mounts on the host, so `$GITHUB_WORKSPACE` is empty/missing.

## Solution

For every managed runner the control plane:

1. Ensures a per-runner named volume `gha-runner-<name>-work`
2. Reads that volume’s Docker **Mountpoint**
3. Bind-mounts `Mountpoint` → `Mountpoint` into the runner
4. Sets `RUNNER_WORKDIR` to that Mountpoint

## Lifecycle

- **Create / recreate / scrub:** ensure work volume → resolve Mountpoint → mount + set `RUNNER_WORKDIR`. Registration volume unchanged (no re-registration).
- **Delete / failed-create rollback:** remove `gha-runner-<name>-work` with the runner.
- **Upgrade from 0.3.0:** recreate each runner so the container remounts the Mountpoint bind (keep registration volume).

## Permissions

Docker volume Mountpoints are typically root-owned. The default `myoung34/github-runner` image runs as root. Non-root runner images need a writable Mountpoint (or an image that runs as root for workspace setup).

## Related

- [0003 Persistent Runner Cache](0003-persistent-runner-cache.md)
- [Container runtime](../patterns/container-runtime.md)

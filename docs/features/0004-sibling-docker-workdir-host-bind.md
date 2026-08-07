# 0004 — Sibling-Docker workdir (host same-path bind)

## Status

Corrected after 0.3.1–0.3.3 Mountpoint approach proved insufficient: env/mount alone does not update the agent `workFolder`.

## Goal

Make Actions jobs that call `docker run -v ${{ github.workspace }}:…` (or `$GITHUB_WORKSPACE`) work when the runner mounts the host Docker socket.

## Problem

1. Sibling containers resolve bind mounts on the **Docker host**. Paths that exist only inside the runner (or only as a named volume) are empty/wrong for `-v $GITHUB_WORKSPACE`.
2. [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner) applies `--work` **only at configure time**. Recreating the container with a new `RUNNER_WORKDIR` while reusing `*-data` leaves `.runner` `"workFolder"` unchanged (often `/tmp/runner/work`). Jobs keep checking out into the old path; the host bind stays empty.

Docker volume Mountpoints under `/var/lib/docker/volumes/…/_data` are **not** a valid operator recipe for this.

## Solution

For every managed runner the control plane:

1. Resolves a **host directory** — default `/srv/gha-work/<normalized-name>`, or `workdir_host_path` override
2. Same-path bind-mounts that path into the runner
3. Sets `RUNNER_WORKDIR` to that path
4. On create / when `.runner` `workFolder` ≠ desired path: clears `.runner` on the registration volume and **reconfigures** with a registration token or PAT so `config.sh --work` matches the bind

## Lifecycle

| Action | Behavior |
|--------|----------|
| Create | Configure with `--work` = host path; bind mount; keep credentials on `*-data` |
| Recreate / Save & apply | Remount + env; if agent `workFolder` mismatches, clear `.runner` and reconfigure (token/PAT required) |
| Delete | Removes container + `*-data`; **never** deletes the host workdir tree; best-effort removes obsolete `*-work` volumes from older releases |

## Diagnostics

API/UI expose:

- `workdir_effective` — resolved host bind / `RUNNER_WORKDIR`
- `workdir_agent` — `workFolder` from `/runner/data/.runner`
- `workdir_mismatch` — true when they differ (apply/reconfigure needed)

## Operator checklist

```bash
sudo mkdir -p /srv/gha-work/my-runner
sudo chown -R 1000:1000 /srv/gha-work/my-runner   # if not running as root
```

After upgrade from Mountpoint workdirs: **Recreate** (or Save & apply) each runner with PAT/token so `workFolder` moves onto the host bind. Confirm a job’s “Working directory” is under `/srv/gha-work/…` and `ls` on the host shows the checkout during the job.

## Related

- [0003 Persistent Runner Cache](0003-persistent-runner-cache.md)
- [Container runtime](../patterns/container-runtime.md)

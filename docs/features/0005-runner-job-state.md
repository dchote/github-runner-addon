# 0005 — Runner job state (idle / busy)

## Status

Implemented.

## Goal

When a runner container is running, show whether the Actions agent is **idle** or **busy**, and surface current job metadata in the details pane — without calling the GitHub API or requiring a PAT.

## Problem

List/details today expose Docker lifecycle only (`running` / `exited` / `missing`). Operators cannot tell if a green “running” runner is waiting for work or executing a job. `myoung34/github-runner` has no busy metrics; registration files and workdir residue are not reliable live signals. GitHub REST `busy` needs a token.

## Solution

Use official Actions [job hooks](https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/running-scripts-before-or-after-a-job):

1. On create/recreate/start, install hook scripts under `$RUNNER_WORKDIR/.gha-addon/hooks/` on the Docker host (same-path bind).
2. Set managed env:
   - `ACTIONS_RUNNER_HOOK_JOB_STARTED`
   - `ACTIONS_RUNNER_HOOK_JOB_COMPLETED`
3. Hooks write `$RUNNER_WORKDIR/.gha-addon/status.json` (atomic) with `busy` plus job fields from default Actions env vars.
4. Control plane reads that file via `CopyFromContainer` (running runner) and exposes `job_state` + `current_job` on the runner view.

No image fork; no PAT; no job history store.

## API / UI contract

| Field | Role |
|-------|------|
| `status` / `running` | Unchanged Docker lifecycle |
| `job_state` | `idle` \| `busy` \| `unknown` when container is running; omitted when not |
| `current_job` | Present only when `job_state=busy` |

**UI:** Status column shows `idle` / `busy` / `unknown` when the container is running; otherwise shows container status. Details keep container status and add a Current job block when busy. Summary chips remain lifecycle counts (optional Busy count). While `job_state=busy`, the UI disables **Edit**, **Delete**, and **Recreate** (start/stop/restart/logs stay available). This is operator UX only — the REST API does not reject those actions for busy runners. Dialogs already open may still be confirmed.

## Stale busy

- Create/recreate seeds `status.json` to idle before the container starts.
- Start (stopped → running) and Restart seed idle **only after** a successful Docker op; Start is a no-op when already running (does not wipe busy).
- Completed hook clears busy.
- If `busy:true` and `updated_at` is missing, unparseable, or older than 24h → demote to `unknown` (no job block).

## Operator notes

- After upgrade, **Recreate** existing runners so hook env vars are applied; until then running containers may show `unknown`. (Plain Start does not refresh container env.)
- Hooks appear as **Set up runner** / **Complete runner** steps in workflow logs.
- Non-root runner images still need workdir ownership (often `chown 1000:1000`) so hooks can write `status.json`.
- `.gha-addon/` is mode `0777` and `status.json` is `0666` so the runner uid can update it — any job step on that runner can also write the file; treat it as operator telemetry, not a trust boundary.
- Hook scripts must always exit 0 (non-zero fails the job).

## Out of scope

- GitHub API / PAT for busy or job history
- Forking `myoung34/github-runner`
- Historical job list in the UI
- `Runner.Worker` process checks / `_diag` log scraping as primary signal
- API-level rejection of edit/delete/recreate while busy (UI gating only)

## Related

- [0004 Sibling-Docker workdir](0004-sibling-docker-workdir-host-bind.md)
- [Product overview](../product-overview.md)
- [Technical overview](../technical-overview.md)

# GitHub Runner Manager

Manage multiple **GitHub Actions self-hosted runners** from Home Assistant. This app is a local control plane: it creates and supervises Docker containers that run your CI. Each runner container uses the community image [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner) by default (configurable).

For a fuller product description, install notes, and attribution, see the [repository README](https://github.com/dchote/github-runner-addon#readme).

## Requirements

- Docker API access (`docker_api: true` in this app)
- **Protection mode must be turned off** for this app (Supervisor → app → Protection mode). Otherwise the Docker socket is not available and runner create/start will fail.
- Either a registration token from GitHub (**Settings → Actions → Runners → New self-hosted runner**, expires in about one hour), or a **GitHub PAT** configured in app options

## Usage

1. Open the app UI (ingress panel).
2. Click **Create runner**.
3. Enter a unique **name**, the **GitHub project URL** (repository or organization), and a **registration token** (optional when a PAT is configured).
4. Optionally set labels, CPU/memory limits, persistent cache, workdir host path (default `/srv/gha-work/<name>`), and other runtime fields.
5. Monitor status in the table; use **Edit**, **Logs**, **Recreate**, and lifecycle actions as needed. When a container is **running**, the Status column shows **idle** / **busy** / **unknown** (job activity from local hooks, no PAT). Details show current job fields while busy. While **busy**, the UI disables Edit, Delete, and Recreate (start/stop/restart/logs stay available).

Registration tokens are passed only to a short-lived configure-only container, then the long-running listener starts without them. They are not stored in `runners.json`. The PAT is never written into runner containers.

### Job activity (idle / busy)

The manager installs official Actions job hooks under the runner workdir (`.gha-addon/hooks/`) and sets `ACTIONS_RUNNER_HOOK_JOB_*` on the container. Hooks write a small `status.json` the UI reads — no GitHub API call. After upgrading the addon, **Recreate** existing runners so env is refreshed, or running containers may show **unknown** (plain Start does not change env). Workflow runs gain **Set up runner** / **Complete runner** steps from these hooks. The `.gha-addon/` directory is world-writable so the runner uid can update status — treat it as telemetry, not a trust boundary. See [0005](../docs/features/0005-runner-job-state.md).

## Persistence

Runner expected configuration is stored at `/data/runners.json` (Home Assistant’s standard app data directory). That path is always mounted, included in HA backups, and does not require extra `map:` entries.

Registration credentials live in a Docker named volume per runner (`*-data`). Job workdir is a **host directory** same-path bind (default `/srv/gha-work/<name>`) so sibling `docker run -v $GITHUB_WORKSPACE` works with the Docker socket — not a Docker volume `_data` path. Optional **persistent cache** (named volume or host bind at `/cache`) is separate. Large volumes are **not** part of HA `/data` backups.

**Recreate / Save & apply:** remounts the host workdir and sets `RUNNER_WORKDIR`. If the agent `.runner` `workFolder` does not match, the addon clears `.runner` and reconfigures (PAT or registration token required). Env alone never moves the agent workdir. After start, a mismatched `workFolder` fails the request (fail closed). Deleting a runner does **not** delete the host workdir tree.

The manager creates missing workdir (and cache bind) host directories before starting the runner (helper bind-mounts the narrowest root such as `/srv`, then `mkdir -p`). For non-root runner images, `chown` those paths to the runner uid (often `1000`), or enable **Read-only cache** if workflows only need to read. Prune unused Docker volumes / host trees periodically — the addon does not enforce disk quotas.

## Configuration

| Option | Default | Meaning |
|--------|---------|---------|
| `log_level` | `info` | Process log verbosity |
| `mount_docker_sock` | `true` | Mount host Docker socket into *runner* containers (root-equivalent; disable if unused) |
| `runner_image` | `myoung34/github-runner:latest` | Docker image for new runners — default is [myoung34/github-runner](https://hub.docker.com/r/myoung34/github-runner); prefer a digest for pinning |
| `github_pat` | _(empty)_ | Optional PAT to mint registration tokens and deregister on delete |

### PAT scopes

Classic: `repo` for repository runners; org runner admin / `admin:org` for organization runners. Fine-grained tokens need Actions runner administration on the target.

## Consuming repositories (Actions secrets)

This manager’s short-lived container env **`RUNNER_TOKEN`** is a **registration** token (minted from the app PAT or pasted once). It is used only during configure and is **not** an Actions repository secret.

Repos that prefer a managed self-hosted runner when online (then fall open to `ubuntu-latest`) need a **different** credential in **Settings → Secrets and variables → Actions**:

| Actions secret | Used for | Recommended scope |
|----------------|----------|-------------------|
| **`RUNNER_TOKEN`** | Workflow job that lists runners via the GitHub API (`repos/…/actions/runners`) | Fine-grained **Administration: Read** on that repo (or classic **`repo`**) |
| Private-module / BuildKit tokens (e.g. **`GO_PRIVATE_TOKEN`**) | `go mod` / `GIT_AUTH_TOKEN` for other private repos | **Contents: Read** on those sibling repos only |

Keep those secrets separate so rotating runner-list access cannot break private module fetch. `GITHUB_TOKEN` cannot list self-hosted runners.

Same-path workdir bind (default `/srv/gha-work/<name>`) is still required for jobs that `docker run -v $GITHUB_WORKSPACE` — see [0004](../docs/features/0004-sibling-docker-workdir-host-bind.md).

## Notes

- Each runner runs as a separate Docker container (not inside the addon process).
- Deleting a runner removes the local container and registration volume; host workdir and cache bind paths are never deleted. A shared cache volume is removed only when no other runner references it. With a PAT, delete also attempts GitHub deregistration.
- Access is network-trust via Home Assistant ingress — do not expose the app on a public network without a reverse proxy you control.
- Prefer pinning `runner_image` to a digest in production.

## Credits

Runner containers use **[myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner)** (`myoung34/github-runner`) by Matt Young. This addon orchestrates that image; it does not replace the Actions runner agent. See the [repository README credits](https://github.com/dchote/github-runner-addon#credits-and-attribution).

## Image source (this addon)

- **Default:** Supervisor pulls `ghcr.io/dchote/github-runner-addon` (see `image` in `config.yaml`).
- **Local build:** Comment out `image:`, then **Rebuild** so Supervisor builds this directory’s `Dockerfile`.

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
5. Monitor status in the table; use **Edit**, **Logs**, **Recreate**, and lifecycle actions as needed.

Registration tokens are passed to the runner container only until registration succeeds (then scrubbed from container env). They are not stored in `runners.json`. The PAT is never written into runner containers.

## Persistence

Runner expected configuration is stored at `/data/runners.json` (Home Assistant’s standard app data directory). That path is always mounted, included in HA backups, and does not require extra `map:` entries.

Registration credentials live in a Docker named volume per runner (`*-data`). Job workdir is a **host directory** same-path bind (default `/srv/gha-work/<name>`) so sibling `docker run -v $GITHUB_WORKSPACE` works with the Docker socket — not a Docker volume `_data` path. Optional **persistent cache** (named volume or host bind at `/cache`) is separate. Large volumes are **not** part of HA `/data` backups.

**Recreate / Save & apply:** remounts the host workdir and sets `RUNNER_WORKDIR`. If the agent `.runner` `workFolder` does not match, the addon clears `.runner` and reconfigures (PAT or registration token required). Env alone never moves the agent workdir. Deleting a runner does **not** delete the host workdir tree.

Prepare host paths (example):

```bash
sudo mkdir -p /srv/gha-work/my-runner /srv/runner-cache
sudo chown -R 1000:1000 /srv/gha-work/my-runner /srv/runner-cache
```

For non-root runner images, ensure those paths are writable by the runner uid, or enable **Read-only cache** if workflows only need to read. Prune unused Docker volumes / host trees periodically — the addon does not enforce disk quotas.

## Configuration

| Option | Default | Meaning |
|--------|---------|---------|
| `log_level` | `info` | Process log verbosity |
| `mount_docker_sock` | `true` | Mount host Docker socket into *runner* containers (root-equivalent; disable if unused) |
| `runner_image` | `myoung34/github-runner:latest` | Docker image for new runners — default is [myoung34/github-runner](https://hub.docker.com/r/myoung34/github-runner); prefer a digest for pinning |
| `github_pat` | _(empty)_ | Optional PAT to mint registration tokens and deregister on delete |

### PAT scopes

Classic: `repo` for repository runners; org runner admin / `admin:org` for organization runners. Fine-grained tokens need Actions runner administration on the target.

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

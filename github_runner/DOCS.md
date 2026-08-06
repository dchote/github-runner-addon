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
4. Optionally set labels, CPU/memory limits, and other runtime fields.
5. Monitor status in the table; use **Edit**, **Logs**, **Recreate**, and lifecycle actions as needed.

Registration tokens are passed to the runner container only until registration succeeds (then scrubbed from container env). They are not stored in `runners.json`. The PAT is never written into runner containers.

## Persistence

Runner expected configuration is stored at `/data/runners.json` (Home Assistant’s standard app data directory). That path is always mounted, included in HA backups, and does not require extra `map:` entries.

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
- Deleting a runner removes the local container and volume; with a PAT it also attempts GitHub deregistration.
- Access is network-trust via Home Assistant ingress — do not expose the app on a public network without a reverse proxy you control.
- Prefer pinning `runner_image` to a digest in production.

## Credits

Runner containers use **[myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner)** (`myoung34/github-runner`) by Matt Young. This addon orchestrates that image; it does not replace the Actions runner agent. See the [repository README credits](https://github.com/dchote/github-runner-addon#credits-and-attribution).

## Image source (this addon)

- **Default:** Supervisor pulls `ghcr.io/dchote/github-runner-addon` (see `image` in `config.yaml`).
- **Local build:** Comment out `image:`, then **Rebuild** so Supervisor builds this directory’s `Dockerfile`.

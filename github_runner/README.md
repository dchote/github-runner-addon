# GitHub Runner Manager

Home Assistant app that manages multiple GitHub Actions self-hosted runners as Docker containers. It is a **control plane** (UI + API): each runner runs in a separate container, by default using [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner).

## Install

1. **Settings → Add-ons → Add-on store → ⋮ → Repositories**
2. Add `https://github.com/dchote/github-runner-addon`
3. Install **GitHub Runner Manager**
4. **Disable Protection mode** for this app (required for `docker_api`)
5. Start the app and open the UI via ingress

By default the Supervisor **pulls** the pre-built image from GHCR (`image` in `config.yaml`).

### Build from source on the HA host

1. Edit this app’s `config.yaml` and **comment out** the `image:` line.
2. Save, then click **Rebuild** on the app page.

Building locally compiles Node + Go inside Docker and is slower; use it for development or when GHCR is unavailable.

## Configuration

See [DOCS.md](DOCS.md) for options, PAT scopes, and security notes.

## Credits

Default runner image: **[myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner)** by Matt Young. Full attribution: [repository README](../README.md#credits-and-attribution).

## Documentation

- [DOCS.md](DOCS.md) — Supervisor-facing usage
- [Repository README](../README.md) — product overview, standalone Compose, API
- [Product overview](../docs/product-overview.md)
- [Technical overview](../docs/technical-overview.md)

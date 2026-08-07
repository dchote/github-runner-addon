# Product Overview

**GitHub Runner Manager** is a local control plane for managing **multiple GitHub Actions self-hosted runners** as Docker containers. It runs as a Home Assistant addon (or standalone binary) and presents a simple Vue + Vuetify operator UI.

It is aimed at home-lab and small-team operators who want persistent self-hosted CI **without** Kubernetes or Actions Runner Controller. The manager owns fleet configuration and lifecycle; each runner process runs inside a community Docker image — see [Credits](#credits).

## Product Goals

- Register and run self-hosted runners without manually downloading the Actions runner tarball or running `config.sh` on the host.
- Support **many runners for different GitHub projects** (repo or organization URLs).
- Persist expected runner configuration in a JSON file; reconcile runtime state from Docker.
- Stream runner container logs for day-2 diagnostics.
- Optional **personal access token (PAT)** to mint registration tokens and deregister runners from GitHub on delete.
- Recreate missing containers from stored config; update labels and runtime limits from the UI.
- Ship as a **single Go binary** with an embedded SPA.
- Ship as a Home Assistant app: pull from GHCR by default, or build from source on the HA host.

## Target Users

- Home lab operators running CI on Home Assistant OS or a Docker host.
- Teams that need a few persistent self-hosted runners without Kubernetes.

## Core Concepts

- **Manager**: this project’s Go process (API + UI) — the control plane.
- **Runner container**: a sibling Docker container based on [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner) (configurable), registered to a GitHub URL with a short-lived registration token (minted via PAT or pasted manually).
- **Expected config**: records in `/data/runners.json` (name, URL, labels, container/volume names, runtime limits, optional cache/workdir persistence). Registration tokens and PATs are not stored in JSON; registration tokens are passed to the container env only until registration succeeds.
- **Persistent cache**: optional named volume or host bind (default `/cache`) for incremental CI; share across runners by reusing the same volume name or host path.

## Core User Flows

1. **Create runner** — enter name, GitHub project URL, and either a registration token or rely on a configured PAT; optional labels, runtime limits, and persistent cache / workdir.
2. **Monitor** — table of local runners with Docker status; start / stop / restart / recreate / delete; orphan-container warnings when present.
3. **Edit** — update labels, image, resources, env, docker-sock override, optional persistent cache / workdir; optionally apply by recreating the container.
4. **Logs** — view and follow the runner container’s stdout/stderr.
5. **Delete** — remove local container, registration volume, and workdir volume; shared cache volumes only when unreferenced. With PAT, also deregister from GitHub when possible.

## Non-Goals

- Autoscaling / ephemeral runner pools (ARC-style scalesets).
- GitHub App JWT installation auth (classic/fine-grained PAT only).
- Application-level API keys / RBAC (network trust: HA ingress or private network; do not expose the UI publicly).
- Full host OS management.
- Job history / workflow correlation from the GitHub API.
- Replacing or forking the upstream runner image — we orchestrate [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner).

## Security boundary

Access control is **network trust**: Home Assistant ingress, or binding the standalone listener to localhost / a private network. There is no login inside the addon.

## Credits

Runner workloads use **[myoung34/docker-github-actions-runner](https://github.com/myoung34/docker-github-actions-runner)** (`myoung34/github-runner` on Docker Hub) by Matt Young. This project is a manager around that image, not a reimplementation of the Actions runner agent. See the [repository README](../README.md#credits-and-attribution) for full attribution.

## Release Phasing

1. Skeleton: embedded UI, health, Docker ping, runners CRUD + logs. *(shipped)*
2. Hardening: PAT registration/deregistration, recreate/reconcile, runtime controls, labels UX, tests. *(shipped in 0.2.0)*

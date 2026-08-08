# 0002 Hardened Persistent Fleet

## Status

Implemented — close-out of the v1 fleet manager into a robust persistent-runner control plane (0.2.0).

## Goal

Harden the local control plane for **persistent** Docker-based GitHub Actions self-hosted runners: optional PAT registration and deregistration, reconcile/recreate, post-create config, security hardening, UI polish, and tests. No autoscaling or app-level API auth.

Runner agents continue to run in the configured container image (default [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner) — see [README credits](../../README.md#credits-and-attribution)).

## Scope

### In scope

- Optional `GITHUB_PAT` / HA option `github_pat` to mint registration tokens and deregister on delete
- Manual registration token still supported when PAT is unset or overridden
- Store `Update`, schema version, startup/periodic reconcile
- `POST /runners/{id}/recreate`, `PATCH /runners/{id}` (labels / runtime fields)
- Per-runner image override, CPU/memory limits, extra env, docker-sock override
- Configure-only then tokenless run so `RUNNER_TOKEN` never remains on the long-running container
- Network-trust auth (HA ingress or private network); compose binds localhost
- Enriched health, rate limits, sanitized errors
- UI: status summary, PAT-aware create, recreate, settings read-only, logs download/reconnect

### Out of scope

- Autoscaling / ephemeral pools / ARC-style scalesets
- GitHub App JWT (PAT only)
- App-level API keys / RBAC
- Job history / workflow correlation
- Full host OS management

## Auth model

Network trust only. Do not expose `:8099` on a public interface. Home Assistant ingress or a private Docker network is the security boundary.

## Contract notes

- Registration tokens are never stored in `runners.json`.
- PAT is never written into runner containers.
- Delete with PAT: best-effort GitHub deregister, then local container/volume removal (local delete proceeds after warn if GitHub fails).
- Recreate keeps the named volume when possible. When the data volume is **missing** or workdir reconfigure is required, a registration token or configured PAT is **required**. When registration files on the volume are reusable, recreate starts without a token (no mint/scrub) so GitHub sessions are not interrupted.
- Operators can **edit** labels and runtime fields in the UI (`PATCH`); Save & apply recreates the container.
- Orphan managed containers are listed in the UI from health.

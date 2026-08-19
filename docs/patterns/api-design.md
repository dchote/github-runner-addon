# API Design

Prefix: `/api/v1`

Auth: **network trust** (Home Assistant ingress or private bind). Do not expose publicly.

This API belongs to the **manager** (control plane). Runner workloads themselves run in Docker containers (default [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner)).

## Envelope

Success:

```json
{ "result": "ok", "data": { } }
```

Error:

```json
{
  "result": "error",
  "error": { "code": "VALIDATION_ERROR", "message": "…", "details": {} }
}
```

## Common codes

| Code | HTTP | Meaning |
|------|------|---------|
| `VALIDATION_ERROR` | 400 | Bad request body / fields |
| `NOT_FOUND` | 404 | Unknown runner id |
| `CONFLICT` | 409 | Duplicate display name or normalized container name |
| `RUNNER_BUSY` | 409 | Recreate/delete/apply while a **running** runner is busy, or idle cannot be verified |
| `RATE_LIMITED` | 429 | Too many create/recreate requests (quota is not consumed by 400/404) |
| `GITHUB_ERROR` | 502 | GitHub API failure (PAT mint / related) |
| `IMAGE_PULL_ERROR` | 502 | Registry/network failure pulling a runner image |
| `DOCKER_UNAVAILABLE` | 503 | Docker socket/API unreachable (including inspect failures on the busy gate) |
| `INTERNAL_ERROR` | 500 | Unexpected failure |

## Endpoints (fleet)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/health` | Docker, store, runner counts, orphans, `github_pat_configured`, `runner_image`, `mount_docker_sock`, version; `status` is `ok` or `degraded` (HTTP stays 200) |
| GET/POST | `/runners` | List / create (`token` optional when PAT set) |
| POST | `/runners/recreate-missing` | Operator fleet restore; one rate-limit token; sequential recreate; `{recreated, failed[]}` |
| GET/PATCH/DELETE | `/runners/{id}` | PATCH updates labels/runtime; `apply: true` recreates **then** persists; `reset_mount_docker_sock` clears override |
| POST | `/runners/{id}/start\|stop\|restart\|recreate` | Lifecycle; start/restart/logs on a missing container are **400** (recreate it); recreate requires token/PAT if volume missing or empty (no `.runner`) |
| GET | `/runners/{id}/logs` | Plain-text stream (`follow`, `tail`) |

## WebSocket

`GET /ws` (upgrade). Client messages:

```json
{ "type": "subscribe", "channel": "container_logs", "runner_id": "<id>", "tail": "200" }
{ "type": "unsubscribe", "channel": "container_logs" }
```

Server messages: `{ "type": "log_line", "channel": "container_logs", "runner_id", "line" }` or `{ "type": "error", … }`.

Origin checks: empty Origin, same Host, or Origin matching `X-Forwarded-Host`. Ingress headers (`X-Ingress-Path` / `X-Hass-Source`) alone never allow an arbitrary Origin. Log lines and errors are token-redacted.

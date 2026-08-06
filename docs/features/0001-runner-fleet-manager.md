# 0001 Runner Fleet Manager

## Status

Implemented: create / list / lifecycle / logs for Docker-based GitHub runners.

## Goal

Manage multiple self-hosted runners for different GitHub projects from one UI, without hosting `config.sh` on the appliance filesystem. Runner processes run in Docker containers (default image: [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner); see [README credits](../../README.md#credits-and-attribution)).

## Contract

### Create

`POST /api/v1/runners`

```json
{ "name": "lab-1", "url": "https://github.com/owner/repo", "token": "A…", "labels": ["self-hosted","linux"] }
```

- Registration token is **not** written to `runners.json`.
- Token is passed to the runner container as `RUNNER_TOKEN` (needed for `config.sh` inside the image).
- URL with one path segment → org scope; exactly two → repo scope.

### List

`GET /api/v1/runners` returns expected config plus normalized Docker `status` (`running`, `exited`, `missing`, `unknown`).

### Lifecycle

`POST …/start|stop|restart`, `DELETE …/{id}` (removes container + volume; with PAT also best-effort GitHub deregister — see `0002-hardened-persistent-fleet.md`).

### Logs

- `GET …/logs?follow=1` — plain-text chunked stream (demultiplexed stdout/stderr).
- WebSocket `/ws` channel `container_logs` — primary live-follow path used by the UI.

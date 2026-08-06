# Build and Test

For what this project is and how runners relate to [`myoung34/github-runner`](https://github.com/myoung34/docker-github-actions-runner), see the [repository README](../README.md).

## Full binary build (embedded frontend)

```bash
make build
# produces build/github-runner-addon
```

Requires Node 24+ and Go 1.25+. Application sources live under `github_runner/`.

## Skip frontend

```bash
SKIP_FRONTEND=true ./scripts/build.sh
```

## Split-process development

```bash
cd github_runner && go run ./cmd/github-runner-addon -frontend-embed=false
cd github_runner/frontend && npm ci && npm run dev   # proxies /api and /ws to :8099
```

## Tests / lint

```bash
make test          # go test ./cmd/... ./internal/... ./api/...
make vet
make lint          # frontend eslint
cd github_runner/frontend && npm test   # vitest (api URL helpers)
make check         # vet + test
```

CI runs Go vet/test and frontend lint/unit tests before image builds (`.github/workflows/builder.yaml`).

## Home Assistant app image

Build the same image Supervisor would build (context = `github_runner/`):

```bash
make docker-build
# docker build -t ghcr.io/dchote/github-runner-addon:local ./github_runner
```

Standalone run (localhost bind):

```bash
docker compose up --build
# http://127.0.0.1:8099
```

### GHCR vs local Supervisor build

| Mode | How |
|------|-----|
| Pull from GHCR (default) | Keep `image: ghcr.io/dchote/github-runner-addon` in `github_runner/config.yaml` |
| Build on HA host | Comment out `image:`, then **Rebuild** in the Supervisor UI |

CI (`.github/workflows/builder.yaml`) publishes multi-arch images to GHCR using the official Home Assistant BuildKit actions.

## Deprecated

`build.yaml` is not used — base images and labels live in `github_runner/Dockerfile` per the [2026 builder migration](https://developers.home-assistant.io/blog/2026/04/02/builder-migration).

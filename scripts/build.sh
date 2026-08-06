#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_DIR="$ROOT/github_runner"
cd "$APP_DIR"

mkdir -p "$ROOT/build"

if [[ "${SKIP_FRONTEND:-}" != "true" && "${SKIP_FRONTEND:-}" != "1" ]]; then
  echo "==> Building frontend"
  # Prefer a clean install; if rm fails (busy mounts), npm ci still refreshes the tree.
  (cd frontend && rm -rf node_modules 2>/dev/null || true; npm ci && npm run build)
  rm -rf cmd/github-runner-addon/frontend-dist
  cp -r frontend/dist cmd/github-runner-addon/frontend-dist
  TAGS="embed_frontend"
else
  echo "==> Skipping frontend (SKIP_FRONTEND set)"
  TAGS=""
fi

echo "==> Building Go binary"
if [[ -n "$TAGS" ]]; then
  go build -tags "$TAGS" -ldflags "-s -w" -o "$ROOT/build/github-runner-addon" ./cmd/github-runner-addon
else
  go build -ldflags "-s -w" -o "$ROOT/build/github-runner-addon" ./cmd/github-runner-addon
fi

echo "==> Done: build/github-runner-addon"

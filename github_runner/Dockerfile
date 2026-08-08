# Home Assistant app image — build context is this directory (github_runner/).
# https://developers.home-assistant.io/docs/apps/configuration#app-dockerfile
#
# Local test (from repo root):
#   docker build -t ghcr.io/dchote/github-runner-addon:local ./github_runner
#
# Supervisor local build: comment out `image:` in config.yaml, then Rebuild.

ARG BUILD_FROM=ghcr.io/home-assistant/base:3.23

# -----------------------------------------------------------------------------
# Frontend
# -----------------------------------------------------------------------------
FROM node:24-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# -----------------------------------------------------------------------------
# Go binary
# -----------------------------------------------------------------------------
FROM golang:1.25-bookworm AS builder
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./cmd/github-runner-addon/frontend-dist
RUN go build -tags embed_frontend -ldflags "-s -w" -o /github-runner-addon ./cmd/github-runner-addon

# -----------------------------------------------------------------------------
# Runtime (Home Assistant base — multi-arch manifest)
# -----------------------------------------------------------------------------
FROM ${BUILD_FROM}

# Passed by HA builder / CI (config.yaml version). Local builds default to match DefaultVersion.
ARG BUILD_VERSION=0.5.1

COPY --from=builder /github-runner-addon /usr/bin/github-runner-addon
COPY rootfs /
# s6 services must be executable (HA base ENTRYPOINT is /init).
RUN chmod +x \
  /etc/services.d/github-runner/run \
  /etc/services.d/github-runner/finish

ENV HTTP_PORT=8099
ENV DATA_DIR=/data
ENV MOUNT_DOCKER_SOCK=false
ENV LOG_LEVEL=info
ENV RUNNER_IMAGE=myoung34/github-runner:latest
ENV APP_VERSION=${BUILD_VERSION}

WORKDIR /data

LABEL \
  org.opencontainers.image.title="GitHub Runner Manager" \
  org.opencontainers.image.description="Manage multiple GitHub Actions self-hosted runners as Docker containers" \
  org.opencontainers.image.source="https://github.com/dchote/github-runner-addon" \
  org.opencontainers.image.licenses="MIT" \
  org.opencontainers.image.version="${BUILD_VERSION}"

# No CMD — home-assistant/base ENTRYPOINT [/init] starts
# /etc/services.d/github-runner/run via s6-overlay.

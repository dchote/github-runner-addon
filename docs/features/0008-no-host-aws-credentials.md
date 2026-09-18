# 0008 Do not persist AWS access keys on runners

## Status

Implemented.

## Goal

Runner **extra_env** is stored in `runners.json` and included in Home Assistant `/data` backups. Cloud **access keys** must not live there. The manager also never bind-mounts host `~/.aws`.

This addon does **not** disable EC2 instance metadata (IMDS) and does **not** block `AWS_PROFILE` / `AWS_CONFIG_FILE`. Jobs that must not inherit a host or instance role set `AWS_EC2_METADATA_DISABLED` themselves.

## Contract

1. **`extra_env` rejects** (and `buildEnv` skips if already stored): `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_SECURITY_TOKEN`.
2. The manager **never bind-mounts** host `~/.aws` or other credential files. Mounts remain: registration volume, optional cache, workdir, optional Docker socket.
3. **Allowed** in extra_env: `AWS_DEFAULT_REGION`, `AWS_REGION`, `AWS_PROFILE`, `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`, `AWS_EC2_METADATA_DISABLED`, and any other non-reserved keys.

## Custom images (`:local`)

Tags ending in `:local` are never pulled. Build them on the Docker host, then **Recreate** the runner so it starts from the new image. Include whatever tools your workflows need (for example AWS CLI v2 if jobs run `aws`).

## Operator notes

- Do not put access keys in Extra environment. They would land in `runners.json` and `docker inspect`.
- Recreate existing runners after upgrading if you previously stored AWS access keys in extra_env (those keys are skipped even if still in the store).
- Workflows that mint temporary credentials should export them only in that job step and prefer a config file under `$RUNNER_TEMP`, not `~/.aws`.

## Related

- [0002 Hardened persistent fleet](0002-hardened-persistent-fleet.md)
- [Container runtime](../patterns/container-runtime.md)

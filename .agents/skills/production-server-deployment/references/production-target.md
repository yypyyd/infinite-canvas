# Production target

## Access

- Host: `38.76.212.24`
- SSH user: `root`
- SSH host key: `ssh-ed25519 SHA256:pxnLTpgrxoOI2dDESGMn7xpVq5QVcBjn/Wg9xFqz/mI`
- Supply authentication at runtime. Never store it in this repository or print it in command output.

## Runtime

- Public origin: `https://huantu.xyz`
- Application directory: `/opt/infinite-canvas`
- Runtime: manually managed Docker container named `infinite-canvas`
- Port mapping: `127.0.0.1:3001:3000`
- Reverse proxy: BaoTa-managed Nginx for `huantu.xyz`, proxying to `127.0.0.1:3001`
- Environment file: `/opt/infinite-canvas/.env`; inspect names and permissions only, never values
- Persistent data: `/opt/infinite-canvas/data:/app/data`
- Server-specific Compose file: `/opt/infinite-canvas/docker-compose.yml`; preserve its loopback port mapping and update only its image tag after a successful switch

## Release pattern

- Build an exact approved Git commit from a clean archive in `/opt/infinite-canvas-release-<short-sha>-<UTC timestamp>`.
- Tag images as `infinite-canvas:huantu-<short-sha>-<UTC timestamp>` and set the OCI revision label to the full commit SHA.
- Preserve the current source under `/opt/infinite-canvas-deploy-backups/source-pre-<short-sha>-<UTC timestamp>.tgz`, excluding `.env` and `data`.
- Start the production container with the existing environment file, persistent bind mount, loopback port, and `unless-stopped` restart policy.
- Record the successful full commit SHA in `/opt/infinite-canvas/.deployed-revision`.

## Verification

- Local health: `http://127.0.0.1:3001/api/health` must return HTTP 200.
- Public health: `https://huantu.xyz/api/health` must return HTTP 200.
- Personal center: `https://huantu.xyz/account` must return HTTP 200.
- Recheck the running image revision label and recent `infinite-canvas` logs after public requests.

## Restart and rollback

- Normal restart: `docker restart infinite-canvas`, followed by local and public health checks.
- During deployment, stop and rename the previous container to `infinite-canvas-rollback-<UTC timestamp>` before starting the replacement.
- To roll back a failed replacement, remove only the failed `infinite-canvas` container, rename the selected stopped rollback container to `infinite-canvas`, start it, and verify both health endpoints.

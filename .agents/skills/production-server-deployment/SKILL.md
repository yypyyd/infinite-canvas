---
name: production-server-deployment
description: Safely deploy this project to its production Linux server and verify or roll back the release. Use when the user asks to deploy, publish, update online, restart production, check a deployment, or roll back the service on the fixed production host.
---

# Production Server Deployment

Deploy an explicitly approved revision while preserving server configuration and persistent data. Discover the server's actual runtime before changing it; do not assume Docker, systemd, a directory, or a branch.

## Safety rules

- Read the repository `AGENTS.md` and deployment files before connecting.
- Target production only after the user explicitly requests deployment.
- Never write credentials, tokens, environment values, or private keys into this skill, repository files, shell history, logs, or the final response. Keep supplied secrets in memory only and redact command output if needed.
- Confirm the host is `38.76.212.24` and the login user is `root`; discover the application directory and runtime read-only on the first connection.
- Preserve `.env` files, databases, uploads, volumes, reverse-proxy configuration, certificates, and unrelated server files.
- Do not use destructive Git recovery commands, force-push, database resets, recursive cleanup, or Docker pruning.
- If the server worktree contains unknown changes, the target revision is ambiguous, disk space is unsafe, or rollback cannot be identified, stop before mutation and report the finding.

## Workflow

1. Inspect local readiness.
   - Record `git status`, the approved commit, its changed files, and relevant deployment manifests.
   - Confirm the requested feature is committed. Publish the exact approved commit when the server's deployment path pulls from Git.
   - Do not run tests or a local build unless the user explicitly requests them; production image or service construction required by the existing deployment mechanism is part of deployment.

2. Discover production read-only.
   - Check OS identity, free disk space, candidate project directories, running processes, containers, Compose projects, systemd units, listening ports, and reverse-proxy configuration.
   - Identify the current deployed revision or image, application directory, start/restart command, health endpoint, and persistent-data locations.
   - Do not print secret file contents. Inspect only filenames, permissions, and explicitly safe configuration fields.

3. Establish a rollback point.
   - Record the current commit, image tag or ID, service state, and runtime command.
   - Back up mutable data only when the release includes a schema or storage change. Never copy secrets into the repository.

4. Transfer the approved revision.
   - Prefer the server's established deployment mechanism and an immutable commit or image.
   - If Git transport is unavailable, transfer a clean archive of tracked release files. Exclude `.git`, credentials, local caches, dependencies, build output not required by the manifest, and user data.
   - Keep server-owned environment and persistent paths in place.

5. Rebuild or restart narrowly.
   - Use the discovered Compose, container, or systemd command for this application only.
   - Watch startup output and recent service logs. Do not alter unrelated services.

6. Verify before declaring success.
   - Confirm the process or container remains healthy and the deployed revision matches the approved revision.
   - Request `https://infinite-canvas-cpco.onrender.com/api/health` only when that domain still points to this server; otherwise use the server's actual production domain or local health endpoint.
   - Verify `/account` is served or produces the expected authentication redirect, not a 404 or 5xx response.
   - Recheck recent logs after HTTP verification.

7. Roll back on failure.
   - Restore the recorded commit or image and restart with the same runtime command.
   - Verify health after rollback and report both the failed revision and restored revision.

## First-deployment learning

After the first successful deployment, add a concise `references/production-target.md` containing only stable, non-secret facts: project directory, runtime type, service name, deploy/restart commands, health endpoint, persistent paths, and rollback command. Read that reference on later deployments and revalidate it against the server before mutation.

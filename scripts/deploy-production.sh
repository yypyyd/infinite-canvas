#!/bin/bash
set -euo pipefail

TAG="${1:-${TAG:-}}"
IMAGE="${INFINITE_CANVAS_IMAGE:-ghcr.io/yypyyd/infinite-canvas}:${TAG}"
LIVE_NAME="${LIVE_NAME:-infinite-canvas}"
ROLLBACK_NAME="${ROLLBACK_NAME:-infinite-canvas-rollback}"
VIDEO_WORKER_NAME="${VIDEO_WORKER_NAME:-${LIVE_NAME}-video-worker}"
BATCH_WORKER_NAME="${BATCH_WORKER_NAME:-${LIVE_NAME}-batch-worker}"
BIND="${BIND:-127.0.0.1:3050:3000}"
ENV_FILE="${ENV_FILE:-/opt/infinite-canvas/.env}"
DATA_DIR="${DATA_DIR:-/opt/infinite-canvas/data}"
BACKUP_ROOT="${BACKUP_ROOT:-/opt/infinite-canvas/deploy-backups}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:3050/api/health/ready}"
# Production always runs claim workers against the same image/env/data as the API.
# Set START_BATCH_WORKER=0 to skip batch-worker; video-worker is required for render jobs.
START_VIDEO_WORKER="${START_VIDEO_WORKER:-1}"
START_BATCH_WORKER="${START_BATCH_WORKER:-1}"

if [[ -z "$TAG" || "$TAG" == *"/"* || "$TAG" == *" "* ]]; then
  echo "usage: $0 <image-tag>" >&2
  exit 1
fi
if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi
if [[ ! -d "$DATA_DIR" ]]; then
  echo "missing data dir: $DATA_DIR" >&2
  exit 1
fi

if [[ -n "${GHCR_READ_TOKEN:-}" ]]; then
  printf '%s' "$GHCR_READ_TOKEN" | docker login ghcr.io -u "${GHCR_USERNAME:-yypyyd}" --password-stdin >/dev/null
fi

echo "pull $IMAGE"
docker pull "$IMAGE"

stamp="$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_ROOT/$stamp"
if [[ -f "$DATA_DIR/infinite-canvas.db" ]]; then
  cp -a "$DATA_DIR/infinite-canvas.db" "$BACKUP_ROOT/$stamp/"
  if [[ -f "$DATA_DIR/infinite-canvas.db-wal" ]]; then
    cp -a "$DATA_DIR/infinite-canvas.db-wal" "$BACKUP_ROOT/$stamp/"
  fi
fi
ls -1dt "$BACKUP_ROOT"/*/ 2>/dev/null | tail -n +3 | xargs -r rm -rf

live=""
if docker inspect "$LIVE_NAME" >/dev/null 2>&1; then
  live="$LIVE_NAME"
else
  live="$(docker ps --filter publish=3050 --format '{{.Names}}' | head -n 1 || true)"
fi

if [[ -n "$live" ]]; then
  echo "stop $live"
  docker stop "$live" >/dev/null
  if docker inspect "$ROLLBACK_NAME" >/dev/null 2>&1; then
    docker rm -f "$ROLLBACK_NAME" >/dev/null
  fi
  if [[ "$live" != "$ROLLBACK_NAME" ]]; then
    docker rename "$live" "$ROLLBACK_NAME"
  fi
fi

cleanup_failed_start() {
  docker rm -f "$LIVE_NAME" >/dev/null 2>&1 || true
  if docker inspect "$ROLLBACK_NAME" >/dev/null 2>&1; then
    docker rename "$ROLLBACK_NAME" "$LIVE_NAME"
    docker start "$LIVE_NAME" >/dev/null
    echo "rolled back to $LIVE_NAME" >&2
  fi
}

# Recreate a sibling worker container on the same image/env/data as the API.
# Workers share the SQLite (or postgres) volume so they claim the same job tables.
start_worker() {
  local name="$1"
  local binary="$2"
  echo "start worker $name -> $binary"
  docker rm -f "$name" >/dev/null 2>&1 || true
  if ! docker run -d \
    --name "$name" \
    --restart unless-stopped \
    --health-cmd "grep -qx '${binary##*/}' /proc/1/comm" \
    --health-interval 10s \
    --health-timeout 3s \
    --health-start-period 5s \
    --health-retries 3 \
    --env-file "$ENV_FILE" \
    -v "$DATA_DIR:/app/data" \
    "$IMAGE" \
    "$binary"; then
    echo "failed to start worker $name" >&2
    return 1
  fi
  # Workers have no HTTP listener. Check their own PID 1 instead of inheriting
  # the API health probe, and do not report a deployment success after a crash.
  for _ in $(seq 1 20); do
    if [[ "$(docker inspect -f '{{.State.Running}}' "$name")" != "true" ]]; then
      echo "worker exited during startup: $name" >&2
      return 1
    fi
    if [[ "$(docker inspect -f '{{.State.Health.Status}}' "$name")" == "healthy" ]]; then
      return 0
    fi
    sleep 2
  done
  echo "worker health check failed: $name" >&2
  return 1
}

if ! docker run -d \
  --name "$LIVE_NAME" \
  --restart unless-stopped \
  --env-file "$ENV_FILE" \
  -e PORT=3000 \
  -v "$DATA_DIR:/app/data" \
  -p "$BIND" \
  "$IMAGE"; then
  cleanup_failed_start
  echo "container failed to start" >&2
  exit 1
fi

ready=0
for _ in $(seq 1 40); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "$HEALTH_URL" || true)"
  if [[ "$code" == "200" ]]; then
    ready=1
    break
  fi
  sleep 2
done

if [[ "$ready" != "1" ]]; then
  docker logs --tail 80 "$LIVE_NAME" || true
  cleanup_failed_start
  echo "health check failed: $HEALTH_URL" >&2
  exit 1
fi

# Start claim workers only after the API is healthy so a bad image never
# leaves orphan workers on a failed roll-forward. Same env + /app/data volume
# as the API so workers claim the same SQLite/Postgres job tables.
workers_ok=1
case "$(printf '%s' "$START_VIDEO_WORKER" | tr '[:upper:]' '[:lower:]')" in
  0|false|no|off) echo "skip video-worker (START_VIDEO_WORKER=$START_VIDEO_WORKER)" ;;
  *)
    if ! start_worker "$VIDEO_WORKER_NAME" "/app/video-worker"; then
      workers_ok=0
    fi
    ;;
esac
case "$(printf '%s' "$START_BATCH_WORKER" | tr '[:upper:]' '[:lower:]')" in
  0|false|no|off) echo "skip batch-worker (START_BATCH_WORKER=$START_BATCH_WORKER)" ;;
  *)
    if ! start_worker "$BATCH_WORKER_NAME" "/app/batch-worker"; then
      workers_ok=0
    fi
    ;;
esac
if [[ "$workers_ok" != "1" ]]; then
  echo "worker start failed; API is up but claim workers need attention" >&2
  docker logs --tail 40 "$VIDEO_WORKER_NAME" 2>/dev/null || true
  docker logs --tail 40 "$BATCH_WORKER_NAME" 2>/dev/null || true
  exit 1
fi

keep_images="$(
  docker inspect -f '{{.Image}}' "$LIVE_NAME" 2>/dev/null || true
  docker inspect -f '{{.Image}}' "$ROLLBACK_NAME" 2>/dev/null || true
  docker inspect -f '{{.Image}}' "$VIDEO_WORKER_NAME" 2>/dev/null || true
  docker inspect -f '{{.Image}}' "$BATCH_WORKER_NAME" 2>/dev/null || true
)"
while read -r image_id image_ref; do
  [[ -z "$image_id" ]] && continue
  case "$keep_images" in
    *"$image_id"*) ;;
    *)
      echo "rmi $image_ref"
      docker rmi "$image_ref" >/dev/null || true
      ;;
  esac
done < <(docker images --format '{{.ID}} {{.Repository}}:{{.Tag}}' | awk '$2 ~ /(^|\/)infinite-canvas(:|$)/')

printf '%s\n' "$TAG" > /opt/infinite-canvas/.deployed-revision
echo "DEPLOY_OK $LIVE_NAME $IMAGE"
echo "WORKERS_OK video=$VIDEO_WORKER_NAME batch=$BATCH_WORKER_NAME"

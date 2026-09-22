#!/bin/bash
set -euo pipefail

TAG="${1:-${TAG:-}}"
IMAGE="${INFINITE_CANVAS_IMAGE:-ghcr.io/yypyyd/infinite-canvas}:${TAG}"
LIVE_NAME="${LIVE_NAME:-infinite-canvas}"
ROLLBACK_NAME="${ROLLBACK_NAME:-infinite-canvas-rollback}"
BIND="${BIND:-127.0.0.1:3050:3000}"
ENV_FILE="${ENV_FILE:-/opt/infinite-canvas/.env}"
DATA_DIR="${DATA_DIR:-/opt/infinite-canvas/data}"
BACKUP_ROOT="${BACKUP_ROOT:-/opt/infinite-canvas/deploy-backups}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:3050/api/health/ready}"

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

keep_images="$(
  docker inspect -f '{{.Image}}' "$LIVE_NAME" 2>/dev/null || true
  docker inspect -f '{{.Image}}' "$ROLLBACK_NAME" 2>/dev/null || true
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

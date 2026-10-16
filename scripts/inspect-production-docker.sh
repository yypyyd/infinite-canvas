#!/bin/bash
set -euo pipefail

printf '\nPRODUCTION_HEALTH\n'
curl --fail --silent --show-error --max-time 15 http://127.0.0.1:3050/api/health/ready
printf '\nCONTAINERS\n'
docker ps -a --no-trunc --format '{{json .}}'
while read -r container_id; do
  [[ -n "$container_id" ]] || continue
  docker inspect --format '{{.Id}} {{.Name}} {{.Image}} {{.State.Status}} {{.State.StartedAt}}' "$container_id"
done < <(docker ps -aq --no-trunc)
printf '\nIMAGES\n'
docker image ls --no-trunc --digests --format '{{json .}}'
printf '\nDISK_USAGE\n'
docker system df
printf '\nVOLUME_NAMES\n'
docker volume ls --format '{{.Name}}'

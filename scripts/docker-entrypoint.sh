#!/usr/bin/env bash
# Container entrypoint: optional embedded PostgreSQL for the built-in account
# pool, then the Go API (8080, internal) and Next.js (3000).
#
# The account pool needs PostgreSQL. When 幻图 itself runs on SQLite (the
# default single-container deployment) and no POOL_POSTGRES_DSN is given, a
# private PostgreSQL is started inside this container. It listens only on a
# Unix socket and keeps its data in /app/data/pool-postgres, so the existing
# /app/data volume persists it. Set POOL_EMBEDDED_POSTGRES=false to skip it
# (the pool then stays off unless PostgreSQL is configured).
set -uo pipefail

lower() { printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]'; }

embedded_pg=0
case "$(lower "${POOL_ENABLED:-}")" in 0|false|no|off) ;; *)
  case "$(lower "${STORAGE_DRIVER:-}")" in postgres|postgresql) ;; *)
    if [[ -z "${POOL_POSTGRES_DSN:-}" ]]; then
      case "$(lower "${POOL_EMBEDDED_POSTGRES:-true}")" in 0|false|no|off) ;; *) embedded_pg=1 ;; esac
    fi
  ;; esac
;; esac

PGDATA="${POOL_EMBEDDED_PGDATA:-/app/data/pool-postgres}"
PGSOCK=/run/postgresql
PGBIN="$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)"

stop_pg() {
  if [[ "$embedded_pg" == 1 ]]; then
    runuser -u postgres -- "$PGBIN/pg_ctl" -D "$PGDATA" -m fast -w stop >/dev/null 2>&1 || true
  fi
}

if [[ "$embedded_pg" == 1 ]]; then
  if [[ -z "$PGBIN" ]]; then
    echo "embedded PostgreSQL not installed; account pool disabled" >&2
    embedded_pg=0
  else
    install -d -o postgres -g postgres -m 700 "$PGDATA"
    install -d -o postgres -g postgres -m 775 "$PGSOCK"
    if [[ ! -s "$PGDATA/PG_VERSION" ]]; then
      runuser -u postgres -- "$PGBIN/initdb" -D "$PGDATA" --auth-local=peer --auth-host=reject -E UTF8 --locale=C.UTF-8 >/dev/null || { echo "initdb failed" >&2; exit 1; }
    fi
    rm -f "$PGDATA/postmaster.pid"
    runuser -u postgres -- "$PGBIN/pg_ctl" -D "$PGDATA" -l "$PGDATA/server.log" -w \
      -o "-c listen_addresses='' -c unix_socket_directories=$PGSOCK -c jit=off" start >/dev/null || { tail -20 "$PGDATA/server.log" >&2; exit 1; }
    runuser -u postgres -- psql -h "$PGSOCK" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='huantu_pool'" | grep -q 1 \
      || runuser -u postgres -- createdb -h "$PGSOCK" huantu_pool
    # Peer auth over the socket: only the postgres OS user can connect, so
    # the Go server runs its pool connection as that role via the socket.
    runuser -u postgres -- psql -h "$PGSOCK" -d postgres -Atqc "DO \$\$BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='root') THEN CREATE ROLE root LOGIN; END IF; END\$\$; ALTER DATABASE huantu_pool OWNER TO root;" >/dev/null
    export POOL_POSTGRES_DSN="host=$PGSOCK user=root dbname=huantu_pool sslmode=disable"
    # Lets `docker exec ... /app/gateway-import` find the same database.
    printf '%s\n' "$POOL_POSTGRES_DSN" > "$PGSOCK/huantu-pool.dsn"
    echo "account pool: using embedded PostgreSQL in $PGDATA" >&2
  fi
fi

trap 'stop_pg' EXIT
trap 'kill -TERM $api $web 2>/dev/null; wait; exit 143' TERM INT

PORT=8080 /app/server & api=$!
(cd /app/web && PORT=3000 exec node server.js) & web=$!
wait -n
status=$?
echo "process exited with status $status" >&2
kill -TERM $api $web 2>/dev/null
exit $status

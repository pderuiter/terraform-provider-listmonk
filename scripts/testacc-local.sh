#!/usr/bin/env bash
# Local acceptance-test environment: PostgreSQL in Docker plus the listmonk
# release binary (scripts/listmonk-test-server.sh), the same setup CI uses.
#
#   scripts/testacc-local.sh up      start PostgreSQL + listmonk, write .testacc.env
#   scripts/testacc-local.sh test    up + run the acceptance tests + down
#   scripts/testacc-local.sh down    stop listmonk, remove PostgreSQL
#
# LISTMONK_VERSION (default v6.2.0), LISTMONK_PORT (default 9000),
# DB_PORT (default 55432), POSTGRES_IMAGE (default postgres:17-alpine).
# TESTARGS is passed to `go test -run`.
set -euo pipefail

cd "$(dirname "$0")/.."

export LISTMONK_VERSION="${LISTMONK_VERSION:-v6.2.0}"
export LISTMONK_PORT="${LISTMONK_PORT:-9000}"
export DB_PORT="${DB_PORT:-55432}"
POSTGRES_IMAGE="${POSTGRES_IMAGE:-postgres:17-alpine}"
PG_CONTAINER="tf-listmonk-testacc-db"
ENV_FILE=".testacc.env"

# PG_MODE=docker (default) runs PostgreSQL in a container; PG_MODE=native
# uses initdb/pg_ctl from the PATH with a throwaway cluster in .cache/.
PG_MODE="${PG_MODE:-docker}"
PG_DATA=".cache/testacc-pgdata"

down() {
  if [ -f "${ENV_FILE}" ]; then
    # shellcheck disable=SC1090
    . "./${ENV_FILE}"
    if [ -n "${LISTMONK_PID_FILE:-}" ] && [ -f "${LISTMONK_PID_FILE}" ]; then
      kill "$(cat "${LISTMONK_PID_FILE}")" 2>/dev/null || true
      rm -f "${LISTMONK_PID_FILE}"
    fi
    rm -f "${ENV_FILE}"
  fi
  if [ "${PG_MODE}" = native ]; then
    if [ -d "${PG_DATA}" ]; then
      pg_ctl -D "${PG_DATA}" -m immediate stop >/dev/null 2>&1 || true
      rm -rf "${PG_DATA}"
    fi
  else
    docker rm -f "${PG_CONTAINER}" >/dev/null 2>&1 || true
  fi
}

up() {
  down
  if [ "${PG_MODE}" = native ]; then
    mkdir -p .cache
    initdb -D "${PG_DATA}" -U listmonk --auth=trust >/dev/null
    pg_ctl -D "${PG_DATA}" -l "${PG_DATA}.log" -w \
      -o "-p ${DB_PORT} -k /tmp -c listen_addresses=127.0.0.1" start >/dev/null
    createdb -h 127.0.0.1 -p "${DB_PORT}" -U listmonk listmonk
  else
    docker run -d --name "${PG_CONTAINER}" \
      -e POSTGRES_USER=listmonk -e POSTGRES_PASSWORD=listmonk -e POSTGRES_DB=listmonk \
      -p "127.0.0.1:${DB_PORT}:5432" --tmpfs /var/lib/postgresql/data \
      "${POSTGRES_IMAGE}" >/dev/null
    # The port accepts connections before the server is ready for queries.
    for i in $(seq 1 60); do
      docker exec "${PG_CONTAINER}" pg_isready -U listmonk -h 127.0.0.1 >/dev/null 2>&1 && break
      [ "$i" -eq 60 ] && { docker logs "${PG_CONTAINER}" | tail -20; exit 1; }
      sleep 1
    done
  fi
  DB_HOST=127.0.0.1 scripts/listmonk-test-server.sh
}

case "${1:-up}" in
  up) up ;;
  test)
    up
    trap down EXIT
    # shellcheck disable=SC1090
    . "./${ENV_FILE}"
    TF_ACC=1 go test ./internal/provider/ -v -count=1 -timeout 30m -run "${TESTARGS:-.}"
    ;;
  down) down ;;
  *) echo "usage: $0 up|test|down" >&2; exit 2 ;;
esac

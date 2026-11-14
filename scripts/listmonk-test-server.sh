#!/usr/bin/env bash
# Start a throwaway listmonk for acceptance tests, against an existing
# PostgreSQL. Used by CI (PostgreSQL as a job service) and by
# scripts/testacc-local.sh (PostgreSQL in Docker).
#
# Downloads the official release binary for this OS/arch, verifies it against
# the release checksums, installs the schema with a Super Admin API user and
# starts listmonk in the background. Writes the provider credentials to
# $ENV_FILE (default .testacc.env) for `source`.
#
# Env:
#   LISTMONK_VERSION   release tag, e.g. v6.2.0 (required)
#   LISTMONK_PORT      listen port (default 9000)
#   DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME   (default localhost:5432, listmonk/listmonk/listmonk)
#   LISTMONK_CACHE     where binaries are kept (default .cache/listmonk)
set -euo pipefail

: "${LISTMONK_VERSION:?set LISTMONK_VERSION, e.g. v6.2.0}"
LISTMONK_PORT="${LISTMONK_PORT:-9000}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-listmonk}"
DB_PASSWORD="${DB_PASSWORD:-listmonk}"
DB_NAME="${DB_NAME:-listmonk}"
LISTMONK_CACHE="${LISTMONK_CACHE:-.cache/listmonk}"
ENV_FILE="${ENV_FILE:-.testacc.env}"
API_USER="terraform"

ver="${LISTMONK_VERSION#v}"
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

dir="${LISTMONK_CACHE}/${ver}-${os}-${arch}"
bin="${dir}/listmonk"
if [ ! -x "${bin}" ]; then
  mkdir -p "${dir}"
  base="https://github.com/knadh/listmonk/releases/download/v${ver}"
  tarball="listmonk_${ver}_${os}_${arch}.tar.gz"
  echo "Downloading listmonk ${ver} (${os}/${arch})..."
  curl -fsSL -o "${dir}/${tarball}" "${base}/${tarball}"
  curl -fsSL -o "${dir}/checksums.txt" "${base}/listmonk_${ver}_checksums.txt"
  want=$(grep " ${tarball}\$" "${dir}/checksums.txt" | awk '{print $1}')
  if command -v sha256sum >/dev/null; then
    got=$(sha256sum "${dir}/${tarball}" | awk '{print $1}')
  else
    got=$(shasum -a 256 "${dir}/${tarball}" | awk '{print $1}')
  fi
  if [ -z "${want}" ] || [ "${want}" != "${got}" ]; then
    echo "checksum mismatch for ${tarball}: want '${want}', got '${got}'" >&2
    exit 1
  fi
  tar -xzf "${dir}/${tarball}" -C "${dir}" listmonk
  rm -f "${dir}/${tarball}"
fi

# listmonk reads LISTMONK_<section>__<key> environment variables; an empty
# --config skips config.toml.
export LISTMONK_app__address="127.0.0.1:${LISTMONK_PORT}"
export LISTMONK_db__host="${DB_HOST}"
export LISTMONK_db__port="${DB_PORT}"
export LISTMONK_db__user="${DB_USER}"
export LISTMONK_db__password="${DB_PASSWORD}"
export LISTMONK_db__database="${DB_NAME}"
export LISTMONK_db__ssl_mode="disable"
export LISTMONK_ADMIN_USER="admin"
export LISTMONK_ADMIN_PASSWORD="listmonk-admin"
export LISTMONK_ADMIN_API_USER="${API_USER}"

echo "Waiting for PostgreSQL at ${DB_HOST}:${DB_PORT}..."
for i in $(seq 1 60); do
  if (exec 3<>"/dev/tcp/${DB_HOST}/${DB_PORT}") 2>/dev/null; then break; fi
  [ "$i" -eq 60 ] && { echo "PostgreSQL not reachable" >&2; exit 1; }
  sleep 1
done

# The installer prints `export LISTMONK_ADMIN_API_TOKEN="..."` on stderr.
install_log="${dir}/install.log"
"${bin}" --install --idempotent --yes --config '' 2>"${install_log}" || {
  cat "${install_log}" >&2
  exit 1
}
token=$(sed -n 's/.*export LISTMONK_ADMIN_API_TOKEN="\([^"]*\)".*/\1/p' "${install_log}" | tail -1)
if [ -z "${token}" ]; then
  echo "no API token in the installer output (database not empty?):" >&2
  cat "${install_log}" >&2
  exit 1
fi

"${bin}" --config '' >"${dir}/listmonk.log" 2>&1 &
echo $! >"${dir}/listmonk.pid"

url="http://127.0.0.1:${LISTMONK_PORT}"
for i in $(seq 1 60); do
  if curl -fsS -o /dev/null -H "Authorization: token ${API_USER}:${token}" "${url}/api/health"; then break; fi
  [ "$i" -eq 60 ] && { tail -30 "${dir}/listmonk.log" >&2; echo "listmonk did not become healthy" >&2; exit 1; }
  sleep 1
done

cat >"${ENV_FILE}" <<EOF
export LISTMONK_URL=${url}
export LISTMONK_USERNAME=${API_USER}
export LISTMONK_API_TOKEN=${token}
export LISTMONK_VERSION=${LISTMONK_VERSION}
export LISTMONK_PID_FILE=${dir}/listmonk.pid
EOF
echo "listmonk ${LISTMONK_VERSION} is up at ${url} (pid $(cat "${dir}/listmonk.pid"), log ${dir}/listmonk.log)"

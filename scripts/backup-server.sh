#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SERVER_DIR="${REPO_ROOT}/dushengcdn_server"
BACKUP_DIR=""
MODE="auto"
COMPOSE_FILE=""
ENV_FILE=""
DATA_DIR=""
SQLITE_PATH=""
POSTGRES_SERVICE="postgres"
POSTGRES_DB=""
POSTGRES_USER=""

usage() {
  cat <<EOF
DuShengCDN Server Backup Helper

Usage:
  backup-server.sh [OPTIONS]

Options:
  --server-dir DIR          Server compose/source directory (default: REPO/dushengcdn_server)
  --backup-dir DIR          Backup output directory (default: SERVER_DIR/backups)
  --mode auto|postgres|sqlite
                            Backup database mode (default: auto)
  --compose-file FILE       Docker Compose file (default: SERVER_DIR/docker-compose.yaml)
  --env-file FILE           Docker Compose env file (default: SERVER_DIR/.env when present)
  --data-dir DIR            Server data directory to archive (default: SERVER_DIR/dushengcdn-data)
  --sqlite-path FILE        SQLite database file for sqlite mode (default: DATA_DIR/dushengcdn.db)
  --postgres-service NAME   Compose service that runs PostgreSQL (default: postgres)
  --postgres-db NAME        PostgreSQL database name (default: POSTGRES_DB from .env or dushengcdn)
  --postgres-user NAME      PostgreSQL user name (default: POSTGRES_USER from .env or dushengcdn)
  -h, --help                Show this help message

Behavior:
  1. Creates a timestamped backup directory under BACKUP_DIR
  2. In postgres mode, runs pg_dump inside the compose postgres service
  3. In sqlite mode, uses sqlite3 .backup when available, otherwise copies the db file
  4. Archives DATA_DIR when it exists
  5. Writes a manifest with paths and checksums

This script only creates backup files. It does not stop, restore, overwrite, or
delete any production data.
EOF
  exit 0
}

log() {
  echo "==> $*"
}

warn() {
  echo "Warning: $*" >&2
}

die() {
  echo "Error: $*" >&2
  exit 1
}

strip_quotes() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  if [[ "${value}" == \"*\" && "${value}" == *\" ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "${value}" == \'*\' && "${value}" == *\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s' "$value"
}

env_value() {
  local key="$1"
  local fallback="$2"
  local from_env="${!key:-}"
  if [[ -n "$from_env" ]]; then
    printf '%s' "$from_env"
    return
  fi
  if [[ -n "$ENV_FILE" && -f "$ENV_FILE" ]]; then
    local line
    line="$(grep -E "^[[:space:]]*${key}=" "$ENV_FILE" | tail -n 1 || true)"
    if [[ -n "$line" ]]; then
      strip_quotes "${line#*=}"
      return
    fi
  fi
  printf '%s' "$fallback"
}

abs_path() {
  local path="$1"
  if [[ "$path" == /* ]]; then
    printf '%s' "$path"
  else
    printf '%s/%s' "$(pwd)" "$path"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-dir) SERVER_DIR="$2"; shift 2 ;;
    --backup-dir) BACKUP_DIR="$2"; shift 2 ;;
    --mode) MODE="$2"; shift 2 ;;
    --compose-file) COMPOSE_FILE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --sqlite-path) SQLITE_PATH="$2"; shift 2 ;;
    --postgres-service) POSTGRES_SERVICE="$2"; shift 2 ;;
    --postgres-db) POSTGRES_DB="$2"; shift 2 ;;
    --postgres-user) POSTGRES_USER="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) die "unknown option: $1" ;;
  esac
done

case "$MODE" in
  auto|postgres|sqlite) ;;
  *) die "--mode must be one of: auto, postgres, sqlite" ;;
esac

SERVER_DIR="$(abs_path "$SERVER_DIR")"
[[ -d "$SERVER_DIR" ]] || die "server directory not found: $SERVER_DIR"
SERVER_DIR="$(cd "$SERVER_DIR" && pwd)"

COMPOSE_FILE="${COMPOSE_FILE:-${SERVER_DIR}/docker-compose.yaml}"
COMPOSE_FILE="$(abs_path "$COMPOSE_FILE")"
if [[ -z "$ENV_FILE" && -f "${SERVER_DIR}/.env" ]]; then
  ENV_FILE="${SERVER_DIR}/.env"
fi
if [[ -n "$ENV_FILE" ]]; then
  ENV_FILE="$(abs_path "$ENV_FILE")"
fi

BACKUP_DIR="${BACKUP_DIR:-${SERVER_DIR}/backups}"
BACKUP_DIR="$(abs_path "$BACKUP_DIR")"
DATA_DIR="${DATA_DIR:-${SERVER_DIR}/dushengcdn-data}"
DATA_DIR="$(abs_path "$DATA_DIR")"
POSTGRES_DB="${POSTGRES_DB:-$(env_value POSTGRES_DB dushengcdn)}"
POSTGRES_USER="${POSTGRES_USER:-$(env_value POSTGRES_USER dushengcdn)}"

if [[ -z "$SQLITE_PATH" ]]; then
  sqlite_env="$(env_value SQLITE_PATH /data/dushengcdn.db)"
  if [[ "$sqlite_env" == /data/* ]]; then
    SQLITE_PATH="${DATA_DIR}/${sqlite_env#/data/}"
  else
    SQLITE_PATH="$sqlite_env"
  fi
fi
SQLITE_PATH="$(abs_path "$SQLITE_PATH")"

mkdir -p "$BACKUP_DIR"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
DEST_DIR="${BACKUP_DIR}/${TIMESTAMP}"
mkdir -p "$DEST_DIR"

compose_cmd=(docker compose)
if [[ -n "$ENV_FILE" && -f "$ENV_FILE" ]]; then
  compose_cmd+=(--env-file "$ENV_FILE")
fi
if [[ -f "$COMPOSE_FILE" ]]; then
  compose_cmd+=(-f "$COMPOSE_FILE")
fi

postgres_available() {
  command -v docker >/dev/null 2>&1 || return 1
  [[ -f "$COMPOSE_FILE" ]] || return 1
  (cd "$SERVER_DIR" && "${compose_cmd[@]}" exec -T "$POSTGRES_SERVICE" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1)
}

backup_data_dir() {
  if [[ ! -d "$DATA_DIR" ]]; then
    warn "data directory not found, skipping archive: $DATA_DIR"
    return
  fi
  local archive="${DEST_DIR}/dushengcdn-data-${TIMESTAMP}.tar.gz"
  log "archiving data directory: $DATA_DIR"
  tar -czf "$archive" -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")"
}

backup_postgres() {
  command -v docker >/dev/null 2>&1 || die "docker is required for postgres mode"
  [[ -f "$COMPOSE_FILE" ]] || die "compose file not found: $COMPOSE_FILE"
  local dump_file="${DEST_DIR}/postgres-${POSTGRES_DB}-${TIMESTAMP}.sql"
  log "dumping PostgreSQL database ${POSTGRES_DB} from compose service ${POSTGRES_SERVICE}"
  if ! (cd "$SERVER_DIR" && "${compose_cmd[@]}" exec -T "$POSTGRES_SERVICE" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB") > "$dump_file"; then
    rm -f "$dump_file"
    die "pg_dump failed"
  fi
}

backup_sqlite() {
  [[ -f "$SQLITE_PATH" ]] || die "SQLite database file not found: $SQLITE_PATH"
  local sqlite_file="${DEST_DIR}/sqlite-${TIMESTAMP}.db"
  log "backing up SQLite database: $SQLITE_PATH"
  if command -v sqlite3 >/dev/null 2>&1; then
    if ! sqlite3 "$SQLITE_PATH" ".backup '${sqlite_file}'"; then
      warn "sqlite3 .backup failed; copying SQLite file directly"
      cp -p "$SQLITE_PATH" "$sqlite_file"
    fi
  else
    warn "sqlite3 not found; copying SQLite file directly"
    cp -p "$SQLITE_PATH" "$sqlite_file"
  fi
}

ACTUAL_MODE="$MODE"
if [[ "$MODE" == "auto" ]]; then
  if postgres_available; then
    ACTUAL_MODE="postgres"
  elif [[ -f "$SQLITE_PATH" ]]; then
    ACTUAL_MODE="sqlite"
  else
    die "auto mode found neither a reachable postgres compose service nor SQLite file"
  fi
fi

case "$ACTUAL_MODE" in
  postgres) backup_postgres ;;
  sqlite) backup_sqlite ;;
esac
backup_data_dir

MANIFEST="${DEST_DIR}/manifest.txt"
{
  echo "created_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "mode=${ACTUAL_MODE}"
  echo "server_dir=${SERVER_DIR}"
  echo "backup_dir=${DEST_DIR}"
  echo "compose_file=${COMPOSE_FILE}"
  echo "env_file=${ENV_FILE:-}"
  echo "data_dir=${DATA_DIR}"
  echo "sqlite_path=${SQLITE_PATH}"
  echo "postgres_service=${POSTGRES_SERVICE}"
  echo "postgres_db=${POSTGRES_DB}"
  echo "postgres_user=${POSTGRES_USER}"
  echo ""
  echo "files:"
  find "$DEST_DIR" -maxdepth 1 -type f ! -name manifest.txt -printf "  %f\n" | sort
  if command -v sha256sum >/dev/null 2>&1; then
    echo ""
    echo "sha256:"
    (cd "$DEST_DIR" && sha256sum ./* 2>/dev/null | grep -v "manifest.txt" || true)
  fi
} > "$MANIFEST"

log "backup complete: $DEST_DIR"
log "manifest: $MANIFEST"

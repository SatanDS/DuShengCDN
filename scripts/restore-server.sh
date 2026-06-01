#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SERVER_DIR="${REPO_ROOT}/dushengcdn_server"
BACKUP_PATH=""
MODE="auto"
COMPOSE_FILE=""
ENV_FILE=""
DATA_DIR=""
SQLITE_PATH=""
POSTGRES_SERVICE="postgres"
POSTGRES_DB=""
POSTGRES_USER=""
PRE_RESTORE_BACKUP_DIR=""
FORCE="false"
YES="false"
SKIP_CURRENT_BACKUP="false"
SKIP_DATA_DIR="false"
RESTORE_EXTRACT_DIR=""

usage() {
  cat <<EOF
DuShengCDN Server Restore Helper

Usage:
  restore-server.sh --backup-path DIR --yes [OPTIONS]

Options:
  --server-dir DIR              Server compose/source directory (default: REPO/dushengcdn_server)
  --backup-path DIR             Backup directory created by backup-server.sh
  --mode auto|postgres|sqlite   Restore database mode (default: manifest mode or auto-detect)
  --compose-file FILE           Docker Compose file (default: SERVER_DIR/docker-compose.yaml)
  --env-file FILE               Compose env file (default: SERVER_DIR/.env when present)
  --data-dir DIR                Server data directory to restore (default: SERVER_DIR/dushengcdn-data)
  --sqlite-path FILE            SQLite database path for sqlite mode (default: target .env or DATA_DIR/dushengcdn.db)
  --postgres-service NAME       Compose service that runs PostgreSQL (default: postgres)
  --postgres-db NAME            PostgreSQL database name (default: manifest, .env, or dushengcdn)
  --postgres-user NAME          PostgreSQL user name (default: manifest, .env, or dushengcdn)
  --pre-restore-backup-dir DIR  Safety backup output directory (default: SERVER_DIR/backups/pre-restore)
  --skip-current-backup         Do not make a safety backup before restore
  --skip-data-dir               Restore database only; do not restore dushengcdn-data archive
  --force                       Continue even when the Server running state cannot be kept safe
  --yes                         Confirm restore; required to overwrite current data
  -h, --help                    Show this help message

Behavior:
  1. Verifies the backup manifest checksums when possible
  2. Refuses to run unless --yes is provided
  3. Refuses to restore while the compose dushengcdn service is running unless --force is provided
  4. Creates a pre-restore safety backup of the current database/data directory unless skipped
  5. Restores the data directory archive first, then restores PostgreSQL or SQLite
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

manifest_value() {
  local key="$1"
  local fallback="$2"
  local manifest="${BACKUP_PATH}/manifest.txt"
  if [[ -f "$manifest" ]]; then
    local line
    line="$(grep -E "^${key}=" "$manifest" | tail -n 1 || true)"
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

first_match() {
  local pattern="$1"
  find "$BACKUP_PATH" -maxdepth 1 -type f -name "$pattern" | sort | head -n 1
}

cleanup_restore_extract_dir() {
  if [[ -n "${RESTORE_EXTRACT_DIR:-}" && -d "$RESTORE_EXTRACT_DIR" ]]; then
    case "$RESTORE_EXTRACT_DIR" in
      "${BACKUP_PATH}/.restore-data-"*) rm -rf "$RESTORE_EXTRACT_DIR" ;;
    esac
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-dir) SERVER_DIR="$2"; shift 2 ;;
    --backup-path) BACKUP_PATH="$2"; shift 2 ;;
    --mode) MODE="$2"; shift 2 ;;
    --compose-file) COMPOSE_FILE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --sqlite-path) SQLITE_PATH="$2"; shift 2 ;;
    --postgres-service) POSTGRES_SERVICE="$2"; shift 2 ;;
    --postgres-db) POSTGRES_DB="$2"; shift 2 ;;
    --postgres-user) POSTGRES_USER="$2"; shift 2 ;;
    --pre-restore-backup-dir) PRE_RESTORE_BACKUP_DIR="$2"; shift 2 ;;
    --skip-current-backup) SKIP_CURRENT_BACKUP="true"; shift ;;
    --skip-data-dir) SKIP_DATA_DIR="true"; shift ;;
    --force) FORCE="true"; shift ;;
    --yes) YES="true"; shift ;;
    -h|--help) usage ;;
    *) die "unknown option: $1" ;;
  esac
done

case "$MODE" in
  auto|postgres|sqlite) ;;
  *) die "--mode must be one of: auto, postgres, sqlite" ;;
esac

[[ -n "$BACKUP_PATH" ]] || die "--backup-path is required"
[[ "$YES" == "true" ]] || die "restore overwrites current data. Rerun with --yes after stopping Server and checking the backup path."

SERVER_DIR="$(abs_path "$SERVER_DIR")"
[[ -d "$SERVER_DIR" ]] || die "server directory not found: $SERVER_DIR"
SERVER_DIR="$(cd "$SERVER_DIR" && pwd)"

BACKUP_PATH="$(abs_path "$BACKUP_PATH")"
[[ -d "$BACKUP_PATH" ]] || die "backup directory not found: $BACKUP_PATH"
BACKUP_PATH="$(cd "$BACKUP_PATH" && pwd)"

COMPOSE_FILE="${COMPOSE_FILE:-${SERVER_DIR}/docker-compose.yaml}"
COMPOSE_FILE="$(abs_path "$COMPOSE_FILE")"
if [[ -z "$ENV_FILE" && -f "${SERVER_DIR}/.env" ]]; then
  ENV_FILE="${SERVER_DIR}/.env"
fi
if [[ -n "$ENV_FILE" ]]; then
  ENV_FILE="$(abs_path "$ENV_FILE")"
fi

DATA_DIR="${DATA_DIR:-${SERVER_DIR}/dushengcdn-data}"
DATA_DIR="$(abs_path "$DATA_DIR")"
PRE_RESTORE_BACKUP_DIR="${PRE_RESTORE_BACKUP_DIR:-${SERVER_DIR}/backups/pre-restore}"
PRE_RESTORE_BACKUP_DIR="$(abs_path "$PRE_RESTORE_BACKUP_DIR")"

POSTGRES_DB="${POSTGRES_DB:-$(manifest_value postgres_db "")}"
POSTGRES_DB="${POSTGRES_DB:-$(env_value POSTGRES_DB dushengcdn)}"
POSTGRES_USER="${POSTGRES_USER:-$(manifest_value postgres_user "")}"
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

compose_cmd=(docker compose)
if [[ -n "$ENV_FILE" && -f "$ENV_FILE" ]]; then
  compose_cmd+=(--env-file "$ENV_FILE")
fi
if [[ -f "$COMPOSE_FILE" ]]; then
  compose_cmd+=(-f "$COMPOSE_FILE")
fi

verify_manifest_checksums() {
  local manifest="${BACKUP_PATH}/manifest.txt"
  if [[ ! -f "$manifest" ]]; then
    warn "manifest.txt not found; checksum verification skipped."
    return
  fi
  if ! command -v sha256sum >/dev/null 2>&1; then
    warn "sha256sum not found; checksum verification skipped."
    return
  fi
  if ! grep -q "^sha256:" "$manifest"; then
    warn "manifest has no sha256 section; checksum verification skipped."
    return
  fi

  log "verifying backup checksums..."
  if ! (cd "$BACKUP_PATH" && awk 'found && NF >= 2 { print } /^sha256:$/ { found = 1; next }' manifest.txt | sha256sum -c - >/dev/null); then
    die "backup checksum verification failed"
  fi
}

server_service_running() {
  command -v docker >/dev/null 2>&1 || return 1
  [[ -f "$COMPOSE_FILE" ]] || return 1
  (cd "$SERVER_DIR" && "${compose_cmd[@]}" ps --status running --services 2>/dev/null || true) | grep -Fxq "dushengcdn"
}

ensure_server_stopped() {
  if [[ "$FORCE" == "true" ]]; then
    warn "--force was provided; Server running-state guard is bypassed."
    return
  fi
  if server_service_running; then
    die "compose service dushengcdn is still running. Stop it first with: cd ${SERVER_DIR} && docker compose stop dushengcdn"
  fi
}

backup_current_data_dir() {
  if [[ ! -d "$DATA_DIR" ]]; then
    warn "current data directory not found, safety archive skipped: $DATA_DIR"
    return
  fi
  log "creating safety archive for current data directory..."
  tar -czf "${SAFETY_DIR}/current-dushengcdn-data-${TIMESTAMP}.tar.gz" -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")"
}

backup_current_sqlite() {
  if [[ ! -f "$SQLITE_PATH" ]]; then
    warn "current SQLite database not found, safety copy skipped: $SQLITE_PATH"
    return
  fi
  log "creating safety copy for current SQLite database..."
  cp -p "$SQLITE_PATH" "${SAFETY_DIR}/current-sqlite-${TIMESTAMP}.db"
}

backup_current_postgres() {
  command -v docker >/dev/null 2>&1 || die "docker is required to safety-backup PostgreSQL before restore"
  [[ -f "$COMPOSE_FILE" ]] || die "compose file not found: $COMPOSE_FILE"
  log "creating safety dump for current PostgreSQL database..."
  if ! (cd "$SERVER_DIR" && "${compose_cmd[@]}" exec -T "$POSTGRES_SERVICE" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB") > "${SAFETY_DIR}/current-postgres-${POSTGRES_DB}-${TIMESTAMP}.sql"; then
    rm -f "${SAFETY_DIR}/current-postgres-${POSTGRES_DB}-${TIMESTAMP}.sql"
    die "current PostgreSQL safety dump failed. Fix PostgreSQL access or rerun with --skip-current-backup only if you accept the risk."
  fi
}

write_safety_manifest() {
  {
    echo "created_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "restore_backup_path=${BACKUP_PATH}"
    echo "restore_mode=${ACTUAL_MODE}"
    echo "server_dir=${SERVER_DIR}"
    echo "data_dir=${DATA_DIR}"
    echo "sqlite_path=${SQLITE_PATH}"
    echo "postgres_service=${POSTGRES_SERVICE}"
    echo "postgres_db=${POSTGRES_DB}"
    echo "postgres_user=${POSTGRES_USER}"
    echo ""
    echo "files:"
    find "$SAFETY_DIR" -maxdepth 1 -type f ! -name manifest.txt -printf "  %f\n" | sort
  } > "${SAFETY_DIR}/manifest.txt"
}

backup_current_state() {
  if [[ "$SKIP_CURRENT_BACKUP" == "true" ]]; then
    warn "current safety backup skipped by --skip-current-backup."
    return
  fi

  mkdir -p "$PRE_RESTORE_BACKUP_DIR"
  SAFETY_DIR="${PRE_RESTORE_BACKUP_DIR}/${TIMESTAMP}"
  mkdir -p "$SAFETY_DIR"

  case "$ACTUAL_MODE" in
    postgres) backup_current_postgres ;;
    sqlite) backup_current_sqlite ;;
  esac
  if [[ "$SKIP_DATA_DIR" != "true" ]]; then
    backup_current_data_dir
  fi
  write_safety_manifest
  log "pre-restore safety backup: $SAFETY_DIR"
}

restore_data_dir() {
  if [[ "$SKIP_DATA_DIR" == "true" ]]; then
    log "data directory restore skipped."
    return
  fi

  local archive
  archive="$(first_match "dushengcdn-data-*.tar.gz")"
  if [[ -z "$archive" ]]; then
    warn "data directory archive not found in backup; skipping data directory restore."
    return
  fi

  local extract_dir="${BACKUP_PATH}/.restore-data-${TIMESTAMP}"
  local existing_target_backup=""
  mkdir -p "$extract_dir"
  RESTORE_EXTRACT_DIR="$extract_dir"
  trap cleanup_restore_extract_dir EXIT

  log "restoring data directory from $(basename "$archive")..."
  tar -xzf "$archive" -C "$extract_dir"
  local extracted
  extracted="$(find "$extract_dir" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
  [[ -n "$extracted" ]] || die "data archive did not contain a top-level directory"

  mkdir -p "$(dirname "$DATA_DIR")"
  if [[ -e "$DATA_DIR" ]]; then
    if [[ "$SKIP_CURRENT_BACKUP" == "true" ]]; then
      existing_target_backup="${DATA_DIR}.before-restore-${TIMESTAMP}"
    else
      existing_target_backup="${SAFETY_DIR}/$(basename "$DATA_DIR").before-restore"
    fi
    log "moving current data directory to ${existing_target_backup}"
    mv "$DATA_DIR" "$existing_target_backup"
  fi
  mv "$extracted" "$DATA_DIR"
  cleanup_restore_extract_dir
  RESTORE_EXTRACT_DIR=""
}

restore_sqlite() {
  local sqlite_file
  sqlite_file="$(first_match "sqlite-*.db")"
  [[ -n "$sqlite_file" ]] || die "SQLite backup file not found in $BACKUP_PATH"

  log "restoring SQLite database to $SQLITE_PATH..."
  mkdir -p "$(dirname "$SQLITE_PATH")"
  cp -p "$sqlite_file" "$SQLITE_PATH"
}

restore_postgres() {
  local dump_file
  dump_file="$(first_match "postgres-*.sql")"
  [[ -n "$dump_file" ]] || die "PostgreSQL dump file not found in $BACKUP_PATH"
  command -v docker >/dev/null 2>&1 || die "docker is required for postgres restore"
  [[ -f "$COMPOSE_FILE" ]] || die "compose file not found: $COMPOSE_FILE"

  log "resetting PostgreSQL schema ${POSTGRES_DB}.public..."
  (cd "$SERVER_DIR" && "${compose_cmd[@]}" exec -T "$POSTGRES_SERVICE" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;")

  log "restoring PostgreSQL dump $(basename "$dump_file")..."
  if ! (cd "$SERVER_DIR" && "${compose_cmd[@]}" exec -T "$POSTGRES_SERVICE" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB") < "$dump_file"; then
    die "PostgreSQL restore failed. Current data safety backup is at ${SAFETY_DIR:-not created}."
  fi
}

verify_manifest_checksums
ensure_server_stopped

TIMESTAMP="$(date +%Y%m%d-%H%M%S)"

manifest_mode="$(manifest_value mode "")"
ACTUAL_MODE="$MODE"
if [[ "$ACTUAL_MODE" == "auto" ]]; then
  case "$manifest_mode" in
    postgres|sqlite) ACTUAL_MODE="$manifest_mode" ;;
    *)
      if [[ -n "$(first_match "postgres-*.sql")" ]]; then
        ACTUAL_MODE="postgres"
      elif [[ -n "$(first_match "sqlite-*.db")" ]]; then
        ACTUAL_MODE="sqlite"
      else
        die "auto mode found neither a postgres dump nor SQLite backup"
      fi
      ;;
  esac
fi

backup_current_state
restore_data_dir
case "$ACTUAL_MODE" in
  postgres) restore_postgres ;;
  sqlite) restore_sqlite ;;
esac

log "restore complete from $BACKUP_PATH"
log "start Server after checking logs: cd ${SERVER_DIR} && docker compose up -d"

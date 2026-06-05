#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SERVER_DIR="${REPO_ROOT}/dushengcdn_server"
COMPOSE_FILE=""
ENV_FILE=""
SERVER_URL=""
LOG_TAIL="120"
CURL_TIMEOUT="5"
SKIP_LOGS="false"
RAW_LOGS="false"
STATUS=0

usage() {
  cat <<EOF
DuShengCDN Server Diagnostic Helper

Usage:
  diagnose-server.sh [OPTIONS]

Options:
  --server-dir DIR       Server compose/source directory (default: REPO/dushengcdn_server)
  --compose-file FILE    Docker Compose file (default: SERVER_DIR/docker-compose.yaml)
  --env-file FILE        Docker Compose env file (default: SERVER_DIR/.env)
  --server-url URL       Server URL to check (default: http://127.0.0.1:DUSHENGCDN_HTTP_PORT)
  --log-tail NUM         Number of compose log lines to print per service (default: 120)
  --curl-timeout SEC     Curl timeout in seconds for health checks (default: 5)
  --skip-logs            Do not print compose logs
  --raw-logs             Print logs without redacting secrets
  -h, --help             Show this help message

Behavior:
  1. Reads the source Compose .env without printing secrets
  2. Shows Docker Compose service state when available
  3. Checks /api/status through the configured host port
  4. Shows listeners for the host panel port and container port 3000
  5. Prints recent Server/PostgreSQL logs and common failure hints

This script is read-only. It does not restart services, edit files, or change
Docker resources.
EOF
  exit 0
}

log() {
  echo "==> $*"
}

warn() {
  echo "Warning: $*" >&2
}

mark_failed() {
  STATUS=1
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

env_present() {
  local key="$1"
  [[ -n "${!key:-}" ]] && return 0
  [[ -n "$ENV_FILE" && -f "$ENV_FILE" ]] || return 1
  grep -Eq "^[[:space:]]*${key}=" "$ENV_FILE"
}

abs_path() {
  local path="$1"
  if [[ "$path" == /* ]]; then
    printf '%s' "$path"
  else
    printf '%s/%s' "$(pwd)" "$path"
  fi
}

server_status_url() {
  local base_url="$1"
  printf '%s/api/status' "${base_url%/}"
}

build_compose_cmd() {
  COMPOSE_CMD=(docker compose --project-directory "$SERVER_DIR" -f "$COMPOSE_FILE")
  if [[ -f "$ENV_FILE" ]]; then
    COMPOSE_CMD+=(--env-file "$ENV_FILE")
  fi
}

docker_compose_available() {
  command -v docker >/dev/null 2>&1 || return 1
  docker compose version >/dev/null 2>&1 || return 1
  [[ -f "$COMPOSE_FILE" ]] || return 1
}

show_summary() {
  local http_port="$1"

  log "Diagnostic target"
  echo "repo_root=${REPO_ROOT}"
  echo "server_dir=${SERVER_DIR}"
  echo "compose_file=${COMPOSE_FILE}"
  if [[ -f "$ENV_FILE" ]]; then
    echo "env_file=${ENV_FILE}"
  else
    echo "env_file=${ENV_FILE} (not found)"
    warn "Compose .env was not found. Defaults from docker-compose.yaml will be used."
  fi
  echo "server_url=${SERVER_URL}"
  echo "dushengcdn_http_port=${http_port}"
  if env_present DSN || env_present SQL_DSN; then
    echo "database=postgres_dsn_configured"
  else
    echo "database=sqlite_or_default"
  fi
  if env_present SESSION_SECRET; then
    echo "session_secret=configured"
  else
    echo "session_secret=not_configured"
  fi
  echo
}

run_compose_ps() {
  log "Docker Compose status"
  if ! docker_compose_available; then
    warn "docker compose is unavailable or compose file is missing: ${COMPOSE_FILE}"
    mark_failed
    return
  fi
  if ! "${COMPOSE_CMD[@]}" ps; then
    warn "docker compose ps failed."
    mark_failed
  fi
  echo
}

check_health() {
  local label="$1"
  local url="$2"
  local required="${3:-true}"
  local output

  log "HTTP health: ${label}"
  echo "url=${url}"
  if ! command -v curl >/dev/null 2>&1; then
    warn "curl was not found; cannot check ${url}"
    [[ "$required" == "true" ]] && mark_failed
    echo
    return 2
  fi

  output="$(curl -fsS --max-time "$CURL_TIMEOUT" "$url" 2>&1)"
  local rc=$?
  if [[ $rc -eq 0 ]] && printf '%s\n' "$output" | grep -Eq '"success"[[:space:]]*:[[:space:]]*true'; then
    echo "result=healthy"
    printf '%s\n' "$output"
    echo
    return 0
  fi

  echo "result=failed"
  printf '%s\n' "$output"
  warn "${label} did not return a healthy API response."
  [[ "$required" == "true" ]] && mark_failed
  echo
  return 1
}

show_port_listeners() {
  local port="$1"

  log "Port listeners: ${port}"
  if command -v ss >/dev/null 2>&1; then
    ss -lntup 2>/dev/null | grep -E "(:${port}[[:space:]]|:${port}$)" || true
  elif command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true
  else
    warn "neither ss nor lsof was found; cannot list port ${port} listeners."
  fi
  echo
}

collect_logs() {
  local service="$1"
  local tail_lines="$2"

  docker_compose_available || return 1
  "${COMPOSE_CMD[@]}" logs --no-color --tail="$tail_lines" "$service" 2>&1
}

redact_logs() {
  if [[ "$RAW_LOGS" == "true" ]]; then
    cat
    return
  fi
  sed -E \
    -e 's#(postgres(ql)?://[^:/@[:space:]]+:)[^@[:space:]]+@#\1<redacted>@#Ig' \
    -e 's#(\"[^\"]*(password|passwd|pwd|token|secret|authorization)[^\"]*\"[[:space:]]*:[[:space:]]*\")[^\"]+\"#\1<redacted>\"#Ig' \
    -e 's#((password|passwd|pwd|token|secret|authorization|x-agent-token|x-dns-worker-token)[_[:alnum:] .:-]*[=:][[:space:]]*)[^,;[:space:]\"]+#\1<redacted>#Ig' \
    -e 's#(Bearer[[:space:]]+)[A-Za-z0-9._~+/=-]+#\1<redacted>#Ig'
}

diagnose_server_logs() {
  local logs="$1"

  if printf '%s\n' "$logs" | grep -Eiq 'password authentication failed|SASL authentication failed'; then
    warn "Server logs look like PostgreSQL authentication failed. Check POSTGRES_PASSWORD and DSN in ${ENV_FILE}; they must match existing postgres-data."
  fi
  if printf '%s\n' "$logs" | grep -Eiq 'connection refused|no such host|failed to connect|database.*(connect|open|init)'; then
    warn "Server logs include database connection errors. Check DSN, PostgreSQL health, and compose service names."
  fi
  if printf '%s\n' "$logs" | grep -Eiq 'address already in use|bind:.*in use'; then
    warn "Server logs include a port binding conflict. Check DUSHENGCDN_HTTP_PORT in ${ENV_FILE} and host port usage."
  fi
}

show_logs() {
  local logs

  [[ "$SKIP_LOGS" == "true" ]] && return
  if ! docker_compose_available; then
    return
  fi

  log "Recent dushengcdn logs"
  logs="$(collect_logs dushengcdn "$LOG_TAIL" || true)"
  if [[ -n "$logs" ]]; then
    printf '%s\n' "$logs" | redact_logs
    diagnose_server_logs "$logs"
  else
    warn "could not read dushengcdn logs."
  fi
  echo

  log "Recent postgres logs"
  if ! collect_logs postgres "$LOG_TAIL" | redact_logs; then
    warn "could not read postgres logs; this is expected when PostgreSQL is external or service name differs."
  fi
  echo
}

show_hints() {
  local http_port="$1"

  log "Next checks"
  cat <<EOF
If http://127.0.0.1:${http_port}/api/status is healthy but the browser still cannot open the panel:
  - Point Nginx, Nginx Proxy Manager, Baota, or another reverse proxy upstream to 127.0.0.1:${http_port}.
  - Use 127.0.0.1:3000 only when the host mapping is explicitly 3000:3000 or Server is running directly on 3000.

If the dushengcdn service is not running:
  - Read the dushengcdn logs above first.
  - Check POSTGRES_PASSWORD and DSN when PostgreSQL authentication fails after an upgrade.
  - Check DUSHENGCDN_HTTP_PORT when Docker reports a bind conflict.
EOF
  echo
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-dir) SERVER_DIR="$2"; shift 2 ;;
    --compose-file) COMPOSE_FILE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --server-url) SERVER_URL="$2"; shift 2 ;;
    --log-tail) LOG_TAIL="$2"; shift 2 ;;
    --curl-timeout) CURL_TIMEOUT="$2"; shift 2 ;;
    --skip-logs) SKIP_LOGS="true"; shift ;;
    --raw-logs) RAW_LOGS="true"; shift ;;
    -h|--help) usage ;;
    *) warn "unknown option: $1"; exit 2 ;;
  esac
done

SERVER_DIR="$(abs_path "$SERVER_DIR")"
COMPOSE_FILE="${COMPOSE_FILE:-${SERVER_DIR}/docker-compose.yaml}"
COMPOSE_FILE="$(abs_path "$COMPOSE_FILE")"
ENV_FILE="${ENV_FILE:-${SERVER_DIR}/.env}"
ENV_FILE="$(abs_path "$ENV_FILE")"

if [[ ! -d "$SERVER_DIR" ]]; then
  warn "server directory not found: ${SERVER_DIR}"
  exit 1
fi

http_port="$(env_value DUSHENGCDN_HTTP_PORT 3010)"
if [[ -z "$SERVER_URL" ]]; then
  SERVER_URL="http://127.0.0.1:${http_port}"
fi

build_compose_cmd
show_summary "$http_port"
run_compose_ps

primary_status_url="$(server_status_url "$SERVER_URL")"
default_status_url="http://127.0.0.1:${http_port}/api/status"
container_status_url="http://127.0.0.1:3000/api/status"

check_health "configured Server URL" "$primary_status_url" true || true
if [[ "$default_status_url" != "$primary_status_url" ]]; then
  check_health "host-mapped panel port" "$default_status_url" true || true
fi
if [[ "$container_status_url" != "$primary_status_url" && "$container_status_url" != "$default_status_url" ]]; then
  check_health "direct container port 3000 (informational)" "$container_status_url" false || true
fi

show_port_listeners "$http_port"
if [[ "$http_port" != "3000" ]]; then
  show_port_listeners "3000"
fi

show_logs
show_hints "$http_port"

exit "$STATUS"

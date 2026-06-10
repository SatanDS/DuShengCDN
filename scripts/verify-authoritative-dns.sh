#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SERVER_DIR="${REPO_ROOT}/dushengcdn_server"
COMPOSE_FILE=""
ENV_FILE=""
SERVER_URL=""
DNS_WORKER_INSTALL_DIR="/opt/dushengcdn-dns-worker"
DNS_WORKER_SERVICE="dushengcdn-dns-worker"
PUBLIC_IP=""
ZONE=""
DNS_PORT=""
LOG_TAIL="80"
SKIP_LOGS="false"
STATUS=0

usage() {
  cat <<EOF
DuShengCDN Authoritative DNS Verification Helper

Usage:
  verify-authoritative-dns.sh --public-ip IP --zone ZONE [OPTIONS]

Options:
  --public-ip IP              DNS Worker public IP to query (required)
  --zone ZONE                 Zone name to query for SOA/NS (required)
  --server-dir DIR            Server compose/source directory (default: REPO/dushengcdn_server)
  --compose-file FILE         Docker Compose file (default: SERVER_DIR/docker-compose.yaml)
  --env-file FILE             Docker Compose env file (default: SERVER_DIR/.env)
  --server-url URL            Server URL to check (default: http://127.0.0.1:DUSHENGCDN_HTTP_PORT)
  --dns-worker-install-dir DIR
                              DNS Worker install directory (default: /opt/dushengcdn-dns-worker)
  --dns-worker-service NAME   DNS Worker systemd service name (default: dushengcdn-dns-worker)
  --dns-port PORT             DNS query/listener port override (default: parsed Worker listen port or 53)
  --log-tail NUM              Number of log lines to print per service (default: 80)
  --skip-logs                 Do not print service logs
  -h, --help                  Show this help message

Behavior:
  1. Checks the Server source Compose service and /api/status
  2. Checks local DNS Worker install files and systemd state
  3. Confirms the Worker DNS port is listening
  4. Runs UDP/TCP SOA and NS queries against PUBLIC_IP:PORT for ZONE
  5. Checks that a Worker snapshot file exists and is non-empty
  6. Prints redacted logs unless --skip-logs is set

This script is read-only. It does not restart services, edit files, or change
Docker/systemd/network resources.
EOF
  exit 0
}

log() {
  echo "==> $*"
}

warn() {
  echo "Warning: $*" >&2
}

pass() {
  echo "[PASS] $*"
}

fail() {
  echo "[FAIL] $*" >&2
  STATUS=1
}

skip() {
  echo "[SKIP] $*"
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

env_file_value() {
  local file="$1"
  local key="$2"
  local fallback="$3"
  local from_env="${!key:-}"
  if [[ -n "$from_env" ]]; then
    printf '%s' "$from_env"
    return
  fi
  if [[ -f "$file" ]]; then
    local line
    line="$(grep -E "^[[:space:]]*${key}=" "$file" | tail -n 1 || true)"
    if [[ -n "$line" ]]; then
      strip_quotes "${line#*=}"
      return
    fi
  fi
  printf '%s' "$fallback"
}

env_file_present() {
  local file="$1"
  local key="$2"
  [[ -n "${!key:-}" ]] && return 0
  [[ -f "$file" ]] || return 1
  grep -Eq "^[[:space:]]*${key}=" "$file"
}

redact_logs() {
  sed -E \
    -e 's#(postgres(ql)?://[^:/@[:space:]]+:)[^@[:space:]]+@#\1<redacted>@#Ig' \
    -e 's#("[^"]*(password|passwd|pwd|token|secret|authorization)[^"]*"[[:space:]]*:[[:space:]]*")[^"]+"#\1<redacted>"#Ig' \
    -e 's#((password|passwd|pwd|token|secret|authorization|x-agent-token|x-dns-worker-token)[_[:alnum:] .:-]*[=:][[:space:]]*)[^,;[:space:]\"]+#\1<redacted>#Ig' \
    -e 's#(Bearer[[:space:]]+)[A-Za-z0-9._~+/=-]+#\1<redacted>#Ig'
}

abs_path() {
  local path="$1"
  if [[ "$path" == /* ]]; then
    printf '%s' "$path"
  else
    printf '%s/%s' "$(pwd)" "$path"
  fi
}

listen_port_from_addr() {
  local addr="$1"

  if [[ "$addr" =~ ^\[.*\]:([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
    return 0
  fi
  if [[ "$addr" =~ :([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
    return 0
  fi
  if [[ "$addr" =~ ^([0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
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

check_server_compose() {
  log "Server Compose"
  if ! docker_compose_available; then
    fail "docker compose is unavailable or compose file is missing: ${COMPOSE_FILE}"
    return
  fi

  if "${COMPOSE_CMD[@]}" ps; then
    pass "docker compose ps completed"
  else
    fail "docker compose ps failed"
  fi

  local running
  running="$("${COMPOSE_CMD[@]}" ps --status running --services 2>/dev/null || true)"
  if printf '%s\n' "$running" | grep -Fxq "dushengcdn"; then
    pass "dushengcdn service is running"
  else
    fail "dushengcdn service is not running"
  fi
  echo
}

check_server_health() {
  local url output

  log "Server HTTP health"
  url="$(server_status_url "$SERVER_URL")"
  echo "url=${url}"
  if ! command -v curl >/dev/null 2>&1; then
    fail "curl was not found; cannot check Server HTTP health"
    echo
    return
  fi
  output="$(curl -fsS --max-time 5 "$url" 2>&1)"
  local rc=$?
  printf '%s\n' "$output"
  if [[ $rc -eq 0 ]] && printf '%s\n' "$output" | grep -Eq '"success"[[:space:]]*:[[:space:]]*true'; then
    pass "Server /api/status is healthy"
  else
    fail "Server /api/status is not healthy"
  fi
  echo
}

check_worker_service() {
  log "DNS Worker systemd"
  if ! command -v systemctl >/dev/null 2>&1; then
    fail "systemctl was not found; cannot verify systemd Worker service"
    echo
    return
  fi

  if systemctl cat "${DNS_WORKER_SERVICE}.service" >/dev/null 2>&1; then
    pass "unit exists: ${DNS_WORKER_SERVICE}.service"
  else
    fail "unit missing: ${DNS_WORKER_SERVICE}.service"
  fi
  if systemctl is-active --quiet "$DNS_WORKER_SERVICE"; then
    pass "Worker service is active"
  else
    fail "Worker service is not active: $(systemctl is-active "$DNS_WORKER_SERVICE" 2>/dev/null || echo unknown)"
  fi
  systemctl status "$DNS_WORKER_SERVICE" --no-pager -l 2>/dev/null || true
  echo
}

check_worker_files() {
  log "DNS Worker files"
  local worker_env="${DNS_WORKER_INSTALL_DIR}/dns-worker.env"
  local worker_binary="${DNS_WORKER_INSTALL_DIR}/dushengcdn-dns-worker"

  if [[ -x "$worker_binary" ]]; then
    pass "Worker binary exists: ${worker_binary}"
  else
    fail "Worker binary missing or not executable: ${worker_binary}"
  fi

  if [[ -f "$worker_env" ]]; then
    pass "Worker env file exists: ${worker_env}"
  else
    fail "Worker env file missing: ${worker_env}"
  fi

  local worker_server_url worker_token_configured listen_addr snapshot_path geoip_path
  worker_server_url="$(env_file_value "$worker_env" DUSHENGCDN_DNS_WORKER_SERVER_URL "")"
  if env_file_present "$worker_env" DUSHENGCDN_DNS_WORKER_TOKEN || env_file_present "$worker_env" DUSHENGCDN_DNS_WORKER_TOKEN_FILE; then
    worker_token_configured="true"
  else
    worker_token_configured="false"
  fi
  listen_addr="$(env_file_value "$worker_env" DUSHENGCDN_DNS_WORKER_LISTEN_ADDR ":53")"
  snapshot_path="$(env_file_value "$worker_env" DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH "${DNS_WORKER_INSTALL_DIR}/data/dns-worker-snapshot.json")"
  geoip_path="$(env_file_value "$worker_env" DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH "")"

  echo "worker_server_url=${worker_server_url:-not_configured}"
  echo "worker_token=$([[ "$worker_token_configured" == "true" ]] && echo configured || echo not_configured)"
  echo "worker_listen_addr=${listen_addr}"
  echo "worker_snapshot_path=${snapshot_path}"
  echo "worker_geoip_database=${geoip_path:-not_configured}"

  [[ -n "$worker_server_url" ]] && pass "Worker Server URL is configured" || fail "Worker Server URL is not configured"
  [[ "$worker_token_configured" == "true" ]] && pass "Worker token is configured" || fail "Worker token is not configured"

  if [[ -f "$snapshot_path" && -s "$snapshot_path" ]]; then
    pass "Worker snapshot file exists and is non-empty"
  else
    fail "Worker snapshot file is missing or empty: ${snapshot_path}"
  fi

  if [[ -n "$geoip_path" ]]; then
    if [[ -f "$geoip_path" && -s "$geoip_path" ]]; then
      pass "GeoIP database exists"
    else
      warn "GeoIP database is configured but missing or empty: ${geoip_path}"
    fi
  fi

  if [[ -z "$DNS_PORT" ]]; then
    DNS_PORT="$(listen_port_from_addr "$listen_addr" 2>/dev/null || printf '53')"
  fi
  echo
}

check_port_listeners() {
  log "DNS port listeners"
  local output=""

  if command -v ss >/dev/null 2>&1; then
    output="$(ss -lntup 2>/dev/null | grep -E "(:${DNS_PORT}[[:space:]]|:${DNS_PORT}$)" || true)"
    output="${output}"$'\n'"$(ss -lnuap 2>/dev/null | grep -E "(:${DNS_PORT}[[:space:]]|:${DNS_PORT}$)" || true)"
  elif command -v lsof >/dev/null 2>&1; then
    output="$(lsof -nP -iTCP:"${DNS_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
    output="${output}"$'\n'"$(lsof -nP -iUDP:"${DNS_PORT}" 2>/dev/null || true)"
  elif command -v netstat >/dev/null 2>&1; then
    output="$(netstat -lntup 2>/dev/null | grep -E "[:.]${DNS_PORT}[[:space:]]" || true)"
    output="${output}"$'\n'"$(netstat -lnuap 2>/dev/null | grep -E "[:.]${DNS_PORT}[[:space:]]" || true)"
  else
    fail "neither ss, lsof, nor netstat was found; cannot verify DNS port listeners"
    echo
    return
  fi

  printf '%s\n' "$output" | sed '/^[[:space:]]*$/d'
  if printf '%s\n' "$output" | grep -q 'dushengcdn'; then
    pass "DuShengCDN DNS Worker appears to be listening on port ${DNS_PORT}"
  else
    fail "No DuShengCDN listener found on port ${DNS_PORT}"
  fi
  echo
}

run_dig_check() {
  local query_type="$1"
  local tcp_flag="$2"
  local label="$3"
  local output

  log "DNS query ${label}: ${query_type}"
  if ! command -v dig >/dev/null 2>&1; then
    fail "dig was not found; install dnsutils/bind-utils to verify public DNS responses"
    echo
    return
  fi

  output="$(dig ${tcp_flag} +time=3 +tries=1 @"$PUBLIC_IP" -p "$DNS_PORT" "$ZONE" "$query_type" 2>&1)"
  local rc=$?
  printf '%s\n' "$output"
  if [[ $rc -ne 0 ]] ||
    printf '%s\n' "$output" | grep -Eiq 'connection refused|no servers could be reached|timed out|communications error' ||
    ! printf '%s\n' "$output" | grep -Eq 'status: NOERROR|status: NXDOMAIN'; then
    fail "dig ${label} ${query_type} did not return a valid DNS response"
  else
    pass "dig ${label} ${query_type} returned a DNS response"
  fi
  echo
}

check_dns_queries() {
  run_dig_check "SOA" "" "udp"
  run_dig_check "NS" "" "udp"
  run_dig_check "SOA" "+tcp" "tcp"
  run_dig_check "NS" "+tcp" "tcp"
}

show_logs() {
  [[ "$SKIP_LOGS" == "true" ]] && return

  log "Recent logs"
  if docker_compose_available; then
    echo "--- dushengcdn compose logs ---"
    "${COMPOSE_CMD[@]}" logs --no-color --tail="$LOG_TAIL" dushengcdn 2>/dev/null | redact_logs || true
    echo "--- postgres compose logs ---"
    "${COMPOSE_CMD[@]}" logs --no-color --tail="$LOG_TAIL" postgres 2>/dev/null | redact_logs || true
  else
    skip "Server compose logs unavailable"
  fi
  if command -v journalctl >/dev/null 2>&1; then
    echo "--- DNS Worker journal ---"
    journalctl -u "$DNS_WORKER_SERVICE" -n "$LOG_TAIL" --no-pager 2>/dev/null | redact_logs || true
  else
    skip "journalctl unavailable"
  fi
  echo
}

show_summary() {
  log "Verification summary"
  if [[ "$STATUS" -eq 0 ]]; then
    pass "Authoritative DNS deployment verification passed for ${ZONE} at ${PUBLIC_IP}:${DNS_PORT}"
  else
    fail "Authoritative DNS deployment verification failed. Review [FAIL] lines above before switching production traffic."
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --public-ip) PUBLIC_IP="$2"; shift 2 ;;
    --zone) ZONE="$2"; shift 2 ;;
    --server-dir) SERVER_DIR="$2"; shift 2 ;;
    --compose-file) COMPOSE_FILE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --server-url) SERVER_URL="$2"; shift 2 ;;
    --dns-worker-install-dir) DNS_WORKER_INSTALL_DIR="$2"; shift 2 ;;
    --dns-worker-service) DNS_WORKER_SERVICE="$2"; shift 2 ;;
    --dns-port) DNS_PORT="$2"; shift 2 ;;
    --log-tail) LOG_TAIL="$2"; shift 2 ;;
    --skip-logs) SKIP_LOGS="true"; shift ;;
    -h|--help) usage ;;
    *) warn "unknown option: $1"; exit 2 ;;
  esac
done

if [[ -z "$PUBLIC_IP" || -z "$ZONE" ]]; then
  echo "Error: --public-ip and --zone are required." >&2
  echo "Example: bash scripts/verify-authoritative-dns.sh --public-ip 203.0.113.10 --zone example.com" >&2
  exit 2
fi

SERVER_DIR="$(abs_path "$SERVER_DIR")"
COMPOSE_FILE="${COMPOSE_FILE:-${SERVER_DIR}/docker-compose.yaml}"
COMPOSE_FILE="$(abs_path "$COMPOSE_FILE")"
ENV_FILE="${ENV_FILE:-${SERVER_DIR}/.env}"
ENV_FILE="$(abs_path "$ENV_FILE")"
DNS_WORKER_INSTALL_DIR="$(abs_path "$DNS_WORKER_INSTALL_DIR")"

http_port="$(env_file_value "$ENV_FILE" DUSHENGCDN_HTTP_PORT 3010)"
if [[ -z "$SERVER_URL" ]]; then
  SERVER_URL="http://127.0.0.1:${http_port}"
fi

build_compose_cmd

log "Verification target"
echo "server_dir=${SERVER_DIR}"
echo "compose_file=${COMPOSE_FILE}"
echo "env_file=${ENV_FILE}"
echo "server_url=${SERVER_URL}"
echo "dns_worker_install_dir=${DNS_WORKER_INSTALL_DIR}"
echo "dns_worker_service=${DNS_WORKER_SERVICE}"
echo "public_ip=${PUBLIC_IP}"
echo "zone=${ZONE}"
echo

check_server_compose
check_server_health
check_worker_service
check_worker_files
check_port_listeners
check_dns_queries
show_logs
show_summary

exit "$STATUS"

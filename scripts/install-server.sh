#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

SERVER_DIR="${REPO_ROOT}/dushengcdn_server"
COMPOSE_FILE=""
ENV_FILE=""
SERVER_URL=""
PUBLIC_IP=""
INSTALL_DNS_WORKER="true"
FORCE_DNS_WORKER_REINSTALL="false"
DNS_WORKER_NAME="DNS服务响应端"
DNS_WORKER_SERVICE="dushengcdn-dns-worker"
DNS_WORKER_INSTALL_DIR="/opt/dushengcdn-dns-worker"
DNS_WORKER_LISTEN=""
DNS_WORKER_QUERY_RATE_LIMIT="200"
DNS_WORKER_UDP_RESPONSE_SIZE="1232"
DNS_WORKER_REPO="SatanDS/DuShengCDN"
DNS_WORKER_SOURCE_REF="${SOURCE_REF:-main}"
DNS_WORKER_GEOIP_DOWNLOAD="true"

usage() {
  cat <<EOF
DuShengCDN Server Installer

Usage:
  install-server.sh [OPTIONS]

Options:
  --server-dir DIR                  Server compose/source directory (default: REPO/dushengcdn_server)
  --compose-file FILE               Docker Compose file (default: SERVER_DIR/docker-compose.yaml)
  --env-file FILE                   Docker Compose env file (default: SERVER_DIR/.env)
  --server-url URL                  Server URL for DNS Worker (default: http://127.0.0.1:DUSHENGCDN_HTTP_PORT)
  --public-ip IP                    Public IP saved to Worker and used for --listen IP:53
  --skip-dns-worker                 Deploy panel only; do not create/install DNS Worker
  --force-dns-worker-reinstall      Reinstall DNS Worker even when local deployment is detected
  --dns-worker-name NAME            Worker name created in Server (default: DNS服务响应端)
  --dns-worker-install-dir DIR      DNS Worker install directory (default: /opt/dushengcdn-dns-worker)
  --dns-worker-listen ADDR          DNS Worker listen address (default: PUBLIC_IP:53)
  --dns-worker-query-rate-limit NUM Per-source-IP DNS queries per second (default: 200)
  --dns-worker-udp-response-size NUM
                                  Maximum UDP DNS response payload size (default: 1232)
  --dns-worker-repo REPO            GitHub repository for Worker installer (default: SatanDS/DuShengCDN)
  --dns-worker-source-ref REF       Git branch, tag, or commit for Worker source build (default: main)
  --dns-worker-no-geoip-download    Do not download Country MMDB automatically
  -h, --help                        Show this help message

Behavior:
  1. Creates dushengcdn_server/.env from .env.example when .env does not exist
     and fills SESSION_SECRET plus first-install database secrets with generated values
  2. Starts or updates Server with Docker Compose
  3. When DNS Worker is enabled, checks whether a local Worker is already deployed
  4. If no local Worker is found, detects public IP, creates a DNS Worker Token, and runs install-dns-worker.sh
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

is_ipv4() {
  local ip="$1"
  local a b c d part num
  [[ "$ip" =~ ^[0-9]+(\.[0-9]+){3}$ ]] || return 1
  IFS=. read -r a b c d <<< "$ip"
  for part in "$a" "$b" "$c" "$d"; do
    [[ "$part" =~ ^[0-9]+$ ]] || return 1
    num=$((10#$part))
    (( num >= 0 && num <= 255 )) || return 1
  done
}

random_hex() {
  local bytes="$1"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
    return 0
  fi
  if [[ -r /dev/urandom ]]; then
    od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
    return 0
  fi
  return 1
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\/&]/\\&/g'
}

write_env_key() {
  local key="$1"
  local value="$2"
  local escaped

  escaped="$(escape_sed_replacement "$value")"
  if grep -Eq "^[[:space:]]*${key}=" "$ENV_FILE"; then
    sed -i.bak -E "s|^[[:space:]]*${key}=.*|${key}=${escaped}|" "$ENV_FILE"
    rm -f "${ENV_FILE}.bak"
  else
    printf '\n%s=%s\n' "$key" "$value" >> "$ENV_FILE"
  fi
}

existing_postgres_data_dir() {
  local dir="${SERVER_DIR}/postgres-data"
  [[ -d "$dir" ]] || return 1
  [[ -n "$(find "$dir" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]]
}

initialize_env_file() {
  local postgres_db postgres_user postgres_password session_secret dsn

  if [[ -f "$ENV_FILE" ]]; then
    return
  fi
  [[ -f "${SERVER_DIR}/.env.example" ]] || return

  log "Creating ${ENV_FILE} from .env.example..."
  cp -n "${SERVER_DIR}/.env.example" "$ENV_FILE"

  session_secret="$(random_hex 32 || true)"
  if [[ -n "$session_secret" ]]; then
    write_env_key SESSION_SECRET "$session_secret"
  else
    warn "could not generate SESSION_SECRET; edit ${ENV_FILE} before production use."
  fi

  if existing_postgres_data_dir; then
    warn "existing PostgreSQL data directory detected at ${SERVER_DIR}/postgres-data."
    warn "preserving POSTGRES_PASSWORD and DSN copied from .env.example to avoid breaking existing database authentication."
    warn "after the panel is healthy, rotate the PostgreSQL password deliberately if needed."
    log "Generated SESSION_SECRET in ${ENV_FILE}; preserved existing PostgreSQL credentials."
    return
  fi

  postgres_password="$(random_hex 18 || true)"
  if [[ -z "$postgres_password" ]]; then
    warn "could not generate POSTGRES_PASSWORD; edit ${ENV_FILE} before production use."
    log "Generated SESSION_SECRET in ${ENV_FILE}."
    return
  fi

  postgres_db="$(env_value POSTGRES_DB dushengcdn)"
  postgres_user="$(env_value POSTGRES_USER dushengcdn)"
  dsn="postgres://${postgres_user}:${postgres_password}@postgres:5432/${postgres_db}?sslmode=disable"
  write_env_key POSTGRES_PASSWORD "$postgres_password"
  write_env_key DSN "$dsn"
  log "Generated POSTGRES_PASSWORD, SESSION_SECRET, and DSN in ${ENV_FILE}."
}

detect_public_ip() {
  local endpoint ip
  for endpoint in \
    "https://api.ipify.org" \
    "https://ifconfig.me/ip" \
    "https://ipv4.icanhazip.com"; do
    ip="$(curl -fsS --max-time 5 "$endpoint" 2>/dev/null | tr -d '[:space:]' || true)"
    if is_ipv4 "$ip"; then
      printf '%s' "$ip"
      return 0
    fi
  done

  if command -v dig >/dev/null 2>&1; then
    ip="$(dig +short myip.opendns.com @resolver1.opendns.com 2>/dev/null | tail -n 1 | tr -d '[:space:]' || true)"
    if is_ipv4 "$ip"; then
      printf '%s' "$ip"
      return 0
    fi
  fi

  return 1
}

ensure_docker_compose() {
  command -v docker >/dev/null 2>&1 || die "docker was not found. Install Docker first."
  docker compose version >/dev/null 2>&1 || die "docker compose was not found. Install Docker Compose v2 first."
}

local_git_version() {
  if command -v git >/dev/null 2>&1 && git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || true
  fi
}

build_compose_cmd() {
  COMPOSE_CMD=(docker compose --project-directory "$SERVER_DIR" -f "$COMPOSE_FILE")
  if [[ -f "$ENV_FILE" ]]; then
    COMPOSE_CMD+=(--env-file "$ENV_FILE")
  fi
}

compose_run() {
  "${COMPOSE_CMD[@]}" "$@"
}

dns_worker_already_installed() {
  DNS_WORKER_FOUND_REASON=""

  if command -v systemctl >/dev/null 2>&1 && systemctl cat "${DNS_WORKER_SERVICE}.service" >/dev/null 2>&1; then
    DNS_WORKER_FOUND_REASON="systemd service ${DNS_WORKER_SERVICE}.service already exists"
    return 0
  fi

  if [[ -f "/etc/systemd/system/${DNS_WORKER_SERVICE}.service" || -f "/lib/systemd/system/${DNS_WORKER_SERVICE}.service" || -f "/usr/lib/systemd/system/${DNS_WORKER_SERVICE}.service" ]]; then
    DNS_WORKER_FOUND_REASON="systemd unit file for ${DNS_WORKER_SERVICE}.service already exists"
    return 0
  fi

  if [[ -x "${DNS_WORKER_INSTALL_DIR}/dushengcdn-dns-worker" ]]; then
    DNS_WORKER_FOUND_REASON="binary exists at ${DNS_WORKER_INSTALL_DIR}/dushengcdn-dns-worker"
    return 0
  fi

  if [[ -f "${DNS_WORKER_INSTALL_DIR}/dns-worker.env" ]]; then
    DNS_WORKER_FOUND_REASON="environment file exists at ${DNS_WORKER_INSTALL_DIR}/dns-worker.env"
    return 0
  fi

  if command -v docker >/dev/null 2>&1 && docker ps -a --format '{{.Names}}' 2>/dev/null | grep -Fxq "$DNS_WORKER_SERVICE"; then
    DNS_WORKER_FOUND_REASON="Docker container ${DNS_WORKER_SERVICE} already exists"
    return 0
  fi

  if command -v pgrep >/dev/null 2>&1 && pgrep -f "dushengcdn-dns-worker" >/dev/null 2>&1; then
    DNS_WORKER_FOUND_REASON="dushengcdn-dns-worker process is already running"
    return 0
  fi

  if command -v ss >/dev/null 2>&1 && ss -H -lntup 2>/dev/null | grep -E '(:53[[:space:]]|:53$)' | grep -q 'dushengcdn'; then
    DNS_WORKER_FOUND_REASON="a dushengcdn process is already listening on port 53"
    return 0
  fi

  return 1
}

create_dns_worker_token() {
  local output token

  log "Creating DNS Worker '${DNS_WORKER_NAME}' in Server..." >&2
  if ! output="$(compose_run run --rm -e LOG_LEVEL=error dushengcdn /dushengcdn \
    --create-dns-worker-name "$DNS_WORKER_NAME" \
    --create-dns-worker-public-address "$PUBLIC_IP" 2>&1)"; then
    echo "$output" >&2
    die "failed to create DNS Worker token."
  fi

  token="$(printf '%s\n' "$output" | awk 'NF { last = $0 } END { gsub(/^[ \t]+|[ \t]+$/, "", last); print last }')"
  if [[ ! "$token" =~ ^[0-9a-fA-F]{32}$ ]]; then
    echo "$output" >&2
    die "failed to parse DNS Worker token from Server output."
  fi

  printf '%s' "$token"
}

install_dns_worker() {
  local token="$1"
  local listen_addr="$DNS_WORKER_LISTEN"
  local install_args

  if [[ -z "$listen_addr" ]]; then
    listen_addr="${PUBLIC_IP}:53"
  fi

  install_args=(
    --server-url "$SERVER_URL"
    --token "$token"
    --install-dir "$DNS_WORKER_INSTALL_DIR"
    --listen "$listen_addr"
    --query-rate-limit "$DNS_WORKER_QUERY_RATE_LIMIT"
    --udp-response-size "$DNS_WORKER_UDP_RESPONSE_SIZE"
    --repo "$DNS_WORKER_REPO"
    --source-ref "$DNS_WORKER_SOURCE_REF"
  )
  if [[ "$DNS_WORKER_GEOIP_DOWNLOAD" != "true" ]]; then
    install_args+=(--no-geoip-download)
  fi

  log "Installing DNS Worker on ${listen_addr}..."
  bash "${SCRIPT_DIR}/install-dns-worker.sh" "${install_args[@]}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-dir) SERVER_DIR="$2"; shift 2 ;;
    --compose-file) COMPOSE_FILE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --server-url) SERVER_URL="$2"; shift 2 ;;
    --public-ip) PUBLIC_IP="$2"; shift 2 ;;
    --skip-dns-worker) INSTALL_DNS_WORKER="false"; shift ;;
    --force-dns-worker-reinstall) FORCE_DNS_WORKER_REINSTALL="true"; shift ;;
    --dns-worker-name) DNS_WORKER_NAME="$2"; shift 2 ;;
    --dns-worker-install-dir) DNS_WORKER_INSTALL_DIR="$2"; shift 2 ;;
    --dns-worker-listen) DNS_WORKER_LISTEN="$2"; shift 2 ;;
    --dns-worker-query-rate-limit) DNS_WORKER_QUERY_RATE_LIMIT="$2"; shift 2 ;;
    --dns-worker-udp-response-size) DNS_WORKER_UDP_RESPONSE_SIZE="$2"; shift 2 ;;
    --dns-worker-repo) DNS_WORKER_REPO="$2"; shift 2 ;;
    --dns-worker-source-ref) DNS_WORKER_SOURCE_REF="$2"; shift 2 ;;
    --dns-worker-no-geoip-download) DNS_WORKER_GEOIP_DOWNLOAD="false"; shift ;;
    -h|--help) usage ;;
    *) die "unknown option: $1" ;;
  esac
done

SERVER_DIR="$(abs_path "$SERVER_DIR")"
if [[ -z "$COMPOSE_FILE" ]]; then
  COMPOSE_FILE="${SERVER_DIR}/docker-compose.yaml"
else
  COMPOSE_FILE="$(abs_path "$COMPOSE_FILE")"
fi
if [[ -z "$ENV_FILE" ]]; then
  ENV_FILE="${SERVER_DIR}/.env"
else
  ENV_FILE="$(abs_path "$ENV_FILE")"
fi

[[ -d "$SERVER_DIR" ]] || die "server directory does not exist: ${SERVER_DIR}"
[[ -f "$COMPOSE_FILE" ]] || die "compose file does not exist: ${COMPOSE_FILE}"

initialize_env_file

ensure_docker_compose
build_compose_cmd

if [[ -z "${DUSHENGCDN_VERSION:-}" ]]; then
  detected_version="$(local_git_version)"
  if [[ -n "$detected_version" ]]; then
    export DUSHENGCDN_VERSION="$detected_version"
  fi
fi

if [[ -z "$SERVER_URL" ]]; then
  http_port="$(env_value DUSHENGCDN_HTTP_PORT 3010)"
  SERVER_URL="http://127.0.0.1:${http_port}"
fi

log "Starting DuShengCDN Server with Docker Compose..."
compose_run up -d --build
compose_run ps

if [[ "$INSTALL_DNS_WORKER" != "true" ]]; then
  log "DNS Worker installation skipped."
  exit 0
fi

log "Checking whether DNS Worker is already deployed locally..."
if dns_worker_already_installed && [[ "$FORCE_DNS_WORKER_REINSTALL" != "true" ]]; then
  log "DNS Worker already deployed locally; skipping automatic Worker creation and install."
  log "${DNS_WORKER_FOUND_REASON}"
  log "Use --force-dns-worker-reinstall if you intentionally want to replace the local Worker configuration."
  exit 0
fi

if [[ -z "$PUBLIC_IP" ]]; then
  log "Detecting public IPv4 address for DNS Worker..."
  PUBLIC_IP="$(detect_public_ip || true)"
fi
if ! is_ipv4 "$PUBLIC_IP"; then
  die "could not detect a public IPv4 address. Rerun with --public-ip YOUR_PUBLIC_IP or use --skip-dns-worker."
fi

token="$(create_dns_worker_token)"
install_dns_worker "$token"

log "Server deployment completed."
log "Panel URL for local access: ${SERVER_URL}"
log "DNS Worker public address: ${PUBLIC_IP}"

#!/usr/bin/env bash
set -uo pipefail

INSTALL_DIR="/opt/dushengcdn-dns-worker"
SERVICE_NAME="dushengcdn-dns-worker"
ENV_FILE=""
PUBLIC_IP=""
ZONE=""
DNS_PORT=""
LOG_TAIL="120"
SKIP_LOGS="false"
RAW_LOGS="false"
STATUS=0

usage() {
  cat <<EOF
DuShengCDN DNS Worker Diagnostic Helper

Usage:
  diagnose-dns-worker.sh [OPTIONS]

Options:
  --install-dir DIR      DNS Worker install directory (default: /opt/dushengcdn-dns-worker)
  --service-name NAME    systemd service name (default: dushengcdn-dns-worker)
  --env-file FILE        DNS Worker env file (default: INSTALL_DIR/dns-worker.env)
  --public-ip IP         Public DNS Worker IP to query with dig
  --zone NAME            Zone name to query for SOA/NS when --public-ip is set
  --dns-port PORT        DNS query/listener port override (default: parsed listen port or 53)
  --log-tail NUM         Number of journal lines to print (default: 120)
  --skip-logs            Do not print journal logs
  --raw-logs             Print logs without redacting secrets
  -h, --help             Show this help message

Behavior:
  1. Checks systemd service/unit, install directory, binary, env file, snapshot, and GeoIP file
  2. Shows configured Server URL, listen address, and whether a Worker token is present without printing the token
  3. Lists TCP/UDP listeners for the Worker DNS port
  4. Optionally runs dig @PUBLIC_IP ZONE SOA/NS over UDP and TCP
  5. Prints recent service logs and common failure hints

This script is read-only. It does not restart services, edit files, or change
systemd/network resources.
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

detect_dns_port() {
  local listen_addr="$1"
  if [[ -n "$DNS_PORT" ]]; then
    printf '%s' "$DNS_PORT"
    return
  fi
  listen_port_from_addr "$listen_addr" 2>/dev/null || printf '53'
}

file_status() {
  local label="$1"
  local path="$2"
  local required="${3:-false}"

  if [[ -f "$path" ]]; then
    if command -v stat >/dev/null 2>&1; then
      echo "${label}=present path=${path} size=$(stat -c '%s' "$path" 2>/dev/null || stat -f '%z' "$path" 2>/dev/null || echo unknown)"
    else
      echo "${label}=present path=${path}"
    fi
    return 0
  fi

  echo "${label}=missing path=${path}"
  [[ "$required" == "true" ]] && mark_failed
  return 1
}

show_target_summary() {
  local listen_addr="$1"
  local snapshot_path="$2"
  local geoip_path="$3"
  local server_url="$4"
  local dns_port="$5"

  log "Diagnostic target"
  echo "install_dir=${INSTALL_DIR}"
  echo "service_name=${SERVICE_NAME}"
  echo "env_file=${ENV_FILE}"
  if [[ -f "$ENV_FILE" ]]; then
    echo "env_file_status=present"
  else
    echo "env_file_status=missing"
    warn "DNS Worker env file was not found. The Worker may not be installed by install-dns-worker.sh."
    mark_failed
  fi
  echo "server_url=${server_url:-not_configured}"
  if env_present DUSHENGCDN_DNS_WORKER_TOKEN; then
    echo "token=configured"
  else
    echo "token=not_configured"
    mark_failed
  fi
  echo "listen_addr=${listen_addr}"
  echo "dns_port=${dns_port}"
  echo "snapshot_path=${snapshot_path:-not_configured}"
  echo "geoip_database=${geoip_path:-not_configured}"
  echo
}

show_systemd_status() {
  log "systemd status"
  if ! command -v systemctl >/dev/null 2>&1; then
    warn "systemctl was not found; skipping service status."
    echo
    return
  fi

  if systemctl cat "${SERVICE_NAME}.service" >/dev/null 2>&1; then
    echo "unit=present"
  else
    echo "unit=missing"
    warn "Unit ${SERVICE_NAME}.service was not found."
    mark_failed
  fi

  if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "active=active"
  else
    echo "active=$(systemctl is-active "$SERVICE_NAME" 2>/dev/null || echo unknown)"
    mark_failed
  fi

  if systemctl is-enabled --quiet "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "enabled=enabled"
  else
    echo "enabled=$(systemctl is-enabled "$SERVICE_NAME" 2>/dev/null || echo unknown)"
  fi

  systemctl status "$SERVICE_NAME" --no-pager -l 2>/dev/null || true
  echo
}

show_install_files() {
  local snapshot_path="$1"
  local geoip_path="$2"

  log "Install files"
  if [[ -d "$INSTALL_DIR" ]]; then
    echo "install_dir=present path=${INSTALL_DIR}"
  else
    echo "install_dir=missing path=${INSTALL_DIR}"
    mark_failed
  fi
  file_status "binary" "${INSTALL_DIR}/dushengcdn-dns-worker" true || true
  file_status "env_file" "$ENV_FILE" true || true
  if [[ -n "$snapshot_path" ]]; then
    file_status "snapshot" "$snapshot_path" false || true
  fi
  if [[ -n "$geoip_path" ]]; then
    file_status "geoip_database" "$geoip_path" false || true
  fi
  echo
}

show_port_listeners() {
  local port="$1"

  log "Port listeners: ${port}"
  if command -v ss >/dev/null 2>&1; then
    ss -lntup 2>/dev/null | grep -E "(:${port}[[:space:]]|:${port}$)" || true
    ss -lnuap 2>/dev/null | grep -E "(:${port}[[:space:]]|:${port}$)" || true
  elif command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN 2>/dev/null || true
    lsof -nP -iUDP:"${port}" 2>/dev/null || true
  elif command -v netstat >/dev/null 2>&1; then
    netstat -lntup 2>/dev/null | grep -E "[:.]${port}[[:space:]]" || true
    netstat -lnuap 2>/dev/null | grep -E "[:.]${port}[[:space:]]" || true
  else
    warn "neither ss, lsof, nor netstat was found; cannot list port ${port} listeners."
  fi
  echo
}

check_processes() {
  log "DNS Worker process"
  if command -v pgrep >/dev/null 2>&1; then
    pgrep -af "dushengcdn-dns-worker" || {
      warn "no dushengcdn-dns-worker process found."
      mark_failed
    }
  else
    warn "pgrep was not found; skipping process check."
  fi
  echo
}

run_dig_check() {
  local public_ip="$1"
  local zone="$2"
  local port="$3"
  local query_type="$4"
  local tcp_flag="$5"
  local label="$6"
  local output

  log "dig ${label}: ${query_type}"
  if ! command -v dig >/dev/null 2>&1; then
    warn "dig was not found; install dnsutils/bind-utils to run DNS query checks."
    echo
    return 2
  fi

  output="$(dig ${tcp_flag} +time=3 +tries=1 @"$public_ip" -p "$port" "$zone" "$query_type" 2>&1)"
  local rc=$?
  printf '%s\n' "$output"
  if [[ $rc -ne 0 ]] || printf '%s\n' "$output" | grep -Eiq 'connection refused|no servers could be reached|timed out|communications error'; then
    warn "dig ${label} ${query_type} failed for ${public_ip}:${port}."
    mark_failed
  fi
  echo
}

run_dns_queries() {
  local port="$1"

  if [[ -z "$PUBLIC_IP" || -z "$ZONE" ]]; then
    log "DNS query checks"
    echo "skipped=set --public-ip and --zone to run SOA/NS checks"
    echo
    return
  fi

  run_dig_check "$PUBLIC_IP" "$ZONE" "$port" "SOA" "" "udp"
  run_dig_check "$PUBLIC_IP" "$ZONE" "$port" "NS" "" "udp"
  run_dig_check "$PUBLIC_IP" "$ZONE" "$port" "SOA" "+tcp" "tcp"
  run_dig_check "$PUBLIC_IP" "$ZONE" "$port" "NS" "+tcp" "tcp"
}

diagnose_logs() {
  local logs="$1"

  if printf '%s\n' "$logs" | grep -Eiq 'Token authentication failed|invalid token|unauthorized|forbidden'; then
    warn "Logs look like DNS Worker Token authentication failed. Use the DNS Worker Token from 本地自建解析, not an Agent Token or login password."
  fi
  if printf '%s\n' "$logs" | grep -Eiq 'request to Server URL|connection refused|no such host|certificate|tls|timeout'; then
    warn "Logs include Server URL/connectivity/TLS failures. Check DUSHENGCDN_DNS_WORKER_SERVER_URL, DNS, firewall, and certificate trust."
  fi
  if printf '%s\n' "$logs" | grep -Eiq 'address already in use|bind:.*in use'; then
    warn "Logs include a port binding conflict. Check the configured listen address and port ${DNS_PORT:-53}."
  fi
  if printf '%s\n' "$logs" | grep -Eiq 'snapshot|routes=0'; then
    warn "Logs mention snapshots/routes. If SOA/NS work but A/AAAA targets are empty, confirm the site is switched to 本地自建解析 and the Worker pulled a fresh snapshot."
  fi
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

show_logs() {
  local logs

  [[ "$SKIP_LOGS" == "true" ]] && return
  log "Recent DNS Worker logs"
  if command -v journalctl >/dev/null 2>&1; then
    logs="$(journalctl -u "$SERVICE_NAME" -n "$LOG_TAIL" --no-pager 2>&1 || true)"
    if [[ -n "$logs" ]]; then
      printf '%s\n' "$logs" | redact_logs
      diagnose_logs "$logs"
    else
      warn "no journal logs found for ${SERVICE_NAME}."
    fi
  else
    warn "journalctl was not found; skipping service logs."
  fi
  echo
}

show_hints() {
  local port="$1"

  log "Next checks"
  cat <<EOF
If the systemd unit is missing:
  - Install the Worker with scripts/install-dns-worker.sh or rerun scripts/install-server.sh on the panel host.

If only systemd-resolved is listening on 127.0.0.53/127.0.0.54:
  - Public UDP/TCP ${port} is still not served by DNS Worker.
  - Install the Worker or bind it explicitly with --listen PUBLIC_IP:${port}.

If SOA/NS fail with connection refused or timeout:
  - Check local firewall, cloud security group, NAT/port mapping, and upstream provider UDP/TCP ${port} policy.
  - Check that the registrar NS/Glue records point to this Worker public IP.

If SOA/NS work but A/AAAA returns no target:
  - Switch the site to authoritative DNS mode, bind the correct Zone, and check GSLB simulation for no-target reasons.
EOF
  echo
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --public-ip) PUBLIC_IP="$2"; shift 2 ;;
    --zone) ZONE="$2"; shift 2 ;;
    --dns-port) DNS_PORT="$2"; shift 2 ;;
    --log-tail) LOG_TAIL="$2"; shift 2 ;;
    --skip-logs) SKIP_LOGS="true"; shift ;;
    --raw-logs) RAW_LOGS="true"; shift ;;
    -h|--help) usage ;;
    *) warn "unknown option: $1"; exit 2 ;;
  esac
done

INSTALL_DIR="$(abs_path "$INSTALL_DIR")"
ENV_FILE="${ENV_FILE:-${INSTALL_DIR}/dns-worker.env}"
ENV_FILE="$(abs_path "$ENV_FILE")"

listen_addr="$(env_value DUSHENGCDN_DNS_WORKER_LISTEN_ADDR ":53")"
snapshot_path="$(env_value DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH "${INSTALL_DIR}/data/dns-worker-snapshot.json")"
geoip_path="$(env_value DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH "")"
server_url="$(env_value DUSHENGCDN_DNS_WORKER_SERVER_URL "")"
DNS_PORT="$(detect_dns_port "$listen_addr")"

show_target_summary "$listen_addr" "$snapshot_path" "$geoip_path" "$server_url" "$DNS_PORT"
show_systemd_status
show_install_files "$snapshot_path" "$geoip_path"
show_port_listeners "$DNS_PORT"
check_processes
run_dns_queries "$DNS_PORT"
show_logs
show_hints "$DNS_PORT"

exit "$STATUS"

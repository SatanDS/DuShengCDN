#!/usr/bin/env bash
set -euo pipefail

RELEASE_REPO="${DUSHENGCDN_RELEASE_REPO:-SatanDS/SatanDS-DuShengCDN-releases}"
VERSION_TAG="${DUSHENGCDN_VERSION_TAG:-}"
INSTALL_DIR="${DUSHENGCDN_INSTALL_DIR:-/opt/dushengcdn}"
SERVICE_NAME="${DUSHENGCDN_SERVICE_NAME:-dushengcdn}"
HTTP_PORT="${DUSHENGCDN_HTTP_PORT:-3010}"
DB_MODE="${DUSHENGCDN_DB_MODE:-sqlite}"
LICENSE_TOKEN="${DUSHENGCDN_LICENSE_TOKEN:-}"
LICENSE_REQUIRED="${DUSHENGCDN_LICENSE_REQUIRED:-true}"
ACTIVATION_URL="${DUSHENGCDN_LICENSE_ACTIVATION_URL:-https://www.satandu.com}"
AUTO_START="true"

usage() {
  cat <<EOF
DuShengCDN Commercial Installer

Usage:
  install-commercial.sh [OPTIONS]

Options:
  --release-repo REPO      Release repository (default: ${RELEASE_REPO})
  --version TAG            Install a specific release tag instead of latest stable
  --install-dir DIR        Install directory (default: ${INSTALL_DIR})
  --service-name NAME      systemd service name (default: ${SERVICE_NAME})
  --http-port PORT         Panel HTTP port (default: ${HTTP_PORT})
  --license-token TOKEN    Optional commercial license token to install after startup
  --license-required BOOL  Require valid license for commercial resources (default: ${LICENSE_REQUIRED})
  --activation-url URL     Online activation server URL (default: ${ACTIVATION_URL})
  --no-start               Install files but do not start systemd service
  -h, --help               Show this help message

Environment variables with the same names are also supported:
  DUSHENGCDN_RELEASE_REPO, DUSHENGCDN_INSTALL_DIR, DUSHENGCDN_HTTP_PORT,
  DUSHENGCDN_VERSION_TAG, DUSHENGCDN_LICENSE_TOKEN, DUSHENGCDN_LICENSE_REQUIRED,
  DUSHENGCDN_LICENSE_ACTIVATION_URL
EOF
}

log() {
  echo "==> $*"
}

die() {
  echo "Error: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --release-repo) RELEASE_REPO="$2"; shift 2 ;;
    --version|--tag) VERSION_TAG="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    --http-port) HTTP_PORT="$2"; shift 2 ;;
    --license-token) LICENSE_TOKEN="$2"; shift 2 ;;
    --license-required) LICENSE_REQUIRED="$2"; shift 2 ;;
    --activation-url) ACTIVATION_URL="$2"; shift 2 ;;
    --no-start) AUTO_START="false"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

if [[ "$(uname -s)" != "Linux" ]]; then
  die "this installer currently supports Linux only"
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported architecture: ${ARCH}" ;;
esac

case "$INSTALL_DIR" in
  /*) ;;
  *) die "--install-dir must be an absolute path" ;;
esac

case "$INSTALL_DIR" in
  /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var)
    die "refusing to install directly into unsafe directory: ${INSTALL_DIR}"
    ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required"
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  die "sha256sum or shasum is required"
fi

run_as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "this operation requires root or sudo"
  fi
}

random_hex() {
  local bytes="$1"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
  else
    od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
  fi
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  else
    shasum -a 256 "$file" | awk '{print $1}'
  fi
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

wait_for_server() {
  local url="http://127.0.0.1:${HTTP_PORT}/api/status"
  local attempt
  for attempt in $(seq 1 60); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

install_license_token() {
  local root_password="$1"
  local token="$2"
  local base_url="http://127.0.0.1:${HTTP_PORT}"
  local cookie_jar login_body install_body login_response install_response

  if [[ -z "$root_password" || -z "$token" ]]; then
    return 1
  fi

  cookie_jar="$(mktemp "/tmp/dushengcdn-cookie.XXXXXX")"

  login_body="{\"username\":\"root\",\"password\":\"$(json_escape "$root_password")\"}"
  if ! login_response="$(curl -fsS --max-time 10 -c "$cookie_jar" -H 'Content-Type: application/json' -d "$login_body" "${base_url}/api/user/login" 2>/dev/null)"; then
    rm -f "$cookie_jar"
    return 1
  fi
  if ! echo "$login_response" | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
    rm -f "$cookie_jar"
    return 1
  fi

  install_body="{\"token\":\"$(json_escape "$token")\"}"
  if ! install_response="$(curl -fsS --max-time 20 -b "$cookie_jar" -H 'Content-Type: application/json' -d "$install_body" "${base_url}/api/license/install" 2>/dev/null)"; then
    rm -f "$cookie_jar"
    return 1
  fi
  rm -f "$cookie_jar"
  echo "$install_response" | grep -q '"success"[[:space:]]*:[[:space:]]*true'
}

if [[ -n "$VERSION_TAG" ]]; then
  release_json="$(curl -fsSL "https://api.github.com/repos/${RELEASE_REPO}/releases/tags/${VERSION_TAG}")"
else
  release_json="$(curl -fsSL "https://api.github.com/repos/${RELEASE_REPO}/releases/latest")"
fi
asset_name="dushengcdn-server-linux-${ARCH}"
download_url="$(echo "$release_json" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${asset_name}\"" | grep -o 'https://[^"]*' | grep -v '\.sha256$' | head -n 1 || true)"
sha256_url="$(echo "$release_json" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${asset_name}\.sha256\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
tag_name="$(echo "$release_json" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"' || true)"

[[ -n "$download_url" ]] || die "release asset not found: ${asset_name}"
[[ -n "$sha256_url" ]] || die "checksum asset not found: ${asset_name}.sha256"

tmp_binary="$(mktemp "/tmp/dushengcdn-server.XXXXXX")"
tmp_sha="$(mktemp "/tmp/dushengcdn-server.sha256.XXXXXX")"
trap 'rm -f "$tmp_binary" "$tmp_sha"' EXIT

log "Downloading ${asset_name} from ${RELEASE_REPO} ${tag_name:-latest}"
curl -fsSL -o "$tmp_binary" "$download_url"
curl -fsSL -o "$tmp_sha" "$sha256_url"

expected="$(awk '{print $1}' "$tmp_sha")"
actual="$(sha256_file "$tmp_binary")"
[[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]] || die "checksum asset is invalid"
[[ "$actual" == "$expected" ]] || die "downloaded binary checksum mismatch"
chmod +x "$tmp_binary"

log "Installing to ${INSTALL_DIR}"
run_as_root mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
run_as_root install -m 0755 "$tmp_binary" "$INSTALL_DIR/dushengcdn"

env_file="$INSTALL_DIR/dushengcdn.env"
root_password=""
if [[ ! -f "$env_file" ]]; then
  session_secret="$(random_hex 32)"
  initial_root_password="$(random_hex 16)"
  root_password="$initial_root_password"
  run_as_root tee "$env_file" >/dev/null <<EOF
PORT=${HTTP_PORT}
GIN_MODE=release
LOG_LEVEL=info
SESSION_SECRET=${session_secret}
SQLITE_PATH=${INSTALL_DIR}/data/dushengcdn.db
DUSHENGCDN_INITIAL_ROOT_PASSWORD=${initial_root_password}
DUSHENGCDN_LICENSE_REQUIRED=${LICENSE_REQUIRED}
DUSHENGCDN_LICENSE_ACTIVATION_URL=${ACTIVATION_URL}
DUSHENGCDN_LICENSE_ONLINE_ACTIVATION_REQUIRED=true
DUSHENGCDN_LICENSE_LEASE_DURATION_HOURS=72
DUSHENGCDN_LICENSE_LEASE_RENEW_BEFORE_HOURS=6
DUSHENGCDN_SERVER_UPDATE_REPO=${RELEASE_REPO}
EOF
  run_as_root chmod 0600 "$env_file"
else
  log "Keeping existing environment file: ${env_file}"
  root_password="$(run_as_root grep '^DUSHENGCDN_INITIAL_ROOT_PASSWORD=' "$env_file" 2>/dev/null | tail -n1 | sed 's/^DUSHENGCDN_INITIAL_ROOT_PASSWORD=//' || true)"
fi

unit_file="/etc/systemd/system/${SERVICE_NAME}.service"
run_as_root tee "$unit_file" >/dev/null <<EOF
[Unit]
Description=DuShengCDN Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${env_file}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/dushengcdn
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

run_as_root systemctl daemon-reload
run_as_root systemctl enable "$SERVICE_NAME" >/dev/null

if [[ "$AUTO_START" == "true" ]]; then
  log "Starting ${SERVICE_NAME}"
  run_as_root systemctl restart "$SERVICE_NAME"
fi

if [[ -n "$LICENSE_TOKEN" ]]; then
  if [[ "$AUTO_START" == "true" ]] && wait_for_server && install_license_token "$root_password" "$LICENSE_TOKEN"; then
    log "Commercial license token installed and activation was requested."
  else
    log "License token was provided, but automatic install did not complete. Install it from the panel after login: 系统治理 -> 商业授权"
  fi
fi

echo
echo "DuShengCDN commercial server installed."
echo "  Release: ${tag_name:-latest}"
echo "  URL:     http://SERVER_IP:${HTTP_PORT}"
echo "  User:    root"
echo "  Env:     ${env_file}"
echo
echo "Initial root password:"
run_as_root grep '^DUSHENGCDN_INITIAL_ROOT_PASSWORD=' "$env_file" | sed 's/^DUSHENGCDN_INITIAL_ROOT_PASSWORD=//'
echo
echo "Useful commands:"
echo "  systemctl status ${SERVICE_NAME} --no-pager"
echo "  journalctl -u ${SERVICE_NAME} -f"

#!/usr/bin/env bash
set -euo pipefail

RELEASE_REPO="${DUSHENGCDN_RELEASE_REPO:-SatanDS/SatanDS-DuShengCDN-releases}"
VERSION_TAG="${DUSHENGCDN_VERSION_TAG:-}"
INSTALL_DIR="${DUSHENGCDN_INSTALL_DIR:-/opt/dushengcdn}"
SERVICE_NAME="${DUSHENGCDN_SERVICE_NAME:-dushengcdn}"
SERVICE_USER="${DUSHENGCDN_SERVICE_USER:-dushengcdn}"
HTTP_PORT="${DUSHENGCDN_HTTP_PORT:-3010}"
LISTEN_ADDRESS="${DUSHENGCDN_LISTEN_ADDRESS:-127.0.0.1}"
DB_MODE="${DUSHENGCDN_DB_MODE:-sqlite}"
LICENSE_TOKEN="${DUSHENGCDN_LICENSE_TOKEN:-}"
LICENSE_TOKEN_FILE="${DUSHENGCDN_LICENSE_TOKEN_FILE:-}"
ALLOW_INSECURE_TOKEN_ARGV="false"
LICENSE_REQUIRED="${DUSHENGCDN_LICENSE_REQUIRED:-true}"
ACTIVATION_URL="${DUSHENGCDN_LICENSE_ACTIVATION_URL:-https://www.satandu.com}"
RELEASE_SIGNATURE_PUBLIC_KEY="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY__}"
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
  --service-user USER      systemd user to run the Server (default: ${SERVICE_USER})
  --http-port PORT         Panel HTTP port (default: ${HTTP_PORT})
  --listen-address ADDR    Panel bind address (default: ${LISTEN_ADDRESS}; use 0.0.0.0 only behind a firewall)
  --license-token TOKEN    Optional commercial license token to install after startup (prefer --license-token-file)
  --license-token-file FILE Read commercial license token from FILE
  --allow-insecure-token-argv
                          Allow license token in argv for legacy automation; prefer --license-token-file
  --license-required BOOL  Require valid license for commercial resources (default: ${LICENSE_REQUIRED})
  --activation-url URL     Online activation server URL (default: ${ACTIVATION_URL})
  --no-start               Install files but do not start systemd service
  -h, --help               Show this help message

Environment variables with the same names are also supported:
  DUSHENGCDN_RELEASE_REPO, DUSHENGCDN_INSTALL_DIR, DUSHENGCDN_SERVICE_USER,
  DUSHENGCDN_HTTP_PORT, DUSHENGCDN_LISTEN_ADDRESS,
  DUSHENGCDN_VERSION_TAG, DUSHENGCDN_LICENSE_TOKEN, DUSHENGCDN_LICENSE_TOKEN_FILE,
  DUSHENGCDN_LICENSE_REQUIRED, DUSHENGCDN_LICENSE_ACTIVATION_URL
EOF
}

log() {
  echo "==> $*"
}

die() {
  echo "Error: $*" >&2
  exit 1
}

accept_insecure_token_arg() {
  local option_name="$1"
  if [[ "$ALLOW_INSECURE_TOKEN_ARGV" != "true" ]]; then
    die "${option_name} exposes the token in shell history and process arguments; use ${option_name}-file or pass --allow-insecure-token-argv only for legacy automation"
  fi
  echo "Warning: ${option_name} exposes the token in shell history and process arguments; prefer ${option_name}-file" >&2
}

for arg in "$@"; do
  if [[ "$arg" == "--allow-insecure-token-argv" ]]; then
    ALLOW_INSECURE_TOKEN_ARGV="true"
    break
  fi
done

while [[ $# -gt 0 ]]; do
  case "$1" in
    --release-repo) RELEASE_REPO="$2"; shift 2 ;;
    --version|--tag) VERSION_TAG="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    --service-user) SERVICE_USER="$2"; shift 2 ;;
    --http-port) HTTP_PORT="$2"; shift 2 ;;
    --listen-address|--bind-address) LISTEN_ADDRESS="$2"; shift 2 ;;
    --allow-insecure-token-argv) ALLOW_INSECURE_TOKEN_ARGV="true"; shift ;;
    --license-token) accept_insecure_token_arg "--license-token"; LICENSE_TOKEN="$2"; shift 2 ;;
    --license-token-file) LICENSE_TOKEN_FILE="$2"; shift 2 ;;
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

validate_service_name() {
  if [[ -z "$SERVICE_NAME" ]]; then
    die "--service-name must not be empty"
  fi
  case "$SERVICE_NAME" in
    *[!A-Za-z0-9_.@-]*|.*|*-|*@|*..*|*/*)
      die "refusing to use unsafe systemd service name: ${SERVICE_NAME}"
      ;;
  esac
}

validate_service_name

validate_service_user() {
  if [[ -z "$SERVICE_USER" ]]; then
    die "--service-user must not be empty"
  fi
  case "$SERVICE_USER" in
    *[!A-Za-z0-9_.@-]*|.*|*-|*@|*..*|*/*)
      die "refusing to use unsafe systemd service user: ${SERVICE_USER}"
      ;;
  esac
}

validate_service_user

if [[ -n "$LICENSE_TOKEN" && -n "$LICENSE_TOKEN_FILE" ]]; then
  die "use only one of --license-token or --license-token-file"
fi
if [[ -n "$LICENSE_TOKEN_FILE" ]]; then
  [[ -r "$LICENSE_TOKEN_FILE" ]] || die "--license-token-file is not readable"
  LICENSE_TOKEN="$(tr -d '\r\n' < "$LICENSE_TOKEN_FILE")"
fi

command -v curl >/dev/null 2>&1 || die "curl is required"
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  die "sha256sum or shasum is required"
fi
command -v openssl >/dev/null 2>&1 || die "openssl is required for release signature verification"

run_as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "this operation requires root or sudo"
  fi
}

warn() {
  echo "Warning: $*" >&2
}

install_secret_file_from_stdin() {
  local target="$1"
  local tmp
  tmp="$(mktemp "/tmp/dushengcdn-secret.XXXXXX")"
  chmod 0600 "$tmp"
  if ! cat > "$tmp"; then
    rm -f "$tmp"
    return 1
  fi
  if ! run_as_root install -m 0600 "$tmp" "$target"; then
    rm -f "$tmp"
    return 1
  fi
  rm -f "$tmp"
}

ensure_service_user() {
  if [[ "$SERVICE_USER" == "root" ]]; then
    warn "running ${SERVICE_NAME} as root is not recommended"
    return
  fi
  if id -u "$SERVICE_USER" >/dev/null 2>&1; then
    return
  fi
  command -v useradd >/dev/null 2>&1 || die "useradd is required to create service user ${SERVICE_USER}; pass --service-user root only if you accept the risk"
  local nologin_shell
  nologin_shell="/usr/sbin/nologin"
  if [[ ! -x "$nologin_shell" ]]; then
    nologin_shell="/sbin/nologin"
  fi
  if [[ ! -x "$nologin_shell" ]]; then
    nologin_shell="/bin/false"
  fi
  run_as_root useradd --system --home-dir "$INSTALL_DIR" --shell "$nologin_shell" --user-group "$SERVICE_USER"
}

harden_server_install_permissions() {
  if [[ "$SERVICE_USER" == "root" ]]; then
    return
  fi
  run_as_root chown root:root "$INSTALL_DIR" "$INSTALL_DIR/dushengcdn"
  run_as_root chmod 0755 "$INSTALL_DIR" "$INSTALL_DIR/dushengcdn"
  run_as_root mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
  run_as_root chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
  if [[ -f "$env_file" ]]; then
    run_as_root chown root:"$SERVICE_USER" "$env_file"
    run_as_root chmod 0640 "$env_file"
  fi
  if [[ "$root_password_file" == "$INSTALL_DIR/data/initial-root-password.txt" && -f "$root_password_file" ]]; then
    run_as_root chown root:"$SERVICE_USER" "$root_password_file"
    run_as_root chmod 0640 "$root_password_file"
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

base64_with_padding() {
  local value="$1"
  local remainder
  value="$(printf '%s' "$value" | tr -d '[:space:]')"
  remainder=$((${#value} % 4))
  case "$remainder" in
    0) ;;
    2) value="${value}==" ;;
    3) value="${value}=" ;;
    *) return 1 ;;
  esac
  printf '%s' "$value"
}

parse_release_checksum() {
  local file="$1"
  local asset="$2"
  awk -v asset="$asset" '
    function issha(value) { return length(value) == 64 && value !~ /[^0-9A-Fa-f]/ }
    {
      line = $0
      sub(/^[[:space:]]+/, "", line)
      sub(/[[:space:]]+$/, "", line)
      if (line == "" || line ~ /^#/) next
      n = split(line, fields, /[[:space:]]+/)
      if (n == 1 && issha(fields[1])) {
        print tolower(fields[1])
        exit
      }
      if (n >= 2 && issha(fields[1])) {
        name = fields[2]
        sub(/^\*/, "", name)
        base = name
        sub(/^.*\//, "", base)
        if (name == asset || base == asset) {
          print tolower(fields[1])
          exit
        }
      }
      if (index(tolower(line), "sha256(") == 1) {
        end = index(line, ")")
        if (end > 8) {
          name = substr(line, 8, end - 8)
          value = substr(line, end + 1)
          sub(/^[[:space:]]*=[[:space:]]*/, "", value)
          sub(/^[[:space:]]+/, "", value)
          sub(/[[:space:]]+$/, "", value)
          base = name
          sub(/^.*\//, "", base)
          if (issha(value) && (name == asset || base == asset)) {
            print tolower(value)
            exit
          }
        }
      }
    }
  ' "$file"
}

verify_release_signature() {
  local tag="$1"
  local asset="$2"
  local checksum="$3"
  local signature_file="$4"
  local public_key placeholder key_b64 sig_text sig_b64 verify_dir pub_raw sig_raw pub_der pub_pem payload pub_len sig_len

  placeholder="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC""_KEY__"
  public_key="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-$RELEASE_SIGNATURE_PUBLIC_KEY}"
  [[ -n "$tag" && -n "$asset" && -n "$checksum" ]] || return 1
  [[ -n "$public_key" && "$public_key" != "$placeholder" ]] || return 1
  command -v openssl >/dev/null 2>&1 || return 1

  key_b64="$(base64_with_padding "$public_key")" || return 1
  sig_text="$(awk 'NF { print $1; exit }' "$signature_file")"
  [[ -n "$sig_text" ]] || return 1
  sig_b64="$(base64_with_padding "$sig_text")" || return 1

  verify_dir="$(mktemp -d "/tmp/dushengcdn-release-verify.XXXXXX")"
  pub_raw="${verify_dir}/public.raw"
  sig_raw="${verify_dir}/signature.raw"
  pub_der="${verify_dir}/public.der"
  pub_pem="${verify_dir}/public.pem"
  payload="${verify_dir}/payload.txt"

  if ! printf '%s' "$key_b64" | openssl base64 -d -A > "$pub_raw" 2>/dev/null; then
    rm -rf -- "$verify_dir"
    return 1
  fi
  pub_len="$(wc -c < "$pub_raw" | tr -d '[:space:]')"
  if [[ "$pub_len" != "32" ]]; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  if ! printf '%s' "$sig_b64" | openssl base64 -d -A > "$sig_raw" 2>/dev/null; then
    rm -rf -- "$verify_dir"
    return 1
  fi
  sig_len="$(wc -c < "$sig_raw" | tr -d '[:space:]')"
  if [[ "$sig_len" != "64" ]]; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00' > "$pub_der"
  cat "$pub_raw" >> "$pub_der"
  if ! openssl pkey -pubin -inform DER -in "$pub_der" -out "$pub_pem" >/dev/null 2>&1; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  {
    printf 'dushengcdn-release-v1\n'
    printf '%s\n' "$tag"
    printf '%s\n' "$asset"
    printf '%s\n' "$checksum"
  } > "$payload"

  if ! openssl pkeyutl -verify -pubin -inkey "$pub_pem" -sigfile "$sig_raw" -rawin -in "$payload" >/dev/null 2>&1; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  rm -rf -- "$verify_dir"
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
  if ! login_response="$(printf '%s' "$login_body" | curl -fsS --max-time 10 -c "$cookie_jar" -H 'Content-Type: application/json' --data-binary @- "${base_url}/api/user/login" 2>/dev/null)"; then
    rm -f "$cookie_jar"
    return 1
  fi
  if ! echo "$login_response" | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
    rm -f "$cookie_jar"
    return 1
  fi

  install_body="{\"token\":\"$(json_escape "$token")\"}"
  if ! install_response="$(printf '%s' "$install_body" | curl -fsS --max-time 20 -b "$cookie_jar" -H 'Content-Type: application/json' --data-binary @- "${base_url}/api/license/install" 2>/dev/null)"; then
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
download_url="$(echo "$release_json" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${asset_name}\"" | grep -o 'https://[^"]*' | grep -v '\.sha256$' | grep -v '\.sig$' | head -n 1 || true)"
sha256_url="$(echo "$release_json" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${asset_name}\.sha256\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
sig_url="$(echo "$release_json" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${asset_name}\.sig\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
tag_name="$(echo "$release_json" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"' || true)"

[[ -n "$download_url" ]] || die "release asset not found: ${asset_name}"
[[ -n "$sha256_url" ]] || die "checksum asset not found: ${asset_name}.sha256"
[[ -n "$sig_url" ]] || die "signature asset not found: ${asset_name}.sig"
[[ -n "$tag_name" ]] || die "release tag not found"

tmp_binary="$(mktemp "/tmp/dushengcdn-server.XXXXXX")"
tmp_sha="$(mktemp "/tmp/dushengcdn-server.sha256.XXXXXX")"
tmp_sig="$(mktemp "/tmp/dushengcdn-server.sig.XXXXXX")"
trap 'rm -f "$tmp_binary" "$tmp_sha" "$tmp_sig"' EXIT

log "Downloading ${asset_name} from ${RELEASE_REPO} ${tag_name:-latest}"
curl -fsSL -o "$tmp_binary" "$download_url"
curl -fsSL -o "$tmp_sha" "$sha256_url"
curl -fsSL -o "$tmp_sig" "$sig_url"

expected="$(parse_release_checksum "$tmp_sha" "$asset_name")"
actual="$(sha256_file "$tmp_binary")"
[[ "$expected" =~ ^[A-Fa-f0-9]{64}$ ]] || die "checksum asset is invalid"
[[ "$actual" == "$expected" ]] || die "downloaded binary checksum mismatch"
verify_release_signature "$tag_name" "$asset_name" "$expected" "$tmp_sig" || die "release signature verification failed"
log "Release asset checksum and signature verified."
chmod +x "$tmp_binary"

log "Installing to ${INSTALL_DIR}"
ensure_service_user
run_as_root mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/logs"
run_as_root install -m 0755 "$tmp_binary" "$INSTALL_DIR/dushengcdn"

env_file="$INSTALL_DIR/dushengcdn.env"
root_password=""
root_password_file="$INSTALL_DIR/data/initial-root-password.txt"
if [[ ! -f "$env_file" ]]; then
  session_secret="$(random_hex 32)"
  initial_root_password="$(random_hex 16)"
  root_password="$initial_root_password"
  install_secret_file_from_stdin "$root_password_file" <<EOF
username=root
password=${initial_root_password}
EOF
  install_secret_file_from_stdin "$env_file" <<EOF
PORT=${HTTP_PORT}
DUSHENGCDN_LISTEN_ADDRESS=${LISTEN_ADDRESS}
GIN_MODE=release
LOG_LEVEL=info
SESSION_SECRET=${session_secret}
SQLITE_PATH=${INSTALL_DIR}/data/dushengcdn.db
DUSHENGCDN_INITIAL_ROOT_PASSWORD=
DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE=${root_password_file}
DUSHENGCDN_LICENSE_REQUIRED=${LICENSE_REQUIRED}
DUSHENGCDN_LICENSE_ACTIVATION_URL=${ACTIVATION_URL}
DUSHENGCDN_LICENSE_ONLINE_ACTIVATION_REQUIRED=true
DUSHENGCDN_LICENSE_LEASE_DURATION_HOURS=72
DUSHENGCDN_LICENSE_LEASE_RENEW_BEFORE_HOURS=6
DUSHENGCDN_SERVER_UPDATE_REPO=${RELEASE_REPO}
EOF
else
  log "Keeping existing environment file: ${env_file}"
  configured_root_password_file="$(run_as_root grep '^DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE=' "$env_file" 2>/dev/null | tail -n1 | sed 's/^DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE=//' || true)"
  if [[ -n "$configured_root_password_file" ]]; then
    if [[ "$configured_root_password_file" == "$INSTALL_DIR/data/initial-root-password.txt" ]]; then
      root_password_file="$configured_root_password_file"
      root_password="$(run_as_root awk -F= '/^password=/ {print substr($0, index($0, $2)); exit} /^[^=]+$/ {print; exit}' "$root_password_file" 2>/dev/null || true)"
    else
      warn "ignoring non-default DUSHENGCDN_INITIAL_ROOT_PASSWORD_FILE in existing environment file"
      root_password_file=""
      root_password=""
    fi
  else
    root_password_file=""
    root_password="$(run_as_root grep '^DUSHENGCDN_INITIAL_ROOT_PASSWORD=' "$env_file" 2>/dev/null | tail -n1 | sed 's/^DUSHENGCDN_INITIAL_ROOT_PASSWORD=//' || true)"
  fi
fi

if [[ "$SERVICE_USER" != "root" ]]; then
  harden_server_install_permissions
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
User=${SERVICE_USER}
Group=${SERVICE_USER}
ExecStart=${INSTALL_DIR}/dushengcdn
Restart=always
RestartSec=10
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=${INSTALL_DIR}/data ${INSTALL_DIR}/logs

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
echo "  URL:     http://${LISTEN_ADDRESS}:${HTTP_PORT}"
echo "  Login:   root"
echo "  Service user: ${SERVICE_USER}"
echo "  Env:     ${env_file}"
if [[ "$LISTEN_ADDRESS" == "127.0.0.1" || "$LISTEN_ADDRESS" == "::1" || "$LISTEN_ADDRESS" == "localhost" ]]; then
  echo "  Note:    The panel is bound to loopback by default. Put it behind an HTTPS reverse proxy, or pass --listen-address 0.0.0.0 only with firewall protection."
fi
if [[ -n "$root_password_file" ]]; then
  echo "  Initial root password file: ${root_password_file}"
else
  echo "  Initial root password: configured in ${env_file} (not printed)"
fi
echo
echo "Useful commands:"
echo "  systemctl status ${SERVICE_NAME} --no-pager"
echo "  journalctl -u ${SERVICE_NAME} -f"

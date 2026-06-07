#!/usr/bin/env bash
set -euo pipefail

# DuShengCDN DNS Worker Installer
# Usage:
#   curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \
#     --server-url https://cdn.example.com \
#     --token your-dns-worker-token

INSTALL_DIR="/opt/dushengcdn-dns-worker"
REPO="${DUSHENGCDN_RELEASE_REPO:-SatanDS/SatanDS-DuShengCDN-releases}"
RELEASE_SIGNATURE_PUBLIC_KEY="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY__"
SOURCE_REF="${SOURCE_REF:-main}"
ALLOW_SOURCE_BUILD="${DUSHENGCDN_ALLOW_SOURCE_BUILD:-false}"
RELEASE_CHANNEL="${DUSHENGCDN_DNS_WORKER_RELEASE_CHANNEL:-stable}"
RELEASE_TAG="${DUSHENGCDN_DNS_WORKER_RELEASE_TAG:-}"
SERVER_URL=""
TOKEN=""
SERVICE_NAME="${DUSHENGCDN_DNS_WORKER_SERVICE_NAME:-}"
CREATE_SERVICE="true"
AUTO_INSTALL_DEPS="true"
LISTEN_ADDR="${DUSHENGCDN_DNS_WORKER_LISTEN_ADDR:-}"
SNAPSHOT_PATH=""
SOURCE_DATABASE_PROFILE="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE:-}"
GEOIP_DATABASE=""
GEOIP_DATABASE_EXPLICIT="false"
GEOIP_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL:-}"
AUTO_GEOIP_DOWNLOAD="true"
ASN_DATABASE=""
ASN_DATABASE_EXPLICIT="false"
ASN_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL:-}"
AUTO_ASN_DOWNLOAD="true"
OPERATOR_CIDR_DATABASE=""
OPERATOR_CIDR_DATABASE_EXPLICIT="false"
OPERATOR_CIDR_BASE_URL="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL:-}"
OPERATOR_CIDR_FILES="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES:-}"
AUTO_OPERATOR_CIDR_DOWNLOAD="true"
SOURCE_DATABASE_METADATA_DIR=""
SOURCE_DATABASE_UPDATE_TIMER="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_UPDATE_TIMER:-}"
HEARTBEAT_INTERVAL="${DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL:-}"
REQUEST_TIMEOUT="${DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT:-}"
SNAPSHOT_MAX_AGE="${DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE:-}"
QUERY_RATE_LIMIT="${DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT:-}"
UDP_RESPONSE_SIZE="${DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE:-}"
LOG_LEVEL_VALUE="${LOG_LEVEL:-}"
DUSHENGCDN_BUILD_GO_DIR="${DUSHENGCDN_BUILD_GO_DIR:-/opt/dushengcdn-build/go}"
OPENSSL_BIN=""
FORCE_OVERWRITE_ENV="false"

usage() {
  cat <<EOF
DuShengCDN DNS Worker Installer

Usage:
  install-dns-worker.sh [OPTIONS]

Options:
  --server-url URL           Server URL (required)
  --token TOKEN              DNS Worker token (required)
  --dns-worker-token TOKEN   Alias of --token
  --install-dir DIR          Installation directory (default: /opt/dushengcdn-dns-worker)
  --listen ADDR              DNS UDP/TCP listen address (default: :53)
  --snapshot-path PATH       Snapshot cache path (default: INSTALL_DIR/data/dns-worker-snapshot.json)
  --source-database-profile PROFILE
                             Source database preset: full, country, asn, operator, none (default: full)
  --source-database-metadata-dir DIR
                             Source database metadata directory (default: INSTALL_DIR/data/source-database-metadata)
  --geoip-database PATH      Optional local MaxMind Country/City/Enterprise MMDB path
  --geoip-database-url URL   Country MMDB download URL (default: Loyalsoldier GeoLite2-Country)
  --asn-database PATH        Optional local MaxMind ASN MMDB path
  --asn-database-url URL     ASN MMDB download URL (default: Loyalsoldier GeoLite2-ASN)
  --operator-cidr-database PATH
                             Optional local gaoyifan/china-operator-ip CIDR directory or file
  --operator-cidr-base-url URL
                             Operator CIDR raw base URL (default: gaoyifan/china-operator-ip ip-lists)
  --operator-cidr-files LIST Space-separated operator CIDR files to download
  --no-geoip-download        Do not download Country MMDB automatically
  --no-asn-download          Do not download ASN MMDB automatically
  --no-operator-cidr-download
                             Do not download China operator CIDR lists automatically
  --no-source-database-download
                             Do not download any source database automatically
  --no-source-database-update-timer
                             Do not create the 7-day source database update timer
  --heartbeat-interval DUR   Heartbeat and snapshot pull interval (default: 10s)
  --request-timeout DUR      Server request timeout (default: 10s)
  --snapshot-max-age DUR     Maximum dynamic-answer snapshot age (default: 5m)
  --query-rate-limit NUM     Per-source-IP DNS queries per second; 0 disables (default: 200)
  --udp-response-size NUM    Maximum UDP DNS response payload size (default: 1232)
  --log-level LEVEL          debug, info, warn, or error (default: info)
  --service-name NAME        systemd service name (default: dushengcdn-dns-worker)
  --repo REPO                GitHub release repository (default: ${REPO})
  --release-channel CHANNEL  Release channel: stable or preview (default: stable)
  --release-tag TAG          Install a specific release tag
  --source-ref REF           Git branch, tag, or commit used when building from source (default: main)
  --allow-source-build       Allow fallback source build when no release binary is available
  --install-deps             Install missing download/build dependencies automatically (default)
  --no-install-deps          Do not install missing dependencies automatically
  --no-service               Do not create systemd service
  --force-overwrite-env      Overwrite existing dns-worker.env instead of reusing it as defaults
  -h, --help                 Show this help message

Examples:
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token --geoip-database /var/lib/GeoLite2-Country.mmdb
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token --source-database-profile operator
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token --no-source-database-download

Notes:
  The full preset downloads Country + ASN MMDBs and gaoyifan/china-operator-ip operator CIDR lists.
  Reinstall keeps the data directory and snapshot cache, then replaces the binary
  and environment file. Source databases are updated in place and old managed
  database directories are removed after a successful replacement.
  Use uninstall-dns-worker.sh to remove local data.
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-url) SERVER_URL="$2"; shift 2 ;;
    --token|--dns-worker-token) TOKEN="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --listen) LISTEN_ADDR="$2"; shift 2 ;;
    --snapshot-path) SNAPSHOT_PATH="$2"; shift 2 ;;
    --source-database-profile) SOURCE_DATABASE_PROFILE="$2"; shift 2 ;;
    --source-database-metadata-dir) SOURCE_DATABASE_METADATA_DIR="$2"; shift 2 ;;
    --geoip-database) GEOIP_DATABASE="$2"; GEOIP_DATABASE_EXPLICIT="true"; shift 2 ;;
    --geoip-database-url) GEOIP_DATABASE_URL="$2"; shift 2 ;;
    --asn-database) ASN_DATABASE="$2"; ASN_DATABASE_EXPLICIT="true"; shift 2 ;;
    --asn-database-url) ASN_DATABASE_URL="$2"; shift 2 ;;
    --operator-cidr-database) OPERATOR_CIDR_DATABASE="$2"; OPERATOR_CIDR_DATABASE_EXPLICIT="true"; shift 2 ;;
    --operator-cidr-base-url) OPERATOR_CIDR_BASE_URL="$2"; shift 2 ;;
    --operator-cidr-files) OPERATOR_CIDR_FILES="$2"; shift 2 ;;
    --no-geoip-download) AUTO_GEOIP_DOWNLOAD="false"; shift ;;
    --no-asn-download) AUTO_ASN_DOWNLOAD="false"; shift ;;
    --no-operator-cidr-download) AUTO_OPERATOR_CIDR_DOWNLOAD="false"; shift ;;
    --no-source-database-download) AUTO_GEOIP_DOWNLOAD="false"; AUTO_ASN_DOWNLOAD="false"; AUTO_OPERATOR_CIDR_DOWNLOAD="false"; shift ;;
    --no-source-database-update-timer) SOURCE_DATABASE_UPDATE_TIMER="false"; shift ;;
    --heartbeat-interval) HEARTBEAT_INTERVAL="$2"; shift 2 ;;
    --request-timeout) REQUEST_TIMEOUT="$2"; shift 2 ;;
    --snapshot-max-age) SNAPSHOT_MAX_AGE="$2"; shift 2 ;;
    --query-rate-limit) QUERY_RATE_LIMIT="$2"; shift 2 ;;
    --udp-response-size) UDP_RESPONSE_SIZE="$2"; shift 2 ;;
    --log-level) LOG_LEVEL_VALUE="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    --release-channel) RELEASE_CHANNEL="$2"; shift 2 ;;
    --release-tag) RELEASE_TAG="$2"; shift 2 ;;
    --source-ref) SOURCE_REF="$2"; shift 2 ;;
    --allow-source-build) ALLOW_SOURCE_BUILD="true"; shift ;;
    --install-deps) AUTO_INSTALL_DEPS="true"; shift ;;
    --no-install-deps) AUTO_INSTALL_DEPS="false"; shift ;;
    --no-service) CREATE_SERVICE="false"; shift ;;
    --force-overwrite-env) FORCE_OVERWRITE_ENV="true"; shift ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

log() {
  echo "==> $*"
}

die() {
  echo "Error: $*" >&2
  exit 1
}

run_as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "this operation requires root or sudo."
  fi
}

write_file_as_root() {
  local target="$1"
  local mode="$2"
  local tmp

  tmp="$(mktemp)"
  cat > "$tmp"
  run_as_root install -m "$mode" "$tmp" "$target"
  rm -f "$tmp"
}

env_quote() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '"%s"' "$value"
}

load_existing_env_defaults() {
  local env_file="${INSTALL_DIR}/dns-worker.env"
  [[ "$FORCE_OVERWRITE_ENV" != "true" && -f "$env_file" ]] || return 0
  set -a
  # shellcheck disable=SC1090
  . "$env_file"
  set +a
  [[ -n "$SERVER_URL" ]] || SERVER_URL="${DUSHENGCDN_DNS_WORKER_SERVER_URL:-}"
  [[ -n "$TOKEN" ]] || TOKEN="${DUSHENGCDN_DNS_WORKER_TOKEN:-}"
  [[ -n "$LISTEN_ADDR" ]] || LISTEN_ADDR="${DUSHENGCDN_DNS_WORKER_LISTEN_ADDR:-:53}"
  [[ -n "$SNAPSHOT_PATH" ]] || SNAPSHOT_PATH="${DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH:-}"
  [[ -n "$GEOIP_DATABASE" ]] || GEOIP_DATABASE="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH:-}"
  [[ -n "$ASN_DATABASE" ]] || ASN_DATABASE="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH:-}"
  [[ -n "$OPERATOR_CIDR_DATABASE" ]] || OPERATOR_CIDR_DATABASE="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH:-}"
  [[ -n "$SOURCE_DATABASE_PROFILE" ]] || SOURCE_DATABASE_PROFILE="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE:-}"
  [[ -n "$SOURCE_DATABASE_METADATA_DIR" ]] || SOURCE_DATABASE_METADATA_DIR="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_METADATA_DIR:-}"
  [[ -n "$GEOIP_DATABASE_URL" ]] || GEOIP_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL:-}"
  [[ -n "$ASN_DATABASE_URL" ]] || ASN_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL:-}"
  [[ -n "$OPERATOR_CIDR_BASE_URL" ]] || OPERATOR_CIDR_BASE_URL="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL:-}"
  [[ -n "$OPERATOR_CIDR_FILES" ]] || OPERATOR_CIDR_FILES="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES:-}"
  [[ -n "$SERVICE_NAME" ]] || SERVICE_NAME="${DUSHENGCDN_DNS_WORKER_SERVICE_NAME:-}"
  [[ -n "$SOURCE_DATABASE_UPDATE_TIMER" ]] || SOURCE_DATABASE_UPDATE_TIMER="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_UPDATE_TIMER:-}"
  [[ -n "$HEARTBEAT_INTERVAL" ]] || HEARTBEAT_INTERVAL="${DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL:-}"
  [[ -n "$REQUEST_TIMEOUT" ]] || REQUEST_TIMEOUT="${DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT:-}"
  [[ -n "$SNAPSHOT_MAX_AGE" ]] || SNAPSHOT_MAX_AGE="${DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE:-}"
  [[ -n "$QUERY_RATE_LIMIT" ]] || QUERY_RATE_LIMIT="${DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT:-}"
  [[ -n "$UDP_RESPONSE_SIZE" ]] || UDP_RESPONSE_SIZE="${DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE:-}"
  [[ -n "$LOG_LEVEL_VALUE" ]] || LOG_LEVEL_VALUE="${LOG_LEVEL:-}"
}

apply_dns_worker_defaults() {
  [[ -n "$SERVICE_NAME" ]] || SERVICE_NAME="dushengcdn-dns-worker"
  [[ -n "$LISTEN_ADDR" ]] || LISTEN_ADDR=":53"
  [[ -n "$SOURCE_DATABASE_PROFILE" ]] || SOURCE_DATABASE_PROFILE="full"
  [[ -n "$GEOIP_DATABASE_URL" ]] || GEOIP_DATABASE_URL="https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb"
  [[ -n "$ASN_DATABASE_URL" ]] || ASN_DATABASE_URL="https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-ASN.mmdb"
  [[ -n "$OPERATOR_CIDR_BASE_URL" ]] || OPERATOR_CIDR_BASE_URL="https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists"
  [[ -n "$OPERATOR_CIDR_FILES" ]] || OPERATOR_CIDR_FILES="chinanet.txt chinanet6.txt cmcc.txt cmcc6.txt unicom.txt unicom6.txt cernet.txt cernet6.txt cstnet.txt cstnet6.txt drpeng.txt drpeng6.txt googlecn.txt googlecn6.txt"
  [[ -n "$SOURCE_DATABASE_UPDATE_TIMER" ]] || SOURCE_DATABASE_UPDATE_TIMER="true"
  [[ -n "$HEARTBEAT_INTERVAL" ]] || HEARTBEAT_INTERVAL="10s"
  [[ -n "$REQUEST_TIMEOUT" ]] || REQUEST_TIMEOUT="10s"
  [[ -n "$SNAPSHOT_MAX_AGE" ]] || SNAPSHOT_MAX_AGE="5m"
  [[ -n "$QUERY_RATE_LIMIT" ]] || QUERY_RATE_LIMIT="200"
  [[ -n "$UDP_RESPONSE_SIZE" ]] || UDP_RESPONSE_SIZE="1232"
  [[ -n "$LOG_LEVEL_VALUE" ]] || LOG_LEVEL_VALUE="info"
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

is_wildcard_listen_addr() {
  case "$1" in
    :*|0.0.0.0:*|[[]::[]]:*|\*:*) return 0 ;;
    *) return 1 ;;
  esac
}

listen_port_in_use() {
  local port="$1"
  local output

  if command -v ss >/dev/null 2>&1; then
    if output="$(ss -H -lntu "( sport = :${port} )" 2>/dev/null)" && [[ -n "$output" ]]; then
      return 0
    fi
    if output="$(ss -H -lntu 2>/dev/null)" && echo "$output" | awk -v port=":${port}" '
      {
        for (i = 1; i <= NF; i++) {
          if ($i ~ port "$") {
            found = 1
          }
        }
      }
      END { exit found ? 0 : 1 }
    '; then
      return 0
    fi
  fi

  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | awk 'NR > 1 { found = 1 } END { exit found ? 0 : 1 }'; then
      return 0
    fi
    if lsof -nP -iUDP:"$port" 2>/dev/null | awk 'NR > 1 { found = 1 } END { exit found ? 0 : 1 }'; then
      return 0
    fi
  fi

  if command -v netstat >/dev/null 2>&1; then
    if netstat -an 2>/dev/null | awk -v port="$port" '
      ($0 ~ /LISTEN/ || $1 ~ /^udp/i) {
        for (i = 1; i <= NF; i++) {
          if ($i ~ ("[.:]" port "$")) {
            found = 1
          }
        }
      }
      END { exit found ? 0 : 1 }
    '; then
      return 0
    fi
  fi

  return 1
}

check_listen_port_available() {
  local port

  port="$(listen_port_from_addr "$LISTEN_ADDR" || true)"
  if [[ -z "$port" || "$port" == "0" ]]; then
    return
  fi
  if ! is_wildcard_listen_addr "$LISTEN_ADDR"; then
    return
  fi
  if ! listen_port_in_use "$port"; then
    return
  fi

  cat >&2 <<EOF
Error: UDP/TCP port ${port} is already in use, and --listen ${LISTEN_ADDR} binds all local addresses.
Stop or reconfigure the existing local DNS service first. Common examples are systemd-resolved, named, and dnsmasq.
If the existing service only binds a loopback address, rerun with an explicit public address such as --listen PUBLIC_IP:${port}; for local testing, use a high port such as --listen 127.0.0.1:1053.
Useful checks:
  ss -lntu '( sport = :${port} )'
  lsof -nP -i :${port}
EOF
  exit 1
}

validate_install_dir() {
  while [[ "$INSTALL_DIR" != "/" && "$INSTALL_DIR" == */ ]]; do
    INSTALL_DIR="${INSTALL_DIR%/}"
  done

  case "$INSTALL_DIR" in
    /*) ;;
    *) die "--install-dir must be an absolute path." ;;
  esac

  case "$INSTALL_DIR" in
    *"/../"*|*/..|*"/./"*|*/.)
      die "refusing to use non-normalized install directory: ${INSTALL_DIR}"
      ;;
  esac

  case "$INSTALL_DIR" in
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var|/Applications)
      die "refusing to use unsafe install directory: ${INSTALL_DIR}"
      ;;
  esac
}

validate_build_go_dir() {
  while [[ "$DUSHENGCDN_BUILD_GO_DIR" != "/" && "$DUSHENGCDN_BUILD_GO_DIR" == */ ]]; do
    DUSHENGCDN_BUILD_GO_DIR="${DUSHENGCDN_BUILD_GO_DIR%/}"
  done

  case "$DUSHENGCDN_BUILD_GO_DIR" in
    /*) ;;
    *) die "DUSHENGCDN_BUILD_GO_DIR must be an absolute path." ;;
  esac

  case "$DUSHENGCDN_BUILD_GO_DIR" in
    *"/../"*|*/..|*"/./"*|*/.)
      die "refusing to use non-normalized Go build directory: ${DUSHENGCDN_BUILD_GO_DIR}"
      ;;
  esac

  case "$DUSHENGCDN_BUILD_GO_DIR" in
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var|/Applications|/usr/local|/usr/local/go)
      die "refusing to use unsafe Go build directory: ${DUSHENGCDN_BUILD_GO_DIR}"
      ;;
  esac
}

install_common_linux_dependencies() {
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get update
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl openssl
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl openssl
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl openssl
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl openssl
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl openssl
  else
    die "no supported package manager found. Install curl manually or rerun with --no-install-deps after preparing dependencies."
  fi
}

install_source_build_dependencies_linux() {
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get update
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl git tar
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl git tar
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl git tar
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl git tar
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl git tar
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl git tar
  else
    die "no supported package manager found. Install git, tar, and Go manually, or publish release assets."
  fi
}

ensure_curl() {
  if command -v curl >/dev/null 2>&1; then
    return
  fi
  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    die "curl was not found. Install curl first or rerun without --no-install-deps."
  fi
  case "$OS" in
    linux) install_common_linux_dependencies ;;
    darwin) die "curl was not found. Install curl first, then rerun the installer." ;;
    *) die "unsupported OS for automatic dependency installation: $OS" ;;
  esac
}

install_go_linux() {
  local go_version="${DUSHENGCDN_GO_VERSION:-1.26.4}"
  local archive
  archive="$(mktemp "/tmp/go${go_version}.linux-${ARCH}.XXXXXX.tar.gz")"
  local default_bases="https://go.dev/dl https://dl.google.com/go https://golang.google.cn/dl"
  local urls=()
  local base url attempt

  if [[ -n "${DUSHENGCDN_GO_DOWNLOAD_URL:-}" ]]; then
    urls+=("$DUSHENGCDN_GO_DOWNLOAD_URL")
  fi
  for base in ${DUSHENGCDN_GO_DOWNLOAD_BASE_URLS:-$default_bases}; do
    urls+=("${base%/}/go${go_version}.linux-${ARCH}.tar.gz")
  done

  log "Installing Go ${go_version} for linux/${ARCH}..."
  for url in "${urls[@]}"; do
    for attempt in 1 2 3; do
      rm -f "$archive"
      log "Downloading Go from ${url} (attempt ${attempt}/3)..."
      if curl --fail --location --show-error --silent --connect-timeout 20 --retry 2 --retry-delay 2 --retry-max-time 300 -o "$archive" "$url" && tar -tzf "$archive" >/dev/null 2>&1; then
        install_go_archive "$archive"
        rm -f "$archive"
        return
      fi
      log "Go download failed or archive is invalid; trying again if possible."
    done
  done

  rm -f "$archive"
  die "failed to download Go ${go_version}. Install Go manually, set DUSHENGCDN_GO_DOWNLOAD_URL, or publish release assets."
}

install_go_archive() {
  local archive="$1"
  local parent
  parent="$(dirname "$DUSHENGCDN_BUILD_GO_DIR")"
  run_as_root mkdir -p "$parent"
  run_as_root rm -rf -- "${DUSHENGCDN_BUILD_GO_DIR}.tmp"
  run_as_root mkdir -p "${DUSHENGCDN_BUILD_GO_DIR}.tmp"
  run_as_root tar -C "${DUSHENGCDN_BUILD_GO_DIR}.tmp" --strip-components=1 -xzf "$archive"
  run_as_root rm -rf -- "$DUSHENGCDN_BUILD_GO_DIR"
  run_as_root mv "${DUSHENGCDN_BUILD_GO_DIR}.tmp" "$DUSHENGCDN_BUILD_GO_DIR"
}

use_local_go_if_available() {
  if [[ -x "${DUSHENGCDN_BUILD_GO_DIR}/bin/go" ]]; then
    export PATH="${DUSHENGCDN_BUILD_GO_DIR}/bin:${PATH}"
    return
  fi
  if [[ -x /usr/local/go/bin/go ]]; then
    export PATH="/usr/local/go/bin:${PATH}"
  fi
}

ensure_go() {
  use_local_go_if_available
  if command -v go >/dev/null 2>&1; then
    return
  fi
  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    die "go was not found and no release binary is available. Install Go first or rerun without --no-install-deps."
  fi
  case "$OS" in
    linux)
      install_source_build_dependencies_linux
      install_go_linux
      ;;
    darwin)
      if ! command -v brew >/dev/null 2>&1; then
        die "Homebrew is required to install Go automatically on macOS. Install Go manually or publish release assets."
      fi
      brew install go
      ;;
    *) die "unsupported OS for automatic Go installation: $OS" ;;
  esac
  use_local_go_if_available
  command -v go >/dev/null 2>&1 || die "Go installation completed, but go is still not available in PATH."
}

ensure_source_build_tools() {
  if command -v git >/dev/null 2>&1 && command -v tar >/dev/null 2>&1; then
    return
  fi
  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    die "git or tar was not found and no release binary is available. Install git/tar first or rerun without --no-install-deps."
  fi
  case "$OS" in
    linux) install_source_build_dependencies_linux ;;
    darwin) die "git or tar was not found. Install Xcode Command Line Tools or Git, then rerun the installer." ;;
    *) die "unsupported OS for automatic source build dependencies: $OS" ;;
  esac
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    return 1
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
    function issha(value) { return value ~ /^[0-9A-Fa-f]{64}$/ }
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

find_openssl() {
  local candidate
  if [[ -n "$OPENSSL_BIN" && -x "$OPENSSL_BIN" ]]; then
    return 0
  fi
  if [[ "${OS:-}" == "darwin" ]]; then
    for candidate in \
      /opt/homebrew/opt/openssl@3/bin/openssl \
      /usr/local/opt/openssl@3/bin/openssl \
      /opt/homebrew/bin/openssl \
      /usr/local/bin/openssl; do
      if [[ -x "$candidate" ]]; then
        OPENSSL_BIN="$candidate"
        return 0
      fi
    done
  fi
  if command -v openssl >/dev/null 2>&1; then
    OPENSSL_BIN="$(command -v openssl)"
    return 0
  fi
  return 1
}

install_release_signature_dependencies_linux() {
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get update
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl openssl
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl openssl
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl openssl
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl openssl
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl openssl
  else
    die "no supported package manager found. Install openssl manually or rerun with --allow-source-build."
  fi
}

ensure_release_signature_openssl() {
  if find_openssl; then
    return
  fi
  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    die "openssl was not found. Install openssl first or rerun with --allow-source-build."
  fi
  case "$OS" in
    linux)
      log "openssl was not found. Installing release signature verification dependency..."
      install_release_signature_dependencies_linux
      ;;
    darwin)
      if command -v brew >/dev/null 2>&1; then
        log "openssl was not found. Installing openssl via Homebrew..."
        brew install openssl@3 || brew install openssl
      else
        die "openssl was not found. Install OpenSSL 3, or rerun with --allow-source-build."
      fi
      ;;
    *) die "unsupported OS for automatic openssl installation: $OS" ;;
  esac
  find_openssl || die "openssl installation completed, but openssl was still not found."
}

verify_release_signature() {
  local tag="$1"
  local asset="$2"
  local checksum="$3"
  local signature_file="$4"
  local public_key placeholder key_b64 sig_text sig_b64 verify_dir pub_raw sig_raw pub_der pub_pem payload pub_len sig_len verify_log

  placeholder="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC""_KEY__"
  public_key="$RELEASE_SIGNATURE_PUBLIC_KEY"
  if [[ "$public_key" == "$placeholder" ]]; then
    public_key="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-}"
  fi
  [[ -n "$tag" && -n "$asset" && -n "$checksum" ]] || return 1
  if [[ -z "$public_key" || "$public_key" == "$placeholder" ]]; then
    log "Release signature public key is not configured."
    return 1
  fi

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
  verify_log="${verify_dir}/openssl-verify.log"

  if ! printf '%s' "$key_b64" | "$OPENSSL_BIN" base64 -d -A > "$pub_raw" 2>/dev/null; then
    log "Release signature public key is not valid base64."
    rm -rf -- "$verify_dir"
    return 1
  fi
  pub_len="$(wc -c < "$pub_raw" | tr -d '[:space:]')"
  if [[ "$pub_len" != "32" ]]; then
    log "Release signature public key length is invalid: ${pub_len} bytes."
    rm -rf -- "$verify_dir"
    return 1
  fi

  if ! printf '%s' "$sig_b64" | "$OPENSSL_BIN" base64 -d -A > "$sig_raw" 2>/dev/null; then
    log "Release signature asset is not valid base64."
    rm -rf -- "$verify_dir"
    return 1
  fi
  sig_len="$(wc -c < "$sig_raw" | tr -d '[:space:]')"
  if [[ "$sig_len" != "64" ]]; then
    log "Release signature asset length is invalid: ${sig_len} bytes."
    rm -rf -- "$verify_dir"
    return 1
  fi

  printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00' > "$pub_der"
  cat "$pub_raw" >> "$pub_der"
  if ! "$OPENSSL_BIN" pkey -pubin -inform DER -in "$pub_der" -out "$pub_pem" >/dev/null 2>"$verify_log"; then
    log "OpenSSL cannot import the Ed25519 release public key: $(tr '\n' ' ' < "$verify_log" | sed 's/[[:space:]]\+/ /g')"
    rm -rf -- "$verify_dir"
    return 1
  fi

  {
    printf 'dushengcdn-release-v1\n'
    printf '%s\n' "$tag"
    printf '%s\n' "$asset"
    printf '%s\n' "$checksum"
  } > "$payload"

  if ! "$OPENSSL_BIN" pkeyutl -verify -pubin -inkey "$pub_pem" -sigfile "$sig_raw" -rawin -in "$payload" >/dev/null 2>"$verify_log"; then
    log "OpenSSL release signature verification failed: $(tr '\n' ' ' < "$verify_log" | sed 's/[[:space:]]\+/ /g')"
    if "$OPENSSL_BIN" version >/dev/null 2>&1; then
      log "OpenSSL version: $("$OPENSSL_BIN" version 2>/dev/null)"
    fi
    rm -rf -- "$verify_dir"
    return 1
  fi

  rm -rf -- "$verify_dir"
}

now_utc() {
  date -u '+%Y-%m-%dT%H:%M:%SZ'
}

date_to_epoch() {
  local value="$1"
  value="${value%%.*}Z"
  value="${value%ZZ}Z"
  if date -u -d "$value" '+%s' >/dev/null 2>&1; then
    date -u -d "$value" '+%s'
    return 0
  fi
  if date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$value" '+%s' >/dev/null 2>&1; then
    date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$value" '+%s'
    return 0
  fi
  return 1
}

file_mtime_epoch() {
  local file="$1"
  if stat -c '%Y' "$file" >/dev/null 2>&1; then
    stat -c '%Y' "$file"
    return 0
  fi
  if stat -f '%m' "$file" >/dev/null 2>&1; then
    stat -f '%m' "$file"
    return 0
  fi
  return 1
}

source_database_meta_path() {
  local kind="$1"
  local name="$2"
  name="$(basename "$name")"
  echo "${SOURCE_DATABASE_METADATA_DIR}/${kind}.${name}.env"
}

read_source_database_meta_value() {
  local kind="$1"
  local name="$2"
  local key="$3"
  local meta

  meta="$(source_database_meta_path "$kind" "$name")"
  if [[ ! -f "$meta" ]]; then
    return 1
  fi
  awk -F= -v key="$key" '$1 == key { print substr($0, length(key) + 2); exit }' "$meta"
}

write_source_database_metadata() {
  local kind="$1"
  local name="$2"
  local path="$3"
  local source="$4"
  local updated_at="$5"
  local checksum="$6"

  if [[ -z "$SOURCE_DATABASE_METADATA_DIR" ]]; then
    return
  fi
  if [[ -z "$checksum" && -f "$path" ]]; then
    checksum="$(sha256_file "$path" 2>/dev/null || true)"
  fi
  if [[ -z "$updated_at" ]]; then
    updated_at="$(now_utc)"
  fi
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root mkdir -p "$SOURCE_DATABASE_METADATA_DIR"
    write_file_as_root "$(source_database_meta_path "$kind" "$name")" "0644" <<METAE0F
kind=$kind
name=$name
path=$path
source=$source
updated_at=$updated_at
sha256=$checksum
METAE0F
  else
    mkdir -p "$SOURCE_DATABASE_METADATA_DIR"
    cat > "$(source_database_meta_path "$kind" "$name")" <<METAE0F
kind=$kind
name=$name
path=$path
source=$source
updated_at=$updated_at
sha256=$checksum
METAE0F
    chmod 0644 "$(source_database_meta_path "$kind" "$name")"
  fi
}

metadata_updated_epoch_or_file_mtime() {
  local kind="$1"
  local name="$2"
  local path="$3"
  local updated epoch

  updated="$(read_source_database_meta_value "$kind" "$name" "updated_at" 2>/dev/null || true)"
  if [[ -n "$updated" ]] && epoch="$(date_to_epoch "$updated" 2>/dev/null)"; then
    echo "$epoch"
    return 0
  fi
  if [[ -f "$path" ]] && epoch="$(file_mtime_epoch "$path" 2>/dev/null)"; then
    echo "$epoch"
    return 0
  fi
  return 1
}

fetch_server_source_database_manifest() {
  local output="$1"
  local url="${SERVER_URL%/}/api/dns-source-databases/manifest"

  if [[ -z "$SERVER_URL" || -z "$TOKEN" ]]; then
    return 1
  fi
  curl -fsSL -H "X-DNS-Worker-Token: ${TOKEN}" -o "$output" "$url"
}

manifest_source_updated_at() {
  local manifest="$1"
  local kind="$2"
  awk -v kind="$kind" '
    $0 ~ "\"" kind "\"[[:space:]]*:" { in_entry = 1; next }
    in_entry && $0 ~ /"updated_at"[[:space:]]*:/ { print; exit }
    in_entry && $0 ~ /^    }/ { in_entry = 0 }
  ' "$manifest" | sed -n 's/.*"updated_at"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

manifest_file_field() {
  local manifest="$1"
  local kind="$2"
  local name="$3"
  local field="$4"
  awk -v kind="$kind" -v name="$name" -v field="$field" '
    $0 ~ "\"" kind "\"[[:space:]]*:" { in_entry = 1; next }
    in_entry && $0 ~ "\"name\"[[:space:]]*:[[:space:]]*\"" name "\"" { in_file = 1 }
    in_file && $0 ~ "\"" field "\"[[:space:]]*:" { print; exit }
    in_entry && $0 ~ /^    }/ { in_entry = 0; in_file = 0 }
  ' "$manifest" | sed -n 's/.*"'"$field"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

server_source_database_is_newer() {
  local manifest="$1"
  local kind="$2"
  local name="$3"
  local target="$4"
  local server_updated local_epoch server_epoch local_sha server_sha

  if [[ ! -f "$target" ]]; then
    return 0
  fi

  server_sha="$(manifest_file_field "$manifest" "$kind" "$name" "sha256" || true)"
  local_sha="$(read_source_database_meta_value "$kind" "$name" "sha256" 2>/dev/null || true)"
  if [[ -n "$server_sha" && -n "$local_sha" && "$server_sha" == "$local_sha" ]]; then
    return 1
  fi

  server_updated="$(manifest_file_field "$manifest" "$kind" "$name" "updated_at" || true)"
  if [[ -z "$server_updated" ]]; then
    server_updated="$(manifest_source_updated_at "$manifest" "$kind" || true)"
  fi
  if [[ -z "$server_updated" ]]; then
    return 1
  fi
  if ! server_epoch="$(date_to_epoch "$server_updated" 2>/dev/null)"; then
    return 1
  fi
  if ! local_epoch="$(metadata_updated_epoch_or_file_mtime "$kind" "$name" "$target" 2>/dev/null)"; then
    return 0
  fi
  [[ "$server_epoch" -gt "$local_epoch" ]]
}

download_source_database_file_from_server() {
  local target="$1"
  local kind="$2"
  local name="$3"
  local label="$4"
  local manifest="${5:-}"
  local parent tmp headers bytes url expected actual updated local_epoch server_epoch local_sha

  if [[ -z "$target" || -z "$kind" || -z "$name" ]]; then
    return 1
  fi
  parent="$(dirname "$target")"
  url="${SERVER_URL%/}/api/dns-source-databases/files/${kind}/${name}"

  log "Downloading ${label} from panel mirror..."
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root mkdir -p "$parent"
    tmp="$(mktemp "/tmp/dushengcdn-dns-worker-${name}.XXXXXX")"
  else
    mkdir -p "$parent"
    tmp="$(mktemp "${parent}/.${name}.XXXXXX")"
  fi
  headers="$(mktemp "/tmp/dushengcdn-dns-worker-source-headers.XXXXXX")"
  if ! curl -fsSL -D "$headers" -H "X-DNS-Worker-Token: ${TOKEN}" -o "$tmp" "$url"; then
    rm -f "$tmp"
    rm -f "$headers"
    return 1
  fi
  expected="$(awk -F': ' 'tolower($1) == "x-dushengcdn-source-database-sha256" { gsub(/\r/, "", $2); print $2; exit }' "$headers")"
  updated="$(awk -F': ' 'tolower($1) == "x-dushengcdn-source-database-updated-at" { gsub(/\r/, "", $2); print $2; exit }' "$headers")"
  rm -f "$headers"
  bytes="$(wc -c < "$tmp" | tr -d '[:space:]')"
  if [[ "$name" == *.mmdb && "${bytes:-0}" -lt 1024 ]]; then
    rm -f "$tmp"
    return 1
  fi
  if [[ "$name" != *.mmdb && "${bytes:-0}" -le 16 ]]; then
    rm -f "$tmp"
    return 1
  fi
  actual="$(sha256_file "$tmp" 2>/dev/null || true)"
  if [[ -n "$expected" && "$actual" != "$expected" ]]; then
    rm -f "$tmp"
    return 1
  fi
  if [[ -f "$target" ]]; then
    local_sha="$(read_source_database_meta_value "$kind" "$name" "sha256" 2>/dev/null || true)"
    if [[ -z "$local_sha" ]]; then
      local_sha="$(sha256_file "$target" 2>/dev/null || true)"
    fi
    if [[ -n "$expected" && -n "$local_sha" && "$expected" == "$local_sha" ]]; then
      rm -f "$tmp"
      log "Panel mirror ${label} is unchanged; keeping ${target}."
      return 0
    fi
    if [[ -n "$updated" ]] && server_epoch="$(date_to_epoch "$updated" 2>/dev/null)" && local_epoch="$(metadata_updated_epoch_or_file_mtime "$kind" "$name" "$target" 2>/dev/null)"; then
      if [[ "$server_epoch" -le "$local_epoch" ]]; then
        rm -f "$tmp"
        log "Panel mirror ${label} is not newer than local database; keeping ${target}."
        return 0
      fi
    fi
  fi
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root install -m 0644 "$tmp" "$target"
    rm -f "$tmp"
  else
    mv -f "$tmp" "$target"
    chmod 0644 "$target"
  fi
  write_source_database_metadata "$kind" "$name" "$target" "panel" "$updated" "$actual"
  log "${label} ready from panel mirror: ${target}"
  return 0
}

normalize_source_database_profile() {
  SOURCE_DATABASE_PROFILE="$(printf '%s' "$SOURCE_DATABASE_PROFILE" | tr '[:upper:]' '[:lower:]' | tr '_' '-')"
  case "$SOURCE_DATABASE_PROFILE" in
    full|country|asn|operator|none) ;;
    no|false|disabled|off) SOURCE_DATABASE_PROFILE="none" ;;
    *) die "--source-database-profile must be full, country, asn, operator, or none." ;;
  esac
}

source_profile_wants_country() {
  case "$SOURCE_DATABASE_PROFILE" in
    full|country) return 0 ;;
    *) return 1 ;;
  esac
}

source_profile_wants_asn() {
  case "$SOURCE_DATABASE_PROFILE" in
    full|asn) return 0 ;;
    *) return 1 ;;
  esac
}

source_profile_wants_operator() {
  case "$SOURCE_DATABASE_PROFILE" in
    full|operator) return 0 ;;
    *) return 1 ;;
  esac
}

download_source_database_file() {
  local target="$1"
  local url="$2"
  local label="$3"
  local tmp_prefix="$4"
  local kind="${5:-}"
  local name="${6:-$tmp_prefix}"
  local parent tmp bytes

  if [[ -z "$target" || -z "$url" ]]; then
    return 1
  fi

  parent="$(dirname "$target")"
  log "Downloading ${label}..."
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root mkdir -p "$parent"
    tmp="$(mktemp "/tmp/dushengcdn-dns-worker-${tmp_prefix}.XXXXXX")"
    if ! curl -fsSL -o "$tmp" "$url"; then
      rm -f "$tmp"
      return 1
    fi
    bytes="$(wc -c < "$tmp" | tr -d '[:space:]')"
    if [[ "${bytes:-0}" -lt 1024 ]]; then
      rm -f "$tmp"
      return 1
    fi
    run_as_root install -m 0644 "$tmp" "$target"
    rm -f "$tmp"
  else
    mkdir -p "$parent"
    tmp="$(mktemp "${parent}/.${tmp_prefix}.XXXXXX")"
    if ! curl -fsSL -o "$tmp" "$url"; then
      rm -f "$tmp"
      return 1
    fi
    bytes="$(wc -c < "$tmp" | tr -d '[:space:]')"
    if [[ "${bytes:-0}" -lt 1024 ]]; then
      rm -f "$tmp"
      return 1
    fi
    mv -f "$tmp" "$target"
    chmod 0644 "$target"
  fi

  log "${label} ready: ${target}"
  if [[ -n "$kind" ]]; then
    write_source_database_metadata "$kind" "$name" "$target" "github" "$(now_utc)" ""
  fi
  return 0
}

prepare_geoip_database() {
  if ! source_profile_wants_country; then
    if [[ "$GEOIP_DATABASE_EXPLICIT" != "true" ]]; then
      GEOIP_DATABASE=""
    fi
    return
  fi

  if [[ "$AUTO_GEOIP_DOWNLOAD" != "true" ]]; then
    if [[ "$GEOIP_DATABASE_EXPLICIT" != "true" ]]; then
      GEOIP_DATABASE=""
    fi
    return
  fi

  if [[ "$GEOIP_DATABASE_EXPLICIT" == "true" && -f "$GEOIP_DATABASE" ]]; then
    log "Using existing GeoIP Country database: ${GEOIP_DATABASE}"
    return
  fi

  if download_source_database_file "$GEOIP_DATABASE" "$GEOIP_DATABASE_URL" "GeoIP Country database" "GeoLite2-Country" "country" "GeoLite2-Country.mmdb"; then
    return
  fi

  if download_source_database_file_from_server "$GEOIP_DATABASE" "country" "GeoLite2-Country.mmdb" "GeoIP Country database" ""; then
    return
  fi

  log "GeoIP Country database download failed from GitHub and panel mirror; country-code pool matching will fall back to global unless a valid database already exists."
  if [[ -f "$GEOIP_DATABASE" ]]; then
    log "Using existing GeoIP Country database: ${GEOIP_DATABASE}"
    return
  fi
  if [[ "$GEOIP_DATABASE_EXPLICIT" != "true" ]]; then
    GEOIP_DATABASE=""
  fi
}

prepare_asn_database() {
  if ! source_profile_wants_asn; then
    if [[ "$ASN_DATABASE_EXPLICIT" != "true" ]]; then
      ASN_DATABASE=""
    fi
    return
  fi

  if [[ "$AUTO_ASN_DOWNLOAD" != "true" ]]; then
    if [[ "$ASN_DATABASE_EXPLICIT" != "true" ]]; then
      ASN_DATABASE=""
    fi
    return
  fi

  if [[ "$ASN_DATABASE_EXPLICIT" == "true" && -f "$ASN_DATABASE" ]]; then
    log "Using existing GeoIP ASN database: ${ASN_DATABASE}"
    return
  fi

  if download_source_database_file "$ASN_DATABASE" "$ASN_DATABASE_URL" "GeoIP ASN database" "GeoLite2-ASN" "asn" "GeoLite2-ASN.mmdb"; then
    return
  fi

  if download_source_database_file_from_server "$ASN_DATABASE" "asn" "GeoLite2-ASN.mmdb" "GeoIP ASN database" ""; then
    return
  fi

  log "GeoIP ASN database download failed from GitHub and panel mirror; ASN pool matching will fall back unless a valid database already exists."
  if [[ -f "$ASN_DATABASE" ]]; then
    log "Using existing GeoIP ASN database: ${ASN_DATABASE}"
    return
  fi
  if [[ "$ASN_DATABASE_EXPLICIT" != "true" ]]; then
    ASN_DATABASE=""
  fi
}

download_operator_cidr_database() {
  local parent url target tmp bytes downloaded any_success

  if [[ -z "$OPERATOR_CIDR_DATABASE" || -z "$OPERATOR_CIDR_BASE_URL" || -z "$OPERATOR_CIDR_FILES" ]]; then
    return 1
  fi
  if [[ -e "$OPERATOR_CIDR_DATABASE" && ! -d "$OPERATOR_CIDR_DATABASE" ]]; then
    log "Using existing operator CIDR file: ${OPERATOR_CIDR_DATABASE}"
    return 0
  fi

  parent="$OPERATOR_CIDR_DATABASE"
  log "Downloading China operator CIDR lists from gaoyifan/china-operator-ip..."
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root mkdir -p "$parent"
  else
    mkdir -p "$parent"
  fi

  any_success="false"
  for downloaded in $OPERATOR_CIDR_FILES; do
    target="${parent}/${downloaded}"
    url="${OPERATOR_CIDR_BASE_URL%/}/${downloaded}"
    if [[ "$NEEDS_ROOT" == "true" ]]; then
      tmp="$(mktemp "/tmp/dushengcdn-dns-worker-operator-cidr.XXXXXX")"
      if curl -fsSL -o "$tmp" "$url"; then
        bytes="$(wc -c < "$tmp" | tr -d '[:space:]')"
        if [[ "${bytes:-0}" -gt 16 ]]; then
          run_as_root install -m 0644 "$tmp" "$target"
          any_success="true"
        fi
      fi
      rm -f "$tmp"
    else
      tmp="$(mktemp "${parent}/.operator-cidr.XXXXXX")"
      if curl -fsSL -o "$tmp" "$url"; then
        bytes="$(wc -c < "$tmp" | tr -d '[:space:]')"
        if [[ "${bytes:-0}" -gt 16 ]]; then
          mv -f "$tmp" "$target"
          chmod 0644 "$target"
          any_success="true"
        else
          rm -f "$tmp"
        fi
      else
        rm -f "$tmp"
      fi
    fi
  done

  if [[ "$any_success" == "true" ]]; then
    prune_operator_cidr_directory "$parent"
    write_source_database_metadata "operator" "operator-cidr" "$OPERATOR_CIDR_DATABASE" "github" "$(now_utc)" ""
    log "China operator CIDR database ready: ${OPERATOR_CIDR_DATABASE}"
    return 0
  fi
  return 1
}

prune_operator_cidr_directory() {
  local parent="$1"
  local file base keep

  if [[ -z "$parent" || ! -d "$parent" ]]; then
    return
  fi
  for file in "$parent"/*.txt; do
    [[ -e "$file" ]] || continue
    base="$(basename "$file")"
    keep="false"
    for expected in $OPERATOR_CIDR_FILES; do
      if [[ "$base" == "$expected" ]]; then
        keep="true"
        break
      fi
    done
    if [[ "$keep" != "true" ]]; then
      if [[ "$NEEDS_ROOT" == "true" ]]; then
        run_as_root rm -f "$file"
      else
        rm -f "$file"
      fi
    fi
  done
}

source_database_metadata_should_keep() {
  local base="$1"
  local expected

  if source_profile_wants_country && [[ -n "$GEOIP_DATABASE" && "$base" == "country.GeoLite2-Country.mmdb.env" ]]; then
    return 0
  fi
  if source_profile_wants_asn && [[ -n "$ASN_DATABASE" && "$base" == "asn.GeoLite2-ASN.mmdb.env" ]]; then
    return 0
  fi
  if source_profile_wants_operator && [[ -n "$OPERATOR_CIDR_DATABASE" ]]; then
    if [[ "$base" == "operator.operator-cidr.env" ]]; then
      return 0
    fi
    for expected in $OPERATOR_CIDR_FILES; do
      if [[ "$base" == "operator.${expected}.env" ]]; then
        return 0
      fi
    done
  fi
  return 1
}

prune_source_database_metadata() {
  local file base

  if [[ -z "$SOURCE_DATABASE_METADATA_DIR" || ! -d "$SOURCE_DATABASE_METADATA_DIR" ]]; then
    return
  fi
  for file in "$SOURCE_DATABASE_METADATA_DIR"/*.env; do
    [[ -e "$file" ]] || continue
    base="$(basename "$file")"
    if source_database_metadata_should_keep "$base"; then
      continue
    fi
    if [[ "$NEEDS_ROOT" == "true" ]]; then
      run_as_root rm -f "$file"
    else
      rm -f "$file"
    fi
  done
}

download_operator_cidr_database_from_server() {
  local manifest parent downloaded target any_success

  if [[ -z "$OPERATOR_CIDR_DATABASE" ]]; then
    return 1
  fi
  if [[ -e "$OPERATOR_CIDR_DATABASE" && ! -d "$OPERATOR_CIDR_DATABASE" ]]; then
    log "Using existing operator CIDR file: ${OPERATOR_CIDR_DATABASE}"
    return 0
  fi

  manifest="$(mktemp "/tmp/dushengcdn-dns-worker-source-manifest.XXXXXX")"
  if ! fetch_server_source_database_manifest "$manifest"; then
    rm -f "$manifest"
    return 1
  fi
  parent="$OPERATOR_CIDR_DATABASE"
  any_success="false"
  for downloaded in $OPERATOR_CIDR_FILES; do
    target="${parent}/${downloaded}"
    if download_source_database_file_from_server "$target" "operator" "$downloaded" "China operator CIDR ${downloaded}" "$manifest"; then
      any_success="true"
    fi
  done
  rm -f "$manifest"
  if [[ "$any_success" == "true" ]]; then
    prune_operator_cidr_directory "$parent"
    write_source_database_metadata "operator" "operator-cidr" "$OPERATOR_CIDR_DATABASE" "panel" "$(now_utc)" ""
    log "China operator CIDR database ready from panel mirror: ${OPERATOR_CIDR_DATABASE}"
    return 0
  fi
  return 1
}

prepare_operator_cidr_database() {
  if ! source_profile_wants_operator; then
    if [[ "$OPERATOR_CIDR_DATABASE_EXPLICIT" != "true" ]]; then
      OPERATOR_CIDR_DATABASE=""
    fi
    return
  fi

  if [[ "$AUTO_OPERATOR_CIDR_DOWNLOAD" != "true" ]]; then
    if [[ "$OPERATOR_CIDR_DATABASE_EXPLICIT" != "true" ]]; then
      OPERATOR_CIDR_DATABASE=""
    fi
    return
  fi

  if [[ "$OPERATOR_CIDR_DATABASE_EXPLICIT" == "true" && -e "$OPERATOR_CIDR_DATABASE" ]]; then
    log "Using existing China operator CIDR database: ${OPERATOR_CIDR_DATABASE}"
    return
  fi

  if download_operator_cidr_database; then
    return
  fi

  if download_operator_cidr_database_from_server; then
    return
  fi

  log "China operator CIDR download failed from GitHub and panel mirror; operator pool matching will fall back unless a valid database already exists."
  if [[ -e "$OPERATOR_CIDR_DATABASE" ]]; then
    log "Using existing China operator CIDR database: ${OPERATOR_CIDR_DATABASE}"
    return
  fi
  if [[ "$OPERATOR_CIDR_DATABASE_EXPLICIT" != "true" ]]; then
    OPERATOR_CIDR_DATABASE=""
  fi
}

resolve_release_binary() {
  local release_info
  local normalized_channel

  normalized_channel="$(echo "${RELEASE_CHANNEL:-stable}" | tr '[:upper:]' '[:lower:]')"
  if [[ -n "${RELEASE_TAG:-}" ]]; then
    log "Fetching release ${RELEASE_TAG} from ${REPO}..."
    if ! release_info="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/tags/${RELEASE_TAG}")"; then
      log "Release tag ${RELEASE_TAG} was not found. Falling back to source build."
      return 1
    fi
  elif [[ "$normalized_channel" == "preview" ]]; then
    log "Fetching latest preview release from ${REPO}..."
    if ! release_info="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=20" | awk '
      BEGIN { depth=0; capture=0; obj=""; prerelease=0 }
      /^[[:space:]]*{/ { depth=1; capture=1; obj=$0 ORS; prerelease=0; next }
      capture {
        obj = obj $0 ORS
        if ($0 ~ /"prerelease"[[:space:]]*:[[:space:]]*true/) prerelease=1
        if ($0 ~ /^[[:space:]]*}[,]?[[:space:]]*$/) {
          if (prerelease) { printf "%s", obj; exit }
          capture=0; obj=""; prerelease=0
        }
      }
    ')"; then
      log "No preview release list was found. Falling back to source build."
      return 1
    fi
    if [[ -z "$release_info" ]]; then
      log "No preview release was found. Falling back to source build."
      return 1
    fi
  else
    log "Fetching latest stable release from ${REPO}..."
    if ! release_info="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"; then
      log "No latest stable release was found. Falling back to source build."
      return 1
    fi
  fi

  DOWNLOAD_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\"" | grep -o 'https://[^"]*' | grep -v '\.sha256$' | grep -v '\.sig$' | head -n 1 || true)"
  SHA256_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\.sha256\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
  SIG_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\.sig\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
  if [[ -z "$DOWNLOAD_URL" ]]; then
    log "No matching asset '${ASSET_NAME}' found in latest release. Falling back to source build."
    return 1
  fi
  if [[ -z "$SHA256_URL" ]]; then
    die "matching checksum asset '${ASSET_NAME}.sha256' was not found in latest release; refusing to install an unverified DNS Worker binary."
  fi
  if [[ -z "$SIG_URL" ]]; then
    die "matching signature asset '${ASSET_NAME}.sig' was not found in latest release; refusing to install an unsigned DNS Worker binary."
  fi

  TAG="$(echo "$release_info" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"')"
  if [[ -z "$TAG" ]]; then
    die "release tag was not found; refusing to verify an unsigned DNS Worker binary."
  fi
  return 0
}

download_release_binary() {
  local actual expected sha_file sig_file

  ensure_release_signature_openssl
  log "Latest release: ${TAG}"
  log "Downloading ${ASSET_NAME}..."
  curl -fsSL -o "$TMP_BINARY" "$DOWNLOAD_URL"

  sha_file="$(mktemp "/tmp/dushengcdn-dns-worker.sha256.XXXXXX")"
  if ! curl -fsSL -o "$sha_file" "$SHA256_URL"; then
    rm -f "$sha_file"
    die "failed to download DNS Worker checksum asset."
  fi
  expected="$(parse_release_checksum "$sha_file" "$ASSET_NAME")"
  rm -f "$sha_file"
  if [[ ! "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    die "DNS Worker checksum asset is invalid."
  fi
  if ! actual="$(sha256_file "$TMP_BINARY")"; then
    die "sha256 tool was not found, cannot verify downloaded DNS Worker asset."
  fi
  if [[ "$actual" != "$expected" ]]; then
    die "downloaded DNS Worker checksum mismatch."
  fi

  sig_file="$(mktemp "/tmp/dushengcdn-dns-worker.sig.XXXXXX")"
  if ! curl -fsSL -o "$sig_file" "$SIG_URL"; then
    rm -f "$sig_file"
    die "failed to download DNS Worker signature asset."
  fi
  if ! verify_release_signature "$TAG" "$ASSET_NAME" "$expected" "$sig_file"; then
    rm -f "$sig_file"
    die "downloaded DNS Worker signature verification failed."
  fi
  rm -f "$sig_file"
  log "Release asset checksum and signature verified."

  chmod +x "$TMP_BINARY"
}

build_binary_from_source() {
  local source_dir source_version

  source_dir="$(mktemp -d "/tmp/dushengcdn-source.XXXXXX")"
  ensure_source_build_tools
  ensure_go

  log "Fetching ${REPO}@${SOURCE_REF} and building ${ASSET_NAME}..."
  git init "$source_dir" >/dev/null 2>&1
  git -C "$source_dir" remote add origin "https://github.com/${REPO}.git"
  git -C "$source_dir" fetch --depth 1 origin "$SOURCE_REF" >/dev/null 2>&1 || {
    rm -rf -- "$source_dir"
    die "failed to fetch ${REPO}@${SOURCE_REF}. Publish release assets or pass --source-ref with a valid branch, tag, or commit."
  }
  git -C "$source_dir" checkout --detach FETCH_HEAD >/dev/null 2>&1
  source_version="$(git -C "$source_dir" describe --tags --always --dirty 2>/dev/null || git -C "$source_dir" rev-parse --short HEAD 2>/dev/null || echo dev)"
  log "Building DNS Worker version ${source_version}."

  (
    cd "$source_dir/dushengcdn_server"
    go mod download
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=${source_version}" -o "$TMP_BINARY" ./cmd/dns-worker
  )

  rm -rf -- "$source_dir"
  chmod +x "$TMP_BINARY"
}

write_source_database_updater() {
  local updater="${INSTALL_DIR}/update-source-databases.sh"
  local updater_tmp

  log "Writing DNS Worker source database updater..."
  updater_tmp="$(mktemp)"
  cat > "$updater_tmp" <<'UPDATEREOF'
#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="${INSTALL_DIR}/dns-worker.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

SERVER_URL="${DUSHENGCDN_DNS_WORKER_SERVER_URL:-}"
TOKEN="${DUSHENGCDN_DNS_WORKER_TOKEN:-}"
PROFILE="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE:-full}"
GEOIP_DATABASE="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH:-}"
ASN_DATABASE="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH:-}"
OPERATOR_CIDR_DATABASE="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH:-}"
GEOIP_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL:-https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb}"
ASN_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL:-https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-ASN.mmdb}"
OPERATOR_CIDR_BASE_URL="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL:-https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists}"
OPERATOR_CIDR_FILES="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES:-chinanet.txt chinanet6.txt cmcc.txt cmcc6.txt unicom.txt unicom6.txt cernet.txt cernet6.txt cstnet.txt cstnet6.txt drpeng.txt drpeng6.txt googlecn.txt googlecn6.txt}"
METADATA_DIR="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_METADATA_DIR:-${INSTALL_DIR}/data/source-database-metadata}"
SERVICE_NAME="${DUSHENGCDN_DNS_WORKER_SERVICE_NAME:-dushengcdn-dns-worker}"
WORK_DIR="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_TMP_DIR:-${INSTALL_DIR}/data/source-database-tmp}"
LOCK_FILE="${INSTALL_DIR}/data/source-database-update.lock"
CHANGED="false"

log() { echo "==> $*"; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    return 1
  fi
}

now_utc() {
  date -u '+%Y-%m-%dT%H:%M:%SZ'
}

date_to_epoch() {
  local value="$1"
  value="${value%%.*}Z"
  value="${value%ZZ}Z"
  if date -u -d "$value" '+%s' >/dev/null 2>&1; then
    date -u -d "$value" '+%s'
    return 0
  fi
  if date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$value" '+%s' >/dev/null 2>&1; then
    date -u -j -f '%Y-%m-%dT%H:%M:%SZ' "$value" '+%s'
    return 0
  fi
  return 1
}

file_mtime_epoch() {
  if stat -c '%Y' "$1" >/dev/null 2>&1; then
    stat -c '%Y' "$1"
    return 0
  fi
  if stat -f '%m' "$1" >/dev/null 2>&1; then
    stat -f '%m' "$1"
    return 0
  fi
  return 1
}

wants_country() { [[ "$PROFILE" == "full" || "$PROFILE" == "country" ]]; }
wants_asn() { [[ "$PROFILE" == "full" || "$PROFILE" == "asn" ]]; }
wants_operator() { [[ "$PROFILE" == "full" || "$PROFILE" == "operator" ]]; }

meta_path() {
  echo "${METADATA_DIR}/$1.$(basename "$2").env"
}

read_meta_value() {
  local meta
  meta="$(meta_path "$1" "$2")"
  [[ -f "$meta" ]] || return 1
  awk -F= -v key="$3" '$1 == key { print substr($0, length(key) + 2); exit }' "$meta"
}

write_meta() {
  local kind="$1"
  local name="$2"
  local path="$3"
  local source="$4"
  local updated_at="$5"
  local checksum="${6:-}"
  mkdir -p "$METADATA_DIR"
  [[ -n "$checksum" ]] || checksum="$(sha256_file "$path" 2>/dev/null || true)"
  [[ -n "$updated_at" ]] || updated_at="$(now_utc)"
  cat > "$(meta_path "$kind" "$name")" <<METADATAEOF
kind=$kind
name=$name
path=$path
source=$source
updated_at=$updated_at
sha256=$checksum
METADATAEOF
  chmod 0644 "$(meta_path "$kind" "$name")"
}

local_epoch() {
  local updated epoch
  updated="$(read_meta_value "$1" "$2" updated_at 2>/dev/null || true)"
  if [[ -n "$updated" ]] && epoch="$(date_to_epoch "$updated" 2>/dev/null)"; then
    echo "$epoch"
    return 0
  fi
  if [[ -f "$3" ]] && epoch="$(file_mtime_epoch "$3" 2>/dev/null)"; then
    echo "$epoch"
    return 0
  fi
  return 1
}

cleanup_target_temps() {
  local target="$1"
  local name="${2:-}"
  local parent base file
  [[ -n "$target" ]] || return 0
  parent="$(dirname "$target")"
  base="$(basename "${name:-$target}")"
  [[ -d "$parent" ]] || return 0
  for file in "$parent/.${base}."*; do
    [[ -e "$file" ]] || continue
    rm -f "$file"
  done
}

cleanup_source_database_artifacts() {
  local name
  rm -f "${WORK_DIR}"/headers.* "${WORK_DIR}"/manifest.* 2>/dev/null || true
  cleanup_target_temps "$GEOIP_DATABASE" "GeoLite2-Country.mmdb"
  cleanup_target_temps "$ASN_DATABASE" "GeoLite2-ASN.mmdb"
  if [[ -n "$OPERATOR_CIDR_DATABASE" && -d "$OPERATOR_CIDR_DATABASE" ]]; then
    for name in $OPERATOR_CIDR_FILES; do
      cleanup_target_temps "${OPERATOR_CIDR_DATABASE}/${name}" "$name"
    done
  fi
  rmdir "$WORK_DIR" 2>/dev/null || true
}

mkdir -p "$(dirname "$LOCK_FILE")" "$WORK_DIR"
exec 9>"$LOCK_FILE"
if command -v flock >/dev/null 2>&1; then
  if ! flock -n 9; then
    log "Source database update is already running; skipping this cycle."
    exit 0
  fi
fi
cleanup_source_database_artifacts
mkdir -p "$WORK_DIR"
trap cleanup_source_database_artifacts EXIT

download_file() {
  local target="$1"
  local url="$2"
  local kind="$3"
  local name="$4"
  local source="$5"
  local updated_at="${6:-}"
  local expected="${7:-}"
  local auth="${8:-false}"
  local parent tmp headers bytes checksum header_sha header_updated local_checksum current_epoch server_epoch
  parent="$(dirname "$target")"
  mkdir -p "$parent"
  tmp="$(mktemp "${parent}/.${name}.XXXXXX")"
  headers="$(mktemp "${WORK_DIR}/headers.XXXXXX")"
  if [[ "$auth" == "true" ]]; then
    if ! curl -fsSL -D "$headers" -H "X-DNS-Worker-Token: ${TOKEN}" -o "$tmp" "$url"; then
      rm -f "$tmp" "$headers"
      return 1
    fi
  elif ! curl -fsSL -D "$headers" -o "$tmp" "$url"; then
    rm -f "$tmp"
    rm -f "$headers"
    return 1
  fi
  header_sha="$(awk -F': ' 'tolower($1) == "x-dushengcdn-source-database-sha256" { gsub(/\r/, "", $2); print $2; exit }' "$headers")"
  header_updated="$(awk -F': ' 'tolower($1) == "x-dushengcdn-source-database-updated-at" { gsub(/\r/, "", $2); print $2; exit }' "$headers")"
  rm -f "$headers"
  [[ -n "$expected" ]] || expected="$header_sha"
  [[ -n "$updated_at" ]] || updated_at="$header_updated"
  bytes="$(wc -c < "$tmp" | tr -d '[:space:]')"
  if [[ "$name" == *.mmdb && "${bytes:-0}" -lt 1024 ]]; then
    rm -f "$tmp"
    return 1
  fi
  if [[ "$name" != *.mmdb && "${bytes:-0}" -le 16 ]]; then
    rm -f "$tmp"
    return 1
  fi
  checksum="$(sha256_file "$tmp" 2>/dev/null || true)"
  if [[ -n "$expected" && "$checksum" != "$expected" ]]; then
    rm -f "$tmp"
    return 1
  fi
  if [[ "$source" == "panel" && -f "$target" ]]; then
    local_checksum="$(read_meta_value "$kind" "$name" sha256 2>/dev/null || true)"
    [[ -n "$local_checksum" ]] || local_checksum="$(sha256_file "$target" 2>/dev/null || true)"
    if [[ -n "$checksum" && -n "$local_checksum" && "$checksum" == "$local_checksum" ]]; then
      rm -f "$tmp"
      write_meta "$kind" "$name" "$target" "$source" "$updated_at" "$checksum"
      log "Panel mirror ${kind}/${name} is unchanged; keeping local file."
      return 0
    fi
    if [[ -n "$updated_at" ]] && server_epoch="$(date_to_epoch "$updated_at" 2>/dev/null)" && current_epoch="$(local_epoch "$kind" "$name" "$target" 2>/dev/null)"; then
      if [[ "$server_epoch" -le "$current_epoch" ]]; then
        rm -f "$tmp"
        log "Panel mirror ${kind}/${name} is not newer; keeping local file."
        return 0
      fi
    fi
  fi
  if [[ -f "$target" && -n "$checksum" && "$checksum" == "$(sha256_file "$target" 2>/dev/null || true)" ]]; then
    rm -f "$tmp"
    write_meta "$kind" "$name" "$target" "$source" "$updated_at" "$checksum"
    return 0
  fi
  mv -f "$tmp" "$target"
  chmod 0644 "$target"
  write_meta "$kind" "$name" "$target" "$source" "$updated_at" "$checksum"
  CHANGED="true"
  return 0
}

fetch_manifest() {
  local output="$1"
  [[ -n "$SERVER_URL" && -n "$TOKEN" ]] || return 1
  curl -fsSL -H "X-DNS-Worker-Token: ${TOKEN}" -o "$output" "${SERVER_URL%/}/api/dns-source-databases/manifest"
}

manifest_source_updated_at() {
  awk -v kind="$2" '
    $0 ~ "\"" kind "\"[[:space:]]*:" { in_entry = 1; next }
    in_entry && $0 ~ /"updated_at"[[:space:]]*:/ { print; exit }
    in_entry && $0 ~ /^    }/ { in_entry = 0 }
  ' "$1" | sed -n 's/.*"updated_at"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

manifest_file_field() {
  local manifest="$1"
  local kind="$2"
  local name="$3"
  local field="$4"
  awk -v kind="$kind" -v name="$name" -v field="$field" '
    $0 ~ "\"" kind "\"[[:space:]]*:" { in_entry = 1; next }
    in_entry && $0 ~ "\"name\"[[:space:]]*:[[:space:]]*\"" name "\"" { in_file = 1 }
    in_file && $0 ~ "\"" field "\"[[:space:]]*:" { print; exit }
    in_entry && $0 ~ /^    }/ { in_entry = 0; in_file = 0 }
  ' "$manifest" | sed -n 's/.*"'"$field"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

server_mirror_newer() {
  local manifest="$1"
  local kind="$2"
  local name="$3"
  local target="$4"
  local server_sha local_sha updated server_epoch current_epoch

  [[ ! -f "$target" ]] && return 0
  server_sha="$(manifest_file_field "$manifest" "$kind" "$name" sha256 || true)"
  local_sha="$(read_meta_value "$kind" "$name" sha256 2>/dev/null || true)"
  if [[ -n "$server_sha" && -n "$local_sha" && "$server_sha" == "$local_sha" ]]; then
    return 1
  fi
  updated="$(manifest_file_field "$manifest" "$kind" "$name" updated_at || true)"
  [[ -n "$updated" ]] || updated="$(manifest_source_updated_at "$manifest" "$kind" || true)"
  [[ -n "$updated" ]] || return 1
  server_epoch="$(date_to_epoch "$updated" 2>/dev/null || true)"
  [[ -n "$server_epoch" ]] || return 1
  current_epoch="$(local_epoch "$kind" "$name" "$target" 2>/dev/null || true)"
  [[ -z "$current_epoch" || "$server_epoch" -gt "$current_epoch" ]]
}

download_from_panel() {
  local manifest="$1"
  local target="$2"
  local kind="$3"
  local name="$4"
  local expected updated

  if ! server_mirror_newer "$manifest" "$kind" "$name" "$target"; then
    [[ -f "$target" ]] && log "Panel mirror ${kind}/${name} is not newer; keeping local file."
    return 0
  fi
  expected="$(manifest_file_field "$manifest" "$kind" "$name" sha256 || true)"
  updated="$(manifest_file_field "$manifest" "$kind" "$name" updated_at || true)"
  [[ -n "$updated" ]] || updated="$(manifest_source_updated_at "$manifest" "$kind" || true)"
  download_file "$target" "${SERVER_URL%/}/api/dns-source-databases/files/${kind}/${name}" "$kind" "$name" panel "$updated" "$expected" true
}

prune_operator_dir() {
  local parent="$1"
  local file base keep expected
  [[ -d "$parent" ]] || return 0
  for file in "$parent"/*.txt; do
    [[ -e "$file" ]] || continue
    base="$(basename "$file")"
    keep=false
    for expected in $OPERATOR_CIDR_FILES; do
      [[ "$base" == "$expected" ]] && keep=true && break
    done
    [[ "$keep" == true ]] || rm -f "$file"
  done
}

metadata_should_keep() {
  local base="$1"
  local expected

  if wants_country && [[ -n "$GEOIP_DATABASE" && "$base" == "country.GeoLite2-Country.mmdb.env" ]]; then
    return 0
  fi
  if wants_asn && [[ -n "$ASN_DATABASE" && "$base" == "asn.GeoLite2-ASN.mmdb.env" ]]; then
    return 0
  fi
  if wants_operator && [[ -n "$OPERATOR_CIDR_DATABASE" ]]; then
    if [[ "$base" == "operator.operator-cidr.env" ]]; then
      return 0
    fi
    for expected in $OPERATOR_CIDR_FILES; do
      if [[ "$base" == "operator.${expected}.env" ]]; then
        return 0
      fi
    done
  fi
  return 1
}

prune_metadata() {
  local file base
  [[ -d "$METADATA_DIR" ]] || return 0
  for file in "$METADATA_DIR"/*.env; do
    [[ -e "$file" ]] || continue
    base="$(basename "$file")"
    metadata_should_keep "$base" || rm -f "$file"
  done
}

update_one() {
  local target="$1"
  local kind="$2"
  local name="$3"
  local github_url="$4"
  local manifest="$5"
  [[ -n "$target" ]] || return 0
  if download_file "$target" "$github_url" "$kind" "$name" github "$(now_utc)" ""; then
    log "${kind}/${name} updated from GitHub."
    return 0
  fi
  if [[ -n "$manifest" && -f "$manifest" ]] && download_from_panel "$manifest" "$target" "$kind" "$name"; then
    log "${kind}/${name} checked against panel mirror."
    return 0
  fi
  [[ -f "$target" ]] && log "${kind}/${name} update failed; keeping existing local file." && return 0
  return 1
}

PROFILE="$(printf '%s' "$PROFILE" | tr '[:upper:]' '[:lower:]' | tr '_' '-')"
MANIFEST="$(mktemp "${WORK_DIR}/manifest.XXXXXX")"
if ! fetch_manifest "$MANIFEST"; then
  rm -f "$MANIFEST"
  MANIFEST=""
fi

if wants_country; then
  update_one "$GEOIP_DATABASE" country GeoLite2-Country.mmdb "$GEOIP_DATABASE_URL" "$MANIFEST" || true
fi
if wants_asn; then
  update_one "$ASN_DATABASE" asn GeoLite2-ASN.mmdb "$ASN_DATABASE_URL" "$MANIFEST" || true
fi
if wants_operator && [[ -n "$OPERATOR_CIDR_DATABASE" ]]; then
  mkdir -p "$OPERATOR_CIDR_DATABASE"
  for name in $OPERATOR_CIDR_FILES; do
    update_one "${OPERATOR_CIDR_DATABASE}/${name}" operator "$name" "${OPERATOR_CIDR_BASE_URL%/}/${name}" "$MANIFEST" || true
  done
  prune_operator_dir "$OPERATOR_CIDR_DATABASE"
fi
prune_metadata
[[ -z "$MANIFEST" ]] || rm -f "$MANIFEST"

if [[ "$CHANGED" == "true" ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl restart "$SERVICE_NAME" || true
fi
UPDATEREOF
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root install -m 0755 "$updater_tmp" "$updater"
    rm -f "$updater_tmp"
  else
    mv -f "$updater_tmp" "$updater"
    chmod 0755 "$updater"
  fi
}

write_dns_worker_updater() {
  local updater="${INSTALL_DIR}/update-dns-worker.sh"
  local updater_tmp

  log "Writing DNS Worker updater..."
  updater_tmp="$(mktemp)"
  cat > "$updater_tmp" <<'UPDATEWORKEREOF'
#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="${INSTALL_DIR}/dns-worker.env"
if [[ -f "$ENV_FILE" ]]; then
  set -a
  . "$ENV_FILE"
  set +a
fi

SERVER_URL="${DUSHENGCDN_DNS_WORKER_SERVER_URL:-}"
TOKEN="${DUSHENGCDN_DNS_WORKER_TOKEN:-}"
LISTEN_ADDR="${DUSHENGCDN_DNS_WORKER_LISTEN_ADDR:-:53}"
SNAPSHOT_PATH="${DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH:-${INSTALL_DIR}/data/dns-worker-snapshot.json}"
GEOIP_DATABASE="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH:-}"
ASN_DATABASE="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH:-}"
OPERATOR_CIDR_DATABASE="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH:-}"
SOURCE_DATABASE_PROFILE="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE:-full}"
SOURCE_DATABASE_METADATA_DIR="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_METADATA_DIR:-${INSTALL_DIR}/data/source-database-metadata}"
GEOIP_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL:-https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb}"
ASN_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL:-https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-ASN.mmdb}"
OPERATOR_CIDR_BASE_URL="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL:-https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists}"
OPERATOR_CIDR_FILES="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES:-chinanet.txt chinanet6.txt cmcc.txt cmcc6.txt unicom.txt unicom6.txt cernet.txt cernet6.txt cstnet.txt cstnet6.txt drpeng.txt drpeng6.txt googlecn.txt googlecn6.txt}"
SERVICE_NAME="${DUSHENGCDN_DNS_WORKER_SERVICE_NAME:-dushengcdn-dns-worker}"
HEARTBEAT_INTERVAL="${DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL:-10s}"
REQUEST_TIMEOUT="${DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT:-10s}"
SNAPSHOT_MAX_AGE="${DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE:-5m}"
QUERY_RATE_LIMIT="${DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT:-200}"
UDP_RESPONSE_SIZE="${DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE:-1232}"
REPO="${DUSHENGCDN_RELEASE_REPO:-SatanDS/SatanDS-DuShengCDN-releases}"
CHANNEL="${DUSHENGCDN_DNS_WORKER_UPDATE_CHANNEL:-stable}"
TAG="${DUSHENGCDN_DNS_WORKER_UPDATE_TAG:-}"
RELEASE_SIGNATURE_PUBLIC_KEY="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY__"
RELEASE_SIGNATURE_PUBLIC_KEY_PLACEHOLDER="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC""_KEY__"
if [[ "$RELEASE_SIGNATURE_PUBLIC_KEY" == "$RELEASE_SIGNATURE_PUBLIC_KEY_PLACEHOLDER" ]]; then
  RELEASE_SIGNATURE_PUBLIC_KEY="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-$RELEASE_SIGNATURE_PUBLIC_KEY}"
fi

if [[ -z "$SERVER_URL" || -z "$TOKEN" ]]; then
  echo "DNS Worker update requires SERVER_URL and TOKEN in ${ENV_FILE}" >&2
  exit 1
fi

args=(
  --server-url "$SERVER_URL"
  --token "$TOKEN"
  --install-dir "$INSTALL_DIR"
  --listen "$LISTEN_ADDR"
  --snapshot-path "$SNAPSHOT_PATH"
  --source-database-profile "$SOURCE_DATABASE_PROFILE"
  --source-database-metadata-dir "$SOURCE_DATABASE_METADATA_DIR"
  --geoip-database-url "$GEOIP_DATABASE_URL"
  --asn-database-url "$ASN_DATABASE_URL"
  --operator-cidr-base-url "$OPERATOR_CIDR_BASE_URL"
  --operator-cidr-files "$OPERATOR_CIDR_FILES"
  --service-name "$SERVICE_NAME"
  --heartbeat-interval "$HEARTBEAT_INTERVAL"
  --request-timeout "$REQUEST_TIMEOUT"
  --snapshot-max-age "$SNAPSHOT_MAX_AGE"
  --query-rate-limit "$QUERY_RATE_LIMIT"
  --udp-response-size "$UDP_RESPONSE_SIZE"
  --repo "$REPO"
  --release-channel "$CHANNEL"
)

if [[ -n "$GEOIP_DATABASE" ]]; then
  args+=(--geoip-database "$GEOIP_DATABASE")
fi
if [[ -n "$ASN_DATABASE" ]]; then
  args+=(--asn-database "$ASN_DATABASE")
fi
if [[ -n "$OPERATOR_CIDR_DATABASE" ]]; then
  args+=(--operator-cidr-database "$OPERATOR_CIDR_DATABASE")
fi
if [[ -n "$TAG" ]]; then
  args+=(--release-tag "$TAG")
fi

installer="$(mktemp)"
sha_file="$(mktemp)"
sig_file="$(mktemp)"
release_json="$(mktemp)"
cleanup() {
  rm -f "$installer" "$sha_file" "$sig_file" "$release_json"
}
trap cleanup EXIT

if [[ -n "$TAG" ]]; then
  curl -fsSL -o "$release_json" "https://api.github.com/repos/${REPO}/releases/tags/${TAG}"
elif [[ "$CHANNEL" == "preview" ]]; then
  curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=20" | awk '
    BEGIN { block=""; depth=0; found=0 }
    {
      line=$0
      if (index(line, "{") > 0) { depth++ }
      if (depth > 0) { block = block line "\n" }
      if (index(line, "}") > 0) {
        depth--
        if (depth == 0) {
          if (block ~ /"prerelease"[[:space:]]*:[[:space:]]*true/ && block !~ /"draft"[[:space:]]*:[[:space:]]*true/) {
            printf "%s", block
            found=1
            exit
          }
          block=""
        }
      }
    }
    END { if (!found) exit 1 }
  ' > "$release_json"
else
  curl -fsSL -o "$release_json" "https://api.github.com/repos/${REPO}/releases/latest"
fi

release_tag="$(grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' "$release_json" | head -n1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"//;s/"$//')"
installer_url="$(grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*install-dns-worker.sh"' "$release_json" | grep -o 'https://[^"]*' | head -n1 || true)"
sha_url="$(grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*install-dns-worker.sh\.sha256"' "$release_json" | grep -o 'https://[^"]*' | head -n1 || true)"
sig_url="$(grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*install-dns-worker.sh\.sig"' "$release_json" | grep -o 'https://[^"]*' | head -n1 || true)"
if [[ -z "$release_tag" || -z "$installer_url" || -z "$sha_url" || -z "$sig_url" ]]; then
  echo "release installer assets are incomplete" >&2
  exit 1
fi

curl -fsSL -o "$installer" "$installer_url"
curl -fsSL -o "$sha_file" "$sha_url"
curl -fsSL -o "$sig_file" "$sig_url"

expected="$(awk 'NF { print $1; exit }' "$sha_file")"
if [[ -z "$expected" ]]; then
  echo "installer checksum is empty" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$installer" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$installer" | awk '{print $1}')"
fi
if [[ "$actual" != "$expected" ]]; then
  echo "installer checksum verification failed" >&2
  exit 1
fi
if [[ -z "$RELEASE_SIGNATURE_PUBLIC_KEY" || "$RELEASE_SIGNATURE_PUBLIC_KEY" == "$RELEASE_SIGNATURE_PUBLIC_KEY_PLACEHOLDER" ]]; then
  echo "release signature public key is not configured" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to verify installer signature" >&2
  exit 1
fi
verify_dir="$(mktemp -d)"
trap 'cleanup; rm -rf "$verify_dir"' EXIT
{
  printf 'dushengcdn-release-v1\n'
  printf '%s\n' "$release_tag"
  printf '%s\n' "install-dns-worker.sh"
  printf '%s\n' "$expected"
} > "${verify_dir}/payload"
printf '%s' "$RELEASE_SIGNATURE_PUBLIC_KEY" | openssl base64 -d -A > "${verify_dir}/pub.raw"
printf '%s' "$(awk 'NF { print $1; exit }' "$sig_file")" | openssl base64 -d -A > "${verify_dir}/sig.raw"
printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00' > "${verify_dir}/prefix.der"
cat "${verify_dir}/prefix.der" "${verify_dir}/pub.raw" > "${verify_dir}/pub.der"
openssl pkey -pubin -inform DER -in "${verify_dir}/pub.der" -out "${verify_dir}/pub.pem" >/dev/null 2>&1
if ! openssl pkeyutl -verify -pubin -inkey "${verify_dir}/pub.pem" -sigfile "${verify_dir}/sig.raw" -rawin -in "${verify_dir}/payload" >/dev/null 2>&1; then
  echo "installer signature verification failed" >&2
  exit 1
fi

bash "$installer" "${args[@]}"
UPDATEWORKEREOF
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root install -m 0755 "$updater_tmp" "$updater"
    rm -f "$updater_tmp"
  else
    mv -f "$updater_tmp" "$updater"
    chmod 0755 "$updater"
  fi
}

write_dns_worker_uninstaller() {
  local uninstaller="${INSTALL_DIR}/uninstall-dns-worker.sh"
  local uninstaller_tmp

  log "Writing DNS Worker uninstaller..."
  uninstaller_tmp="$(mktemp)"
  cat > "$uninstaller_tmp" <<'UNINSTALLEREOF'
#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/dushengcdn-dns-worker"
SERVICE_NAME="${DUSHENGCDN_DNS_WORKER_SERVICE_NAME:-dushengcdn-dns-worker}"
SELF_UNINSTALL="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    --self-uninstall) SELF_UNINSTALL="true"; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

run_as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "Error: this operation requires root or sudo." >&2
    exit 1
  fi
}

validate_install_dir() {
  while [[ "$INSTALL_DIR" != "/" && "$INSTALL_DIR" == */ ]]; do
    INSTALL_DIR="${INSTALL_DIR%/}"
  done
  case "$INSTALL_DIR" in
    /*) ;;
    *) echo "Refusing to remove non-absolute install directory: '${INSTALL_DIR}'" >&2; exit 1 ;;
  esac
  case "$INSTALL_DIR" in
    *"/../"*|*/..|*"/./"*|*/.)
      echo "Refusing to remove non-normalized install directory: '${INSTALL_DIR}'" >&2
      exit 1
      ;;
  esac
  case "$INSTALL_DIR" in
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var|/Applications)
      echo "Refusing to remove unsafe install directory: '${INSTALL_DIR}'" >&2
      exit 1
      ;;
  esac
}

validate_install_dir

WORKER_BINARY="${INSTALL_DIR}/dushengcdn-dns-worker"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
SOURCE_DATABASE_SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}-source-database-update.service"
SOURCE_DATABASE_TIMER_FILE="/etc/systemd/system/${SERVICE_NAME}-source-database-update.timer"
SOURCE_DATABASE_TIMER_NAME="${SERVICE_NAME}-source-database-update.timer"

SYSTEMCTL_AVAILABLE="false"
if command -v systemctl >/dev/null 2>&1; then
  SYSTEMCTL_AVAILABLE="true"
fi

echo "Uninstalling DuShengCDN DNS Worker from ${INSTALL_DIR}..."

if [[ "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  if systemctl is-active --quiet "$SOURCE_DATABASE_TIMER_NAME"; then
    echo "Stopping source database update timer: ${SOURCE_DATABASE_TIMER_NAME}"
    run_as_root systemctl stop "$SOURCE_DATABASE_TIMER_NAME" || true
  fi
  if systemctl is-enabled --quiet "$SOURCE_DATABASE_TIMER_NAME" >/dev/null 2>&1; then
    echo "Disabling source database update timer: ${SOURCE_DATABASE_TIMER_NAME}"
    run_as_root systemctl disable "$SOURCE_DATABASE_TIMER_NAME" >/dev/null 2>&1 || true
  fi
  if [[ "$SELF_UNINSTALL" != "true" ]] && systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "Stopping service: ${SERVICE_NAME}"
    run_as_root systemctl stop "$SERVICE_NAME"
  fi
  if systemctl is-enabled --quiet "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "Disabling service: ${SERVICE_NAME}"
    run_as_root systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
  fi
fi

stop_worker_processes() {
  if ! command -v pgrep >/dev/null 2>&1; then
    return
  fi
  worker_pids="$(pgrep -f "$WORKER_BINARY" || true)"
  if [[ -n "$worker_pids" ]]; then
    echo "Stopping DNS Worker process: ${worker_pids}"
    # shellcheck disable=SC2086
    run_as_root kill $worker_pids || true
    sleep 1
    remaining_worker_pids="$(pgrep -f "$WORKER_BINARY" || true)"
    if [[ -n "$remaining_worker_pids" ]]; then
      echo "Force stopping remaining DNS Worker process: ${remaining_worker_pids}"
      # shellcheck disable=SC2086
      run_as_root kill -9 $remaining_worker_pids || true
    fi
  fi
}

if [[ "$SELF_UNINSTALL" != "true" ]]; then
  stop_worker_processes
fi

for file in "$SERVICE_FILE" "$SOURCE_DATABASE_SERVICE_FILE" "$SOURCE_DATABASE_TIMER_FILE"; do
  if [[ -f "$file" ]]; then
    echo "Removing systemd file: ${file}"
    run_as_root rm -f "$file"
  fi
done

if [[ "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  run_as_root systemctl daemon-reload || true
  run_as_root systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
  run_as_root systemctl reset-failed "$SOURCE_DATABASE_TIMER_NAME" >/dev/null 2>&1 || true
fi

if [[ -d "$INSTALL_DIR" ]]; then
  echo "Removing installation directory: ${INSTALL_DIR}"
  run_as_root rm -rf -- "$INSTALL_DIR"
else
  echo "Installation directory not found, skipping: ${INSTALL_DIR}"
fi

echo "DuShengCDN DNS Worker uninstall finished."
if [[ "$SELF_UNINSTALL" == "true" ]]; then
  stop_worker_processes
fi
UNINSTALLEREOF
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root install -m 0755 "$uninstaller_tmp" "$uninstaller"
    rm -f "$uninstaller_tmp"
  else
    mv -f "$uninstaller_tmp" "$uninstaller"
    chmod 0755 "$uninstaller"
  fi
}

load_existing_env_defaults
apply_dns_worker_defaults

if [[ -z "$SERVER_URL" ]]; then
  die "--server-url is required"
fi
if [[ -z "$TOKEN" ]]; then
  die "--token is required"
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac
if [[ "$OS" != "linux" && "$OS" != "darwin" ]]; then
  die "Unsupported OS: $OS"
fi

validate_install_dir
validate_build_go_dir
normalize_source_database_profile
if [[ -z "$SNAPSHOT_PATH" ]]; then
  SNAPSHOT_PATH="${INSTALL_DIR}/data/dns-worker-snapshot.json"
fi
if [[ -z "$SOURCE_DATABASE_METADATA_DIR" ]]; then
  SOURCE_DATABASE_METADATA_DIR="${INSTALL_DIR}/data/source-database-metadata"
fi
if [[ -z "$GEOIP_DATABASE" && "$AUTO_GEOIP_DOWNLOAD" == "true" ]] && source_profile_wants_country; then
  GEOIP_DATABASE="${INSTALL_DIR}/data/geoip/GeoLite2-Country.mmdb"
fi
if [[ "$AUTO_GEOIP_DOWNLOAD" != "true" && "$GEOIP_DATABASE_EXPLICIT" != "true" ]] || ! source_profile_wants_country; then
  GEOIP_DATABASE=""
fi
if [[ -z "$ASN_DATABASE" && "$AUTO_ASN_DOWNLOAD" == "true" ]] && source_profile_wants_asn; then
  ASN_DATABASE="${INSTALL_DIR}/data/geoip/GeoLite2-ASN.mmdb"
fi
if [[ "$AUTO_ASN_DOWNLOAD" != "true" && "$ASN_DATABASE_EXPLICIT" != "true" ]] || ! source_profile_wants_asn; then
  ASN_DATABASE=""
fi
if [[ -z "$OPERATOR_CIDR_DATABASE" && "$AUTO_OPERATOR_CIDR_DOWNLOAD" == "true" ]] && source_profile_wants_operator; then
  OPERATOR_CIDR_DATABASE="${INSTALL_DIR}/data/operator-cidr"
fi
if [[ "$AUTO_OPERATOR_CIDR_DOWNLOAD" != "true" && "$OPERATOR_CIDR_DATABASE_EXPLICIT" != "true" ]] || ! source_profile_wants_operator; then
  OPERATOR_CIDR_DATABASE=""
fi

if [[ "$OS" == "linux" && "$CREATE_SERVICE" == "true" && ! -d /etc/systemd/system ]]; then
  CREATE_SERVICE="false"
fi

INSTALL_PARENT="$(dirname "$INSTALL_DIR")"
SNAPSHOT_PARENT="$(dirname "$SNAPSHOT_PATH")"
NEEDS_ROOT="false"
if [[ ! -e "$INSTALL_PARENT" || ! -w "$INSTALL_PARENT" ]]; then
  NEEDS_ROOT="true"
fi
if [[ -d "$INSTALL_DIR" && ! -w "$INSTALL_DIR" ]]; then
  NEEDS_ROOT="true"
fi
if [[ ! -e "$SNAPSHOT_PARENT" || ! -w "$SNAPSHOT_PARENT" ]]; then
  NEEDS_ROOT="true"
fi
if [[ -n "$GEOIP_DATABASE" ]]; then
  GEOIP_PARENT="$(dirname "$GEOIP_DATABASE")"
  if [[ ! -e "$GEOIP_PARENT" || ! -w "$GEOIP_PARENT" ]]; then
    NEEDS_ROOT="true"
  fi
fi
if [[ -n "$ASN_DATABASE" ]]; then
  ASN_PARENT="$(dirname "$ASN_DATABASE")"
  if [[ ! -e "$ASN_PARENT" || ! -w "$ASN_PARENT" ]]; then
    NEEDS_ROOT="true"
  fi
fi
if [[ -n "$OPERATOR_CIDR_DATABASE" ]]; then
  if [[ "$OPERATOR_CIDR_DATABASE" == */* ]]; then
    OPERATOR_CIDR_PARENT="$(dirname "$OPERATOR_CIDR_DATABASE")"
    if [[ ! -e "$OPERATOR_CIDR_PARENT" || ! -w "$OPERATOR_CIDR_PARENT" ]]; then
      NEEDS_ROOT="true"
    fi
  fi
fi
if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" ]]; then
  NEEDS_ROOT="true"
fi

ensure_curl

ASSET_NAME="dushengcdn-dns-worker-${OS}-${ARCH}"
echo "Detected platform: ${OS}/${ARCH}"

TMP_BINARY="$(mktemp "/tmp/dushengcdn-dns-worker.tmp.XXXXXX")"
cleanup() {
  rm -f "$TMP_BINARY"
}
trap cleanup EXIT

DOWNLOAD_URL=""
SHA256_URL=""
SIG_URL=""
TAG=""
if resolve_release_binary; then
  download_release_binary
else
  if [[ "$ALLOW_SOURCE_BUILD" == "true" ]]; then
    build_binary_from_source
  else
    die "no verified release binary is available for ${ASSET_NAME} in ${REPO}. Publish the binary release asset, or rerun with --allow-source-build and a source repository."
  fi
fi

SYSTEMCTL_AVAILABLE="false"
if command -v systemctl >/dev/null 2>&1; then
  SYSTEMCTL_AVAILABLE="true"
fi

if [[ "$OS" == "linux" && "$SYSTEMCTL_AVAILABLE" == "true" ]] && systemctl is-active --quiet "$SERVICE_NAME"; then
  log "Stopping running service before reinstall: ${SERVICE_NAME}"
  run_as_root systemctl stop "$SERVICE_NAME"
fi

check_listen_port_available

log "Installing to ${INSTALL_DIR}..."
if [[ "$NEEDS_ROOT" == "true" ]]; then
  run_as_root mkdir -p "${INSTALL_DIR}/data"
  run_as_root mkdir -p "$(dirname "$SNAPSHOT_PATH")"
  run_as_root install -m 0755 "$TMP_BINARY" "${INSTALL_DIR}/dushengcdn-dns-worker"
  rm -f "$TMP_BINARY"
else
  mkdir -p "${INSTALL_DIR}/data"
  mkdir -p "$(dirname "$SNAPSHOT_PATH")"
  mv -f "$TMP_BINARY" "${INSTALL_DIR}/dushengcdn-dns-worker"
fi
trap - EXIT

prepare_geoip_database
prepare_asn_database
prepare_operator_cidr_database
prune_source_database_metadata

ENV_FILE="${INSTALL_DIR}/dns-worker.env"
ENV_MODE="0600"
UPDATE_ENABLED_VALUE="${DUSHENGCDN_DNS_WORKER_UPDATE_ENABLED:-true}"
log "Writing DNS Worker environment file..."
if [[ "$NEEDS_ROOT" == "true" ]]; then
  write_file_as_root "$ENV_FILE" "$ENV_MODE" <<ENVEOF
DUSHENGCDN_DNS_WORKER_SERVER_URL=$(env_quote "$SERVER_URL")
DUSHENGCDN_DNS_WORKER_TOKEN=$(env_quote "$TOKEN")
DUSHENGCDN_DNS_WORKER_INSTALL_DIR=$(env_quote "$INSTALL_DIR")
DUSHENGCDN_DNS_WORKER_UPDATE_SCRIPT=$(env_quote "${INSTALL_DIR}/update-dns-worker.sh")
DUSHENGCDN_DNS_WORKER_UPDATE_ENABLED=$(env_quote "$UPDATE_ENABLED_VALUE")
DUSHENGCDN_DNS_WORKER_LISTEN_ADDR=$(env_quote "$LISTEN_ADDR")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH=$(env_quote "$SNAPSHOT_PATH")
DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=$(env_quote "$GEOIP_DATABASE")
DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH=$(env_quote "$ASN_DATABASE")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH=$(env_quote "$OPERATOR_CIDR_DATABASE")
DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE=$(env_quote "$SOURCE_DATABASE_PROFILE")
DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_METADATA_DIR=$(env_quote "$SOURCE_DATABASE_METADATA_DIR")
DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_UPDATE_TIMER=$(env_quote "$SOURCE_DATABASE_UPDATE_TIMER")
DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL=$(env_quote "$GEOIP_DATABASE_URL")
DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL=$(env_quote "$ASN_DATABASE_URL")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL=$(env_quote "$OPERATOR_CIDR_BASE_URL")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES=$(env_quote "$OPERATOR_CIDR_FILES")
DUSHENGCDN_DNS_WORKER_SERVICE_NAME=$(env_quote "$SERVICE_NAME")
DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL=$(env_quote "$HEARTBEAT_INTERVAL")
DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT=$(env_quote "$REQUEST_TIMEOUT")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE=$(env_quote "$SNAPSHOT_MAX_AGE")
DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=$(env_quote "$QUERY_RATE_LIMIT")
DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=$(env_quote "$UDP_RESPONSE_SIZE")
LOG_LEVEL=$(env_quote "$LOG_LEVEL_VALUE")
ENVEOF
else
  cat > "$ENV_FILE" <<ENVEOF
DUSHENGCDN_DNS_WORKER_SERVER_URL=$(env_quote "$SERVER_URL")
DUSHENGCDN_DNS_WORKER_TOKEN=$(env_quote "$TOKEN")
DUSHENGCDN_DNS_WORKER_INSTALL_DIR=$(env_quote "$INSTALL_DIR")
DUSHENGCDN_DNS_WORKER_UPDATE_SCRIPT=$(env_quote "${INSTALL_DIR}/update-dns-worker.sh")
DUSHENGCDN_DNS_WORKER_UPDATE_ENABLED=$(env_quote "$UPDATE_ENABLED_VALUE")
DUSHENGCDN_DNS_WORKER_LISTEN_ADDR=$(env_quote "$LISTEN_ADDR")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH=$(env_quote "$SNAPSHOT_PATH")
DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=$(env_quote "$GEOIP_DATABASE")
DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH=$(env_quote "$ASN_DATABASE")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH=$(env_quote "$OPERATOR_CIDR_DATABASE")
DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE=$(env_quote "$SOURCE_DATABASE_PROFILE")
DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_METADATA_DIR=$(env_quote "$SOURCE_DATABASE_METADATA_DIR")
DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_UPDATE_TIMER=$(env_quote "$SOURCE_DATABASE_UPDATE_TIMER")
DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL=$(env_quote "$GEOIP_DATABASE_URL")
DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL=$(env_quote "$ASN_DATABASE_URL")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL=$(env_quote "$OPERATOR_CIDR_BASE_URL")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES=$(env_quote "$OPERATOR_CIDR_FILES")
DUSHENGCDN_DNS_WORKER_SERVICE_NAME=$(env_quote "$SERVICE_NAME")
DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL=$(env_quote "$HEARTBEAT_INTERVAL")
DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT=$(env_quote "$REQUEST_TIMEOUT")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE=$(env_quote "$SNAPSHOT_MAX_AGE")
DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=$(env_quote "$QUERY_RATE_LIMIT")
DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=$(env_quote "$UDP_RESPONSE_SIZE")
LOG_LEVEL=$(env_quote "$LOG_LEVEL_VALUE")
ENVEOF
  chmod "$ENV_MODE" "$ENV_FILE"
fi

write_source_database_updater
write_dns_worker_updater
write_dns_worker_uninstaller

if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" && -d /etc/systemd/system && "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  log "Creating systemd service..."
  write_file_as_root "/etc/systemd/system/${SERVICE_NAME}.service" "0644" <<SVCEOF
[Unit]
Description=DuShengCDN DNS Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/dushengcdn-dns-worker
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=10
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
SVCEOF

  run_as_root systemctl daemon-reload
  run_as_root systemctl enable "$SERVICE_NAME"
  run_as_root systemctl start "$SERVICE_NAME"
  if [[ "$SOURCE_DATABASE_UPDATE_TIMER" == "true" ]]; then
    log "Creating source database update timer..."
    write_file_as_root "/etc/systemd/system/${SERVICE_NAME}-source-database-update.service" "0644" <<UPDSVCEOF
[Unit]
Description=Update DuShengCDN DNS Worker source databases
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=${INSTALL_DIR}/update-source-databases.sh
UPDSVCEOF
    write_file_as_root "/etc/systemd/system/${SERVICE_NAME}-source-database-update.timer" "0644" <<UPDTIMEREOF
[Unit]
Description=Run DuShengCDN DNS Worker source database update every 7 days

[Timer]
OnBootSec=15m
OnUnitActiveSec=7d
Persistent=true

[Install]
WantedBy=timers.target
UPDTIMEREOF
    run_as_root systemctl daemon-reload
    run_as_root systemctl enable --now "${SERVICE_NAME}-source-database-update.timer"
    echo "Source database update timer created: ${SERVICE_NAME}-source-database-update.timer"
  fi
  echo "Service created and started: ${SERVICE_NAME}"
else
  echo ""
  echo "To start the DNS Worker manually:"
  echo "  set -a; . ${ENV_FILE}; set +a; ${INSTALL_DIR}/dushengcdn-dns-worker"
  if [[ "$LISTEN_ADDR" == *":53" ]]; then
    echo "  Listening on port 53 may require root or CAP_NET_BIND_SERVICE."
  fi
fi

echo ""
echo "DuShengCDN DNS Worker installed successfully!"
echo "  Binary:   ${INSTALL_DIR}/dushengcdn-dns-worker"
echo "  Env file: ${ENV_FILE}"
echo "  Data:     ${INSTALL_DIR}/data"
echo "  Listen:   ${LISTEN_ADDR}"

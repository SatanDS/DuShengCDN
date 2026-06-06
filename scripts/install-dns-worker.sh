#!/usr/bin/env bash
set -euo pipefail

# DuShengCDN DNS Worker Installer
# Usage:
#   curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-dns-worker.sh | bash -s -- \
#     --server-url https://cdn.example.com \
#     --token your-dns-worker-token

INSTALL_DIR="/opt/dushengcdn-dns-worker"
REPO="${DUSHENGCDN_RELEASE_REPO:-SatanDS/SatanDS-DuShengCDN-releases}"
SOURCE_REF="${SOURCE_REF:-main}"
ALLOW_SOURCE_BUILD="${DUSHENGCDN_ALLOW_SOURCE_BUILD:-false}"
SERVER_URL=""
TOKEN=""
SERVICE_NAME="dushengcdn-dns-worker"
CREATE_SERVICE="true"
AUTO_INSTALL_DEPS="true"
LISTEN_ADDR=":53"
SNAPSHOT_PATH=""
SOURCE_DATABASE_PROFILE="${DUSHENGCDN_DNS_WORKER_SOURCE_DATABASE_PROFILE:-full}"
GEOIP_DATABASE=""
GEOIP_DATABASE_EXPLICIT="false"
GEOIP_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_URL:-https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb}"
AUTO_GEOIP_DOWNLOAD="true"
ASN_DATABASE=""
ASN_DATABASE_EXPLICIT="false"
ASN_DATABASE_URL="${DUSHENGCDN_DNS_WORKER_ASN_DATABASE_URL:-https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-ASN.mmdb}"
AUTO_ASN_DOWNLOAD="true"
OPERATOR_CIDR_DATABASE=""
OPERATOR_CIDR_DATABASE_EXPLICIT="false"
OPERATOR_CIDR_BASE_URL="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_BASE_URL:-https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists}"
OPERATOR_CIDR_FILES="${DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_FILES:-chinanet.txt chinanet6.txt cmcc.txt cmcc6.txt unicom.txt unicom6.txt cernet.txt cernet6.txt cstnet.txt cstnet6.txt drpeng.txt drpeng6.txt googlecn.txt googlecn6.txt}"
AUTO_OPERATOR_CIDR_DOWNLOAD="true"
HEARTBEAT_INTERVAL="10s"
REQUEST_TIMEOUT="10s"
SNAPSHOT_MAX_AGE="5m"
QUERY_RATE_LIMIT="200"
UDP_RESPONSE_SIZE="1232"
LOG_LEVEL_VALUE="info"
DUSHENGCDN_BUILD_GO_DIR="${DUSHENGCDN_BUILD_GO_DIR:-/opt/dushengcdn-build/go}"

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
  --heartbeat-interval DUR   Heartbeat and snapshot pull interval (default: 10s)
  --request-timeout DUR      Server request timeout (default: 10s)
  --snapshot-max-age DUR     Maximum dynamic-answer snapshot age (default: 5m)
  --query-rate-limit NUM     Per-source-IP DNS queries per second; 0 disables (default: 200)
  --udp-response-size NUM    Maximum UDP DNS response payload size (default: 1232)
  --log-level LEVEL          debug, info, warn, or error (default: info)
  --repo REPO                GitHub release repository (default: ${REPO})
  --source-ref REF           Git branch, tag, or commit used when building from source (default: main)
  --allow-source-build       Allow fallback source build when no release binary is available
  --install-deps             Install missing download/build dependencies automatically (default)
  --no-install-deps          Do not install missing dependencies automatically
  --no-service               Do not create systemd service
  -h, --help                 Show this help message

Examples:
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token --geoip-database /var/lib/GeoLite2-Country.mmdb
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token --source-database-profile operator
  install-dns-worker.sh --server-url https://cdn.example.com --token worker-token --no-source-database-download

Notes:
  The full preset downloads Country + ASN MMDBs and gaoyifan/china-operator-ip operator CIDR lists.
  Reinstall keeps the data directory and snapshot cache, then replaces the binary
  and environment file. Use uninstall-dns-worker.sh to remove local data.
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
    --heartbeat-interval) HEARTBEAT_INTERVAL="$2"; shift 2 ;;
    --request-timeout) REQUEST_TIMEOUT="$2"; shift 2 ;;
    --snapshot-max-age) SNAPSHOT_MAX_AGE="$2"; shift 2 ;;
    --query-rate-limit) QUERY_RATE_LIMIT="$2"; shift 2 ;;
    --udp-response-size) UDP_RESPONSE_SIZE="$2"; shift 2 ;;
    --log-level) LOG_LEVEL_VALUE="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    --source-ref) SOURCE_REF="$2"; shift 2 ;;
    --allow-source-build) ALLOW_SOURCE_BUILD="true"; shift ;;
    --install-deps) AUTO_INSTALL_DEPS="true"; shift ;;
    --no-install-deps) AUTO_INSTALL_DEPS="false"; shift ;;
    --no-service) CREATE_SERVICE="false"; shift ;;
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
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl
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
  local go_version="${DUSHENGCDN_GO_VERSION:-1.25.0}"
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

  if download_source_database_file "$GEOIP_DATABASE" "$GEOIP_DATABASE_URL" "GeoIP Country database" "GeoLite2-Country"; then
    return
  fi

  log "GeoIP Country database download failed; country-code pool matching will fall back to global unless a valid database already exists."
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

  if download_source_database_file "$ASN_DATABASE" "$ASN_DATABASE_URL" "GeoIP ASN database" "GeoLite2-ASN"; then
    return
  fi

  log "GeoIP ASN database download failed; ASN pool matching will fall back unless a valid database already exists."
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
    log "China operator CIDR database ready: ${OPERATOR_CIDR_DATABASE}"
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

  log "China operator CIDR download failed; operator pool matching will fall back unless a valid database already exists."
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

  log "Fetching latest release from ${REPO}..."
  if ! release_info="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"; then
    log "No latest release was found. Falling back to source build."
    return 1
  fi

  DOWNLOAD_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\"" | grep -o 'https://[^"]*' | grep -v '\.sha256$' | head -n 1 || true)"
  SHA256_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\.sha256\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
  if [[ -z "$DOWNLOAD_URL" ]]; then
    log "No matching asset '${ASSET_NAME}' found in latest release. Falling back to source build."
    return 1
  fi
  if [[ -z "$SHA256_URL" ]]; then
    die "matching checksum asset '${ASSET_NAME}.sha256' was not found in latest release; refusing to install an unverified DNS Worker binary."
  fi

  TAG="$(echo "$release_info" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"')"
  return 0
}

download_release_binary() {
  local actual expected sha_file

  log "Latest release: ${TAG}"
  log "Downloading ${ASSET_NAME}..."
  curl -fsSL -o "$TMP_BINARY" "$DOWNLOAD_URL"

  sha_file="$(mktemp "/tmp/dushengcdn-dns-worker.sha256.XXXXXX")"
  if ! curl -fsSL -o "$sha_file" "$SHA256_URL"; then
    rm -f "$sha_file"
    die "failed to download DNS Worker checksum asset."
  fi
  expected="$(awk '{print $1}' "$sha_file")"
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
  log "Release asset checksum verified."

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
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${source_version}" -o "$TMP_BINARY" ./cmd/dns-worker
  )

  rm -rf -- "$source_dir"
  chmod +x "$TMP_BINARY"
}

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

ENV_FILE="${INSTALL_DIR}/dns-worker.env"
ENV_MODE="0600"
log "Writing DNS Worker environment file..."
if [[ "$NEEDS_ROOT" == "true" ]]; then
  write_file_as_root "$ENV_FILE" "$ENV_MODE" <<ENVEOF
DUSHENGCDN_DNS_WORKER_SERVER_URL=$(env_quote "$SERVER_URL")
DUSHENGCDN_DNS_WORKER_TOKEN=$(env_quote "$TOKEN")
DUSHENGCDN_DNS_WORKER_LISTEN_ADDR=$(env_quote "$LISTEN_ADDR")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH=$(env_quote "$SNAPSHOT_PATH")
DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=$(env_quote "$GEOIP_DATABASE")
DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH=$(env_quote "$ASN_DATABASE")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH=$(env_quote "$OPERATOR_CIDR_DATABASE")
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
DUSHENGCDN_DNS_WORKER_LISTEN_ADDR=$(env_quote "$LISTEN_ADDR")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_PATH=$(env_quote "$SNAPSHOT_PATH")
DUSHENGCDN_DNS_WORKER_GEOIP_DATABASE_PATH=$(env_quote "$GEOIP_DATABASE")
DUSHENGCDN_DNS_WORKER_ASN_DATABASE_PATH=$(env_quote "$ASN_DATABASE")
DUSHENGCDN_DNS_WORKER_OPERATOR_CIDR_DATABASE_PATH=$(env_quote "$OPERATOR_CIDR_DATABASE")
DUSHENGCDN_DNS_WORKER_HEARTBEAT_INTERVAL=$(env_quote "$HEARTBEAT_INTERVAL")
DUSHENGCDN_DNS_WORKER_REQUEST_TIMEOUT=$(env_quote "$REQUEST_TIMEOUT")
DUSHENGCDN_DNS_WORKER_SNAPSHOT_MAX_AGE=$(env_quote "$SNAPSHOT_MAX_AGE")
DUSHENGCDN_DNS_WORKER_QUERY_RATE_LIMIT=$(env_quote "$QUERY_RATE_LIMIT")
DUSHENGCDN_DNS_WORKER_UDP_RESPONSE_SIZE=$(env_quote "$UDP_RESPONSE_SIZE")
LOG_LEVEL=$(env_quote "$LOG_LEVEL_VALUE")
ENVEOF
  chmod "$ENV_MODE" "$ENV_FILE"
fi

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

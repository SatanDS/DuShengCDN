#!/usr/bin/env bash
set -euo pipefail

# DuShengCDN Agent Installer
# Usage:
#   curl -fsSL https://github.com/SatanDS/SatanDS-DuShengCDN-releases/releases/latest/download/install-agent.sh | bash -s -- \
#     --server-url http://your-server:3000 \
#     --discovery-token your-token

INSTALL_DIR="/opt/dushengcdn-agent"
REPO="${DUSHENGCDN_RELEASE_REPO:-SatanDS/SatanDS-DuShengCDN-releases}"
RELEASE_SIGNATURE_PUBLIC_KEY="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY__}"
SERVER_URL=""
DISCOVERY_TOKEN=""
AGENT_TOKEN=""
CREATE_SERVICE="true"
SERVICE_NAME="${DUSHENGCDN_AGENT_SERVICE_NAME:-dushengcdn-agent}"
OPENRESTY_PATH=""
AUTO_INSTALL_DEPS="true"
REINSTALL="false"
WIPE_DATA="false"
SOURCE_REF="${SOURCE_REF:-main}"
ALLOW_SOURCE_BUILD="${DUSHENGCDN_ALLOW_SOURCE_BUILD:-false}"
GEOIP_LOOKUP_API_URL=""
GEOIP_LOOKUP_API_TOKEN=""
DUSHENGCDN_BUILD_GO_DIR="${DUSHENGCDN_BUILD_GO_DIR:-/opt/dushengcdn-build/go}"
OPENSSL_BIN=""

usage() {
  cat <<EOF
DuShengCDN Agent Installer

Usage:
  install-agent.sh [OPTIONS]

Options:
  --server-url URL          Server URL (required)
  --discovery-token TOKEN   Discovery token for auto-registration
  --agent-token TOKEN       Node-specific agent token
  --install-dir DIR         Installation directory (default: /opt/dushengcdn-agent)
  --openresty-path PATH     OpenResty binary path (default: auto-detect from PATH)
  --service-name NAME       systemd service name (default: ${SERVICE_NAME})
  --repo REPO               GitHub release repository (default: ${REPO})
  --source-ref REF          Git branch, tag, or commit used when building from source (default: main)
  --allow-source-build      Allow fallback source build when no release binary is available
  --geoip-api-url URL       Optional precise IP lookup API URL used when local GeoIP has no country
  --geoip-api-token TOKEN   Optional bearer token for --geoip-api-url
  --install-deps            Install missing runtime dependencies automatically (default)
  --no-install-deps         Do not install missing dependencies automatically
  --reinstall               Reinstall the Agent binary; preserves existing data unless --wipe-data is also set
  --wipe-data               With --reinstall, remove the whole install directory before installing
  --no-service              Do not create systemd service
  -h, --help                Show this help message

Examples:
  # Install with discovery token (auto-register)
  install-agent.sh --server-url http://10.0.0.1:3000 --discovery-token abc123

  # Install with node-specific token
  install-agent.sh --server-url http://10.0.0.1:3000 --agent-token node-token-xyz

Notes:
  Rerunning the installer upgrades the Agent binary in place and preserves
  agent.json, local state, cached data, certificates, and observability buffers.
  A destructive clean reinstall requires both --reinstall and --wipe-data.
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-url)   SERVER_URL="$2"; shift 2 ;;
    --discovery-token) DISCOVERY_TOKEN="$2"; shift 2 ;;
    --agent-token)  AGENT_TOKEN="$2"; shift 2 ;;
    --install-dir)  INSTALL_DIR="$2"; shift 2 ;;
    --openresty-path) OPENRESTY_PATH="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    --repo)         REPO="$2"; shift 2 ;;
    --source-ref)   SOURCE_REF="$2"; shift 2 ;;
    --allow-source-build) ALLOW_SOURCE_BUILD="true"; shift ;;
    --geoip-api-url) GEOIP_LOOKUP_API_URL="$2"; shift 2 ;;
    --geoip-api-token) GEOIP_LOOKUP_API_TOKEN="$2"; shift 2 ;;
    --install-deps) AUTO_INSTALL_DEPS="true"; shift ;;
    --no-install-deps) AUTO_INSTALL_DEPS="false"; shift ;;
    --reinstall)    REINSTALL="true"; shift ;;
    --wipe-data)    WIPE_DATA="true"; shift ;;
    --no-service)   CREATE_SERVICE="false"; shift ;;
    -h|--help)      usage ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$SERVER_URL" ]]; then
  echo "Error: --server-url is required"
  exit 1
fi

if [[ -z "$DISCOVERY_TOKEN" && -z "$AGENT_TOKEN" ]]; then
  echo "Error: either --discovery-token or --agent-token is required"
  exit 1
fi

if [[ "$WIPE_DATA" == "true" && "$REINSTALL" != "true" ]]; then
  echo "Error: --wipe-data requires --reinstall" >&2
  exit 1
fi

geoip_api_config_json() {
  if [[ -z "$GEOIP_LOOKUP_API_URL" ]]; then
    return
  fi
  printf ',\n  "geoip_lookup_api_url": "%s"' "$(json_escape "$GEOIP_LOOKUP_API_URL")"
  if [[ -n "$GEOIP_LOOKUP_API_TOKEN" ]]; then
    printf ',\n  "geoip_lookup_api_token": "%s"' "$(json_escape "$GEOIP_LOOKUP_API_TOKEN")"
  fi
}

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}

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
    die "installing dependencies requires root or sudo. Install OpenResty manually, pass --openresty-path, or rerun as root."
  fi
}

write_file_as_root() {
  local target="$1"
  local tmp

  tmp="$(mktemp)"
  cat > "$tmp"
  run_as_root install -m 0644 "$tmp" "$target"
  rm -f "$tmp"
}

SERVICE_AUTOSTART_POLICY_CREATED="false"

disable_service_autostart() {
  if [[ "$OS" != "linux" || ! -d /usr/sbin ]]; then
    return
  fi
  if [[ -e /usr/sbin/policy-rc.d ]]; then
    return
  fi

  local tmp
  tmp="$(mktemp)"
  cat > "$tmp" <<'POLICYEOF'
#!/bin/sh
exit 101
POLICYEOF
  run_as_root install -m 0755 "$tmp" /usr/sbin/policy-rc.d
  rm -f "$tmp"
  SERVICE_AUTOSTART_POLICY_CREATED="true"
}

restore_service_autostart() {
  if [[ "$SERVICE_AUTOSTART_POLICY_CREATED" == "true" ]]; then
    run_as_root rm -f /usr/sbin/policy-rc.d
    SERVICE_AUTOSTART_POLICY_CREATED="false"
  fi
}

with_service_autostart_disabled() {
  disable_service_autostart
  set +e
  "$@"
  local status=$?
  set -e
  restore_service_autostart
  return "$status"
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

validate_service_name() {
  if [[ "$CREATE_SERVICE" != "true" ]]; then
    return
  fi
  if [[ -z "$SERVICE_NAME" ]]; then
    die "--service-name must not be empty."
  fi
  case "$SERVICE_NAME" in
    *[!A-Za-z0-9_.@-]*|.*|*-|*@|*..*|*/*)
      die "refusing to use unsafe systemd service name: ${SERVICE_NAME}"
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

find_openresty_path() {
  if command -v openresty >/dev/null 2>&1; then
    command -v openresty
    return 0
  fi

  local candidates=(
    "/usr/bin/openresty"
    "/usr/local/bin/openresty"
    "/usr/local/openresty/bin/openresty"
    "/usr/local/openresty/nginx/sbin/openresty"
    "/opt/openresty/nginx/sbin/openresty"
    "/opt/homebrew/bin/openresty"
  )
  local candidate
  for candidate in "${candidates[@]}"; do
    if [[ -x "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
  done

  return 1
}

find_opm_path() {
  if command -v opm >/dev/null 2>&1; then
    command -v opm
    return 0
  fi

  local candidates=(
    "/usr/bin/opm"
    "/usr/local/bin/opm"
    "/usr/local/openresty/bin/opm"
    "/opt/openresty/bin/opm"
    "/opt/homebrew/bin/opm"
  )
  local candidate
  for candidate in "${candidates[@]}"; do
    if [[ -x "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
  done

  return 1
}

ensure_libmaxminddb_link() {
  if [[ "$OS" != "linux" ]]; then
    return
  fi
  if [[ -e /usr/lib/libmaxminddb.so || -e /usr/lib64/libmaxminddb.so || -e /usr/local/lib/libmaxminddb.so ]]; then
    return
  fi

  local lib
  lib="$(find /usr/lib /usr/lib64 /usr/local/lib -name 'libmaxminddb.so.*' -type f -o -name 'libmaxminddb.so.*' -type l 2>/dev/null | head -n 1 || true)"
  if [[ -n "$lib" ]]; then
    run_as_root ln -sf "$lib" /usr/lib/libmaxminddb.so
  fi
}

openresty_package_needs_configure() {
  if ! command -v dpkg-query >/dev/null 2>&1; then
    return 1
  fi

  local status
  status="$(dpkg-query -W -f='${db:Status-Abbrev}' openresty 2>/dev/null || true)"
  if [[ -z "$status" || "$status" == ii* ]]; then
    return 1
  fi
  return 0
}

finish_pending_openresty_package_configuration() {
  if ! openresty_package_needs_configure; then
    return
  fi

  log "Completing pending OpenResty package configuration without auto-starting the default service..."
  with_service_autostart_disabled run_as_root dpkg --configure -a
}

remove_temporary_trusted_openresty_source() {
  local source_file="/etc/apt/sources.list.d/openresty.list"
  if [[ -f "$source_file" ]] && grep -q "trusted=yes" "$source_file"; then
    log "Removing temporary trusted OpenResty apt source."
    run_as_root rm -f "$source_file"
  fi
}

disable_default_openresty_service() {
  if [[ "$OS" != "linux" ]] || ! command -v systemctl >/dev/null 2>&1; then
    return
  fi
  if ! systemctl list-unit-files openresty.service >/dev/null 2>&1; then
    return
  fi

  log "Disabling the package default openresty.service; DuShengCDN Agent will manage OpenResty directly."
  run_as_root systemctl disable --now openresty.service >/dev/null 2>&1 || true
  run_as_root systemctl reset-failed openresty.service >/dev/null 2>&1 || true
}

ensure_geoip_lua_dependencies() {
  local opm_path
  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    if ! opm_path="$(find_opm_path)"; then
      die "opm was not found. Install lua-resty-maxminddb and lua-resty-http manually or rerun without --no-install-deps."
    fi
    ensure_libmaxminddb_link
    return
  fi

  log "Ensuring local GeoIP runtime dependencies..."
  case "$OS" in
    linux)
      install_common_linux_dependencies
      ;;
    darwin)
      if ! command -v brew >/dev/null 2>&1; then
        die "Homebrew is required to install libmaxminddb automatically on macOS."
      fi
      brew install libmaxminddb || true
      ;;
    *)
      die "unsupported OS for automatic GeoIP dependency installation: $OS"
      ;;
  esac

  ensure_libmaxminddb_link

  if ! opm_path="$(find_opm_path)"; then
    die "opm was not found after OpenResty installation. Install OpenResty with opm support, install lua-resty-maxminddb/lua-resty-http manually, or use Docker Agent."
  fi

  if ! "$opm_path" get anjia0532/lua-resty-maxminddb; then
    die "failed to install lua-resty-maxminddb via opm."
  fi
  if ! "$opm_path" get ledgetech/lua-resty-http; then
    die "failed to install lua-resty-http via opm."
  fi
}

load_os_release() {
  OS_ID=""
  OS_ID_LIKE=""
  OS_VERSION_ID=""
  OS_VERSION_CODENAME=""
  OS_UBUNTU_CODENAME=""
  if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    OS_ID="${ID:-}"
    OS_ID_LIKE="${ID_LIKE:-}"
    OS_VERSION_ID="${VERSION_ID:-}"
    OS_VERSION_CODENAME="${VERSION_CODENAME:-}"
    OS_UBUNTU_CODENAME="${UBUNTU_CODENAME:-}"
  fi
}

version_major() {
  local version="${OS_VERSION_ID%%.*}"
  if [[ "$version" =~ ^[0-9]+$ ]]; then
    echo "$version"
  else
    echo "0"
  fi
}

install_common_linux_dependencies() {
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get update
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gnupg openssl libmaxminddb0 libmaxminddb-dev build-essential
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl openssl libmaxminddb libmaxminddb-devel gcc make
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl openssl libmaxminddb libmaxminddb-devel gcc make
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl openssl libmaxminddb libmaxminddb-dev gcc musl-dev make
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl openssl libmaxminddb0 libmaxminddb-devel gcc make
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl openssl libmaxminddb gcc make
  else
    die "no supported package manager found. Install curl and OpenResty manually or pass --openresty-path."
  fi
}

install_source_build_dependencies_linux() {
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get update
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl git tar libmaxminddb0 libmaxminddb-dev build-essential
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl git tar libmaxminddb libmaxminddb-devel gcc make
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl git tar libmaxminddb libmaxminddb-devel gcc make
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl git tar libmaxminddb libmaxminddb-dev gcc musl-dev make
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl git tar libmaxminddb0 libmaxminddb-devel gcc make
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl git tar libmaxminddb gcc make
  else
    die "no supported package manager found. Install git, tar, and Go manually, or publish release assets."
  fi
}

install_openresty_source_build_dependencies_linux() {
  if command -v apt-get >/dev/null 2>&1; then
    run_as_root apt-get update
    if ! run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gnupg tar perl make build-essential libssl-dev zlib1g-dev libpcre2-dev; then
      run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gnupg tar perl make build-essential libssl-dev zlib1g-dev libpcre3-dev
    fi
  elif command -v dnf >/dev/null 2>&1; then
    run_as_root dnf install -y ca-certificates curl gnupg2 tar perl gcc make openssl-devel zlib-devel pcre2-devel
  elif command -v yum >/dev/null 2>&1; then
    run_as_root yum install -y ca-certificates curl gnupg2 tar perl gcc make openssl-devel zlib-devel pcre-devel
  elif command -v apk >/dev/null 2>&1; then
    run_as_root apk add --no-cache ca-certificates curl gnupg tar perl gcc musl-dev make openssl-dev zlib-dev pcre2-dev
  elif command -v zypper >/dev/null 2>&1; then
    run_as_root zypper --non-interactive install ca-certificates curl gpg2 tar perl gcc make libopenssl-devel zlib-devel pcre2-devel
  elif command -v pacman >/dev/null 2>&1; then
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl gnupg tar perl gcc make openssl zlib pcre2
  else
    die "no supported package manager found. Install OpenResty manually or pass --openresty-path."
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
    linux)
      log "curl was not found. Installing download dependencies..."
      install_common_linux_dependencies
      ;;
    darwin)
      die "curl was not found. Install curl first, then rerun the installer."
      ;;
    *)
      die "unsupported OS for automatic dependency installation: $OS"
      ;;
  esac

  if ! command -v curl >/dev/null 2>&1; then
    die "curl installation completed, but curl is still not available in PATH."
  fi
}

install_go_linux() {
  local go_version="${DUSHENGCDN_GO_VERSION:-1.26.4}"
  local go_arch="$ARCH"
  local archive
  archive="$(mktemp "/tmp/go${go_version}.linux-${go_arch}.XXXXXX.tar.gz")"
  local default_bases="https://go.dev/dl https://dl.google.com/go https://golang.google.cn/dl"
  local urls=()
  local base url attempt

  if [[ -n "${DUSHENGCDN_GO_DOWNLOAD_URL:-}" ]]; then
    urls+=("$DUSHENGCDN_GO_DOWNLOAD_URL")
  fi
  for base in ${DUSHENGCDN_GO_DOWNLOAD_BASE_URLS:-$default_bases}; do
    urls+=("${base%/}/go${go_version}.linux-${go_arch}.tar.gz")
  done

  log "Installing Go ${go_version} for linux/${go_arch}..."
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
      install_go_darwin
      ;;
    *)
      die "unsupported OS for automatic Go installation: $OS"
      ;;
  esac

  use_local_go_if_available
  if ! command -v go >/dev/null 2>&1; then
    die "Go installation completed, but go is still not available in PATH."
  fi
}

install_go_darwin() {
  if ! command -v brew >/dev/null 2>&1; then
    die "Homebrew is required to install Go automatically on macOS. Install Homebrew, install Go manually, or publish release assets."
  fi
  log "Installing Go via Homebrew..."
  brew install go
}

ensure_source_build_tools() {
  if command -v git >/dev/null 2>&1 && command -v tar >/dev/null 2>&1; then
    return
  fi

  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    die "git or tar was not found and no release binary is available. Install git/tar first or rerun without --no-install-deps."
  fi

  case "$OS" in
    linux)
      log "Installing source build dependencies..."
      install_source_build_dependencies_linux
      ;;
    darwin)
      if ! command -v git >/dev/null 2>&1; then
        die "git was not found. Install Xcode Command Line Tools or Git, then rerun the installer."
      fi
      ;;
    *)
      die "unsupported OS for automatic source build dependencies: $OS"
      ;;
  esac
}

apt_repository_base_url() {
  local distro="$1"
  if [[ "$ARCH" == "arm64" ]]; then
    echo "https://openresty.org/package/arm64/${distro}"
  else
    echo "https://openresty.org/package/${distro}"
  fi
}

is_debian_next_apt_release() {
  local codename="$1"
  [[ "$codename" == "trixie" || "$codename" == "testing" || "$codename" == "sid" ]]
}

openresty_source_jobs() {
  local jobs="${DUSHENGCDN_OPENRESTY_BUILD_JOBS:-}"
  if [[ "$jobs" =~ ^[0-9]+$ && "$jobs" -gt 0 ]]; then
    echo "$jobs"
    return
  fi
  if command -v nproc >/dev/null 2>&1; then
    jobs="$(nproc)"
  else
    jobs="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
  fi
  if ! [[ "$jobs" =~ ^[0-9]+$ ]] || [[ "$jobs" -lt 1 ]]; then
    jobs=2
  fi
  if [[ "$jobs" -gt 4 ]]; then
    jobs=4
  fi
  echo "$jobs"
}

import_openresty_release_key() {
  local keyring="$1"
  local fingerprint="${DUSHENGCDN_OPENRESTY_PGP_FINGERPRINT:-25451EB088460026195BD62CB550E09EA0E98066}"
  local key_url="${DUSHENGCDN_OPENRESTY_PGP_KEY_URL:-https://openresty.org/package/pubkey.gpg}"
  local key_tmp
  local keyservers=(
    "${DUSHENGCDN_OPENRESTY_PGP_KEYSERVER:-}"
    "hkps://keys.openpgp.org"
    "hkps://keyserver.ubuntu.com"
    "hkp://keyserver.ubuntu.com:80"
  )
  local keyserver

  key_tmp="$(mktemp "/tmp/dushengcdn-openresty-key.XXXXXX.gpg")"
  if curl --fail --location --show-error --silent --connect-timeout 20 --retry 2 --retry-delay 2 --retry-max-time 120 -o "$key_tmp" "$key_url" &&
    gpg --batch --no-default-keyring --keyring "$keyring" --import "$key_tmp" >/dev/null 2>&1 &&
    gpg --batch --no-default-keyring --keyring "$keyring" --fingerprint "$fingerprint" >/dev/null 2>&1; then
    rm -f "$key_tmp"
    return 0
  fi
  rm -f "$key_tmp"

  for keyserver in "${keyservers[@]}"; do
    [[ -n "$keyserver" ]] || continue
    if gpg --batch --no-default-keyring --keyring "$keyring" --keyserver "$keyserver" --recv-keys "$fingerprint" >/dev/null 2>&1; then
      return 0
    fi
  done

  return 1
}

verify_openresty_source_signature() {
  local archive="$1"
  local signature="$2"
  local expected_fingerprint="${DUSHENGCDN_OPENRESTY_PGP_FINGERPRINT:-25451EB088460026195BD62CB550E09EA0E98066}"
  local keyring verify_output fingerprint

  if [[ "${DUSHENGCDN_OPENRESTY_SKIP_PGP_VERIFY:-false}" == "true" ]]; then
    log "Skipping OpenResty source PGP verification because DUSHENGCDN_OPENRESTY_SKIP_PGP_VERIFY=true."
    return 0
  fi

  keyring="$(mktemp "/tmp/dushengcdn-openresty-keyring.XXXXXX.gpg")"
  if ! import_openresty_release_key "$keyring"; then
    rm -f "$keyring"
    die "failed to import OpenResty release PGP key ${expected_fingerprint}; install OpenResty manually or set DUSHENGCDN_OPENRESTY_SKIP_PGP_VERIFY=true only if you trust the download path."
  fi

  verify_output="$(gpg --batch --no-default-keyring --keyring "$keyring" --status-fd 1 --verify "$signature" "$archive" 2>/dev/null || true)"
  fingerprint="$(printf '%s\n' "$verify_output" | awk '/^\[GNUPG:\] VALIDSIG / { print $3; exit }')"
  rm -f "$keyring"

  if [[ "$fingerprint" != "$expected_fingerprint" ]]; then
    die "OpenResty source PGP verification failed or signer mismatch."
  fi
  log "OpenResty source PGP signature verified."
}

install_openresty_from_source() {
  local version="${DUSHENGCDN_OPENRESTY_VERSION:-1.31.1.1}"
  local prefix="${DUSHENGCDN_OPENRESTY_PREFIX:-/usr/local/openresty}"
  local source_name="openresty-${version}"
  local archive signature workdir source_dir jobs default_bases url base
  local urls=()

  case "$prefix" in
    /*) ;;
    *) die "DUSHENGCDN_OPENRESTY_PREFIX must be an absolute path." ;;
  esac
  case "$prefix" in
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var)
      die "refusing to install OpenResty directly into unsafe prefix: ${prefix}"
      ;;
  esac

  install_openresty_source_build_dependencies_linux

  workdir="$(mktemp -d "/tmp/dushengcdn-openresty-build.XXXXXX")"
  archive="${workdir}/${source_name}.tar.gz"
  signature="${archive}.asc"
  default_bases="https://openresty.org/download"

  if [[ -n "${DUSHENGCDN_OPENRESTY_DOWNLOAD_URL:-}" ]]; then
    urls+=("$DUSHENGCDN_OPENRESTY_DOWNLOAD_URL")
  fi
  for base in ${DUSHENGCDN_OPENRESTY_DOWNLOAD_BASE_URLS:-$default_bases}; do
    urls+=("${base%/}/${source_name}.tar.gz")
  done

  log "Installing OpenResty ${version} from verified source..."
  for url in "${urls[@]}"; do
    rm -f "$archive" "$signature"
    log "Downloading OpenResty source from ${url}..."
    if curl --fail --location --show-error --silent --connect-timeout 20 --retry 2 --retry-delay 2 --retry-max-time 300 -o "$archive" "$url" &&
      curl --fail --location --show-error --silent --connect-timeout 20 --retry 2 --retry-delay 2 --retry-max-time 300 -o "$signature" "${url}.asc" &&
      tar -tzf "$archive" >/dev/null 2>&1; then
      verify_openresty_source_signature "$archive" "$signature"
      break
    fi
    log "OpenResty source download failed or archive is invalid; trying next source if available."
  done

  if [[ ! -s "$archive" || ! -s "$signature" ]]; then
    rm -rf -- "$workdir"
    die "failed to download OpenResty source. Install OpenResty manually or pass --openresty-path."
  fi

  tar -xzf "$archive" -C "$workdir"
  source_dir="${workdir}/${source_name}"
  if [[ ! -d "$source_dir" ]]; then
    rm -rf -- "$workdir"
    die "OpenResty source archive did not contain ${source_name}."
  fi

  jobs="$(openresty_source_jobs)"
  log "Building OpenResty source with ${jobs} parallel job(s)..."
  if ! (
    cd "$source_dir"
    ./configure -j"$jobs" \
      --prefix="$prefix" \
      --with-pcre-jit \
      --with-http_ssl_module \
      --with-http_stub_status_module \
      --with-http_v2_module \
      --with-stream \
      --with-stream_ssl_module \
      --with-stream_ssl_preread_module
    make -j"$jobs"
  ); then
    rm -rf -- "$workdir"
    die "OpenResty source build failed."
  fi
  if ! run_as_root make -C "$source_dir" install; then
    rm -rf -- "$workdir"
    die "OpenResty source installation failed."
  fi
  rm -rf -- "$workdir"
}

install_openresty_with_apt() {
  load_os_release

  local distro="$OS_ID"
  local codename="$OS_VERSION_CODENAME"
  local detected_codename="$codename"
  local component="main"
  case "$OS_ID" in
    ubuntu)
      distro="ubuntu"
      codename="${OS_UBUNTU_CODENAME:-$codename}"
      ;;
    debian)
      distro="debian"
      component="openresty"
      ;;
    linuxmint|pop|elementary|zorin)
      distro="ubuntu"
      codename="${OS_UBUNTU_CODENAME:-$codename}"
      ;;
    *)
      if [[ " $OS_ID_LIKE " == *" ubuntu "* ]]; then
        distro="ubuntu"
        codename="${OS_UBUNTU_CODENAME:-$codename}"
      elif [[ " $OS_ID_LIKE " == *" debian "* ]]; then
        distro="debian"
        component="openresty"
      else
        distro="ubuntu"
      fi
      ;;
  esac

  if [[ -z "$codename" ]] && command -v lsb_release >/dev/null 2>&1; then
    codename="$(lsb_release -sc)"
  fi
  if [[ -z "$codename" ]]; then
    die "cannot detect apt distribution codename. Install OpenResty manually or pass --openresty-path."
  fi

  local repo_base
  repo_base="$(apt_repository_base_url "$distro")"
  if [[ "$distro" == "debian" ]] && [[ "$codename" == "trixie" || "$codename" == "testing" || "$codename" == "sid" ]]; then
    if ! curl -fsSL -o /dev/null "${repo_base}/dists/${codename}/Release" 2>/dev/null; then
      log "OpenResty apt repository does not provide ${codename}; falling back to Debian bookworm packages."
      codename="bookworm"
    fi
  fi

  log "Installing OpenResty via apt (${distro} ${codename})..."
  run_as_root rm -f /etc/apt/sources.list.d/openresty.list
  run_as_root apt-get update
  run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gnupg

  local key_tmp
  key_tmp="$(mktemp)"
  curl -fsSL https://openresty.org/package/pubkey.gpg | gpg --dearmor > "$key_tmp"
  run_as_root install -m 0644 "$key_tmp" /usr/share/keyrings/openresty.gpg
  rm -f "$key_tmp"

  local source_line
  source_line="deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/openresty.gpg] ${repo_base} ${codename} ${component}"
  echo "$source_line" | run_as_root tee /etc/apt/sources.list.d/openresty.list >/dev/null
  if ! run_as_root apt-get update; then
    run_as_root rm -f /etc/apt/sources.list.d/openresty.list
    if [[ "$distro" == "debian" ]] && is_debian_next_apt_release "$detected_codename"; then
      log "OpenResty apt repository signature is not accepted by this Debian release; falling back to verified source build."
      install_openresty_from_source
      return
    fi
    die "apt update failed after enabling the signed OpenResty repository. Refusing unauthenticated packages; install OpenResty manually or pass --openresty-path."
  fi
  if ! with_service_autostart_disabled run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y openresty; then
    run_as_root rm -f /etc/apt/sources.list.d/openresty.list
    if [[ "$distro" == "debian" ]] && is_debian_next_apt_release "$detected_codename"; then
      log "OpenResty apt package installation failed on this Debian release; falling back to verified source build."
      install_openresty_from_source
      return
    fi
    die "OpenResty package installation failed."
  fi
  disable_default_openresty_service
}

rpm_repo_url() {
  load_os_release
  local major
  major="$(version_major)"
  case "$OS_ID" in
    fedora)
      echo "https://openresty.org/package/fedora/openresty.repo"
      ;;
    rhel)
      if [[ "$major" -ge 9 ]]; then
        echo "https://openresty.org/package/rhel/openresty2.repo"
      else
        echo "https://openresty.org/package/rhel/openresty.repo"
      fi
      ;;
    centos|almalinux)
      if [[ "$major" -ge 9 ]]; then
        echo "https://openresty.org/package/centos/openresty2.repo"
      else
        echo "https://openresty.org/package/centos/openresty.repo"
      fi
      ;;
    rocky)
      if [[ "$major" -ge 9 ]]; then
        echo "https://openresty.org/package/rocky/openresty2.repo"
      else
        echo "https://openresty.org/package/rocky/openresty.repo"
      fi
      ;;
    ol|oracle)
      echo "https://openresty.org/package/oracle/openresty.repo"
      ;;
    amzn|amazon)
      echo "https://openresty.org/package/amazon/openresty.repo"
      ;;
    alinux)
      echo "https://openresty.org/package/alinux/openresty.repo"
      ;;
    tlinux)
      echo "https://openresty.org/package/tlinux/openresty.repo"
      ;;
    *)
      if [[ " $OS_ID_LIKE " == *" fedora "* ]]; then
        echo "https://openresty.org/package/fedora/openresty.repo"
      else
        echo "https://openresty.org/package/centos/openresty.repo"
      fi
      ;;
  esac
}

install_openresty_with_rpm_package_manager() {
  local manager="$1"
  local repo_tmp

  log "Installing OpenResty via ${manager}..."
  run_as_root "$manager" install -y ca-certificates curl || true
  if run_as_root "$manager" install -y openresty; then
    return 0
  fi

  repo_tmp="$(mktemp)"
  curl -fsSL -o "$repo_tmp" "$(rpm_repo_url)"
  run_as_root mkdir -p /etc/yum.repos.d
  run_as_root install -m 0644 "$repo_tmp" /etc/yum.repos.d/openresty.repo
  rm -f "$repo_tmp"
  run_as_root "$manager" makecache || true
  run_as_root "$manager" install -y openresty
}

install_openresty_with_dnf() {
  install_openresty_with_rpm_package_manager dnf
}

install_openresty_with_yum() {
  install_openresty_with_rpm_package_manager yum
}

install_openresty_with_zypper() {
  load_os_release

  local repo_url="https://openresty.org/package/opensuse/openresty.repo"
  if [[ "$OS_ID" == "sles" || "$OS_ID" == "suse" || "$OS_ID_LIKE" == *"suse"* ]]; then
    repo_url="https://openresty.org/package/sles/openresty.repo"
  fi

  log "Installing OpenResty via zypper..."
  run_as_root zypper --non-interactive install ca-certificates curl || true
  run_as_root rpm --import https://openresty.org/package/pubkey.gpg || true
  run_as_root zypper --non-interactive ar -g --refresh --check "$repo_url" openresty || true
  run_as_root zypper --non-interactive --gpg-auto-import-keys refresh openresty || true
  run_as_root zypper --non-interactive install openresty
}

install_openresty_linux() {
  if command -v apt-get >/dev/null 2>&1; then
    install_openresty_with_apt
  elif command -v dnf >/dev/null 2>&1; then
    install_openresty_with_dnf
  elif command -v yum >/dev/null 2>&1; then
    install_openresty_with_yum
  elif command -v apk >/dev/null 2>&1; then
    log "Installing OpenResty via apk..."
    run_as_root apk add --no-cache ca-certificates curl openresty
  elif command -v zypper >/dev/null 2>&1; then
    install_openresty_with_zypper
  elif command -v pacman >/dev/null 2>&1; then
    log "Installing OpenResty via pacman..."
    run_as_root pacman -Sy --needed --noconfirm ca-certificates curl openresty
  else
    die "no supported package manager found. Install OpenResty manually or pass --openresty-path."
  fi
}

install_openresty_darwin() {
  if ! command -v brew >/dev/null 2>&1; then
    die "Homebrew is required to install OpenResty automatically on macOS. Install Homebrew, install OpenResty manually, or pass --openresty-path."
  fi
  log "Installing OpenResty via Homebrew..."
  brew install openresty/brew/openresty || brew install openresty
}

ensure_openresty() {
  if [[ -n "$OPENRESTY_PATH" ]]; then
    return
  fi

  if OPENRESTY_PATH="$(find_openresty_path)"; then
    finish_pending_openresty_package_configuration
    remove_temporary_trusted_openresty_source
    disable_default_openresty_service
    return
  fi

  if [[ "$AUTO_INSTALL_DEPS" != "true" ]]; then
    die "openresty was not found in PATH. Install OpenResty first or pass --openresty-path."
  fi

  log "OpenResty was not found. Installing missing runtime dependency..."
  case "$OS" in
    linux) install_openresty_linux ;;
    darwin) install_openresty_darwin ;;
    *) die "unsupported OS for automatic OpenResty installation: $OS" ;;
  esac

  if ! OPENRESTY_PATH="$(find_openresty_path)"; then
    die "OpenResty installation completed, but openresty binary was still not found. Pass --openresty-path manually."
  fi
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
  local public_key placeholder key_b64 sig_text sig_b64 verify_dir pub_raw sig_raw pub_der pub_pem payload pub_len sig_len

  placeholder="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC""_KEY__"
  public_key="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-$RELEASE_SIGNATURE_PUBLIC_KEY}"
  [[ -n "$tag" && -n "$asset" && -n "$checksum" ]] || return 1
  [[ -n "$public_key" && "$public_key" != "$placeholder" ]] || return 1

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

  if ! printf '%s' "$key_b64" | "$OPENSSL_BIN" base64 -d -A > "$pub_raw" 2>/dev/null; then
    rm -rf -- "$verify_dir"
    return 1
  fi
  pub_len="$(wc -c < "$pub_raw" | tr -d '[:space:]')"
  if [[ "$pub_len" != "32" ]]; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  if ! printf '%s' "$sig_b64" | "$OPENSSL_BIN" base64 -d -A > "$sig_raw" 2>/dev/null; then
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
  if ! "$OPENSSL_BIN" pkey -pubin -inform DER -in "$pub_der" -out "$pub_pem" >/dev/null 2>&1; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  {
    printf 'dushengcdn-release-v1\n'
    printf '%s\n' "$tag"
    printf '%s\n' "$asset"
    printf '%s\n' "$checksum"
  } > "$payload"

  if ! "$OPENSSL_BIN" pkeyutl -verify -pubin -inkey "$pub_pem" -sigfile "$sig_raw" -rawin -in "$payload" >/dev/null 2>&1; then
    rm -rf -- "$verify_dir"
    return 1
  fi

  rm -rf -- "$verify_dir"
}

effective_release_signature_public_key() {
  local placeholder public_key
  placeholder="__DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC""_KEY__"
  public_key="${DUSHENGCDN_RELEASE_SIGNATURE_PUBLIC_KEY:-$RELEASE_SIGNATURE_PUBLIC_KEY}"
  if [[ -n "$public_key" && "$public_key" != "$placeholder" ]]; then
    printf '%s' "$public_key"
  fi
}

resolve_release_binary() {
  local release_info

  log "Fetching latest release from ${REPO}..."
  if ! release_info="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"; then
    log "No latest release was found. Falling back to source build."
    return 1
  fi

  DOWNLOAD_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\"" | grep -o 'https://[^"]*' | grep -v '\.sha256$' | grep -v '\.sig$' | head -n 1 || true)"
  SHA256_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\.sha256\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
  SIG_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\.sig\"" | grep -o 'https://[^"]*' | head -n 1 || true)"
  if [[ -z "$DOWNLOAD_URL" ]]; then
    log "No matching asset '${ASSET_NAME}' found in latest release. Falling back to source build."
    return 1
  fi
  if [[ -z "$SHA256_URL" ]]; then
    die "matching checksum asset '${ASSET_NAME}.sha256' was not found in latest release; refusing to install an unverified Agent binary."
  fi
  if [[ -z "$SIG_URL" ]]; then
    die "matching signature asset '${ASSET_NAME}.sig' was not found in latest release; refusing to install an unsigned Agent binary."
  fi

  TAG="$(echo "$release_info" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"')"
  if [[ -z "$TAG" ]]; then
    die "release tag was not found; refusing to verify an unsigned Agent binary."
  fi
  return 0
}

download_release_binary() {
  local actual expected sha_file sig_file

  ensure_release_signature_openssl
  log "Latest release: ${TAG}"
  log "Downloading ${ASSET_NAME}..."
  curl -fsSL -o "$TMP_BINARY" "$DOWNLOAD_URL"

  sha_file="$(mktemp "/tmp/dushengcdn-agent.sha256.XXXXXX")"
  if ! curl -fsSL -o "$sha_file" "$SHA256_URL"; then
    rm -f "$sha_file"
    die "failed to download Agent checksum asset."
  fi
  expected="$(parse_release_checksum "$sha_file" "$ASSET_NAME")"
  rm -f "$sha_file"
  if [[ ! "$expected" =~ ^[A-Fa-f0-9]{64}$ ]]; then
    die "Agent checksum asset is invalid."
  fi
  if ! actual="$(sha256_file "$TMP_BINARY")"; then
    die "sha256 tool was not found, cannot verify downloaded Agent asset."
  fi
  if [[ "$actual" != "$expected" ]]; then
    die "downloaded Agent checksum mismatch."
  fi

  sig_file="$(mktemp "/tmp/dushengcdn-agent.sig.XXXXXX")"
  if ! curl -fsSL -o "$sig_file" "$SIG_URL"; then
    rm -f "$sig_file"
    die "failed to download Agent signature asset."
  fi
  if ! verify_release_signature "$TAG" "$ASSET_NAME" "$expected" "$sig_file"; then
    rm -f "$sig_file"
    die "downloaded Agent signature verification failed."
  fi
  rm -f "$sig_file"
  log "Release asset checksum and signature verified."

  chmod +x "$TMP_BINARY"
}

build_binary_from_source() {
  local source_dir
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
  local source_version
  source_version="$(git -C "$source_dir" describe --tags --always --dirty 2>/dev/null || git -C "$source_dir" rev-parse --short HEAD 2>/dev/null || echo dev)"
  log "Building Agent version ${source_version}."
  local release_public_key ldflags
  release_public_key="$(effective_release_signature_public_key || true)"
  ldflags="-s -w -X=dushengcdn-agent/internal/config.AgentVersion=${source_version}"
  if [[ -n "$release_public_key" ]]; then
    ldflags="${ldflags} -X=dushengcdn-agent/internal/config.ReleaseSignaturePublicKey=${release_public_key}"
  else
    log "Release signature public key is not configured; source-built Agent self-upgrade will be unavailable."
  fi

  (
    cd "$source_dir/dushengcdn_agent"
    go mod download
    CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$TMP_BINARY" ./cmd/agent
  )

  rm -rf -- "$source_dir"
  chmod +x "$TMP_BINARY"
}

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

if [[ "$OS" != "linux" && "$OS" != "darwin" ]]; then
  echo "Unsupported OS: $OS"
  exit 1
fi

validate_install_dir
validate_service_name
validate_build_go_dir

if [[ "$OS" == "linux" && "$CREATE_SERVICE" == "true" && ! -d /etc/systemd/system ]]; then
  CREATE_SERVICE="false"
fi

INSTALL_PARENT="$(dirname "$INSTALL_DIR")"
NEEDS_ROOT="false"
if [[ ! -e "$INSTALL_PARENT" || ! -w "$INSTALL_PARENT" ]]; then
  NEEDS_ROOT="true"
fi
if [[ -d "$INSTALL_DIR" && ! -w "$INSTALL_DIR" ]]; then
  NEEDS_ROOT="true"
fi
if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" ]]; then
  NEEDS_ROOT="true"
fi

ensure_curl
ensure_openresty
ensure_geoip_lua_dependencies

if [[ ! -x "$OPENRESTY_PATH" ]]; then
  echo "Error: OpenResty binary is not executable: ${OPENRESTY_PATH}"
  exit 1
fi

ASSET_NAME="dushengcdn-agent-${OS}-${ARCH}"
echo "Detected platform: ${OS}/${ARCH}"

SYSTEMCTL_AVAILABLE="false"
if command -v systemctl >/dev/null 2>&1; then
  SYSTEMCTL_AVAILABLE="true"
fi

TMP_BINARY="$(mktemp "/tmp/dushengcdn-agent.tmp.XXXXXX")"
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
    echo "Error: no verified release binary is available for ${ASSET_NAME} in ${REPO}." >&2
    echo "Publish the binary release asset, or rerun with --allow-source-build and a source repository." >&2
    exit 1
  fi
fi

SERVICE_WAS_ACTIVE="false"
if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" && "$SYSTEMCTL_AVAILABLE" == "true" ]] && systemctl is-active --quiet "$SERVICE_NAME"; then
  SERVICE_WAS_ACTIVE="true"
  echo "Stopping running service before reinstall: ${SERVICE_NAME}"
  run_as_root systemctl stop "$SERVICE_NAME"
fi

if [[ -d "$INSTALL_DIR" ]]; then
  if [[ "$REINSTALL" == "true" && "$WIPE_DATA" == "true" ]]; then
    echo "Removing existing installation directory: ${INSTALL_DIR}"
    if [[ "$NEEDS_ROOT" == "true" ]]; then
      run_as_root rm -rf -- "$INSTALL_DIR"
    else
      rm -rf -- "$INSTALL_DIR"
    fi
  else
    echo "Existing installation found; upgrading in place and preserving local data."
  fi
fi

echo "Installing to ${INSTALL_DIR}..."
if [[ "$NEEDS_ROOT" == "true" ]]; then
  run_as_root mkdir -p "${INSTALL_DIR}/data"
  run_as_root install -m 0755 "$TMP_BINARY" "${INSTALL_DIR}/dushengcdn-agent"
else
  mkdir -p "${INSTALL_DIR}/data"
  mv -f "$TMP_BINARY" "${INSTALL_DIR}/dushengcdn-agent"
fi
trap - EXIT

# Generate config
CONFIG_FILE="${INSTALL_DIR}/agent.json"
if [[ -f "$CONFIG_FILE" && ( "$REINSTALL" != "true" || "$WIPE_DATA" != "true" ) ]]; then
  echo "Preserving existing agent.json: ${CONFIG_FILE}"
elif [[ -n "$AGENT_TOKEN" ]]; then
  echo "Generating agent.json..."
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    write_file_as_root "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "$(json_escape "$SERVER_URL")",
  "agent_token": "$(json_escape "$AGENT_TOKEN")",
  "openresty_path": "$(json_escape "$OPENRESTY_PATH")",
  "data_dir": "${INSTALL_DIR}/data",
  "geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "openresty_geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "heartbeat_interval": 30000,
  "request_timeout": 10000$(geoip_api_config_json)
}
CFGEOF
  else
    cat > "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "$(json_escape "$SERVER_URL")",
  "agent_token": "$(json_escape "$AGENT_TOKEN")",
  "openresty_path": "$(json_escape "$OPENRESTY_PATH")",
  "data_dir": "${INSTALL_DIR}/data",
  "geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "openresty_geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "heartbeat_interval": 30000,
  "request_timeout": 10000$(geoip_api_config_json)
}
CFGEOF
  fi
else
  echo "Generating agent.json..."
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    write_file_as_root "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "$(json_escape "$SERVER_URL")",
  "discovery_token": "$(json_escape "$DISCOVERY_TOKEN")",
  "openresty_path": "$(json_escape "$OPENRESTY_PATH")",
  "data_dir": "${INSTALL_DIR}/data",
  "geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "openresty_geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "heartbeat_interval": 30000,
  "request_timeout": 10000$(geoip_api_config_json)
}
CFGEOF
  else
    cat > "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "$(json_escape "$SERVER_URL")",
  "discovery_token": "$(json_escape "$DISCOVERY_TOKEN")",
  "openresty_path": "$(json_escape "$OPENRESTY_PATH")",
  "data_dir": "${INSTALL_DIR}/data",
  "geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "openresty_geoip_database_path": "${INSTALL_DIR}/data/var/lib/dushengcdn/geoip/GeoLite2-Country.mmdb",
  "heartbeat_interval": 30000,
  "request_timeout": 10000$(geoip_api_config_json)
}
CFGEOF
  fi
fi

# Create systemd service
if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" && -d /etc/systemd/system && "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  echo "Creating systemd service..."
  write_file_as_root "/etc/systemd/system/${SERVICE_NAME}.service" <<SVCEOF
[Unit]
Description=DuShengCDN Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/dushengcdn-agent -config ${CONFIG_FILE}
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SVCEOF

  run_as_root systemctl daemon-reload
  run_as_root systemctl enable "$SERVICE_NAME"
  run_as_root systemctl start "$SERVICE_NAME"
  if [[ "$SERVICE_WAS_ACTIVE" == "true" ]]; then
    echo "Service restarted with updated binary: ${SERVICE_NAME}"
  else
    echo "Service created and started: ${SERVICE_NAME}"
  fi
else
  echo ""
  echo "To start the agent manually:"
  echo "  ${INSTALL_DIR}/dushengcdn-agent -config ${CONFIG_FILE}"
fi

echo ""
echo "DuShengCDN Agent installed successfully!"
echo "  Binary: ${INSTALL_DIR}/dushengcdn-agent"
echo "  Config: ${CONFIG_FILE}"
echo "  Data:   ${INSTALL_DIR}/data"
echo "  OpenResty: ${OPENRESTY_PATH}"

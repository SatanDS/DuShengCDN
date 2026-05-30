#!/usr/bin/env bash
set -euo pipefail

# OpenFlare Agent Installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/SatanDS/OpenCDN/main/scripts/install-agent.sh | bash -s -- \
#     --server-url http://your-server:3000 \
#     --discovery-token your-token

INSTALL_DIR="/opt/openflare-agent"
REPO="SatanDS/OpenCDN"
SERVER_URL=""
DISCOVERY_TOKEN=""
AGENT_TOKEN=""
CREATE_SERVICE="true"
SERVICE_NAME="openflare-agent"
OPENRESTY_PATH=""
AUTO_INSTALL_DEPS="true"
SOURCE_REF="${SOURCE_REF:-main}"

usage() {
  cat <<EOF
OpenFlare Agent Installer

Usage:
  install-agent.sh [OPTIONS]

Options:
  --server-url URL          Server URL (required)
  --discovery-token TOKEN   Discovery token for auto-registration
  --agent-token TOKEN       Node-specific agent token
  --install-dir DIR         Installation directory (default: /opt/openflare-agent)
  --openresty-path PATH     OpenResty binary path (default: auto-detect from PATH)
  --repo REPO               GitHub repository (default: SatanDS/OpenCDN)
  --source-ref REF          Git branch, tag, or commit used when building from source (default: main)
  --install-deps            Install missing runtime dependencies automatically (default)
  --no-install-deps         Do not install missing dependencies automatically
  --no-service              Do not create systemd service
  -h, --help                Show this help message

Examples:
  # Install with discovery token (auto-register)
  install-agent.sh --server-url http://10.0.0.1:3000 --discovery-token abc123

  # Install with node-specific token
  install-agent.sh --server-url http://10.0.0.1:3000 --agent-token node-token-xyz

Notes:
  Reinstall will remove the entire install directory before installing again,
  including the old agent.json, local state, cached data, and downloaded binary.
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
    --repo)         REPO="$2"; shift 2 ;;
    --source-ref)   SOURCE_REF="$2"; shift 2 ;;
    --install-deps) AUTO_INSTALL_DEPS="true"; shift ;;
    --no-install-deps) AUTO_INSTALL_DEPS="false"; shift ;;
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
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var|/Applications)
      die "refusing to use unsafe install directory: ${INSTALL_DIR}"
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

  log "Disabling the package default openresty.service; OpenFlare Agent will manage OpenResty directly."
  run_as_root systemctl disable --now openresty.service >/dev/null 2>&1 || true
  run_as_root systemctl reset-failed openresty.service >/dev/null 2>&1 || true
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
    run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gnupg
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
    die "no supported package manager found. Install curl and OpenResty manually or pass --openresty-path."
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
  local go_version="1.25.0"
  local go_arch="$ARCH"
  local archive="/tmp/go${go_version}.linux-${go_arch}.tar.gz"

  log "Installing Go ${go_version} via go.dev..."
  curl -fsSL -o "$archive" "https://go.dev/dl/go${go_version}.linux-${go_arch}.tar.gz"
  run_as_root rm -rf /usr/local/go
  run_as_root tar -C /usr/local -xzf "$archive"
  rm -f "$archive"
}

install_go_darwin() {
  if ! command -v brew >/dev/null 2>&1; then
    die "Homebrew is required to install Go automatically on macOS. Install Homebrew, install Go manually, or publish release assets."
  fi
  log "Installing Go via Homebrew..."
  brew install go
}

ensure_go() {
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

  export PATH="/usr/local/go/bin:${PATH}"
  if ! command -v go >/dev/null 2>&1; then
    die "Go installation completed, but go is still not available in PATH."
  fi
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
  local used_trusted_openresty_source="false"
  source_line="deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/openresty.gpg] ${repo_base} ${codename} ${component}"
  echo "$source_line" | run_as_root tee /etc/apt/sources.list.d/openresty.list >/dev/null
  if ! run_as_root apt-get update; then
    if [[ "$distro" == "debian" && "$detected_codename" != "$codename" ]]; then
      log "OpenResty apt signature was rejected by the current Debian policy; retrying with a temporary trusted HTTPS source for OpenResty only."
      source_line="deb [arch=$(dpkg --print-architecture) trusted=yes] ${repo_base} ${codename} ${component}"
      echo "$source_line" | run_as_root tee /etc/apt/sources.list.d/openresty.list >/dev/null
      used_trusted_openresty_source="true"
      run_as_root apt-get -o Acquire::AllowInsecureRepositories=true update
    else
      die "apt update failed after enabling the OpenResty repository."
    fi
  fi
  if ! with_service_autostart_disabled run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y openresty; then
    if [[ "$used_trusted_openresty_source" == "true" ]]; then
      log "Retrying OpenResty install with --allow-unauthenticated because Debian rejected the repository signature policy."
      if ! with_service_autostart_disabled run_as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y --allow-unauthenticated openresty; then
        die "OpenResty package installation failed."
      fi
    else
      die "OpenResty package installation failed."
    fi
  fi
  if [[ "$used_trusted_openresty_source" == "true" ]]; then
    log "Removing temporary trusted OpenResty apt source."
    run_as_root rm -f /etc/apt/sources.list.d/openresty.list
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

resolve_release_binary() {
  local release_info

  log "Fetching latest release from ${REPO}..."
  if ! release_info="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")"; then
    log "No latest release was found. Falling back to source build."
    return 1
  fi

  DOWNLOAD_URL="$(echo "$release_info" | grep -o "\"browser_download_url\"[[:space:]]*:[[:space:]]*\"[^\"]*${ASSET_NAME}\"" | grep -o 'https://[^"]*' || true)"
  if [[ -z "$DOWNLOAD_URL" ]]; then
    log "No matching asset '${ASSET_NAME}' found in latest release. Falling back to source build."
    return 1
  fi

  TAG="$(echo "$release_info" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | grep -o '"[^"]*"$' | tr -d '"')"
  return 0
}

download_release_binary() {
  log "Latest release: ${TAG}"
  log "Downloading ${ASSET_NAME}..."
  curl -fsSL -o "$TMP_BINARY" "$DOWNLOAD_URL"
  chmod +x "$TMP_BINARY"
}

build_binary_from_source() {
  local source_dir
  source_dir="$(mktemp -d "/tmp/opencdn-source.XXXXXX")"

  ensure_source_build_tools
  ensure_go

  log "Fetching ${REPO}@${SOURCE_REF} and building ${ASSET_NAME}..."
  git init "$source_dir" >/dev/null 2>&1
  git -C "$source_dir" remote add origin "https://github.com/${REPO}.git"
  git -C "$source_dir" fetch --depth 1 origin "$SOURCE_REF" >/dev/null 2>&1 || {
    rm -rf "$source_dir"
    die "failed to fetch ${REPO}@${SOURCE_REF}. Publish release assets or pass --source-ref with a valid branch, tag, or commit."
  }
  git -C "$source_dir" checkout --detach FETCH_HEAD >/dev/null 2>&1

  (
    cd "$source_dir/openflare_agent"
    go mod download
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$TMP_BINARY" ./cmd/agent
  )

  rm -rf "$source_dir"
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

if [[ ! -x "$OPENRESTY_PATH" ]]; then
  echo "Error: OpenResty binary is not executable: ${OPENRESTY_PATH}"
  exit 1
fi

ASSET_NAME="openflare-agent-${OS}-${ARCH}"
echo "Detected platform: ${OS}/${ARCH}"

SYSTEMCTL_AVAILABLE="false"
if command -v systemctl >/dev/null 2>&1; then
  SYSTEMCTL_AVAILABLE="true"
fi

TMP_BINARY="$(mktemp "/tmp/openflare-agent.tmp.XXXXXX")"
cleanup() {
  rm -f "$TMP_BINARY"
}
trap cleanup EXIT

DOWNLOAD_URL=""
TAG=""
if resolve_release_binary; then
  download_release_binary
else
  build_binary_from_source
fi

SERVICE_WAS_ACTIVE="false"
if [[ "$OS" == "linux" && "$SYSTEMCTL_AVAILABLE" == "true" ]] && systemctl is-active --quiet "$SERVICE_NAME"; then
  SERVICE_WAS_ACTIVE="true"
  echo "Stopping running service before reinstall..."
  run_as_root systemctl stop "$SERVICE_NAME"
fi

if [[ -d "$INSTALL_DIR" ]]; then
  echo "Removing existing installation directory: ${INSTALL_DIR}"
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    run_as_root rm -rf -- "$INSTALL_DIR"
  else
    rm -rf -- "$INSTALL_DIR"
  fi
fi

echo "Installing to ${INSTALL_DIR}..."
if [[ "$NEEDS_ROOT" == "true" ]]; then
  run_as_root mkdir -p "${INSTALL_DIR}/data"
  run_as_root install -m 0755 "$TMP_BINARY" "${INSTALL_DIR}/openflare-agent"
else
  mkdir -p "${INSTALL_DIR}/data"
  mv -f "$TMP_BINARY" "${INSTALL_DIR}/openflare-agent"
fi
trap - EXIT

# Generate config
CONFIG_FILE="${INSTALL_DIR}/agent.json"
echo "Generating agent.json..."
if [[ -n "$AGENT_TOKEN" ]]; then
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    write_file_as_root "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "${SERVER_URL}",
  "agent_token": "${AGENT_TOKEN}",
  "openresty_path": "${OPENRESTY_PATH}",
  "data_dir": "${INSTALL_DIR}/data",
  "heartbeat_interval": 30000,
  "request_timeout": 10000
}
CFGEOF
  else
    cat > "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "${SERVER_URL}",
  "agent_token": "${AGENT_TOKEN}",
  "openresty_path": "${OPENRESTY_PATH}",
  "data_dir": "${INSTALL_DIR}/data",
  "heartbeat_interval": 30000,
  "request_timeout": 10000
}
CFGEOF
  fi
else
  if [[ "$NEEDS_ROOT" == "true" ]]; then
    write_file_as_root "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "${SERVER_URL}",
  "discovery_token": "${DISCOVERY_TOKEN}",
  "openresty_path": "${OPENRESTY_PATH}",
  "data_dir": "${INSTALL_DIR}/data",
  "heartbeat_interval": 30000,
  "request_timeout": 10000
}
CFGEOF
  else
    cat > "$CONFIG_FILE" <<CFGEOF
{
  "server_url": "${SERVER_URL}",
  "discovery_token": "${DISCOVERY_TOKEN}",
  "openresty_path": "${OPENRESTY_PATH}",
  "data_dir": "${INSTALL_DIR}/data",
  "heartbeat_interval": 30000,
  "request_timeout": 10000
}
CFGEOF
  fi
fi

# Create systemd service
if [[ "$CREATE_SERVICE" == "true" && "$OS" == "linux" && -d /etc/systemd/system && "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  echo "Creating systemd service..."
  write_file_as_root /etc/systemd/system/openflare-agent.service <<SVCEOF
[Unit]
Description=OpenFlare Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/openflare-agent -config ${CONFIG_FILE}
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
  echo "  ${INSTALL_DIR}/openflare-agent -config ${CONFIG_FILE}"
fi

echo ""
echo "OpenFlare Agent installed successfully!"
echo "  Binary: ${INSTALL_DIR}/openflare-agent"
echo "  Config: ${CONFIG_FILE}"
echo "  Data:   ${INSTALL_DIR}/data"
echo "  OpenResty: ${OPENRESTY_PATH}"

#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/dushengcdn-agent"
SERVICE_NAME="dushengcdn-agent"

usage() {
  cat <<EOF
DuShengCDN Agent Uninstaller

Usage:
  uninstall-agent.sh [OPTIONS]

Options:
  --install-dir DIR         Installation directory (default: /opt/dushengcdn-agent)
  --service-name NAME       systemd service name (default: dushengcdn-agent)
  -h, --help                Show this help message

Behavior:
  1. Stop the agent service/process and remove the entire installation directory
  2. Remove the systemd service definition when present
  3. Check the saved agent config to identify the OpenResty mode
  4. If Docker mode was used, remove the OpenResty container and try to remove its image
  5. If local openresty_path mode was used, do not modify the local OpenResty install

Examples:
  uninstall-agent.sh
  uninstall-agent.sh --install-dir /srv/dushengcdn-agent
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)  INSTALL_DIR="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    -h|--help)      usage ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

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

validate_service_name() {
  if [[ -z "$SERVICE_NAME" ]]; then
    echo "Refusing to use empty systemd service name" >&2
    exit 1
  fi
  case "$SERVICE_NAME" in
    *[!A-Za-z0-9_.@-]*|.*|*-|*@|*..*|*/*)
      echo "Refusing to use unsafe systemd service name: '${SERVICE_NAME}'" >&2
      exit 1
      ;;
  esac
}

validate_install_dir
validate_service_name

json_get_string() {
  local file="$1"
  local key="$2"
  local match

  match=$(grep -o "\"${key}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" "$file" 2>/dev/null | head -n 1 || true)
  if [[ -z "$match" ]]; then
    return 0
  fi

  printf '%s\n' "$match" | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/'
}

AGENT_BINARY="${INSTALL_DIR}/dushengcdn-agent"
CONFIG_FILE="${INSTALL_DIR}/agent.json"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

OPENRESTY_PATH=""
OPENRESTY_CONTAINER_NAME="dushengcdn-openresty"
OPENRESTY_DOCKER_IMAGE="openresty/openresty:alpine"
DOCKER_BINARY="docker"
OPENRESTY_MODE="unknown"

if [[ -f "$CONFIG_FILE" ]]; then
  OPENRESTY_PATH="$(json_get_string "$CONFIG_FILE" "openresty_path")"
  OPENRESTY_CONTAINER_NAME="$(json_get_string "$CONFIG_FILE" "openresty_container_name")"
  OPENRESTY_DOCKER_IMAGE="$(json_get_string "$CONFIG_FILE" "openresty_docker_image")"
  DOCKER_BINARY="$(json_get_string "$CONFIG_FILE" "docker_binary")"

  if [[ -z "$OPENRESTY_CONTAINER_NAME" ]]; then
    OPENRESTY_CONTAINER_NAME="dushengcdn-openresty"
  fi
  if [[ -z "$OPENRESTY_DOCKER_IMAGE" ]]; then
    OPENRESTY_DOCKER_IMAGE="openresty/openresty:alpine"
  fi
  if [[ -z "$DOCKER_BINARY" ]]; then
    DOCKER_BINARY="docker"
  fi

  if [[ -n "$OPENRESTY_PATH" ]]; then
    OPENRESTY_MODE="local"
  else
    OPENRESTY_MODE="docker"
  fi
fi

SYSTEMCTL_AVAILABLE="false"
if command -v systemctl >/dev/null 2>&1; then
  SYSTEMCTL_AVAILABLE="true"
fi

echo "Uninstalling DuShengCDN Agent from ${INSTALL_DIR}..."

if [[ "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "Stopping service: ${SERVICE_NAME}"
    systemctl stop "$SERVICE_NAME"
  fi

  if systemctl is-enabled --quiet "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "Disabling service: ${SERVICE_NAME}"
    systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
  fi
fi

if command -v pgrep >/dev/null 2>&1; then
  mapfile -t agent_pids < <(pgrep -f "$AGENT_BINARY" || true)
  if (( ${#agent_pids[@]} > 0 )); then
    echo "Stopping agent process: ${agent_pids[*]}"
    kill "${agent_pids[@]}" || true
    sleep 1

    mapfile -t remaining_agent_pids < <(pgrep -f "$AGENT_BINARY" || true)
    if (( ${#remaining_agent_pids[@]} > 0 )); then
      echo "Force stopping remaining agent process: ${remaining_agent_pids[*]}"
      kill -9 "${remaining_agent_pids[@]}" || true
    fi
  fi
fi

if [[ -f "$SERVICE_FILE" ]]; then
  echo "Removing service file: ${SERVICE_FILE}"
  rm -f "$SERVICE_FILE"
fi

if [[ "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  systemctl daemon-reload || true
  systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
fi

if [[ -d "$INSTALL_DIR" ]]; then
  echo "Removing installation directory: ${INSTALL_DIR}"
  rm -rf -- "$INSTALL_DIR"
else
  echo "Installation directory not found, skipping: ${INSTALL_DIR}"
fi

echo "Agent uninstall complete."
echo ""
echo "Checking OpenResty installation mode..."

if [[ "$OPENRESTY_MODE" == "docker" ]]; then
  echo "Detected Docker OpenResty mode."

  if ! command -v "$DOCKER_BINARY" >/dev/null 2>&1; then
    echo "Docker binary '${DOCKER_BINARY}' was not found."
    echo "Please remove container '${OPENRESTY_CONTAINER_NAME}' and image '${OPENRESTY_DOCKER_IMAGE}' manually."
    exit 0
  fi

  if "$DOCKER_BINARY" inspect "$OPENRESTY_CONTAINER_NAME" >/dev/null 2>&1; then
    if [[ -z "$OPENRESTY_DOCKER_IMAGE" ]]; then
      OPENRESTY_DOCKER_IMAGE="$("$DOCKER_BINARY" inspect -f '{{.Config.Image}}' "$OPENRESTY_CONTAINER_NAME" 2>/dev/null || true)"
    fi

    echo "Removing Docker container: ${OPENRESTY_CONTAINER_NAME}"
    "$DOCKER_BINARY" rm -f "$OPENRESTY_CONTAINER_NAME"
  else
    echo "Docker container not found, skipping: ${OPENRESTY_CONTAINER_NAME}"
  fi

  if [[ -n "$OPENRESTY_DOCKER_IMAGE" ]] && "$DOCKER_BINARY" image inspect "$OPENRESTY_DOCKER_IMAGE" >/dev/null 2>&1; then
    other_container_ids="$("$DOCKER_BINARY" ps -a --filter "ancestor=${OPENRESTY_DOCKER_IMAGE}" --format '{{.ID}}' 2>/dev/null || true)"
    if [[ -z "$other_container_ids" ]]; then
      echo "Removing Docker image: ${OPENRESTY_DOCKER_IMAGE}"
      if ! "$DOCKER_BINARY" image rm "$OPENRESTY_DOCKER_IMAGE"; then
        echo "Image removal skipped because Docker reported it is still in use."
      fi
    else
      echo "Docker image is still used by other containers, skipping image removal: ${OPENRESTY_DOCKER_IMAGE}"
    fi
  fi

  echo "Docker OpenResty cleanup complete."
elif [[ "$OPENRESTY_MODE" == "local" ]]; then
  echo "Detected local OpenResty mode via openresty_path:"
  echo "  ${OPENRESTY_PATH}"
  echo "Agent has been removed, but the local OpenResty installation was not modified."
  echo "Please uninstall the local OpenResty manually if you no longer need it."
else
  echo "OpenResty mode could not be determined because ${CONFIG_FILE} was not found before uninstall."
  echo "If you were using Docker OpenResty, please remove its container and image manually if needed."
fi

echo ""
echo "DuShengCDN Agent uninstall finished."

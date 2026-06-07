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
  3. Leave the local OpenResty installation untouched

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

AGENT_BINARY="${INSTALL_DIR}/dushengcdn-agent"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

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
echo "Local OpenResty was not modified. Remove it manually if you no longer need it."
echo "DuShengCDN Agent uninstall finished."

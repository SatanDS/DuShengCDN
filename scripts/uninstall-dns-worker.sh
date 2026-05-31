#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/dushengcdn-dns-worker"
SERVICE_NAME="dushengcdn-dns-worker"

usage() {
  cat <<EOF
DuShengCDN DNS Worker Uninstaller

Usage:
  uninstall-dns-worker.sh [OPTIONS]

Options:
  --install-dir DIR         Installation directory (default: /opt/dushengcdn-dns-worker)
  --service-name NAME       systemd service name (default: dushengcdn-dns-worker)
  -h, --help                Show this help message

Behavior:
  1. Stop the DNS Worker service/process and remove the entire installation directory
  2. Remove the systemd service definition when present
  3. Remove the local snapshot cache and DNS Worker environment file

Examples:
  uninstall-dns-worker.sh
  uninstall-dns-worker.sh --install-dir /srv/dushengcdn-dns-worker
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --service-name) SERVICE_NAME="$2"; shift 2 ;;
    -h|--help) usage ;;
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
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/sys|/tmp|/usr|/var|/Applications)
      echo "Refusing to remove unsafe install directory: '${INSTALL_DIR}'" >&2
      exit 1
      ;;
  esac
}

validate_install_dir

WORKER_BINARY="${INSTALL_DIR}/dushengcdn-dns-worker"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

SYSTEMCTL_AVAILABLE="false"
if command -v systemctl >/dev/null 2>&1; then
  SYSTEMCTL_AVAILABLE="true"
fi

echo "Uninstalling DuShengCDN DNS Worker from ${INSTALL_DIR}..."

if [[ "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "Stopping service: ${SERVICE_NAME}"
    run_as_root systemctl stop "$SERVICE_NAME"
  fi

  if systemctl is-enabled --quiet "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "Disabling service: ${SERVICE_NAME}"
    run_as_root systemctl disable "$SERVICE_NAME" >/dev/null 2>&1 || true
  fi
fi

if command -v pgrep >/dev/null 2>&1; then
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
fi

if [[ -f "$SERVICE_FILE" ]]; then
  echo "Removing service file: ${SERVICE_FILE}"
  run_as_root rm -f "$SERVICE_FILE"
fi

if [[ "$SYSTEMCTL_AVAILABLE" == "true" ]]; then
  run_as_root systemctl daemon-reload || true
  run_as_root systemctl reset-failed "$SERVICE_NAME" >/dev/null 2>&1 || true
fi

if [[ -d "$INSTALL_DIR" ]]; then
  echo "Removing installation directory: ${INSTALL_DIR}"
  run_as_root rm -rf -- "$INSTALL_DIR"
else
  echo "Installation directory not found, skipping: ${INSTALL_DIR}"
fi

echo "DuShengCDN DNS Worker uninstall finished."

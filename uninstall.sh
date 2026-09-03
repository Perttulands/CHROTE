#!/usr/bin/env bash
set -euo pipefail

PREFIX="${CHROTE_INSTALL_PREFIX:-$HOME/.local}"
CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
SERVICE_DIR="${CHROTE_SERVICE_DIR:-$CONFIG_HOME/systemd/user}"
MANAGE_SYSTEMD=1
ASSUME_YES=0
PURGE_STATE=0
PURGE_PRIVATE_CONFIG=0

usage() {
  cat <<'EOF'
Usage: ./uninstall.sh [options]

Options:
  --prefix PATH           Installation prefix (default: $HOME/.local)
  --no-systemd            Do not stop, disable, or reload the user service manager
  --purge-state           Remove CHROTE state under XDG_STATE_HOME
  --purge-private-config  Remove secrets.env as well as managed config
  -y, --yes               Do not prompt
  -h, --help              Show this help

The configured workspace is never deleted.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --prefix)
      [ "$#" -ge 2 ] || { echo "--prefix requires a path" >&2; exit 2; }
      PREFIX="$2"
      shift 2
      ;;
    --no-systemd)
      MANAGE_SYSTEMD=0
      shift
      ;;
    --purge-state)
      PURGE_STATE=1
      shift
      ;;
    --purge-private-config)
      PURGE_PRIVATE_CONFIG=1
      shift
      ;;
    -y|--yes)
      ASSUME_YES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

PREFIX="$(realpath -m -- "$PREFIX")"
CONFIG_HOME="$(realpath -m -- "$CONFIG_HOME")"
STATE_HOME="$(realpath -m -- "$STATE_HOME")"
SERVICE_DIR="$(realpath -m -- "$SERVICE_DIR")"

if [ "$ASSUME_YES" -ne 1 ]; then
  read -r -p "Remove CHROTE executables and user service? [y/N] " reply
  [[ "$reply" =~ ^[Yy]$ ]] || { echo "Cancelled."; exit 0; }
fi

if [ "$MANAGE_SYSTEMD" -eq 1 ]; then
  command -v systemctl >/dev/null 2>&1 || { echo "systemctl is required unless --no-systemd is used" >&2; exit 1; }
  systemctl --user disable --now chrote.service 2>/dev/null || true
fi

rm -f \
  "$PREFIX/bin/chrote-server" \
  "$PREFIX/bin/chrote-agent-event" \
  "$SERVICE_DIR/chrote.service" \
  "$CONFIG_HOME/chrote/chrote.env"

# CHROTE serves terminals from its own process since ADR-0018 and installs
# neither of these any more. They are removed so an upgraded install does not
# keep a stray helper binary and launch script behind.
rm -f \
  "$PREFIX/bin/ttyd" \
  "$PREFIX/lib/chrote/terminal-launch.sh"
rmdir "$PREFIX/lib/chrote" 2>/dev/null || true
rmdir "$PREFIX/lib" 2>/dev/null || true

if [ "$PURGE_PRIVATE_CONFIG" -eq 1 ]; then
  rm -f "$CONFIG_HOME/chrote/secrets.env"
fi
rmdir "$CONFIG_HOME/chrote" 2>/dev/null || true

if [ "$PURGE_STATE" -eq 1 ]; then
  rm -rf "$STATE_HOME/chrote"
fi

if [ "$MANAGE_SYSTEMD" -eq 1 ]; then
  systemctl --user daemon-reload
fi

printf '[CHROTE] Removed executables, managed config, and user unit.\n'
printf '[CHROTE] Workspace preserved.\n'
if [ "$PURGE_STATE" -eq 1 ]; then
  printf '[CHROTE] State removed: %s\n' "$STATE_HOME/chrote"
else
  printf '[CHROTE] State preserved: %s\n' "$STATE_HOME/chrote"
fi
if [ "$PURGE_PRIVATE_CONFIG" -eq 1 ]; then
  printf '[CHROTE] Private overrides removed: %s\n' "$CONFIG_HOME/chrote/secrets.env"
else
  printf '[CHROTE] Private overrides preserved: %s\n' "$CONFIG_HOME/chrote/secrets.env"
fi

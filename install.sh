#!/usr/bin/env bash
set -euo pipefail

readonly REPO="Perttulands/CHROTE"
readonly TTYD_VERSION="1.7.7"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PREFIX="${CHROTE_INSTALL_PREFIX:-$HOME/.local}"
CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
SERVICE_DIR="${CHROTE_SERVICE_DIR:-$CONFIG_HOME/systemd/user}"
WORKSPACE="${CHROTE_WORKSPACE:-$HOME}"
PORT="8094"
TTYD_PORT="7683"
BINARY_SOURCE=""
MANAGE_SYSTEMD=1
ENABLE_SERVICE=1
START_SERVICE=1

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { printf '%b[CHROTE]%b %s\n' "$CYAN" "$NC" "$1"; }
success() { printf '%b[CHROTE]%b %s\n' "$GREEN" "$NC" "$1"; }
warn() { printf '%b[CHROTE]%b %s\n' "$YELLOW" "$NC" "$1"; }
die() { printf '%b[CHROTE]%b %s\n' "$RED" "$NC" "$1" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: ./install.sh [options]

Build and install the checked-out CHROTE source as a user service.

Options:
  --binary PATH       Install an already-built chrote-server instead of building
  --workspace PATH    Allowed file root and default session directory (default: $HOME)
  --port PORT         Loopback dashboard port (default: 8094)
  --ttyd-port PORT    Loopback ttyd port managed by CHROTE (default: 7683)
  --prefix PATH       Installation prefix (default: $HOME/.local)
  --no-systemd        Write the unit but do not reload, enable, or start systemd
  --no-enable         Do not enable the user service
  --no-start          Do not start or health-check the user service
  -h, --help          Show this help

Environment overrides used by tests and packaging:
  CHROTE_INSTALL_PREFIX, CHROTE_SERVICE_DIR, XDG_CONFIG_HOME, XDG_STATE_HOME
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary)
      [ "$#" -ge 2 ] || die "--binary requires a path"
      BINARY_SOURCE="$2"
      shift 2
      ;;
    --workspace)
      [ "$#" -ge 2 ] || die "--workspace requires a path"
      WORKSPACE="$2"
      shift 2
      ;;
    --port)
      [ "$#" -ge 2 ] || die "--port requires a value"
      PORT="$2"
      shift 2
      ;;
    --ttyd-port)
      [ "$#" -ge 2 ] || die "--ttyd-port requires a value"
      TTYD_PORT="$2"
      shift 2
      ;;
    --prefix)
      [ "$#" -ge 2 ] || die "--prefix requires a path"
      PREFIX="$2"
      shift 2
      ;;
    --no-systemd)
      MANAGE_SYSTEMD=0
      ENABLE_SERVICE=0
      START_SERVICE=0
      shift
      ;;
    --no-enable)
      ENABLE_SERVICE=0
      shift
      ;;
    --no-start)
      START_SERVICE=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

validate_port() {
  local name="$1" value="$2"
  [[ "$value" =~ ^[0-9]+$ ]] || die "$name must be an integer"
  [ "$value" -ge 1 ] && [ "$value" -le 65535 ] || die "$name must be between 1 and 65535"
}

absolute_path() {
  realpath -m -- "$1"
}

quote_env_value() {
  local value="$1"
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || die "environment values may not contain newlines"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '"%s"' "$value"
}

quote_unit_value() {
  local value="$1"
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || die "unit paths may not contain newlines"
  value="${value//\\/\\x5c}"
  value="${value//%/%%}"
  value="${value// /\\x20}"
  value="${value//$'\t'/\\x09}"
  printf '%s' "$value"
}

version_from_source() {
  local raw
  raw="$(tr -d '\r\n' < "$SCRIPT_DIR/VERSION")"
  [ -n "$raw" ] || die "VERSION is empty"
  printf '%s\n' "$raw"
}

validate_node_version() {
  local version major minor patch
  version="$(node -p 'process.versions.node')"
  IFS=. read -r major minor patch <<<"$version"
  if [ "$major" -eq 20 ] && [ "$minor" -ge 19 ]; then
    return
  fi
  if { [ "$major" -eq 22 ] && [ "$minor" -ge 12 ]; } || [ "$major" -gt 22 ]; then
    return
  fi
  die "Node.js 20.19+ or 22.12+ is required; found $version"
}

build_server() {
  local destination="$1" version
  if [ -n "$BINARY_SOURCE" ]; then
    BINARY_SOURCE="$(absolute_path "$BINARY_SOURCE")"
    [ -x "$BINARY_SOURCE" ] || die "--binary path is not executable: $BINARY_SOURCE"
    install -m 0755 "$BINARY_SOURCE" "$destination"
    return
  fi

  require_command go
  require_command node
  require_command npm
  validate_node_version
  [ -f "$SCRIPT_DIR/src/go.mod" ] || die "install.sh must run from a CHROTE source checkout"
  [ -f "$SCRIPT_DIR/dashboard/package.json" ] || die "dashboard source is missing"

  version="$(version_from_source)"
  log "Building dashboard and Go server from this checkout ($version)..."
  GOTOOLCHAIN=auto "$SCRIPT_DIR/scripts/build-embedded-dashboard.sh"
  (
    cd "$SCRIPT_DIR/src"
    GOTOOLCHAIN=auto go build -trimpath -ldflags "-X main.Version=$version" -o "$destination" ./cmd/server
  )
}

install_ttyd() {
  local destination="$1" existing arch asset url
  existing="$(command -v ttyd || true)"
  if [ -n "$existing" ]; then
    if [ "$(absolute_path "$existing")" != "$(absolute_path "$destination")" ]; then
      install -m 0755 "$existing" "$destination"
    fi
    return
  fi

  require_command curl
  arch="$(uname -m)"
  case "$arch" in
    x86_64|aarch64) asset="$arch" ;;
    *) die "unsupported ttyd architecture: $arch" ;;
  esac
  url="https://github.com/tsl0922/ttyd/releases/download/${TTYD_VERSION}/ttyd.${asset}"
  log "Downloading ttyd $TTYD_VERSION for $asset..."
  curl --fail --location --silent --show-error "$url" -o "$destination"
  chmod 0755 "$destination"
}

write_environment() {
  local env_file="$1" state_dir="$2" launch_script="$3" service_path
  service_path="$PREFIX/bin:${PATH:-/usr/local/bin:/usr/bin:/bin}"
  cat > "$env_file" <<EOF
# Managed by CHROTE install.sh. Put private optional service values in secrets.env.
HOST=$(quote_env_value "127.0.0.1")
PORT=$(quote_env_value "$PORT")
TTYD_PORT=$(quote_env_value "$TTYD_PORT")
CHROTE_ROOTS=$(quote_env_value "$WORKSPACE")
CHROTE_WRITE_ROOTS=$(quote_env_value "$WORKSPACE")
CHROTE_WORKDIR=$(quote_env_value "$WORKSPACE")
CHROTE_DEFAULT_TMUX_WORKDIR=$(quote_env_value "$WORKSPACE")
CHROTE_LAUNCH_SCRIPT=$(quote_env_value "$launch_script")
CHROTE_BEADS_WORKSPACES=$(quote_env_value "$WORKSPACE")
CHROTE_SESSION_DROPS_DIR=$(quote_env_value "$state_dir/session-drops")
CHROTE_SCHEDULED_TASKS_DIR=$(quote_env_value "$state_dir/scheduled-tasks")
PATH=$(quote_env_value "$service_path")
EOF
  chmod 0600 "$env_file"
}

write_service() {
  local unit_file="$1" binary="$2" env_file="$3" secrets_file="$4"
  cat > "$unit_file" <<EOF
[Unit]
Description=CHROTE private workspace cockpit
Documentation=https://github.com/$REPO
After=network.target

[Service]
Type=simple
WorkingDirectory=$(quote_unit_value "$WORKSPACE")
EnvironmentFile=$(quote_unit_value "$env_file")
EnvironmentFile=-$(quote_unit_value "$secrets_file")
ExecStart=$(quote_unit_value "$binary")
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
}

health_check() {
  local url="http://127.0.0.1:$PORT/api/health" attempt expected_version payload compact
  require_command curl
  expected_version="$(version_from_source)"
  for attempt in $(seq 1 80); do
    if systemctl --user is-active --quiet chrote.service; then
      if payload="$(curl --fail --silent --show-error "$url" 2>/dev/null)"; then
        compact="${payload//[[:space:]]/}"
        if [[ "$compact" == *'"status":"ok"'* && "$compact" == *"\"version\":\"$expected_version\""* ]]; then
          success "CHROTE $expected_version is healthy at http://127.0.0.1:$PORT"
          return
        fi
        die "health endpoint responded, but not as CHROTE $expected_version; another process may own port $PORT"
      fi
    fi
    sleep 0.1
  done
  die "service started but health check failed; inspect: journalctl --user -u chrote.service"
}

main() {
  local bin_dir lib_dir config_dir state_dir binary launch_script env_file secrets_file unit_file build_tmp

  require_command install
  require_command realpath
  require_command tmux
  # Send to Session uses POSIX ACLs so only the target Unix user can read drops.
  require_command setfacl
  if [ "$MANAGE_SYSTEMD" -eq 1 ]; then
    require_command systemctl
  fi
  validate_port "--port" "$PORT"
  validate_port "--ttyd-port" "$TTYD_PORT"
  [ "$PORT" != "$TTYD_PORT" ] || die "dashboard and ttyd ports must differ"

  PREFIX="$(absolute_path "$PREFIX")"
  CONFIG_HOME="$(absolute_path "$CONFIG_HOME")"
  STATE_HOME="$(absolute_path "$STATE_HOME")"
  SERVICE_DIR="$(absolute_path "$SERVICE_DIR")"
  WORKSPACE="$(absolute_path "$WORKSPACE")"

  mkdir -p "$WORKSPACE"
  bin_dir="$PREFIX/bin"
  lib_dir="$PREFIX/lib/chrote"
  config_dir="$CONFIG_HOME/chrote"
  state_dir="$STATE_HOME/chrote"
  binary="$bin_dir/chrote-server"
  launch_script="$lib_dir/terminal-launch.sh"
  env_file="$config_dir/chrote.env"
  secrets_file="$config_dir/secrets.env"
  unit_file="$SERVICE_DIR/chrote.service"

  install -d -m 0755 "$bin_dir" "$lib_dir" "$SERVICE_DIR"
  install -d -m 0700 "$config_dir" "$state_dir"
  install -d -m 0700 \
    "$state_dir/session-drops" \
    "$state_dir/scheduled-tasks"

  build_tmp="$(mktemp "$bin_dir/.chrote-server.XXXXXX")"
  trap 'rm -f "$build_tmp"' EXIT
  build_server "$build_tmp"
  chmod 0755 "$build_tmp"
  mv -f "$build_tmp" "$binary"
  trap - EXIT

  install_ttyd "$bin_dir/ttyd"
  install -m 0755 "$SCRIPT_DIR/terminal-launch.sh" "$launch_script"
  write_environment "$env_file" "$state_dir" "$launch_script"
  [ -e "$secrets_file" ] || { : > "$secrets_file"; chmod 0600 "$secrets_file"; }
  write_service "$unit_file" "$binary" "$env_file" "$secrets_file"

  if [ "$MANAGE_SYSTEMD" -eq 1 ]; then
    systemctl --user daemon-reload
    if [ "$ENABLE_SERVICE" -eq 1 ]; then
      systemctl --user enable chrote.service
    fi
    if [ "$START_SERVICE" -eq 1 ]; then
      systemctl --user restart chrote.service
      health_check
    else
      warn "Installed without starting. Start with: systemctl --user start chrote.service"
    fi
  else
    warn "Installed without touching systemd; unit written to $unit_file"
  fi

  success "Installed CHROTE binary: $binary"
  success "Workspace root: $WORKSPACE"
  success "Managed config: $env_file"
  success "Private overrides: $secrets_file"
}

main

#!/usr/bin/env bash
set -euo pipefail

readonly REPO="Perttulands/CHROTE"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PREFIX="${CHROTE_INSTALL_PREFIX:-$HOME/.local}"
CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
SERVICE_DIR="${CHROTE_SERVICE_DIR:-$CONFIG_HOME/systemd/user}"
WORKSPACE="${CHROTE_WORKSPACE:-$HOME}"
PORT="8094"
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

default_tmux_socket() {
  local socket_root="${TMUX_TMPDIR:-/tmp}"
  printf '%s/tmux-%s/default\n' "${socket_root%/}" "$(id -u)"
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

write_environment() {
  local env_file="$1" state_dir="$2" service_path tmux_mapping
  service_path="$PREFIX/bin:${PATH:-/usr/local/bin:/usr/bin:/bin}"
  tmux_mapping="${CHROTE_TMUX_SOCKET:-$(id -un)=$(default_tmux_socket)}"
  cat > "$env_file" <<EOF
# Managed by CHROTE install.sh. Put private optional service values in secrets.env.
HOST=$(quote_env_value "127.0.0.1")
PORT=$(quote_env_value "$PORT")
CHROTE_ROOTS=$(quote_env_value "$WORKSPACE")
CHROTE_WORKDIR=$(quote_env_value "$WORKSPACE")
CHROTE_TMUX_SOCKET=$(quote_env_value "$tmux_mapping")
CHROTE_BEADS_WORKSPACES=$(quote_env_value "$WORKSPACE")
CHROTE_SESSION_DROPS_DIR=$(quote_env_value "$state_dir/session-drops")
CHROTE_SCHEDULED_TASKS_DIR=$(quote_env_value "$state_dir/scheduled-tasks")
CHROTE_AGENT_HOOKS_DIR=$(quote_env_value "$state_dir/agent-hooks")
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
KillMode=process
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

run_as_tmux_owner() {
  local owner="$1"
  shift
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$owner" -- "$@"
  else
    sudo -n -u "$owner" "$@"
  fi
}

grant_tmux_access() {
  local mappings="${CHROTE_TMUX_SOCKET:-}"
  [ -n "$mappings" ] || return 0

  [ "$(id -u)" -eq 0 ] || die "CHROTE_TMUX_SOCKET grants require a root install"
  local service_user="${CHROTE_TMUX_GRANT_USER:-chrote}"
  id "$service_user" >/dev/null 2>&1 || die "tmux grant user does not exist: $service_user"
  require_command setfacl
  require_command stat

  local tmux_bin
  if [ -n "${CHROTE_TMUX_BIN:-}" ]; then
    [ -x "$CHROTE_TMUX_BIN" ] || die "CHROTE_TMUX_BIN is not executable: $CHROTE_TMUX_BIN"
    tmux_bin="$CHROTE_TMUX_BIN"
  else
    require_command tmux
    tmux_bin="$(command -v tmux)"
  fi
  local entry owner socket uid roots root allowed dir path part socket_owner
  local -a entries=() root_entries=() parts=()
  IFS=',' read -r -a entries <<< "$mappings"
  for entry in "${entries[@]}"; do
    entry="${entry//[[:space:]]/}"
    [ -n "$entry" ] || continue
    owner="${entry%%=*}"
    socket="${entry#*=}"
    [ -n "$owner" ] && [ -n "$socket" ] && [ "$owner" != "$socket" ] || die "invalid tmux socket mapping: $entry"
    id "$owner" >/dev/null 2>&1 || die "tmux socket owner does not exist: $owner"
    case "$socket" in
      /*) ;;
      *) die "tmux socket path must be absolute: $entry" ;;
    esac
    case "$socket" in
      *'/../'*|*'/..'|*'/./'*|*'/.'|*'//'*) die "tmux socket path must be canonical: $entry" ;;
    esac

    uid="$(id -u "$owner")"
    roots="${CHROTE_TMUX_GRANT_SOCKET_ROOTS:-/run/user/%u:/tmp/tmux-%u}"
    IFS=':' read -r -a root_entries <<< "$roots"
    allowed=0
    for root in "${root_entries[@]}"; do
      [ -n "$root" ] || continue
      root="${root//%u/$uid}"
      case "$socket" in "${root%/}"/*) allowed=1 ;; esac
    done
    [ "$allowed" -eq 1 ] || die "tmux socket is outside configured roots: $entry"

    if [ -e "$socket" ]; then
      socket_owner="$(stat -c '%U' "$socket")"
      [ "$socket_owner" = "$owner" ] || die "tmux socket belongs to $socket_owner, not $owner: $socket"
    fi
    dir="$(dirname "$socket")"
    path=""
    IFS='/' read -r -a parts <<< "${dir#/}"
    for part in "${parts[@]}"; do
      [ -n "$part" ] || continue
      path="$path/$part"
      case "$path" in /run|/tmp) continue ;; esac
      [ ! -d "$path" ] || setfacl -m "u:${service_user}:--x" "$path"
    done
    [ ! -d "$dir" ] || setfacl -d -m "u:${service_user}:rwx" "$dir"
    if [ -e "$socket" ]; then
      [ -S "$socket" ] || die "configured tmux path is not a socket: $socket"
      setfacl -m "u:${service_user}:rw" "$socket"
      run_as_tmux_owner "$owner" env TERM="${TERM:-xterm-256color}" "$tmux_bin" -S "$socket" server-access -a "$service_user"
      log "Granted $service_user access to $owner's tmux socket"
    else
      warn "Configured tmux socket is not running yet; rerun install after it starts: $socket"
    fi
  done
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
  local bin_dir config_dir state_dir binary env_file secrets_file unit_file build_tmp

  require_command install
  require_command realpath
  require_command tmux
  # Send to Session uses POSIX ACLs so only the target Unix user can read drops.
  require_command setfacl
  if [ "$MANAGE_SYSTEMD" -eq 1 ]; then
    require_command systemctl
  fi
  validate_port "--port" "$PORT"

  PREFIX="$(absolute_path "$PREFIX")"
  CONFIG_HOME="$(absolute_path "$CONFIG_HOME")"
  STATE_HOME="$(absolute_path "$STATE_HOME")"
  SERVICE_DIR="$(absolute_path "$SERVICE_DIR")"
  WORKSPACE="$(absolute_path "$WORKSPACE")"

  mkdir -p "$WORKSPACE"
  bin_dir="$PREFIX/bin"
  config_dir="$CONFIG_HOME/chrote"
  state_dir="$STATE_HOME/chrote"
  binary="$bin_dir/chrote-server"
  env_file="$config_dir/chrote.env"
  secrets_file="$config_dir/secrets.env"
  unit_file="$SERVICE_DIR/chrote.service"

  install -d -m 0755 "$bin_dir" "$SERVICE_DIR"
  install -d -m 0700 "$config_dir" "$state_dir"
  install -d -m 0700 \
    "$state_dir/session-drops" \
    "$state_dir/scheduled-tasks" \
    "$state_dir/agent-hooks"

  build_tmp="$(mktemp "$bin_dir/.chrote-server.XXXXXX")"
  trap 'rm -f "$build_tmp"' EXIT
  build_server "$build_tmp"
  chmod 0755 "$build_tmp"
  mv -f "$build_tmp" "$binary"
  trap - EXIT
  # The hook a launched agent reports through lives beside the server, which
  # is where the server looks for it.
  install -m 0755 "$SCRIPT_DIR/scripts/chrote-agent-event" "$bin_dir/chrote-agent-event"

  write_environment "$env_file" "$state_dir"
  [ -e "$secrets_file" ] || { : > "$secrets_file"; chmod 0600 "$secrets_file"; }
  write_service "$unit_file" "$binary" "$env_file" "$secrets_file"
  grant_tmux_access

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

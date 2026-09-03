#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
installer="$repo_root/install.sh"
uninstaller="$repo_root/uninstall.sh"
binary="${1:-}"
expected_version="$(tr -d '\r\n' < "$repo_root/VERSION")"
expected_commit="${CHROTE_EXPECTED_BUILD_COMMIT:-}"
tmux_bin="${CHROTE_TEST_TMUX_BIN:-}"
if [ -z "$tmux_bin" ]; then
  tmux_bin="$(command -v tmux || true)"
fi
case "$tmux_bin" in
  /*) ;;
  *) tmux_bin="$(command -v "$tmux_bin" || true)" ;;
esac
[ -x "$tmux_bin" ] || { echo "test tmux executable is not available: $tmux_bin" >&2; exit 1; }
tmux_bin_dir="$(dirname "$tmux_bin")"

# Static preflight deliberately runs before the installer. This keeps the test
# safe against the legacy script, which ignored arguments and wrote into $HOME.
grep -q -- '--binary)' "$installer"
grep -q -- '--no-enable)' "$installer"
grep -q -- '--no-start)' "$installer"
! grep -q 'CHROTE_PERSISTENT_AGENTS_PATH' "$installer"
grep -q 'CHROTE_SCHEDULED_TASKS_DIR' "$installer"
if grep -qi 'ttyd' "$installer"; then
  echo "the installer must not install ttyd: CHROTE serves terminals itself (ADR-0018)" >&2
  exit 1
fi
grep -q -- '--purge-state)' "$uninstaller"
grep -q 'Workspace preserved' "$uninstaller"

if [ -n "$binary" ]; then
  case "$binary" in
    /*) ;;
    *) binary="$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")" ;;
  esac
  [ -x "$binary" ] || { echo "test binary is not executable: $binary" >&2; exit 1; }
fi

# shellcheck source=scripts/lib/server-teardown.sh
. "$repo_root/scripts/lib/server-teardown.sh"

tmp="$(mktemp -d)"
server_pid=""
runtime_dir="$(mktemp -d /tmp/chrote-tmux.XXXXXX)"
tmux_socket="$runtime_dir/tmux-$(id -u)/default"
tmux_timeout="${CHROTE_TEST_TMUX_TIMEOUT:-5s}"
port_receipt="$tmp/ports"
port_reserver_pid=""
if ! timeout --version 2>&1 | grep -q 'GNU coreutils'; then
  echo 'GNU timeout is required for bounded private tmux cleanup' >&2
  exit 1
fi
require_server_teardown_tools
tmux_cmd() {
  local tmux_capture status
  tmux_capture="$(mktemp "$tmp/tmux-command.XXXXXX")"
  if TMUX_TMPDIR="$runtime_dir" timeout --kill-after=1s "$tmux_timeout" \
    "$tmux_bin" -S "$tmux_socket" "$@" >"$tmux_capture" 2>&1; then
    status=0
  else
    status=$?
  fi
  cat "$tmux_capture"
  rm -f "$tmux_capture"
  return "$status"
}
release_port_reserver() {
  local status=0 pid="$port_reserver_pid" wait_status
  if [ -n "$pid" ]; then
    if ! kill -TERM "$pid" 2>/dev/null; then
      printf 'port reserver PID %s exited before TERM release\n' "$pid" >&2
      status=1
    fi
    if wait "$pid"; then
      if [ "$status" -ne 0 ]; then
        printf 'port reserver PID %s did not accept TERM release\n' "$pid" >&2
      fi
    else
      wait_status=$?
      printf 'port reserver PID %s exited unexpectedly with status %s\n' "$pid" "$wait_status" >&2
      status=1
    fi
    port_reserver_pid=""
  fi
  return "$status"
}
tmux_no_server_output() {
  case "$1" in
    *'no server running'*|*'No such file or directory'*|*'server exited unexpectedly'*) return 0 ;;
    *) return 1 ;;
  esac
}
tmux_session_absent_output() {
  case "$1" in
    *"can't find session: public-smoke"*) return 0 ;;
    *) return 1 ;;
  esac
}
cleanup_tmux() {
  local probe_output="" probe_status=0 kill_output="" sessions="" list_status=0
  if probe_output="$(tmux_cmd has-session -t =public-smoke 2>&1)"; then
    if [ -n "$probe_output" ]; then
      printf 'test tmux has-session returned unexpected output: %s\n' "$probe_output" >&2
      return 1
    fi
    if ! kill_output="$(tmux_cmd kill-session -t =public-smoke 2>&1)"; then
      printf 'test tmux failed to kill exact public-smoke session: %s\n' "$kill_output" >&2
      return 1
    fi
    if [ -n "$kill_output" ]; then
      printf 'test tmux kill-session returned unexpected output: %s\n' "$kill_output" >&2
      return 1
    fi
    for _ in $(seq 1 50); do
      if probe_output="$(tmux_cmd has-session -t =public-smoke 2>&1)"; then
        printf 'test tmux exact public-smoke session survived cleanup: %s\n' "$probe_output" >&2
        return 1
      fi
      probe_status=$?
      if tmux_session_absent_output "$probe_output" || tmux_no_server_output "$probe_output"; then
        break
      fi
      printf 'test tmux exact-session verification failed (status %s): %s\n' \
        "$probe_status" "$probe_output" >&2
      return 1
    done
  else
    probe_status=$?
    if tmux_session_absent_output "$probe_output" || tmux_no_server_output "$probe_output"; then
      :
    else
      printf 'test tmux exact-session probe failed (status %s): %s\n' \
        "$probe_status" "$probe_output" >&2
      return 1
    fi
  fi
  for _ in $(seq 1 50); do
    if sessions="$(tmux_cmd list-sessions -F '#{session_name}' 2>&1)"; then
      if [ -z "$sessions" ]; then
        return 0
      fi
    else
      list_status=$?
      if tmux_no_server_output "$sessions"; then
        return 0
      fi
      printf 'test tmux session listing failed (status %s): %s\n' \
        "$list_status" "$sessions" >&2
      return 1
    fi
    sleep 0.1
  done
  printf 'test tmux cleanup found surviving private sessions: %s\n' "$sessions" >&2
  return 1
}
cleanup() {
  local status=$?
  status="${1:-$status}"
  trap - EXIT INT TERM HUP
  if ! release_port_reserver; then
    status=1
  fi
  stop_server
  if ! cleanup_tmux; then
    status=1
    printf 'test tmux runtime retained for diagnosis: %s\n' "$runtime_dir" >&2
  else
    rm -rf "$runtime_dir"
  fi
  if ! assert_server_released "^${installed_binary:-}" "${port:-}"; then
    status=1
  fi
  # Anything that ran under this fake HOME may have left a Go module cache in
  # it, and Go marks those directories read-only. rm then fails, and under
  # `set -e` the smoke exits non-zero after having already printed PASS --
  # a green gate reported as red. Take write permission back first.
  chmod -R u+w "$tmp" 2>/dev/null || true
  rm -rf "$tmp"
  exit "$status"
}
# Every exit path runs the teardown, not only a clean return: an interrupted run
# is the one most likely to leave a server behind.
trap 'cleanup' EXIT
trap 'cleanup 130' INT
trap 'cleanup 143' TERM
trap 'cleanup 129' HUP

home="$tmp/home"
prefix="$tmp/prefix"
config_home="$tmp/config"
state_home="$tmp/state"
service_dir="$tmp/systemd-user"
workspace="$tmp/workspace with spaces%25"
mkdir -p "$home" "$workspace" "$runtime_dir"
mkdir -p "$(dirname "$tmux_socket")"

python3 - "$port_receipt" <<'PY' &
import os
import signal
import socket
import sys

sockets = []

def close_sockets():
    while sockets:
        sockets.pop().close()

def stop(_signum, _frame):
    close_sockets()
    raise SystemExit(0)

signal.signal(signal.SIGTERM, stop)
try:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(('127.0.0.1', 0))
    sock.listen(1)
    sockets.append(sock)
    with open(sys.argv[1], 'x', encoding='ascii') as receipt:
        receipt.write(f'{sock.getsockname()[1]}\n')
        receipt.flush()
        os.fsync(receipt.fileno())
    signal.pause()
except SystemExit:
    raise
except Exception as exc:
    print(f'failed to reserve a loopback port: {exc}', file=sys.stderr)
    raise SystemExit(1)
finally:
    close_sockets()
PY
port_reserver_pid=$!
for _ in $(seq 1 50); do
  if [ -s "$port_receipt" ]; then
    break
  fi
  if ! kill -0 "$port_reserver_pid" 2>/dev/null; then
    reserver_status=0
    if wait "$port_reserver_pid"; then
      reserver_status=0
    else
      reserver_status=$?
    fi
    printf 'port reserver PID %s exited before writing its receipt with status %s\n' \
      "$port_reserver_pid" "$reserver_status" >&2
    port_reserver_pid=""
    exit 1
  fi
  sleep 0.1
done
if [ ! -s "$port_receipt" ]; then
  echo 'port reserver did not write its receipt' >&2
  exit 1
fi
if ! read -r port < "$port_receipt"; then
  echo 'failed to read the reserved loopback port' >&2
  exit 1
fi

install_args=(
  --workspace "$workspace"
  --port "$port"
  --no-systemd
  --no-enable
  --no-start
)
if [ -n "$binary" ]; then
  install_args=(--binary "$binary" "${install_args[@]}")
fi

HOME="$home" \
XDG_CONFIG_HOME="$config_home" \
XDG_STATE_HOME="$state_home" \
CHROTE_INSTALL_PREFIX="$prefix" \
CHROTE_SERVICE_DIR="$service_dir" \
TMUX_TMPDIR="$runtime_dir" \
  "$installer" "${install_args[@]}"

installed_binary="$prefix/bin/chrote-server"
env_file="$config_home/chrote/chrote.env"
unit_file="$service_dir/chrote.service"

for path in "$installed_binary" "$env_file" "$unit_file"; do
  [ -f "$path" ] || { echo "missing installed path: $path" >&2; exit 1; }
done
[ -x "$installed_binary" ]
# The terminal is served by chrote-server itself, so nothing else is installed
# alongside it (ADR-0018).
[ ! -e "$prefix/bin/ttyd" ]
[ ! -e "$prefix/lib/chrote" ]

grep -F 'CHROTE_ROOTS=' "$env_file" | grep -Fq "$workspace"
grep -F 'CHROTE_TMUX_SOCKET=' "$env_file" | grep -Fq "$(id -un)=$tmux_socket"
grep -F 'CHROTE_SCHEDULED_TASKS_DIR=' "$env_file" | grep -Fq "$state_home/chrote/scheduled-tasks"
grep -Fq "ExecStart=$installed_binary" "$unit_file"
grep -Fq 'KillMode=process' "$unit_file"
! grep -Fq 'Environment=TMUX_TMPDIR=' "$unit_file"

systemd-analyze verify "$unit_file" >/dev/null

set -a
# The generated environment file is intentionally shell-compatible as well as
# systemd EnvironmentFile-compatible.
# shellcheck disable=SC1090
. "$env_file"
set +a
export HOME
export TMUX_TMPDIR="$runtime_dir"
export CHROTE_TMUX_BIN="$tmux_bin"
export PATH="$prefix/bin:$tmux_bin_dir:/usr/local/bin:/usr/bin:/bin"

tmux_cmd new-session -d -s public-smoke -c "$workspace"
release_port_reserver
"$installed_binary" >"$tmp/server.log" 2>&1 &
server_pid=$!

healthy=0
for _ in $(seq 1 80); do
  if curl -fsS "http://127.0.0.1:$port/api/health" >"$tmp/health.json" 2>/dev/null; then
    healthy=1
    break
  fi
  sleep 0.1
done
if [ "$healthy" -ne 1 ]; then
  cat "$tmp/server.log" >&2
  exit 1
fi

python3 - "$tmp/health.json" "$expected_version" "$expected_commit" <<'PY'
import json,sys
payload=json.load(open(sys.argv[1]))
assert payload['status']=='ok', payload
assert payload['version']==sys.argv[2], payload
if sys.argv[3]:
    assert payload['commit']==sys.argv[3], payload
PY

curl -fsS "http://127.0.0.1:$port/api/tmux/sessions" >"$tmp/sessions.json"
python3 - "$tmp/sessions.json" <<'PY'
import json,sys
payload=json.load(open(sys.argv[1]))
names={item['name'] for item in payload.get('sessions',[])}
assert 'public-smoke' in names, payload
PY

# The terminal is a WebSocket CHROTE serves itself: nothing under /terminal
# answers a plain HTTP request, and the upgrade route is still wired.
terminal_page_status="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/terminal/")"
if [ "$terminal_page_status" != "404" ]; then
  echo "plain HTTP under /terminal/ returned $terminal_page_status, expected 404" >&2
  exit 1
fi
terminal_ws_status="$(curl -s -o /dev/null -w '%{http_code}' \
  -H 'Connection: Upgrade' -H 'Upgrade: websocket' \
  "http://127.0.0.1:$port/terminal/ws?arg=tile&arg=public-smoke")"
if [ "$terminal_ws_status" = "404" ]; then
  echo "the terminal WebSocket route is not served" >&2
  exit 1
fi

stop_server

HOME="$home" \
XDG_CONFIG_HOME="$config_home" \
XDG_STATE_HOME="$state_home" \
CHROTE_INSTALL_PREFIX="$prefix" \
CHROTE_SERVICE_DIR="$service_dir" \
  "$uninstaller" --yes --no-systemd

for path in "$installed_binary" "$env_file" "$unit_file"; do
  [ ! -e "$path" ] || { echo "uninstaller left managed path: $path" >&2; exit 1; }
done
[ -d "$workspace" ]
[ -f "$config_home/chrote/secrets.env" ]

HOME="$home" \
XDG_CONFIG_HOME="$config_home" \
XDG_STATE_HOME="$state_home" \
CHROTE_INSTALL_PREFIX="$prefix" \
CHROTE_SERVICE_DIR="$service_dir" \
  "$uninstaller" --yes --no-systemd --purge-state --purge-private-config

[ ! -e "$state_home/chrote" ]
[ ! -e "$config_home/chrote/secrets.env" ]
[ -d "$workspace" ]

printf 'PASS: disposable public installer smoke (health/version, tmux, terminal route, conservative uninstall, explicit purge)\n'

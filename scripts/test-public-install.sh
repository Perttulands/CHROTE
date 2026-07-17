#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
installer="$repo_root/install.sh"
uninstaller="$repo_root/uninstall.sh"
binary="${1:-}"
expected_version="$(tr -d '\r\n' < "$repo_root/VERSION")"

# Static preflight deliberately runs before the installer. This keeps the test
# safe against the legacy script, which ignored arguments and wrote into $HOME.
grep -q -- '--binary)' "$installer"
grep -q -- '--no-enable)' "$installer"
grep -q -- '--no-start)' "$installer"
grep -q 'CHROTE_SESSION_BANK_PATH' "$installer"
grep -q 'CHROTE_PERSISTENT_AGENTS_PATH' "$installer"
grep -q 'CHROTE_SCHEDULED_TASKS_DIR' "$installer"
if grep -q 'chrote-ttyd.service' "$installer" "$uninstaller"; then
  echo "installer must use the Go server's managed ttyd, not a second service" >&2
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

tmp="$(mktemp -d)"
server_pid=""
runtime_dir="$tmp/runtime/chrote-tmux"
cleanup() {
  if [ -n "$server_pid" ] && kill -0 "$server_pid" 2>/dev/null; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  TMUX_TMPDIR="$runtime_dir" tmux kill-server 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

home="$tmp/home"
prefix="$tmp/prefix"
config_home="$tmp/config"
state_home="$tmp/state"
service_dir="$tmp/systemd-user"
workspace="$tmp/workspace with spaces%25"
mkdir -p "$home" "$workspace" "$runtime_dir"

read -r port ttyd_port < <(python3 - <<'PY'
import socket
ports=[]
for _ in range(2):
    sock=socket.socket()
    sock.bind(('127.0.0.1',0))
    ports.append(sock.getsockname()[1])
    sock.close()
print(*ports)
PY
)

install_args=(
  --workspace "$workspace"
  --port "$port"
  --ttyd-port "$ttyd_port"
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
  "$installer" "${install_args[@]}"

installed_binary="$prefix/bin/chrote-server"
launch_script="$prefix/lib/chrote/terminal-launch.sh"
env_file="$config_home/chrote/chrote.env"
unit_file="$service_dir/chrote.service"

for path in "$installed_binary" "$prefix/bin/ttyd" "$launch_script" "$env_file" "$unit_file"; do
  [ -f "$path" ] || { echo "missing installed path: $path" >&2; exit 1; }
done
[ -x "$installed_binary" ]
[ -x "$prefix/bin/ttyd" ]
[ -x "$launch_script" ]
[ ! -e "$service_dir/chrote-ttyd.service" ]

grep -F 'CHROTE_ROOTS=' "$env_file" | grep -Fq "$workspace"
grep -F 'CHROTE_SESSION_BANK_PATH=' "$env_file" | grep -Fq "$state_home/chrote/session-bank/sessions.json"
grep -F 'CHROTE_PERSISTENT_AGENTS_PATH=' "$env_file" | grep -Fq "$state_home/chrote/persistent-agents/agents.json"
grep -F 'CHROTE_SCHEDULED_TASKS_DIR=' "$env_file" | grep -Fq "$state_home/chrote/scheduled-tasks"
grep -Fq "ExecStart=$installed_binary" "$unit_file"
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
export PATH="$prefix/bin:/usr/local/bin:/usr/bin:/bin"

TMUX_TMPDIR="$runtime_dir" tmux new-session -d -s public-smoke -c "$workspace"
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

python3 - "$tmp/health.json" "$expected_version" <<'PY'
import json,sys
payload=json.load(open(sys.argv[1]))
assert payload['status']=='ok', payload
assert payload['version']==sys.argv[2], payload
PY

curl -fsS "http://127.0.0.1:$port/api/tmux/sessions" >"$tmp/sessions.json"
python3 - "$tmp/sessions.json" <<'PY'
import json,sys
payload=json.load(open(sys.argv[1]))
names={item['name'] for item in payload.get('sessions',[])}
assert 'public-smoke' in names, payload
PY

curl -fsS "http://127.0.0.1:$port/terminal/" >"$tmp/terminal.html"
grep -qi 'ttyd' "$tmp/terminal.html"

kill "$server_pid"
wait "$server_pid" 2>/dev/null || true
server_pid=""
TMUX_TMPDIR="$runtime_dir" tmux kill-server 2>/dev/null || true

HOME="$home" \
XDG_CONFIG_HOME="$config_home" \
XDG_STATE_HOME="$state_home" \
CHROTE_INSTALL_PREFIX="$prefix" \
CHROTE_SERVICE_DIR="$service_dir" \
  "$uninstaller" --yes --no-systemd

for path in "$installed_binary" "$prefix/bin/ttyd" "$launch_script" "$env_file" "$unit_file"; do
  [ ! -e "$path" ] || { echo "uninstaller left managed path: $path" >&2; exit 1; }
done
[ -d "$workspace" ]
[ -d "$state_home/chrote/session-bank" ]
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

printf 'PASS: disposable public installer smoke (health/version, tmux, ttyd, conservative uninstall, explicit purge)\n'

#!/usr/bin/env bash
set -euo pipefail

binary="${1:-}"
[ -x "$binary" ] || { echo 'usage: test-systemd-restart-preserves-tmux.sh /absolute/path/to/chrote-server' >&2; exit 2; }
case "$binary" in
  /*) ;;
  *) echo 'chrote-server path must be absolute' >&2; exit 2 ;;
esac

command -v systemd-run >/dev/null
command -v tmux >/dev/null
command -v curl >/dev/null
systemctl --user show-environment >/dev/null

tmp="$(mktemp -d /tmp/chrote-restart-test.XXXXXX)"
runtime_dir="$(mktemp -d /tmp/chrote-restart-tmux.XXXXXX)"
socket="$runtime_dir/tmux-$(id -u)/default"
session='chrote-restart-smoke'
unit="chrote-restart-smoke-$$"
user_name="$(id -un)"
mkdir -p "$tmp/workspace" "$(dirname "$socket")"

port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
PY
)"

tmux_cmd() {
  TMUX_TMPDIR="$runtime_dir" tmux -S "$socket" "$@"
}

cleanup() {
  status=$?
  curl -fsS -X DELETE "http://127.0.0.1:$port/api/tmux/sessions/$session" >/dev/null 2>&1 || true
  if tmux_cmd has-session -t "=$session" >/dev/null 2>&1; then
    tmux_cmd kill-session -t "=$session" >/dev/null
  fi
  systemctl --user stop "$unit.service" >/dev/null 2>&1 || true
  systemctl --user reset-failed "$unit.service" >/dev/null 2>&1 || true
  rm -rf "$tmp" "$runtime_dir"
  exit "$status"
}
trap cleanup EXIT

systemd-run --user --unit "$unit" \
  --property Type=simple \
  --property KillMode=process \
  --setenv "HOME=$tmp" \
  --setenv 'HOST=127.0.0.1' \
  --setenv "PORT=$port" \
  --setenv "CHROTE_ROOTS=$tmp/workspace" \
  --setenv "CHROTE_TMUX_SOCKET=$user_name=$socket" \
  --setenv "TMUX_TMPDIR=$runtime_dir" \
  "$binary" -start-system-history=false >/dev/null

for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$port/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
curl -fsS "http://127.0.0.1:$port/api/health" >/dev/null

curl -fsS -H 'Content-Type: application/json' \
  -d "{\"name\":\"$session\"}" \
  "http://127.0.0.1:$port/api/tmux/sessions" >/dev/null
tmux_cmd has-session -t "=$session"
tmux_pid_before="$(tmux_cmd display-message -p '#{pid}')"

systemctl --user restart "$unit.service"
for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$port/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
curl -fsS "http://127.0.0.1:$port/api/health" >/dev/null

tmux_cmd has-session -t "=$session"
tmux_pid_after="$(tmux_cmd display-message -p '#{pid}')"
[ "$tmux_pid_before" = "$tmux_pid_after" ] || {
  printf 'tmux server changed across CHROTE restart: before=%s after=%s\n' "$tmux_pid_before" "$tmux_pid_after" >&2
  exit 1
}

curl -fsS -X DELETE "http://127.0.0.1:$port/api/tmux/sessions/$session" >/dev/null
if tmux_cmd has-session -t "=$session" >/dev/null 2>&1; then
  echo 'exact test-owned tmux session survived cleanup' >&2
  exit 1
fi

printf 'PASS: CHROTE restart preserved its private tmux server and test-owned session\n'

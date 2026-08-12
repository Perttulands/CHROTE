#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
server_binary="${CHROTE_SERVER_BINARY:-$repo_root/chrote-server-ci}"
fixture="$repo_root/dashboard/tests/contract/fixtures/ci-contract.formation.toml"

if [ ! -x "$server_binary" ]; then
  echo "Built CHROTE server is not executable: $server_binary" >&2
  exit 1
fi
if [ ! -f "$fixture" ]; then
  echo "Formations contract fixture is missing: $fixture" >&2
  exit 1
fi

if [ -n "${CHROTE_CONTRACT_ARTIFACT_DIR:-}" ]; then
  artifact_root="$CHROTE_CONTRACT_ARTIFACT_DIR"
  if [ -e "$artifact_root" ]; then
    echo "Contract artifact directory already exists: $artifact_root" >&2
    exit 1
  fi
  mkdir -p "$artifact_root"
else
  artifact_root="$(mktemp -d "${TMPDIR:-/tmp}/chrote-tmux.XXXXXX")"
fi

workspace="$artifact_root/workspace"
contract_session="chrote-owc-contract"
sentinel_marker="CHROTE_OWC_TTYD_SENTINEL"
mkdir -p \
  "$workspace/.formations/boards" \
  "$workspace/contract-files-terminal1" \
  "$workspace/contract-files-terminal2" \
  "$artifact_root/agents" \
  "$artifact_root/formations-data" \
  "$artifact_root/formations-tmux" \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/scheduled-tasks" \
  "$artifact_root/session-drops" \
  "$artifact_root/state" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
chmod 700 \
  "$artifact_root/formations-tmux" \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/session-drops" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
cp "$fixture" "$workspace/.formations/boards/ci-contract.formation.toml"

sentinel_port_file="$artifact_root/terminal-sentinel-port"
server_log="$artifact_root/server.log"
sentinel_log="$artifact_root/terminal-sentinel.log"
tmux_socket="$artifact_root/tmux/default"
sentinel_pid=""
server_pid=""

tmux_probe_output=""
tmux_probe_session() {
  local output status
  if output="$(env -u TMUX -u TMUX_PANE \
    TMUX_TMPDIR="$artifact_root/tmux" \
    CHROTE_TMUX_GUARD_LOG="$artifact_root/tmux-guard.log" \
    tmux -S "$tmux_socket" has-session -t "=$contract_session" 2>&1)"; then
    status=0
  else
    status=$?
  fi
  tmux_probe_output="$output"
  if [ "$status" -eq 0 ]; then
    return 0
  fi
  if [ "$status" -eq 1 ] && {
    [ "$output" = "no server running on $tmux_socket" ] ||
    [ "$output" = "can't find session: $contract_session" ]
  }; then
    return 1
  fi
  printf 'Unexpected private tmux probe result (status=%s): %s\n' "$status" "$output" >&2
  return 2
}

tmux_kill_exact_session() {
	local output status
	if output="$(env -u TMUX -u TMUX_PANE \
	  TMUX_TMPDIR="$artifact_root/tmux" \
	  CHROTE_TMUX_GUARD_LOG="$artifact_root/tmux-guard.log" \
	  tmux -S "$tmux_socket" kill-session -t "=$contract_session" 2>&1)"; then
		return 0
	else
		status=$?
	fi
	printf 'Failed to clean exact private tmux session (status=%s): %s\n' "$status" "$output" >&2
	return 1
}

cleanup() {
  local exit_status=$?
  local probe_status
  trap - EXIT INT TERM
  if tmux_probe_session; then
    if tmux_kill_exact_session; then
      if tmux_probe_session; then
        echo "Exact test-owned tmux session remains after cleanup: $contract_session" >&2
        exit_status=1
      else
        probe_status=$?
        if [ "$probe_status" -ne 1 ]; then
          echo "Could not verify exact test-owned tmux session cleanup: $tmux_probe_output" >&2
          exit_status=1
        fi
      fi
    else
      exit_status=1
    fi
  else
    probe_status=$?
    if [ "$probe_status" -ne 1 ]; then
      echo "Could not inspect exact test-owned tmux session: $tmux_probe_output" >&2
      exit_status=1
    fi
  fi
  if [ -n "$sentinel_pid" ]; then
    kill "$sentinel_pid" 2>/dev/null || true
    wait "$sentinel_pid" 2>/dev/null || true
  fi
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  exit "$exit_status"
}
trap cleanup EXIT INT TERM

python3 -c '
import http.server
import os
import sys

marker = sys.argv[1]
receipt_path = sys.argv[2]

class Sentinel(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = f"<!doctype html><title>{marker}</title><body>{marker}</body>".encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass

server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Sentinel)
with open(receipt_path, "x", encoding="ascii") as receipt:
    receipt.write(str(server.server_address[1]))
    receipt.flush()
    os.fsync(receipt.fileno())
server.serve_forever()
' "$sentinel_marker" "$sentinel_port_file" >"$sentinel_log" 2>&1 &
sentinel_pid=$!

sentinel_port=""
sentinel_ready=false
for _ in $(seq 1 100); do
  if [ -s "$sentinel_port_file" ]; then
    candidate_port="$(tr -d '[:space:]' <"$sentinel_port_file")"
    if [[ "$candidate_port" =~ ^[0-9]+$ ]] && [ "$candidate_port" -ge 1 ] && [ "$candidate_port" -le 65535 ]; then
      sentinel_port="$candidate_port"
      sentinel_ready=true
      break
    fi
  fi
  if ! kill -0 "$sentinel_pid" 2>/dev/null; then
    break
  fi
  sleep 0.1
done

if [ "$sentinel_ready" != true ]; then
  echo "Terminal sentinel did not report its bound port; log follows:" >&2
  tail -n 100 "$sentinel_log" >&2 || true
  exit 1
fi

sentinel_http_ready=false
for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$sentinel_port/marker" | grep -Fq "$sentinel_marker"; then
    sentinel_http_ready=true
    break
  fi
  if ! kill -0 "$sentinel_pid" 2>/dev/null; then
    break
  fi
  sleep 0.1
done

if [ "$sentinel_http_ready" != true ]; then
  echo "Terminal sentinel did not serve its marker; log follows:" >&2
  tail -n 100 "$sentinel_log" >&2 || true
  exit 1
fi

port="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"

env -i \
PATH="$PATH" \
HOME="$artifact_root/home" \
LANG=C.UTF-8 \
TMPDIR="$artifact_root/tmp" \
XDG_RUNTIME_DIR="$artifact_root/runtime" \
TMUX_TMPDIR="$artifact_root/tmux" \
CHROTE_DEFAULT_TMUX_SOCKET="$artifact_root/tmux/default" \
CHROTE_WORKDIR="$workspace" \
CHROTE_ROOTS="$workspace" \
CHROTE_WRITE_ROOTS="$workspace" \
CHROTE_AGENTS_DIR="$artifact_root/agents" \
CHROTE_BEADS_WORKSPACES="$workspace" \
CHROTE_BEADS_AUTO_DISCOVER=false \
CHROTE_SCHEDULED_TASKS_DIR="$artifact_root/scheduled-tasks" \
CHROTE_SESSION_DROPS_DIR="$artifact_root/session-drops" \
CHROTE_SESSION_BANK_PATH="$artifact_root/state/session-bank.json" \
CHROTE_MANAGED_RECOVERY_STATUS_PATH="$artifact_root/state/managed-recovery.json" \
CHROTE_FORMATIONS_DATA_ROOT="$artifact_root/formations-data" \
CHROTE_FORMATIONS_LAB_HARNESSES= \
CHROTE_FORMATIONS_LAB_CWD="$workspace" \
CHROTE_FORMATIONS_LAB_ROOTS="$workspace" \
CHROTE_FORMATIONS_TMUX_HARNESSES= \
CHROTE_FORMATIONS_TMUX_SOCKET="$artifact_root/formations-tmux/default" \
CHROTE_FORMATIONS_TMUX_CWD="$workspace" \
CHROTE_FORMATIONS_TMUX_ROOTS="$workspace" \
CHROTE_FORMATIONS_TMUX_SESSION_PREFIX=contract- \
CHROTE_FORMATIONS_TMUX_DEDICATED= \
CHROTE_FORMATIONS_TMUX_PROD_SMOKE= \
TTYD_PORT="$sentinel_port" \
  "$server_binary" -host 127.0.0.1 -port "$port" -start-ttyd=false -start-system-history=false >"$server_log" 2>&1 &
server_pid=$!

ready=false
for _ in $(seq 1 100); do
  if curl -fsS "http://127.0.0.1:$port/api/health" >/dev/null; then
    ready=true
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    break
  fi
  sleep 0.1
done

if [ "$ready" != true ]; then
  echo "Built CHROTE server did not become ready; log follows:" >&2
  tail -n 100 "$server_log" >&2 || true
  exit 1
fi

env -u TMUX -u TMUX_PANE \
  TMUX_TMPDIR="$artifact_root/tmux" \
  CHROTE_TMUX_GUARD_LOG="$artifact_root/tmux-guard.log" \
  tmux -S "$tmux_socket" \
  new-session -d -s "$contract_session" -c "$workspace" sleep 600

echo "Contract artifacts: $artifact_root"
cd "$repo_root/dashboard"
CHROTE_CONTRACT_WORKSPACE="$workspace" \
CHROTE_TEST_URL="http://127.0.0.1:$port" \
  npm run test:server-contract

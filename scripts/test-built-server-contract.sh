#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
server_binary="${CHROTE_SERVER_BINARY:-$repo_root/chrote-server-ci}"

# shellcheck source=scripts/lib/server-teardown.sh
. "$repo_root/scripts/lib/server-teardown.sh"
require_server_teardown_tools

if [ ! -x "$server_binary" ]; then
  echo "Built CHROTE server is not executable: $server_binary" >&2
  exit 1
fi
# Only a directory this run created is this run's to remove. An explicit
# CHROTE_CONTRACT_ARTIFACT_DIR belongs to the caller that named it: CI uploads
# that directory after a failure.
artifact_root_owned=false
if [ -n "${CHROTE_CONTRACT_ARTIFACT_DIR:-}" ]; then
  artifact_root="$CHROTE_CONTRACT_ARTIFACT_DIR"
  if [ -e "$artifact_root" ]; then
    echo "Contract artifact directory already exists: $artifact_root" >&2
    exit 1
  fi
  mkdir -p "$artifact_root"
else
  artifact_root="$(mktemp -d "${TMPDIR:-/tmp}/chrote-tmux.XXXXXX")"
  artifact_root_owned=true
fi

workspace="$artifact_root/workspace"
contract_session="chrote-owc-contract"
mkdir -p \
  "$workspace/contract-files-terminal1" \
  "$workspace/contract-files-terminal2" \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/scheduled-tasks" \
  "$artifact_root/session-drops" \
  "$artifact_root/state" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
chmod 700 \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/session-drops" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
server_log="$artifact_root/server.log"
tmux_socket="$artifact_root/tmux/default"
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
  local probe_status survivor_pattern
  exit_status="${1:-$exit_status}"
  trap - EXIT INT TERM HUP
  stop_server
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
  # The survivor scan is scoped to this run's port, not to the binary path
  # alone: the same built binary is what a concurrent run of this script would
  # start too, and its server is not this run's leak.
  survivor_pattern=""
  if [ -n "${port:-}" ]; then
    survivor_pattern="^$server_binary .*-port $port( |$)"
  fi
  if ! assert_server_released "$survivor_pattern" "${port:-}"; then
    exit_status=1
  fi
  # A passing run owns its leftovers, including the dead tmux socket file the
  # private server leaves behind, and takes them with it. A failing run keeps
  # them: the server log and the guard log are where a failure is diagnosed.
  if [ "$exit_status" -eq 0 ] && [ "$artifact_root_owned" = true ]; then
    rm -rf "$artifact_root"
  else
    printf 'Contract artifacts retained: %s\n' "$artifact_root" >&2
  fi
  exit "$exit_status"
}
# Every exit path runs the teardown, not only a clean return: an interrupted run
# is the one most likely to leave a server behind, and the longest command here
# is the contract run at the end.
trap 'cleanup' EXIT
trap 'cleanup 130' INT
trap 'cleanup 143' TERM
trap 'cleanup 129' HUP

port="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"

env -i \
PATH="$PATH" \
HOME="$artifact_root/home" \
LANG=C.UTF-8 \
TMPDIR="$artifact_root/tmp" \
XDG_RUNTIME_DIR="$artifact_root/runtime" \
TMUX_TMPDIR="$artifact_root/tmux" \
CHROTE_TMUX_SOCKET="$(id -un)=$artifact_root/tmux/default" \
CHROTE_WORKDIR="$workspace" \
CHROTE_ROOTS="$workspace" \
CHROTE_BEADS_WORKSPACES="$workspace" \
CHROTE_BEADS_AUTO_DISCOVER=false \
CHROTE_SCHEDULED_TASKS_DIR="$artifact_root/scheduled-tasks" \
CHROTE_SESSION_DROPS_DIR="$artifact_root/session-drops" \
  "$server_binary" -host 127.0.0.1 -port "$port" -start-system-history=false >"$server_log" 2>&1 &
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

echo "Contract artifacts (kept only if this run fails): $artifact_root"
cd "$repo_root/dashboard"
CHROTE_CONTRACT_WORKSPACE="$workspace" \
CHROTE_TEST_URL="http://127.0.0.1:$port" \
  npm run test:server-contract

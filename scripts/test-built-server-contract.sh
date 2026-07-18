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
  artifact_root="$(mktemp -d "${TMPDIR:-/tmp}/chrote-server-contract.XXXXXX")"
fi

workspace="$artifact_root/workspace"
mkdir -p \
  "$workspace/.formations/boards" \
  "$artifact_root/agents" \
  "$artifact_root/formations-data" \
  "$artifact_root/formations-tmux" \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/scheduled-tasks" \
  "$artifact_root/state" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
chmod 700 \
  "$artifact_root/formations-tmux" \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
cp "$fixture" "$workspace/.formations/boards/ci-contract.formation.toml"

port="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
server_log="$artifact_root/server.log"

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
CHROTE_PERSISTENT_AGENTS_DISABLE=true \
CHROTE_PERSISTENT_AGENTS_PATH="$artifact_root/state/persistent-agents.json" \
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
CHROTE_FORMATIONS_SCRIPT_GATES= \
  "$server_binary" -host 127.0.0.1 -port "$port" -start-ttyd=false >"$server_log" 2>&1 &
server_pid=$!

cleanup() {
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

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

echo "Contract artifacts: $artifact_root"
cd "$repo_root/dashboard"
CHROTE_TEST_URL="http://127.0.0.1:$port" npm run test:server-contract

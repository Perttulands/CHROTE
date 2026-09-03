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
  "$artifact_root/tmp" \
  "$artifact_root/bin" \
  "$artifact_root/agent-hooks"
# The root too, not just what is under it: mktemp -d makes 0700 and an explicit
# CHROTE_CONTRACT_ARTIFACT_DIR took whatever the caller's umask gave it, so the
# same run cleaned up after itself or could not depending on how the shell that
# started it was configured.
chmod 700 \
  "$artifact_root" \
  "$artifact_root/home" \
  "$artifact_root/runtime" \
  "$artifact_root/session-drops" \
  "$artifact_root/tmux" \
  "$artifact_root/tmp"
server_log="$artifact_root/server.log"
tmux_socket="$artifact_root/tmux/default"
server_pid=""

hook_session="chrote-owc-hook"
tmux_probe_output=""
tmux_probe_session() {
  local session="$1" output status
  if output="$(env -u TMUX -u TMUX_PANE \
    TMUX_TMPDIR="$artifact_root/tmux" \
    CHROTE_TMUX_GUARD_LOG="$artifact_root/tmux-guard.log" \
    tmux -S "$tmux_socket" has-session -t "=$session" 2>&1)"; then
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
    [ "$output" = "can't find session: $session" ]
  }; then
    return 1
  fi
  printf 'Unexpected private tmux probe result (status=%s): %s\n' "$status" "$output" >&2
  return 2
}

tmux_kill_exact_session() {
	local session="$1" output status
	if output="$(env -u TMUX -u TMUX_PANE \
	  TMUX_TMPDIR="$artifact_root/tmux" \
	  CHROTE_TMUX_GUARD_LOG="$artifact_root/tmux-guard.log" \
	  tmux -S "$tmux_socket" kill-session -t "=$session" 2>&1)"; then
		return 0
	else
		status=$?
	fi
	printf 'Failed to clean exact private tmux session (status=%s): %s\n' "$status" "$output" >&2
	return 1
}

# cleanup_session removes exactly one test-owned session and proves it gone.
cleanup_session() {
  local session="$1" probe_status
  if tmux_probe_session "$session"; then
    if tmux_kill_exact_session "$session"; then
      if tmux_probe_session "$session"; then
        echo "Exact test-owned tmux session remains after cleanup: $session" >&2
        return 1
      else
        probe_status=$?
        if [ "$probe_status" -ne 1 ]; then
          echo "Could not verify exact test-owned tmux session cleanup: $tmux_probe_output" >&2
          return 1
        fi
      fi
    else
      return 1
    fi
  else
    probe_status=$?
    if [ "$probe_status" -ne 1 ]; then
      echo "Could not inspect exact test-owned tmux session: $tmux_probe_output" >&2
      return 1
    fi
  fi
  return 0
}

cleanup() {
  local exit_status=$?
  local survivor_pattern session
  exit_status="${1:-$exit_status}"
  trap - EXIT INT TERM HUP
  stop_server
  for session in "$contract_session" "$hook_session"; do
    if ! cleanup_session "$session"; then
      exit_status=1
    fi
  done
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

# A stand-in for Claude Code, launched by the server through its launch
# configuration exactly as the real one would be: it reads the settings file
# the launcher handed it, runs the Stop hook the way Claude Code does (the
# payload on stdin), and then stays resident so the session keeps a process.
fake_claude="$artifact_root/bin/claude"
cat >"$fake_claude" <<'FAKE'
#!/bin/sh
settings=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --settings) settings="$2"; shift ;;
  esac
  shift
done
hook="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["hooks"]["Stop"][0]["hooks"][0]["command"])' "$settings")"
printf '{"hook_event_name":"Stop"}' | sh -c "$hook"
exec sleep 600
FAKE
chmod 0755 "$fake_claude"
launch_config="$artifact_root/launch.json"
python3 - "$launch_config" "$fake_claude" "$workspace" <<'PY'
import json, sys
with open(sys.argv[1], 'w', encoding='utf-8') as handle:
    json.dump({
        'harnesses': [{'id': 'claude-code', 'label': 'Claude Code', 'command': sys.argv[2]}],
        'folders': [sys.argv[3]],
    }, handle)
PY

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
CHROTE_LAUNCH_CONFIG="$launch_config" \
CHROTE_AGENT_EVENT_HOOK="$repo_root/scripts/chrote-agent-event" \
CHROTE_AGENT_HOOKS_DIR="$artifact_root/agent-hooks" \
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

# The completion hook, end to end: the launcher installs it through the
# harness's flags, the harness runs it from inside the pane, the script asks
# tmux for the session's name and posts, and the session list carries the
# event until the operator is recorded as having seen it.
launch_response="$artifact_root/launch-response.json"
launch_status="$(curl -s -o "$launch_response" -w '%{http_code}' \
  -H 'Content-Type: application/json' \
  --data "$(python3 -c 'import json, sys; print(json.dumps({"name": sys.argv[1], "harness": "claude-code", "cwd": sys.argv[2]}))' "$hook_session" "$workspace")" \
  "http://127.0.0.1:$port/api/tmux/sessions")"
if [ "$launch_status" != "200" ]; then
  echo "launching the hooked harness returned $launch_status: $(cat "$launch_response")" >&2
  exit 1
fi
python3 - "$launch_response" <<'PY'
import json, sys
payload = json.load(open(sys.argv[1]))
assert payload.get('notify') is True, payload
assert 'warning' not in payload, payload
PY
sessions_json="$artifact_root/sessions.json"
event_arrived=false
for attempt in $(seq 1 50); do
  curl -fsS "http://127.0.0.1:$port/api/tmux/sessions" >"$sessions_json"
  if python3 - "$sessions_json" "$hook_session" <<'PY'
import json, sys
payload = json.load(open(sys.argv[1]))
for session in payload.get('sessions', []):
    if session['name'] == sys.argv[2]:
        event = session.get('lastEvent')
        raise SystemExit(0 if event and event['event'] == 'finished' and event['seen'] is False else 1)
raise SystemExit(1)
PY
  then
    event_arrived=true
    echo "finished event arrived on $hook_session after $attempt poll(s)"
    break
  fi
  sleep 0.1
done
if [ "$event_arrived" != true ]; then
  echo "no finished event arrived on $hook_session; last session list follows:" >&2
  cat "$sessions_json" >&2
  echo "server log follows:" >&2
  tail -n 50 "$server_log" >&2 || true
  exit 1
fi
curl -fsS -o /dev/null -H 'Content-Type: application/json' \
  --data "{\"session\":\"$hook_session\"}" "http://127.0.0.1:$port/api/agent/event/seen"
curl -fsS "http://127.0.0.1:$port/api/tmux/sessions" >"$sessions_json"
python3 - "$sessions_json" "$hook_session" <<'PY'
import json, sys
payload = json.load(open(sys.argv[1]))
events = {session['name']: session.get('lastEvent') for session in payload.get('sessions', [])}
assert events.get(sys.argv[2], {}).get('seen') is True, events
PY

echo "Contract artifacts (kept only if this run fails): $artifact_root"
cd "$repo_root/dashboard"
CHROTE_CONTRACT_WORKSPACE="$workspace" \
CHROTE_TEST_URL="http://127.0.0.1:$port" \
  npm run test:server-contract

#!/usr/bin/env bash
# chrote-agent-ensure -- keep exactly one locked agent session alive.
#
# ExecStart of chrote-agent@<name>.service. Per ADR-0014 this process IS what
# systemd supervises: it ensures the session exists, then blocks watching it and
# exits non-zero when the agent dies, so `Restart=on-failure` means something. A
# launcher that created the session and exited would leave the unit reporting
# `inactive (dead)` seconds after a successful start, and nothing would notice an
# agent dying an hour later.
#
# It never creates a tmux server. `tmux new-session` against a dead socket forks
# a SERVER into the caller's cgroup; if that caller is this unit, a later
# restart of this unit kills every session on the socket. Server lifetime belongs
# to the socket's keeper unit, so a dead socket is a loud failure here.
#
# Configuration is a typed file, never a command string. tmux starts this fixed
# launcher as the pane command; pane mode then invokes the agent with a Bash argv
# array. No config value is ever parsed by a shell as command text.
set -uo pipefail

readonly PROGRAM_NAME=${0##*/}

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }
fail() { log "FATAL: $*"; exit 1; }

usage() {
  cat >&2 <<'USAGE'
usage: chrote-agent-ensure.sh --config <file> [--once]

  --config <file>  typed per-agent config written by CHROTE (mode 0600)
  --once           ensure the session exists, then exit 0 without watching.
                   For preflight only; the unit never passes it, because a
                   launcher that exits is a launcher systemd cannot supervise.
USAGE
}

# --- configuration -----------------------------------------------------------

# Read one key from a strict KEY=value file. The file is never sourced: sourcing
# would execute whatever a config-write bug put there, and this process is the
# one thing standing between a config file and a shell.
config_value() {
  local key=$1 line value
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" == "$key="* ]] || continue
    value=${line#"$key="}
    printf '%s' "$value"
    return 0
  done <"$CONFIG_FILE"
  return 1
}

require_value() {
  local key=$1 value
  value=$(config_value "$key") || fail "config is missing $key: $CONFIG_FILE"
  [[ -n "$value" ]] || fail "config has an empty $key: $CONFIG_FILE"
  printf '%s' "$value"
}

# Fail loud on anything that could become a second tmux argument, a flag, or a
# path escape. Every value below is used as a single argv element, so this is
# defence in depth rather than the only guard -- but a config that reached here
# malformed is a bug worth stopping on, not working around.
validate_token() {
  local name=$1 value=$2 pattern=$3
  [[ "$value" =~ $pattern ]] || fail "$name is not well formed: $CONFIG_FILE"
}

# --- tmux --------------------------------------------------------------------

tmux_run() { "$TMUX_BIN" -S "$TMUX_SOCKET" "$@"; }

server_alive() { tmux_run list-sessions >/dev/null 2>&1; }

session_exists() { tmux_run has-session -t "=$SESSION" 2>/dev/null; }

# The pane's first-process pid. Pane mode execs the agent, so this pid names the
# actual agent rather than a generic shell that survives when the agent exits.
pane_pid() {
  local value
  value=$(tmux_run display-message -p -t "=$SESSION" '#{pane_pid}' 2>/dev/null) || return 1
  [[ "$value" =~ ^[1-9][0-9]*$ ]] || return 1
  printf '%s' "$value"
}

wait_for_pane_pid() {
  local attempts=0 value
  while (( attempts < 100 )); do
    value=$(pane_pid) && { printf '%s' "$value"; return 0; }
    session_exists || return 1
    sleep 0.05
    attempts=$((attempts + 1))
  done
  return 1
}

# Start the agent from typed fields as an actual argv. This function runs only
# as the fixed initial pane command; it never renders text for send-keys or sh -c.
launch_agent() {
  log "resuming $AGENT_KIND session $AGENT_SESSION_ID in $SESSION"
  case "$AGENT_KIND" in
    codex) exec "$AGENT_BIN" resume "$AGENT_SESSION_ID" ;;
    claude) exec "$AGENT_BIN" --resume "$AGENT_SESSION_ID" ;;
    hermes)
      # AGENT_BIN is hermes's own venv interpreter, so the module and profile are
      # part of the canonical form rather than a wrapper script.
      exec "$AGENT_BIN" -m hermes_cli.main --profile "$HERMES_PROFILE" --resume "$AGENT_SESSION_ID"
      ;;
    *) fail "unsupported agent kind: $AGENT_KIND" ;;
  esac
  fail "could not execute $AGENT_KIND agent binary: $AGENT_BIN"
}

# --- receipt -----------------------------------------------------------------

# ADR-0014 decision 5: an active unit is not proof the RIGHT transcript resumed.
# The receipt is what lets status distinguish healthy from degraded. Written
# atomically so a reader never sees a half-file.
write_receipt() {
  local pane=$1 tmp
  [[ -n "$RECEIPT_PATH" ]] || return 0
  mkdir -p "$(dirname "$RECEIPT_PATH")" 2>/dev/null || true
  tmp=$(mktemp "${RECEIPT_PATH}.XXXXXX") || return 0
  printf '{"session":"%s","agentKind":"%s","agentSessionId":"%s","panePid":%s,"startedAt":"%s"}\n' \
    "$SESSION" "$AGENT_KIND" "$AGENT_SESSION_ID" "$pane" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$tmp"
  chmod 600 "$tmp" 2>/dev/null || true
  mv -f "$tmp" "$RECEIPT_PATH"
}

# --- lifecycle ---------------------------------------------------------------

ensure_session() {
  if session_exists; then
    PANE_TARGET=$(sole_pane) \
      || fail "session $SESSION must contain exactly one pane before it can be locked"
    if pane_is_managed "$PANE_TARGET"; then
      log "session $SESSION already runs the trusted pane launcher; adopting it"
      return 0
    fi

    # A shell cannot be made into an exact supervisor: its pane_pid stays alive
    # after the child agent exits. Replace the one pane with the fixed launcher,
    # which resumes the same native transcript and execs the agent directly.
    log "replacing the unmanaged pane in $SESSION with the trusted pane launcher"
    tmux_run set-environment -t "=$SESSION" CHROTE_AGENT_CONFIG "$CONFIG_FILE" \
      || fail "could not bind typed config to $SESSION"
    tmux_run respawn-pane -k -t "$PANE_TARGET" -c "$WORKDIR" "$PANE_COMMAND" \
      || fail "could not replace the unmanaged pane in $SESSION"
    return 0
  fi

  log "creating session $SESSION in $WORKDIR"
  tmux_run new-session -d -s "$SESSION" -c "$WORKDIR" \
    -e "CHROTE_AGENT_CONFIG=$CONFIG_FILE" "$PANE_COMMAND" \
    || fail "tmux refused to create session $SESSION"
  PANE_TARGET=$(sole_pane) \
    || fail "new session $SESSION did not expose exactly one pane"
}

sole_pane() {
  local value
  value=$(tmux_run list-panes -s -t "=$SESSION" -F '#{pane_id}' 2>/dev/null) || return 1
  [[ "$value" =~ ^%[0-9]+$ ]] || return 1
  printf '%s' "$value"
}

pane_start_command() {
  tmux_run display-message -p -t "$1" '#{pane_start_command}' 2>/dev/null
}

session_config() {
  local value
  value=$(tmux_run show-environment -t "=$SESSION" CHROTE_AGENT_CONFIG 2>/dev/null) || return 1
  [[ "$value" == CHROTE_AGENT_CONFIG=* ]] || return 1
  printf '%s' "${value#CHROTE_AGENT_CONFIG=}"
}

pane_is_managed() {
  local target=$1 start config
  start=$(pane_start_command "$target") || return 1
  config=$(session_config) || return 1
  [[ "$config" == "$CONFIG_FILE" ]] || return 1
  [[ "$start" == "$PANE_COMMAND" || "$start" == \"$PANE_COMMAND\" ]]
}

# Block until the session or its pane process goes away. Exiting non-zero is the
# signal that makes Restart= fire; a clean operator `systemctl stop` never
# reaches here because systemd signals the process first.
watch_session() {
  local expected_pane=$1 pane
  while :; do
    sleep "$WATCH_INTERVAL"
    if ! server_alive; then
      log "tmux server on $TMUX_SOCKET stopped answering"
      return 1
    fi
    if ! session_exists; then
      log "session $SESSION disappeared"
      return 1
    fi
    pane=$(pane_pid) || { log "session $SESSION has no readable pane process"; return 1; }
    if [[ "$pane" != "$expected_pane" ]]; then
      log "pane process in $SESSION changed from $expected_pane to $pane"
      return 1
    fi
    if ! kill -0 "$pane" 2>/dev/null; then
      log "pane process $pane in $SESSION exited"
      return 1
    fi
  done
}

# --- main --------------------------------------------------------------------

CONFIG_FILE=""
WATCH=1
PANE_MODE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) [[ $# -ge 2 ]] || { usage; exit 2; }; CONFIG_FILE=$2; shift 2 ;;
    --once) WATCH=0; shift ;;
    --pane) PANE_MODE=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) log "unknown argument: $1"; usage; exit 2 ;;
  esac
done

if (( PANE_MODE == 1 )); then
  [[ -z "$CONFIG_FILE" ]] || fail "pane mode accepts config only through CHROTE_AGENT_CONFIG"
  CONFIG_FILE=${CHROTE_AGENT_CONFIG:-}
fi
[[ -n "$CONFIG_FILE" ]] || { usage; exit 2; }
[[ -f "$CONFIG_FILE" ]] || fail "config file does not exist: $CONFIG_FILE"
[[ ! -L "$CONFIG_FILE" ]] || fail "config file is a symlink, refusing to read it: $CONFIG_FILE"
[[ -r "$CONFIG_FILE" ]] || fail "config file is not readable: $CONFIG_FILE"

SESSION=$(require_value CHROTE_AGENT_SESSION)
TMUX_BIN=$(require_value CHROTE_AGENT_TMUX_BIN)
TMUX_SOCKET=$(require_value CHROTE_AGENT_TMUX_SOCKET)
WORKDIR=$(require_value CHROTE_AGENT_WORKDIR)
AGENT_KIND=$(require_value CHROTE_AGENT_KIND)
AGENT_SESSION_ID=$(require_value CHROTE_AGENT_SESSION_ID)
AGENT_BIN=$(require_value CHROTE_AGENT_BIN)
KEEPER_UNIT=$(config_value CHROTE_AGENT_TMUX_KEEPER_UNIT || true)
HERMES_PROFILE=$(config_value CHROTE_AGENT_HERMES_PROFILE || true)
RECEIPT_PATH=$(config_value CHROTE_AGENT_RECEIPT_PATH || true)
WATCH_INTERVAL=$(config_value CHROTE_AGENT_WATCH_INTERVAL || true)
[[ -n "${WATCH_INTERVAL:-}" ]] || WATCH_INTERVAL=10

validate_token "CHROTE_AGENT_SESSION" "$SESSION" '^[a-zA-Z0-9_-]{1,50}$'
validate_token "CHROTE_AGENT_KIND" "$AGENT_KIND" '^(codex|claude|hermes)$'
# A hermes profile becomes an argv element after --profile, so it is held to the
# same shape as every other value that reaches a command line.
if [[ "$AGENT_KIND" == hermes ]]; then
  [[ -n "${HERMES_PROFILE:-}" ]] || fail "hermes agents require CHROTE_AGENT_HERMES_PROFILE: $CONFIG_FILE"
  validate_token "CHROTE_AGENT_HERMES_PROFILE" "$HERMES_PROFILE" '^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$'
fi
validate_token "CHROTE_AGENT_SESSION_ID" "$AGENT_SESSION_ID" '^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$'
validate_token "CHROTE_AGENT_WATCH_INTERVAL" "$WATCH_INTERVAL" '^[1-9][0-9]{0,3}$'
for path_name in CHROTE_AGENT_TMUX_BIN:$TMUX_BIN CHROTE_AGENT_TMUX_SOCKET:$TMUX_SOCKET \
                 CHROTE_AGENT_WORKDIR:$WORKDIR CHROTE_AGENT_BIN:$AGENT_BIN; do
  validate_token "${path_name%%:*}" "${path_name#*:}" '^/[^[:space:]]*$'
done

[[ -x "$TMUX_BIN" ]] || fail "tmux binary is missing or not executable: $TMUX_BIN"
[[ -x "$AGENT_BIN" ]] || fail "agent binary is missing or not executable: $AGENT_BIN"
[[ -d "$WORKDIR" ]] || fail "agent working directory does not exist: $WORKDIR"

if (( PANE_MODE == 1 )); then
  write_receipt "$$"
  launch_agent
fi

LAUNCHER_BIN=$(readlink -f -- "$0") || fail "cannot resolve launcher path: $0"
validate_token "launcher path" "$LAUNCHER_BIN" '^/[a-zA-Z0-9._/-]+$'
[[ -x "$LAUNCHER_BIN" ]] || fail "launcher is missing or not executable: $LAUNCHER_BIN"
# tmux accepts one shell-command field, not an argv vector. This string contains
# only the fixed, validated launcher path and a constant mode flag; typed config
# crosses separately in the tmux session environment and is never shell text.
readonly PANE_COMMAND="$LAUNCHER_BIN --pane"

# The socket-keeper contract. Refusing here is the whole point of this launcher:
# reviving the server would put it in this unit's cgroup.
if ! server_alive; then
  keeper_description=${KEEPER_UNIT:-the keeper unit for this socket}
  fail "no tmux server answers ${TMUX_SOCKET}; refusing to create one. Server lifetime belongs to ${keeper_description}; start it first."
fi

ensure_session

pane=$(wait_for_pane_pid) || fail "session $SESSION exists but has no readable pane process"

if (( WATCH == 0 )); then
  log "$PROGRAM_NAME --once: session $SESSION ready (pane $pane); not watching"
  exit 0
fi

log "watching $SESSION (pane $pane) every ${WATCH_INTERVAL}s"
watch_session "$pane"
exit 1

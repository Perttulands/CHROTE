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
# Configuration is a typed file, never a command string: the resume argv is
# RENDERED from agent kind + native session id (ADR-0001 -- canonical argv from
# typed fields, shell strings are a rendered view only). A config file therefore
# cannot smuggle an arbitrary command into a pane.
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

# The pane's own pid, used as the liveness subject and recorded in the receipt.
pane_pid() {
  local value
  value=$(tmux_run display-message -p -t "=$SESSION" '#{pane_pid}' 2>/dev/null) || return 1
  [[ "$value" =~ ^[1-9][0-9]*$ ]] || return 1
  printf '%s' "$value"
}

# --- resume argv -------------------------------------------------------------

# Rendered here from typed fields, never read from config as a string.
render_resume_argv() {
  case "$AGENT_KIND" in
    codex) printf '%s resume %s' "$AGENT_BIN" "$AGENT_SESSION_ID" ;;
    claude) printf '%s --resume %s' "$AGENT_BIN" "$AGENT_SESSION_ID" ;;
    hermes)
      # AGENT_BIN is hermes's own venv interpreter, so the module and profile are
      # part of the canonical form rather than a wrapper script.
      printf '%s -m hermes_cli.main --profile %s --resume %s' \
        "$AGENT_BIN" "$HERMES_PROFILE" "$AGENT_SESSION_ID"
      ;;
    *) fail "unsupported agent kind: $AGENT_KIND" ;;
  esac
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
    log "session $SESSION already exists; adopting it without touching the pane"
    return 0
  fi
  local argv
  argv=$(render_resume_argv)
  log "creating session $SESSION in $WORKDIR"
  tmux_run new-session -d -s "$SESSION" -c "$WORKDIR" \
    || fail "tmux refused to create session $SESSION"
  # Two calls, never one: -l sends the text literally so nothing in it is
  # interpreted as a key name, and Enter is a separate keystroke.
  tmux_run send-keys -t "=$SESSION" -l -- "$argv" \
    || fail "could not send the resume command to $SESSION"
  tmux_run send-keys -t "=$SESSION" Enter \
    || fail "could not submit the resume command in $SESSION"
  log "resumed $AGENT_KIND session $AGENT_SESSION_ID in $SESSION"
}

# Block until the session or its pane process goes away. Exiting non-zero is the
# signal that makes Restart= fire; a clean operator `systemctl stop` never
# reaches here because systemd signals the process first.
watch_session() {
  local pane
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
    if ! kill -0 "$pane" 2>/dev/null; then
      log "pane process $pane in $SESSION exited"
      return 1
    fi
  done
}

# --- main --------------------------------------------------------------------

CONFIG_FILE=""
WATCH=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config) [[ $# -ge 2 ]] || { usage; exit 2; }; CONFIG_FILE=$2; shift 2 ;;
    --once) WATCH=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) log "unknown argument: $1"; usage; exit 2 ;;
  esac
done

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
[[ -d "$WORKDIR" ]] || fail "agent working directory does not exist: $WORKDIR"

# The socket-keeper contract. Refusing here is the whole point of this launcher:
# reviving the server would put it in this unit's cgroup.
if ! server_alive; then
  keeper_description=${KEEPER_UNIT:-the keeper unit for this socket}
  fail "no tmux server answers ${TMUX_SOCKET}; refusing to create one. Server lifetime belongs to ${keeper_description}; start it first."
fi

ensure_session

pane=$(pane_pid) || fail "session $SESSION exists but has no readable pane process"
write_receipt "$pane"

if (( WATCH == 0 )); then
  log "$PROGRAM_NAME --once: session $SESSION ready (pane $pane); not watching"
  exit 0
fi

log "watching $SESSION (pane $pane) every ${WATCH_INTERVAL}s"
watch_session
exit 1

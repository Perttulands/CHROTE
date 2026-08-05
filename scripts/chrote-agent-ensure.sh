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
set -euo pipefail

readonly PROGRAM_NAME=${0##*/}

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }
fail() { log "FATAL: $*"; exit 1; }

usage() {
  cat >&2 <<'USAGE'
usage: chrote-agent-ensure.sh --config <file> [--once]

  --config <file>  typed per-agent config written by CHROTE (mode 0640 ACL mask)
  --once           ensure the session exists, then exit 0 without watching.
                   For preflight only; the unit never passes it, because a
                   launcher that exits is a launcher systemd cannot supervise.
USAGE
}

# --- configuration -----------------------------------------------------------

# Read one key from a strict KEY=value file. The file is never sourced: sourcing
# would execute whatever a config-write bug put there, and this process is the
# one thing standing between a config file and a shell.
# Assign one typed config value without a command substitution. A failure inside
# `$(...)` runs in a subshell; without this shape a caller can accidentally keep
# going after fail() exits only that subshell.
read_config_value() {
  local key=$1 destination=$2 requirement=$3 line value="" found=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" == "$key="* ]] || continue
    found=$((found + 1))
    (( found == 1 )) || fail "config has duplicate $key: $CONFIG_FILE"
    value=${line#"$key="}
  done <"$CONFIG_FILE"
  if (( found == 0 )); then
    [[ "$requirement" == optional ]] || fail "config is missing $key: $CONFIG_FILE"
  elif [[ "$requirement" == required && -z "$value" ]]; then
    fail "config has an empty $key: $CONFIG_FILE"
  fi
  printf -v "$destination" '%s' "$value"
}

# Fail loud on anything that could become a second tmux argument, a flag, or a
# path escape. Every value below is used as a single argv element, so this is
# defence in depth rather than the only guard -- but a config that reached here
# malformed is a bug worth stopping on, not working around.
validate_token() {
  local name=$1 value=$2 pattern=$3
  [[ "$value" =~ $pattern ]] || fail "$name is not well formed: $CONFIG_FILE"
}

validate_path() {
  local name=$1 value=$2 canonical
  validate_token "$name" "$value" '^/[a-zA-Z0-9._/-]+$'
  canonical=$(/usr/bin/realpath -m -s -- "$value") \
    || fail "$name cannot be resolved: $CONFIG_FILE"
  [[ "$canonical" == "$value" ]] || fail "$name is not canonical: $CONFIG_FILE"
}

# --- tmux --------------------------------------------------------------------

tmux_run() { "$TMUX_BIN" -S "$TMUX_SOCKET" "$@"; }

server_alive() { tmux_run list-sessions >/dev/null 2>&1; }

session_exists() { tmux_run has-session -t "=$SESSION" 2>/dev/null; }

# The pane's first-process pid. Pane mode execs the agent, so this pid names the
# actual agent rather than a generic shell that survives when the agent exits.
pane_pid() {
  local value
  value=$(tmux_run display-message -p -t "$PANE_TARGET" '#{pane_pid}' 2>/dev/null) || return 1
  [[ "$value" =~ ^[1-9][0-9]*$ ]] || return 1
  printf '%s' "$value"
}

pane_id() {
  local value
  value=$(tmux_run display-message -p -t "$PANE_TARGET" '#{pane_id}' 2>/dev/null) || return 1
  [[ "$value" =~ ^%[0-9]+$ ]] || return 1
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

# ADR-0014 decision 5: desired config is not proof of what exec actually ran.
# The unit-facing process observes /proc for the pane PID, then records that
# independent identity atomically. Pane mode never writes a receipt itself.
monotonic_microseconds() {
  local uptime whole fraction
  read -r uptime _ </proc/uptime || return 1
  whole=${uptime%%.*}
  fraction=${uptime#*.}000000
  fraction=${fraction:0:6}
  [[ "$whole" =~ ^[0-9]+$ && "$fraction" =~ ^[0-9]{6}$ ]] || return 1
  printf '%s' "$((10#$whole * 1000000 + 10#$fraction))"
}

write_receipt() {
  local pane_id_value=$1 pane_pid_value=$2 agent_pid_value=$3 process_start_ticks=$4 \
    observed_kind=$5 observed_session_id=$6 attested_monotonic tmp
  [[ -n "$RECEIPT_PATH" ]] || fail "config has no receipt path: $CONFIG_FILE"
  [[ -d "$(dirname "$RECEIPT_PATH")" ]] \
    || fail "receipt runtime directory is not provisioned: $RECEIPT_PATH"
  tmp=$(mktemp "${RECEIPT_PATH}.XXXXXX") \
    || fail "cannot stage launcher receipt: $RECEIPT_PATH"
  attested_monotonic=$(monotonic_microseconds) \
    || { rm -f -- "$tmp"; fail "cannot read the monotonic clock for launcher receipt"; }
  printf '{"session":"%s","agentKind":"%s","agentSessionId":"%s","paneId":"%s","panePid":%s,"agentPid":%s,"processStartTicks":%s,"invocationId":"%s","attestedAtMonotonic":%s,"startedAt":"%s"}\n' \
    "$SESSION" "$observed_kind" "$observed_session_id" "$pane_id_value" \
    "$pane_pid_value" "$agent_pid_value" "$process_start_ticks" "$UNIT_INVOCATION_ID" \
    "$attested_monotonic" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$tmp" \
    || { rm -f -- "$tmp"; fail "cannot write launcher receipt: $RECEIPT_PATH"; }
  chmod 640 "$tmp" \
    || { rm -f -- "$tmp"; fail "cannot protect launcher receipt: $RECEIPT_PATH"; }
  mv -f "$tmp" "$RECEIPT_PATH" \
    || { rm -f -- "$tmp"; fail "cannot publish launcher receipt: $RECEIPT_PATH"; }
}

observe_agent_process() {
  local pid=$1 start_index=-1 profile_index=-1 i stat_line
  local -a observed=() stat_fields=()
  kill -0 "$pid" 2>/dev/null || return 1
  mapfile -d '' -t observed <"/proc/$pid/cmdline" 2>/dev/null || true
  (( ${#observed[@]} > 0 )) || return 1

  OBSERVED_AGENT_KIND=""
  OBSERVED_AGENT_SESSION_ID=""
  OBSERVED_HERMES_PROFILE=""
  for ((i = 0; i < ${#observed[@]}; i++)); do
    [[ "${observed[$i]}" == "hermes_cli.main" ]] && OBSERVED_AGENT_KIND=hermes
    [[ "${observed[$i]}" == "--profile" ]] && profile_index=$i
    [[ "${observed[$i]}" == "--resume" ]] && start_index=$i
  done
  if [[ "$OBSERVED_AGENT_KIND" == hermes ]]; then
    (( profile_index >= 0 && profile_index + 1 < ${#observed[@]} )) || return 1
    (( start_index >= 0 && start_index + 1 < ${#observed[@]} )) || return 1
    OBSERVED_HERMES_PROFILE=${observed[$((profile_index + 1))]}
    OBSERVED_AGENT_SESSION_ID=${observed[$((start_index + 1))]}
  elif (( start_index >= 0 && start_index + 1 < ${#observed[@]} )); then
    OBSERVED_AGENT_KIND=claude
    OBSERVED_AGENT_SESSION_ID=${observed[$((start_index + 1))]}
  else
    for ((i = 0; i + 1 < ${#observed[@]}; i++)); do
      if [[ "${observed[$i]}" == resume ]]; then
        OBSERVED_AGENT_KIND=codex
        OBSERVED_AGENT_SESSION_ID=${observed[$((i + 1))]}
        break
      fi
    done
  fi

  [[ "$OBSERVED_AGENT_KIND" == "$AGENT_KIND" ]] || return 1
  [[ "$OBSERVED_AGENT_SESSION_ID" == "$AGENT_SESSION_ID" ]] || return 1
  if [[ "$AGENT_KIND" == hermes ]]; then
    [[ "$OBSERVED_HERMES_PROFILE" == "$HERMES_PROFILE" ]] || return 1
  fi

  IFS= read -r stat_line <"/proc/$pid/stat" || return 1
  stat_line=${stat_line##*) }
  read -r -a stat_fields <<<"$stat_line"
  (( ${#stat_fields[@]} > 19 )) || return 1
  PROCESS_START_TICKS=${stat_fields[19]}
  [[ "$PROCESS_START_TICKS" =~ ^[1-9][0-9]*$ ]] || return 1
}

attest_running_agent() {
  local pane_pid_value pane_id_value attempts=0
  [[ -n "$UNIT_INVOCATION_ID" ]] || {
    log "no systemd INVOCATION_ID; running without a health receipt"
    return 0
  }
  while (( attempts < 100 )); do
    if pane_pid_value=$(pane_pid) && pane_id_value=$(pane_id) \
      && observe_agent_process "$pane_pid_value"; then
      break
    fi
    session_exists || fail "session $SESSION disappeared before identity confirmation"
    sleep 0.05
    attempts=$((attempts + 1))
  done
  (( attempts < 100 )) \
    || fail "pane does not run the configured $AGENT_KIND session $AGENT_SESSION_ID"
  write_receipt "$pane_id_value" "$pane_pid_value" "$pane_pid_value" \
    "$PROCESS_START_TICKS" "$OBSERVED_AGENT_KIND" "$OBSERVED_AGENT_SESSION_ID"
  ATTESTED_PANE_PID=$pane_pid_value

  if [[ -n "${NOTIFY_SOCKET:-}" ]]; then
    [[ -x /usr/bin/systemd-notify ]] || fail "systemd-notify is required for readiness"
    /usr/bin/systemd-notify --pid="$$" --ready \
      --status="observed $OBSERVED_AGENT_KIND session $OBSERVED_AGENT_SESSION_ID" \
      || fail "could not notify systemd that identity was confirmed"
  fi
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
validate_path "config file" "$CONFIG_FILE"
[[ -f "$CONFIG_FILE" ]] || fail "config file does not exist: $CONFIG_FILE"
[[ ! -L "$CONFIG_FILE" ]] || fail "config file is a symlink, refusing to read it: $CONFIG_FILE"
[[ -r "$CONFIG_FILE" ]] || fail "config file is not readable: $CONFIG_FILE"

read_config_value CHROTE_AGENT_SESSION SESSION required
read_config_value CHROTE_AGENT_TMUX_BIN TMUX_BIN required
read_config_value CHROTE_AGENT_TMUX_SOCKET TMUX_SOCKET required
read_config_value CHROTE_AGENT_WORKDIR WORKDIR required
read_config_value CHROTE_AGENT_KIND AGENT_KIND required
read_config_value CHROTE_AGENT_SESSION_ID AGENT_SESSION_ID required
read_config_value CHROTE_AGENT_BIN AGENT_BIN required
read_config_value CHROTE_AGENT_TMUX_KEEPER_UNIT KEEPER_UNIT optional
read_config_value CHROTE_AGENT_HERMES_PROFILE HERMES_PROFILE optional
read_config_value CHROTE_AGENT_RECEIPT_PATH RECEIPT_PATH optional
read_config_value CHROTE_AGENT_WATCH_INTERVAL WATCH_INTERVAL optional
[[ -n "${WATCH_INTERVAL:-}" ]] || WATCH_INTERVAL=10
UNIT_INVOCATION_ID=${INVOCATION_ID:-}

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
if [[ -n "$UNIT_INVOCATION_ID" ]]; then
  validate_token "INVOCATION_ID" "$UNIT_INVOCATION_ID" '^[a-fA-F0-9]{32}$'
fi
for path_name in CHROTE_AGENT_TMUX_BIN:$TMUX_BIN CHROTE_AGENT_TMUX_SOCKET:$TMUX_SOCKET \
                 CHROTE_AGENT_WORKDIR:$WORKDIR CHROTE_AGENT_BIN:$AGENT_BIN; do
  validate_path "${path_name%%:*}" "${path_name#*:}"
done
if [[ -n "$RECEIPT_PATH" ]]; then
  validate_path "CHROTE_AGENT_RECEIPT_PATH" "$RECEIPT_PATH"
fi

[[ -x "$TMUX_BIN" ]] || fail "tmux binary is missing or not executable: $TMUX_BIN"
[[ -x "$AGENT_BIN" ]] || fail "agent binary is missing or not executable: $AGENT_BIN"
[[ -d "$WORKDIR" ]] || fail "agent working directory does not exist: $WORKDIR"

if (( PANE_MODE == 1 )); then
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

if [[ -n "$UNIT_INVOCATION_ID" && -n "$RECEIPT_PATH" ]]; then
  rm -f -- "$RECEIPT_PATH" || fail "cannot remove the previous launcher receipt: $RECEIPT_PATH"
fi

ensure_session

attest_running_agent
pane=${ATTESTED_PANE_PID:-}
if [[ -z "$pane" ]]; then
  pane=$(wait_for_pane_pid) || fail "session $SESSION exists but has no readable pane process"
fi

if (( WATCH == 0 )); then
  log "$PROGRAM_NAME --once: session $SESSION ready (pane $pane); not watching"
  exit 0
fi

log "watching $SESSION (pane $pane) every ${WATCH_INTERVAL}s"
watch_session "$pane"
exit 1

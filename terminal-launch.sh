#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal launch script for CHROTE.
# TMUX_TMPDIR and CHROTE_WORKDIR are set by the systemd unit or environment.
export LANG=en_US.UTF-8
# REASON: cd to preferred dir is optional, fallthrough is intentional
cd "${CHROTE_WORKDIR:-$HOME}" 2>/dev/null || cd ~ || exit
SESSION="$1"
UNIX_USER="${2:-}"
# REASON: pin one tmux client. A 3.4 client cannot talk to a 3.6a server at all,
# so bare "tmux" made the attach path depend on whatever PATH happened to be.
TMUX_BIN="${CHROTE_TMUX_BIN:-tmux}"

trim_ws() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s\n' "$value"
}

allowed_terminal_user() {
  local candidate="$1"
  local allowed="${CHROTE_TERMINAL_USERS:-}"
  local entry user
  local -a entries
  [ -n "$candidate" ] || return 1
  if [ -z "$allowed" ]; then
    [ "$candidate" = "$(id -un)" ]
    return $?
  fi
  IFS=',' read -r -a entries <<< "$allowed"
  for entry in "${entries[@]}"; do
    user="$(trim_ws "$entry")"
    [ -n "$user" ] || continue
    [ "$user" = "$candidate" ] && return 0
  done
  return 1
}

socket_for_terminal_user() {
  local candidate="$1"
  local mappings="${CHROTE_TERMINAL_USER_SOCKETS:-}"
  local entry key value match=""
  local -a entries
  IFS=',' read -r -a entries <<< "$mappings"
  # REASON: the Go parser is last-wins and this one used to be first-wins, so a
  # duplicated user key made listing and attaching resolve different tmux
  # servers. Both parsers now refuse a duplicate instead of picking silently.
  for entry in "${entries[@]}"; do
    key="$(trim_ws "${entry%%=*}")"
    value="$(trim_ws "${entry#*=}")"
    if [ "$key" = "$candidate" ] && [ -n "$value" ] && [ "$value" != "$entry" ]; then
      if [ -n "$match" ]; then
        echo "CHROTE_TERMINAL_USER_SOCKETS has duplicate entries for Unix user '$candidate' ('$match' and '$value'); keep exactly one entry per user so terminal listing and terminal attach resolve the same socket" >&2
        return 3
      fi
      match="$value"
    fi
  done
  if [ -n "$match" ]; then
    printf '%s\n' "$match"
    return 0
  fi
  local uid
  if [ "$candidate" = "$(id -un)" ] && [ -n "${CHROTE_TMUX_SOCKET:-}" ]; then
    printf '%s\n' "$CHROTE_TMUX_SOCKET"
    return 0
  fi
  uid="$(id -u "$candidate" 2>/dev/null || true)"
  if [ -n "$uid" ]; then
    printf '/tmp/tmux-%s/default\n' "$uid"
    return 0
  fi
  return 1
}

attach_explicit_socket() {
  local socket="$1"
  local session="$2"
  if [ -z "$session" ]; then
    echo "CHROTE socket terminal requires a tmux session name" >&2
    exit 2
  fi

  # REASON: explicit-socket terminals must fail loud instead of falling back to
  # the invoking user's ambient tmux server when the configured session is
  # unavailable — a silent fallback would attach the operator to the wrong pool.
  if "$TMUX_BIN" -S "$socket" has-session -t "$session" 2>/dev/null; then
    exec "$TMUX_BIN" -S "$socket" attach-session -t "$session"
  fi

  echo "tmux session '$session' is not available on configured socket '$socket'" >&2
  exit 1
}

if [ -n "$UNIX_USER" ]; then
  if ! allowed_terminal_user "$UNIX_USER"; then
    echo "Unix user '$UNIX_USER' is not allowed for CHROTE terminal launch" >&2
    exit 2
  fi
  USER_SOCKET="$(socket_for_terminal_user "$UNIX_USER")"
  SOCKET_RC=$?
  if [ "$SOCKET_RC" -eq 3 ]; then
    # REASON: the duplicate-key message is already on stderr; do not mask it.
    exit 2
  elif [ "$SOCKET_RC" -ne 0 ]; then
    echo "No CHROTE terminal socket configured for Unix user '$UNIX_USER'" >&2
    exit 2
  fi
  attach_explicit_socket "$USER_SOCKET" "$SESSION"
elif [ -n "${CHROTE_TMUX_SOCKET:-}" ]; then
  attach_explicit_socket "$CHROTE_TMUX_SOCKET" "$SESSION"
else
  # REASON: tmux has-session tests existence; stderr is noise, not an error
  if [ -n "$SESSION" ] && "$TMUX_BIN" has-session -t "$SESSION" 2>/dev/null; then
    exec "$TMUX_BIN" attach-session -t "$SESSION"
  fi
fi
exec bash -l

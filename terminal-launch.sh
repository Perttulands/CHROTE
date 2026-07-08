#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal launch script for CHROTE.
# TMUX_TMPDIR and CHROTE_WORKDIR are set by the systemd unit or environment.
export LANG=en_US.UTF-8
# REASON: cd to preferred dir is optional, fallthrough is intentional
cd "${CHROTE_WORKDIR:-$HOME}" 2>/dev/null || cd ~ || exit
SESSION="$1"
UNIX_USER="${2:-}"

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
  local entry key value
  local -a entries
  IFS=',' read -r -a entries <<< "$mappings"
  for entry in "${entries[@]}"; do
    key="$(trim_ws "${entry%%=*}")"
    value="$(trim_ws "${entry#*=}")"
    if [ "$key" = "$candidate" ] && [ -n "$value" ] && [ "$value" != "$entry" ]; then
      printf '%s\n' "$value"
      return 0
    fi
  done
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
  # the ambient perttu tmux server when the configured session is unavailable.
  if tmux -S "$socket" has-session -t "$session" 2>/dev/null; then
    exec tmux -S "$socket" attach-session -t "$session"
  fi

  echo "tmux session '$session' is not available on configured socket '$socket'" >&2
  exit 1
}

if [ -n "$UNIX_USER" ]; then
  if ! allowed_terminal_user "$UNIX_USER"; then
    echo "Unix user '$UNIX_USER' is not allowed for CHROTE terminal launch" >&2
    exit 2
  fi
  USER_SOCKET="$(socket_for_terminal_user "$UNIX_USER")" || {
    echo "No CHROTE terminal socket configured for Unix user '$UNIX_USER'" >&2
    exit 2
  }
  attach_explicit_socket "$USER_SOCKET" "$SESSION"
elif [ -n "${CHROTE_TMUX_SOCKET:-}" ]; then
  attach_explicit_socket "$CHROTE_TMUX_SOCKET" "$SESSION"
else
  # REASON: tmux has-session tests existence; stderr is noise, not an error
  if [ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null; then
    exec tmux attach-session -t "$SESSION"
  fi
fi
exec bash -l

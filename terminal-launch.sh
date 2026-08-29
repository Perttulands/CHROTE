#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal launch script for CHROTE.
# CHROTE_WORKDIR and CHROTE_TMUX_SOCKET are set by the systemd unit or environment.
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

socket_for_terminal_user() {
  local candidate="$1"
  local mappings="${CHROTE_TMUX_SOCKET:-}"
  local entry key value match="" count=0
  local -a entries
  local -A seen
  IFS=',' read -r -a entries <<< "$mappings"
  for entry in "${entries[@]}"; do
    entry="$(trim_ws "$entry")"
    [ -n "$entry" ] || continue
    key="$(trim_ws "${entry%%=*}")"
    value="$(trim_ws "${entry#*=}")"
    if [ -z "$key" ] || [ -z "$value" ] || [ "$value" = "$entry" ]; then
      echo "CHROTE_TMUX_SOCKET entry '$entry' must be unixUser=/absolute/socket" >&2
      return 3
    fi
    case "$value" in
      /*) ;;
      *) echo "CHROTE_TMUX_SOCKET for Unix user '$key' must be an absolute path: '$value'" >&2; return 3 ;;
    esac
    case "$value" in
      *'/../'*|*'/..'|*'/./'*|*'/.'|*'//'*) echo "CHROTE_TMUX_SOCKET for Unix user '$key' must be canonical: '$value'" >&2; return 3 ;;
    esac
    if [ -n "${seen[$key]+x}" ]; then
      echo "CHROTE_TMUX_SOCKET has duplicate entries for Unix user '$key' ('${seen[$key]}' and '$value'); keep exactly one entry per user" >&2
      return 3
    fi
    seen[$key]="$value"
    count=$((count + 1))
    if [ -n "$candidate" ] && [ "$key" = "$candidate" ]; then
      match="$value"
    fi
  done
  if [ -z "$candidate" ] && [ "$count" -eq 1 ]; then
    for key in "${!seen[@]}"; do
      match="${seen[$key]}"
    done
  elif [ -z "$candidate" ] && [ "$count" -gt 1 ]; then
    return 2
  fi
  if [ -n "$match" ]; then
    printf '%s\n' "$match"
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
else
  USER_SOCKET="$(socket_for_terminal_user "")"
  SOCKET_RC=$?
  if [ "$SOCKET_RC" -eq 2 ]; then
    echo "Unix user is required when multiple CHROTE tmux sockets are configured" >&2
    exit 2
  elif [ "$SOCKET_RC" -eq 3 ]; then
    exit 2
  elif [ "$SOCKET_RC" -ne 0 ]; then
    echo "No CHROTE tmux sockets are configured" >&2
    exit 2
  fi
  attach_explicit_socket "$USER_SOCKET" "$SESSION"
fi

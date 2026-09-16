# Shared tmux server-access grant used by the installer.
#
# tmux refuses `server-access -a` for a user it has already added and exits
# non-zero, so reapplying the supported one-shot grant to a live server aborted
# a fail-loud install even though the desired access was already present.
# Read the server's access list first and only issue the command that moves the
# server towards write access for the service account.

# chrote_tmux_as_owner <owner> <command...>
# Runs a command as the tmux server's owner. When the caller already is that
# owner, no privilege change is needed or available.
chrote_tmux_as_owner() {
  local owner="$1"
  shift
  if [ "$owner" = "$(id -un)" ]; then
    "$@"
  elif command -v runuser >/dev/null 2>&1; then
    runuser -u "$owner" -- "$@"
  else
    sudo -n -u "$owner" "$@"
  fi
}

# chrote_tmux_server_access <owner> <socket> <tmux_bin> <args...>
chrote_tmux_server_access() {
  local owner="$1" socket="$2" tmux_bin="$3"
  shift 3
  chrote_tmux_as_owner "$owner" env TERM="${TERM:-xterm-256color}" \
    "$tmux_bin" -S "$socket" server-access "$@"
}

# chrote_ensure_tmux_server_access <owner> <socket> <service_user> <tmux_bin>
# Leaves the service account with write access on the running server, whether or
# not a previous install already granted it. Any failure to read the access list
# or to apply the grant is reported, not swallowed.
chrote_ensure_tmux_server_access() {
  local owner="$1" socket="$2" service_user="$3" tmux_bin="$4"
  local listing="" line access=""

  if ! listing="$(chrote_tmux_server_access "$owner" "$socket" "$tmux_bin" -l 2>&1)"; then
    printf 'could not read tmux server access on %s: %s\n' "$socket" "$listing" >&2
    return 1
  fi
  while IFS= read -r line; do
    case "$line" in
      "$service_user (W)") access="write" ;;
      "$service_user (R)") access="read" ;;
    esac
  done <<<"$listing"

  case "$access" in
    write) return 0 ;;
    read) chrote_tmux_server_access "$owner" "$socket" "$tmux_bin" -w "$service_user" ;;
    *) chrote_tmux_server_access "$owner" "$socket" "$tmux_bin" -a -w "$service_user" ;;
  esac
}

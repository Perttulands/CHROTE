#!/usr/bin/env bash
# Refresh filesystem ACLs and tmux server-access grants so the /srv CHROTE
# service account can use explicitly configured user tmux sockets.
#
# Non-destructive by design: this script does not kill, restart, create, or
# rename tmux sessions. It only runs setfacl and `tmux server-access -a chrote`
# against sockets listed in CHROTE_TERMINAL_USER_SOCKETS.

set -u -o pipefail

SERVICE_USER="${CHROTE_TMUX_GRANT_USER:-chrote}"
SOCKET_MAPPINGS="${CHROTE_TERMINAL_USER_SOCKETS:-}"
TERM_VALUE="${TERM:-xterm-256color}"

log() {
  printf 'chrote-tmux-grants: %s\n' "$*" >&2
}

have_user() {
  getent passwd "$1" >/dev/null 2>&1
}

set_acl() {
  if command -v setfacl >/dev/null 2>&1; then
    setfacl "$@" || log "warning: setfacl $* failed"
  else
    log "warning: setfacl not found; cannot refresh filesystem ACLs"
    return 1
  fi
}

tmux_bin() {
  if [ -n "${CHROTE_TMUX_BIN:-}" ]; then
    if [ -x "${CHROTE_TMUX_BIN}" ]; then
      printf '%s\n' "${CHROTE_TMUX_BIN}"
      return 0
    fi
    log "warning: CHROTE_TMUX_BIN is set but not executable: ${CHROTE_TMUX_BIN}"
    return 1
  fi
  if command -v tmux >/dev/null 2>&1; then
    command -v tmux
    return 0
  fi
  return 1
}

run_as_owner() {
  local owner="$1"
  shift
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$owner" -- "$@"
  else
    sudo -n -u "$owner" "$@"
  fi
}

grant_dir_for_traverse() {
  local dir="$1"
  [ -d "$dir" ] || return 0
  set_acl -m "u:${SERVICE_USER}:--x" "$dir" >/dev/null 2>&1 || true
}

grant_socket_path_acl() {
  local socket="$1"
  local dir
  dir="$(dirname "$socket")"

  case "$socket" in
    /run/user/*/*)
      # /run/user/<uid> is usually mode 0700 after boot; grant traverse only.
      local runtime_dir
      runtime_dir="$(printf '%s\n' "$socket" | awk -F/ '{print "/run/user/"$4}')"
      grant_dir_for_traverse "$runtime_dir"
      ;;
  esac

  # Grant traverse on every existing directory below the socket root. This is
  # intentionally execute-only; it does not grant directory listing rights.
  local path_so_far=""
  local part
  local trimmed="${dir#/}"
  IFS='/' read -r -a parts <<< "$trimmed"
  for part in "${parts[@]}"; do
    [ -n "$part" ] || continue
    path_so_far="${path_so_far}/${part}"
    case "$path_so_far" in
      /run|/run/user|/tmp) continue ;;
    esac
    grant_dir_for_traverse "$path_so_far"
  done

  if [ -d "$dir" ]; then
    # Future sockets created inside the tmux socket directory inherit access.
    set_acl -d -m "u:${SERVICE_USER}:rwx" "$dir" >/dev/null 2>&1 || true
  fi

  if [ -S "$socket" ] || [ -e "$socket" ]; then
    set_acl -m "u:${SERVICE_USER}:rw" "$socket" >/dev/null 2>&1 || true
  fi
}

grant_server_access() {
  local owner="$1"
  local socket="$2"
  local tmux
  tmux="$(tmux_bin)" || {
    log "warning: tmux binary not found; cannot refresh tmux server-access for $owner=$socket"
    return 0
  }

  if [ ! -S "$socket" ]; then
    log "socket missing, skipping server-access: $owner=$socket"
    return 0
  fi
  if ! have_user "$owner"; then
    log "owner user missing, skipping server-access: $owner=$socket"
    return 0
  fi

  if run_as_owner "$owner" env TERM="$TERM_VALUE" "$tmux" -S "$socket" server-access -a "$SERVICE_USER" >/dev/null 2>&1; then
    return 0
  fi

  # Older tmux or dead sockets should not block CHROTE startup; the API will
  # still fail loud if the socket is unusable.
  log "warning: tmux server-access refresh failed for $owner=$socket"
  return 0
}

main() {
  if [ "$(id -u)" -ne 0 ]; then
    log "must run as root so ACLs can be refreshed"
    return 1
  fi
  if ! have_user "$SERVICE_USER"; then
    log "service user missing: $SERVICE_USER"
    return 1
  fi
  if [ -z "$SOCKET_MAPPINGS" ]; then
    log "CHROTE_TERMINAL_USER_SOCKETS is empty; nothing to grant"
    return 0
  fi

  local entry owner socket
  IFS=',' read -r -a entries <<< "$SOCKET_MAPPINGS"
  for entry in "${entries[@]}"; do
    entry="${entry//[[:space:]]/}"
    [ -n "$entry" ] || continue
    owner="${entry%%=*}"
    socket="${entry#*=}"
    if [ -z "$owner" ] || [ -z "$socket" ] || [ "$owner" = "$socket" ]; then
      log "warning: invalid socket mapping '$entry'"
      continue
    fi
    case "$socket" in
      /*) ;;
      *) log "warning: non-absolute socket path skipped: $owner=$socket"; continue ;;
    esac

    grant_socket_path_acl "$socket"
    grant_server_access "$owner" "$socket"
  done
}

main "$@"

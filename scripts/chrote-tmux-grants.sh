#!/usr/bin/env bash
# Refresh filesystem ACLs and tmux server-access grants so the CHROTE service
# account can use explicitly configured owner tmux sockets.
#
# Non-destructive by design: this script does not kill, restart, create, or
# rename tmux sessions. It only runs setfacl and `tmux server-access -a` against
# sockets listed in CHROTE_TERMINAL_USER_SOCKETS.
#
# Every mapping is validated before anything is granted. A mapping that names a
# path outside the allowed socket roots, or a socket that does not belong to the
# named owner, is refused — CHROTE_TERMINAL_USER_SOCKETS would otherwise hand
# the service account rw on any absolute path an operator typed by mistake.

set -euo pipefail

readonly SERVICE_USER="${CHROTE_TMUX_GRANT_USER:-chrote}"
readonly SOCKET_MAPPINGS="${CHROTE_TERMINAL_USER_SOCKETS:-}"
readonly TERM_VALUE="${TERM:-xterm-256color}"

log() {
  printf 'chrote-tmux-grants: %s\n' "$*" >&2
}

have_user() {
  getent passwd "$1" >/dev/null 2>&1
}

# allowed_socket_roots lists the directories an owner's socket may live under.
#
# The default encodes what this host already runs — the per-user runtime dir and
# the classic /tmp/tmux-<uid> location — so enforcing validation does not cut off
# an owner whose socket is already in /tmp. Narrowing the policy (for example to
# runtime directories only) is a configuration change here, not a code change:
#   CHROTE_TMUX_GRANT_SOCKET_ROOTS='/run/user/%u'
# %u expands to the owner's numeric uid.
allowed_socket_roots() {
  local uid="$1"
  local configured="${CHROTE_TMUX_GRANT_SOCKET_ROOTS:-/run/user/%u:/tmp/tmux-%u}"
  local root
  local -a roots=()
  IFS=':' read -r -a roots <<< "$configured"
  for root in "${roots[@]}"; do
    [ -n "$root" ] || continue
    printf '%s\n' "${root//%u/$uid}"
  done
}

# validate_mapping fails closed. Callers must invoke it as
# `validate_mapping ... || return 1`: calling it as a bare statement inside a
# function that is itself called from an `if !` test suspends errexit, which is
# exactly how the previously installed helper granted sockets it meant to refuse.
validate_mapping() {
  local owner="$1"
  local socket="$2"
  local uid root owner_of_socket
  local matched=0

  if ! have_user "$owner"; then
    log "owner user does not exist: $owner"
    return 1
  fi

  case "$socket" in
    /*) ;;
    *) log "socket path is not absolute: $owner=$socket"; return 1 ;;
  esac
  case "$socket" in
    *'/../'*|*'/..'|*'/./'*|*'/.'|*'//'*)
      log "socket path contains non-canonical components: $owner=$socket"
      return 1
      ;;
  esac

  uid="$(id -u "$owner")"
  while IFS= read -r root; do
    [ -n "$root" ] || continue
    case "$socket" in
      "${root%/}"/*) matched=1; break ;;
    esac
  done < <(allowed_socket_roots "$uid")
  if [ "$matched" -ne 1 ]; then
    log "socket path is outside the allowed socket roots: $owner=$socket"
    return 1
  fi

  # A socket that exists must belong to the owner it is mapped to, so a mapping
  # cannot be used to grant access to somebody else's file.
  if [ -e "$socket" ]; then
    owner_of_socket="$(stat -c '%U' "$socket" 2>/dev/null || printf '?')"
    if [ "$owner_of_socket" != "$owner" ]; then
      log "socket is owned by '$owner_of_socket', not '$owner': $socket"
      return 1
    fi
  fi

  return 0
}

set_acl() {
  if ! command -v setfacl >/dev/null 2>&1; then
    log "setfacl not found; cannot refresh filesystem ACLs"
    return 1
  fi
  setfacl "$@"
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
  set_acl -m "u:${SERVICE_USER}:--x" "$dir" >/dev/null 2>&1 || {
    log "warning: could not grant traverse on $dir"
    return 1
  }
}

grant_socket_path_acl() {
  local socket="$1"
  local dir part path_so_far trimmed
  local -a parts=()
  local failed=0
  dir="$(dirname "$socket")"

  # Grant traverse on every existing directory below the socket root. This is
  # intentionally execute-only; it does not grant directory listing rights.
  path_so_far=""
  trimmed="${dir#/}"
  IFS='/' read -r -a parts <<< "$trimmed"
  for part in "${parts[@]}"; do
    [ -n "$part" ] || continue
    path_so_far="${path_so_far}/${part}"
    case "$path_so_far" in
      /run|/tmp) continue ;;
    esac
    grant_dir_for_traverse "$path_so_far" || failed=1
  done

  if [ -d "$dir" ]; then
    # Future sockets created inside the tmux socket directory inherit access.
    set_acl -d -m "u:${SERVICE_USER}:rwx" "$dir" >/dev/null 2>&1 || {
      log "warning: could not set default ACL on $dir"
      failed=1
    }
  fi

  if [ -S "$socket" ] || [ -e "$socket" ]; then
    set_acl -m "u:${SERVICE_USER}:rw" "$socket" >/dev/null 2>&1 || {
      log "could not grant access to socket: $socket"
      failed=1
    }
  fi

  return "$failed"
}

grant_server_access() {
  local owner="$1"
  local socket="$2"
  local tmux

  # An owner may simply not be logged in yet, so an absent configured socket is
  # not a service failure. Once the path exists, however, every part of the
  # grant must work or the unit would report green while access is broken.
  if [ ! -e "$socket" ]; then
    log "socket missing, skipping server-access: $owner=$socket"
    return 0
  fi
  if [ ! -S "$socket" ]; then
    log "configured socket path exists but is not a Unix socket: $owner=$socket"
    return 1
  fi

  tmux="$(tmux_bin)" || {
    log "tmux binary not found; cannot refresh tmux server-access for $owner=$socket"
    return 1
  }

  if run_as_owner "$owner" env TERM="$TERM_VALUE" "$tmux" -S "$socket" server-access -a "$SERVICE_USER" >/dev/null 2>&1; then
    return 0
  fi

  log "tmux server-access refresh failed for $owner=$socket"
  return 1
}

grant_socket() {
  local owner="$1"
  local socket="$2"

  validate_mapping "$owner" "$socket" || return 1
  grant_socket_path_acl "$socket" || return 1
  grant_server_access "$owner" "$socket" || return 1
  return 0
}

main() {
  local entry owner socket
  local -a entries=()
  local failures=0

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

  IFS=',' read -r -a entries <<< "$SOCKET_MAPPINGS"
  for entry in "${entries[@]}"; do
    entry="${entry//[[:space:]]/}"
    [ -n "$entry" ] || continue
    owner="${entry%%=*}"
    socket="${entry#*=}"
    if [ -z "$owner" ] || [ -z "$socket" ] || [ "$owner" = "$socket" ]; then
      log "invalid socket mapping '$entry'"
      failures=$((failures + 1))
      continue
    fi

    if ! grant_socket "$owner" "$socket"; then
      log "grant failed for $owner=$socket"
      failures=$((failures + 1))
    fi
  done

  if [ "$failures" -gt 0 ]; then
    # Exiting non-zero is the point: a green unit during total grant failure is
    # how cross-user access stayed broken without anyone noticing.
    log "$failures socket mapping(s) failed"
    return 1
  fi
  return 0
}

# Sourcing the script exposes the helpers for tests without running the grants.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi

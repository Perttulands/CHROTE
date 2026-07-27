#!/usr/bin/env bash
# Regression tests for scripts/chrote-tmux-grants.sh.
#
# These cover the three failures that let cross-user grants go wrong silently:
# a validator whose refusal was discarded, mappings pointing outside the owner's
# socket roots, and a helper that exited 0 while every grant failed.
#
# Runs unprivileged: the tests exercise validation and exit codes, never real
# ACL changes.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Overridable so the suite can be pointed at another copy of the helper, e.g. to
# check whether an installed copy still carries the fixes.
GRANTS="${CHROTE_GRANTS_SCRIPT:-$SCRIPT_DIR/chrote-tmux-grants.sh}"

failures=0
checks=0

ok() {
  checks=$((checks + 1))
  printf 'ok   %s\n' "$1"
}

fail() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf 'FAIL %s\n' "$1" >&2
}

expect_valid() {
  local label="$1" owner="$2" socket="$3"
  if validate_mapping "$owner" "$socket" 2>/dev/null; then
    ok "$label"
  else
    fail "$label (expected the mapping to be accepted)"
  fi
}

expect_invalid() {
  local label="$1" owner="$2" socket="$3"
  if validate_mapping "$owner" "$socket" 2>/dev/null; then
    fail "$label (expected the mapping to be refused)"
  else
    ok "$label"
  fi
}

# shellcheck source=./chrote-tmux-grants.sh
source "$GRANTS"
# Sourcing brings the helper's `set -euo pipefail` with it; these tests inspect
# failures deliberately, so errexit has to come back off.
set +e

# Without this, a helper that has no validator at all passes the refusal checks
# vacuously: calling a missing function returns 127, which reads as "refused".
for required in validate_mapping grant_socket main; do
  if ! declare -F "$required" >/dev/null; then
    printf 'FAIL %s does not define %s()\n' "$GRANTS" "$required" >&2
    exit 1
  fi
done

ME="$(id -un)"
MY_UID="$(id -u)"

# The default policy is the status quo on this host: the owner's runtime dir and
# the classic /tmp/tmux-<uid> location.
expect_valid   "accepts a socket under the owner runtime directory" "$ME" "/run/user/$MY_UID/tmux/default"
expect_valid   "accepts the classic /tmp/tmux-<uid> socket"         "$ME" "/tmp/tmux-$MY_UID/default"

expect_invalid "refuses a path outside every allowed root"          "$ME" "/etc/passwd"
expect_invalid "refuses another user's runtime directory"           "$ME" "/run/user/999999/tmux/default"
expect_invalid "refuses a relative path"                            "$ME" "tmp/tmux-$MY_UID/default"
expect_invalid "refuses traversal out of an allowed root"           "$ME" "/tmp/tmux-$MY_UID/../../etc/passwd"
expect_invalid "refuses a non-canonical path"                       "$ME" "/tmp/tmux-$MY_UID//default"
expect_invalid "refuses an unknown owner"                           "chrote-no-such-user" "/tmp/tmux-$MY_UID/default"

# Narrowing the allowlist is a configuration change, not a code change.
(
  export CHROTE_TMUX_GRANT_SOCKET_ROOTS='/run/user/%u'
  if validate_mapping "$ME" "/tmp/tmux-$MY_UID/default" 2>/dev/null; then
    exit 1
  fi
  exit 0
) && ok "a narrowed allowlist refuses /tmp sockets" || fail "a narrowed allowlist refuses /tmp sockets"

# An existing socket must belong to the owner it is mapped to. /etc is
# root-owned, so pointing the allowlist at it proves the ownership check without
# needing root to stage a fixture.
(
  export CHROTE_TMUX_GRANT_SOCKET_ROOTS='/etc'
  if [ "$ME" = "root" ]; then
    exit 0
  fi
  if validate_mapping "$ME" "/etc/hostname" 2>/dev/null; then
    exit 1
  fi
  exit 0
) && ok "refuses a socket owned by somebody else" || fail "refuses a socket owned by somebody else"

# The validator must be unbypassable through the call path that granted refused
# sockets before: grant_socket is invoked from an `if !` test, which suspends
# errexit inside the function body.
(
  if ! grant_socket "$ME" "/etc/passwd" 2>/dev/null; then
    exit 0
  fi
  exit 1
) && ok "grant_socket refuses a mapping the validator rejects" || fail "grant_socket refuses a mapping the validator rejects"

# A failing run must exit non-zero rather than leaving the unit green. Running
# unprivileged is itself a hard failure the helper has to report.
CHROTE_TERMINAL_USER_SOCKETS="$ME=/etc/passwd" "$GRANTS" >/dev/null 2>&1
status=$?
if [ "$status" -ne 0 ]; then
  ok "a failing run exits non-zero"
else
  fail "a failing run exits non-zero (got $status)"
fi

# An empty mapping list is not a failure: nothing was asked for.
if [ "$MY_UID" -ne 0 ]; then
  ok "skipped empty-mapping exit check (needs root)"
else
  CHROTE_TERMINAL_USER_SOCKETS="" "$GRANTS" >/dev/null 2>&1
  status=$?
  if [ "$status" -eq 0 ]; then
    ok "an empty mapping list succeeds"
  else
    fail "an empty mapping list succeeds (got $status)"
  fi
fi

printf '\n%d checks, %d failures\n' "$checks" "$failures"
[ "$failures" -eq 0 ]

#!/usr/bin/env bash
#
# formations-dogfood.sh — inspect or exercise the CHROTE Formations dedicated
# tmux executor boundary.
#
# Modes:
#
#   (default) --dedicated
#                        Print (and with --export emit) the exact
#                        CHROTE_FORMATIONS_TMUX_* + CHROTE_FORMATIONS_TMUX_DEDICATED
#                        environment that configures the CHROTE service to run the
#                        executor on the DEDICATED formations socket. This mode
#                        only prepares/documents that runtime; it executes nothing.
#
#   --dedicated-run      EXECUTE the gated dedicated-socket Go test. It creates
#                        its OWN tmux server on a dedicated non-cockpit socket
#                        under the repo, runs a real formation over a deterministic
#                        in-pane agent with Dedicated enabled, and tears down only
#                        its own server. This is a boundary self-test, not proof
#                        that the deployed service is executing missions.
#
# Golden rule: never disrupt running shells/tmux, and NEVER target the cockpit
# socket. Mission execution belongs on the dedicated Formations socket; the
# executor refuses the cockpit/default socket in every mode.
#
# Usage:
#   scripts/formations-dogfood.sh                  # PRINT dedicated runtime env
#   scripts/formations-dogfood.sh --dedicated      # PRINT dedicated runtime env
#   scripts/formations-dogfood.sh --dedicated --export > dedicated.env
#   scripts/formations-dogfood.sh --dedicated-run  # run dedicated boundary self-test
#   scripts/formations-dogfood.sh --dedicated-run -v
#
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src_dir="$repo_root/src"

# Dedicated formations socket facts (descriptive; see the chrote.service systemd
# unit). The CHROTE service runs the executor on this dedicated socket, separated
# from the cockpit's interactive shells.
RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
DEDICATED_SOCKET="${CHROTE_FORMATIONS_TMUX_SOCKET:-$RUNTIME_DIR/chrote-formations-tmux/default}"
DEDICATED_CWD="${CHROTE_FORMATIONS_TMUX_CWD:-$HOME}"
DEDICATED_ROOTS="${CHROTE_FORMATIONS_TMUX_ROOTS:-$HOME}"
DEDICATED_PREFIX="${CHROTE_FORMATIONS_TMUX_SESSION_PREFIX:-mission-}"
DEDICATED_HARNESSES="${CHROTE_FORMATIONS_TMUX_HARNESSES:-openai-codex,claude-code}"

mode="dedicated"
export_env=0
go_test_extra=()

while [ $# -gt 0 ]; do
  case "$1" in
    --dedicated-run|dedicated-run) mode="dedicated-run" ;;
    --dedicated|dedicated) mode="dedicated" ;;
    --export) export_env=1 ;;
    -v|--verbose) go_test_extra+=("-v") ;;
    -h|--help)
      sed -n '2,38p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    isolated)
      echo "ERROR: isolated Formations dogfood mode has been removed. Use the dedicated service runtime or --dedicated-run for a boundary self-test." >&2
      exit 2
      ;;
    *) go_test_extra+=("$1") ;;
  esac
  shift
done

run_dedicated() {
  if ! command -v tmux >/dev/null 2>&1 || ! command -v python3 >/dev/null 2>&1; then
    echo "ERROR: --dedicated-run needs 'tmux' and 'python3' on PATH." >&2
    exit 1
  fi
  echo "== CHROTE Formations DEDICATED-socket boundary self-test =="
  echo "tmux:    $(command -v tmux) ($(tmux -V))"
  echo "This creates its own non-cockpit tmux server under the repo, drives the real"
  echo "executor with Dedicated enabled, and tears down only that server. It is not"
  echo "deployed-service proof; use chrote.service + live API/UI checks for that."
  echo
  ( cd "$src_dir" && CHROTE_FORMATIONS_DEDICATED_RUN=1 \
      go test -count=1 -run 'TestDedicatedSocket|TestDogfoodDedicatedTmux' ./internal/formations/ "${go_test_extra[@]}" )
}

print_dedicated() {
  # Build the exact dedicated-socket runtime environment. This is the sanctioned
  # always-on configuration: the executor runs on a dedicated formations socket,
  # separated from the cockpit's interactive shells. Lab vars must be UNSET so the
  # tmux executor (not the lab executor) is selected.
  local lines=(
    "# CHROTE Formations DEDICATED-socket runtime environment."
    "# This configures the executor to run mission agents on a DEDICATED tmux"
    "# socket, separated from the cockpit's interactive shells. The executor"
    "# ALWAYS refuses the cockpit/default socket. Sessions must already exist"
    "# with the prefix; the executor never creates or kills sessions. Output"
    "# caps, timeouts, redaction, and fail-loud blocks all still apply. Unset"
    "# the lab vars or the lab executor wins."
    "unset CHROTE_FORMATIONS_LAB_HARNESSES CHROTE_FORMATIONS_LAB_CWD CHROTE_FORMATIONS_LAB_ROOTS"
    "export CHROTE_FORMATIONS_TMUX_HARNESSES=$(printf '%q' "$DEDICATED_HARNESSES")"
    "export CHROTE_FORMATIONS_TMUX_SOCKET=$(printf '%q' "$DEDICATED_SOCKET")"
    "export CHROTE_FORMATIONS_TMUX_CWD=$(printf '%q' "$DEDICATED_CWD")"
    "export CHROTE_FORMATIONS_TMUX_ROOTS=$(printf '%q' "$DEDICATED_ROOTS")"
    "export CHROTE_FORMATIONS_TMUX_SESSION_PREFIX=$(printf '%q' "$DEDICATED_PREFIX")"
    "export CHROTE_FORMATIONS_TMUX_DEDICATED=1"
  )

  if [ "$export_env" -eq 1 ]; then
    printf '%s\n' "${lines[@]}"
    return 0
  fi

  cat >&2 <<INFO
== CHROTE Formations DEDICATED-socket runtime ==

The CHROTE service should run mission execution on the dedicated formations socket:
  $DEDICATED_SOCKET
This is a SEPARATE socket from the cockpit socket; the executor refuses the
cockpit/default socket in every mode. This script does NOT run anything against
the deployed service; it only prints the environment the service applies.

Prerequisites for live mission runs on the dedicated socket:
  - The dedicated tmux sessions already exist with prefix "$DEDICATED_PREFIX"
    (e.g. ${DEDICATED_PREFIX}<agent-session-stem>), panes alive, cwd inside roots.
  - The agent harness in each pane reads its prompt and emits the fenced
    chrote-outputs block + the <<<CHROTE-DONE run-id=...>>> sentinel.
  - Lab vars unset so the tmux executor is selected.

Environment to apply (also: re-run with --export to write a sourceable file):

INFO
  printf '%s\n' "${lines[@]}"
  cat >&2 <<'NEXT'

To self-test the dedicated boundary end to end without the service:
  scripts/formations-dogfood.sh --dedicated-run
NEXT
}

case "$mode" in
  dedicated-run) run_dedicated ;;
  dedicated)     print_dedicated ;;
  *) echo "unknown mode: $mode" >&2; exit 2 ;;
esac

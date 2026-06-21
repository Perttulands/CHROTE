#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal launch script for CHROTE.
# TMUX_TMPDIR and CHROTE_WORKDIR are set by the systemd unit or environment.
export LANG=en_US.UTF-8
# REASON: cd to preferred dir is optional, fallthrough is intentional
cd "${CHROTE_WORKDIR:-$HOME}" 2>/dev/null || cd ~ || exit
SESSION="$1"
if [ -n "$CHROTE_TMUX_SOCKET" ]; then
  if [ -z "$SESSION" ]; then
    echo "CHROTE socket terminal requires a tmux session name" >&2
    exit 2
  fi

  # REASON: explicit-socket terminals must fail loud instead of falling back to
  # the ambient perttu tmux server when the configured session is unavailable.
  if tmux -S "$CHROTE_TMUX_SOCKET" has-session -t "$SESSION" 2>/dev/null; then
    exec tmux -S "$CHROTE_TMUX_SOCKET" attach-session -t "$SESSION"
  fi

  echo "tmux session '$SESSION' is not available on configured socket '$CHROTE_TMUX_SOCKET'" >&2
  exit 1
else
  # REASON: tmux has-session tests existence; stderr is noise, not an error
  [ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null && exec tmux attach-session -t "$SESSION"
fi
exec bash -l

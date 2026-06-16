#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal launch script for CHROTE.
# TMUX_TMPDIR and CHROTE_WORKDIR are set by the systemd unit or environment.
#
# CHROTE_TMUX_SOCKET, when set (by the formations ttyd), pins tmux to an EXPLICIT
# socket path via `tmux -S <socket>` — the SAME socket the formations executor
# and session API use. When unset (cockpit ttyd), tmux stays TMUX_TMPDIR-driven
# with no -S flag, exactly as before.
export LANG=en_US.UTF-8
# REASON: cd to preferred dir is optional, fallthrough is intentional
cd "${CHROTE_WORKDIR:-$HOME}" 2>/dev/null || cd ~ || exit
SESSION="$1"
if [ -n "$CHROTE_TMUX_SOCKET" ]; then
  # REASON: tmux has-session tests existence; stderr is noise, not an error
  [ -n "$SESSION" ] && tmux -S "$CHROTE_TMUX_SOCKET" has-session -t "$SESSION" 2>/dev/null && exec tmux -S "$CHROTE_TMUX_SOCKET" attach-session -t "$SESSION"
else
  # REASON: tmux has-session tests existence; stderr is noise, not an error
  [ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null && exec tmux attach-session -t "$SESSION"
fi
exec bash -l

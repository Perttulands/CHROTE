#!/bin/bash
# truthsayer:ignore bad-defaults.missing-pipefail -- launch script uses intentional fallthrough
# Terminal launch script for CHROTE.
# TMUX_TMPDIR and CHROTE_WORKDIR are set by the systemd unit or environment.
export LANG=en_US.UTF-8
# REASON: cd to preferred dir is optional, fallthrough is intentional
cd "${CHROTE_WORKDIR:-$HOME}" 2>/dev/null || cd ~ || exit
SESSION="$1"
# REASON: tmux has-session tests existence; stderr is noise, not an error
[ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null && exec tmux attach-session -t "$SESSION"
exec bash -l

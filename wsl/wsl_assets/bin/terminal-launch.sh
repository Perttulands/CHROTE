#!/bin/bash
# Terminal Launcher for AgentArena
# Ensures ttyd sessions connect to the correct tmux socket
# USAGE: ttyd -W -a /usr/local/bin/terminal-launch.sh
# -----------------------------------------------------------------------------

# Use explicit socket path from systemd or default to /run/tmux/chrote
export TMUX_TMPDIR=${TMUX_TMPDIR:-/run/tmux/chrote}
export LANG=en_US.UTF-8
cd /code 2>/dev/null || cd ~

SESSION="$1"

# Check if session exists and attach, otherwise start fresh shell
if [ -n "$SESSION" ] && tmux has-session -t "$SESSION" 2>/dev/null; then
    exec tmux attach-session -t "$SESSION"
else
    # If session doesn't exist, just drop to shell (don't error out)
    exec bash -l
fi
